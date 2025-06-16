package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

type Todo struct {
	ID                 int        `json:"id"`
	Title              string     `json:"title"`
	Completed          bool       `json:"completed"`
	CreatedAt          time.Time  `json:"created_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	DueDate            *string    `json:"due_date,omitempty"`
	RecurrenceInterval *int       `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string    `json:"recurrence_unit,omitempty"`
	ProjectID          int        `json:"project_id"`
}

type Project struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("sqlite3", "./todos.db")
	if err != nil {
		log.Fatal(err)
	}

	// Run migrations
	if err := runMigrations(); err != nil {
		log.Fatal(err)
	}

	if err != nil {
		log.Fatal(err)
	}
}

func runMigrations() error {
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrate driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://./migrations",
		"sqlite3",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %v", err)
	}

	return nil
}

func getProject(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	var project Project
	err := db.QueryRow("SELECT id, title, position, created_at FROM projects WHERE id = ?", id).
		Scan(&project.ID, &project.Title, &project.Position, &project.CreatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func getProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, title, position, created_at FROM projects ORDER BY position")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Title, &project.Position, &project.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projects = append(projects, project)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func addProject(w http.ResponseWriter, r *http.Request) {
	var project Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the highest position
	var maxPosition int
	err := db.QueryRow("SELECT MAX(position) FROM projects").Scan(&maxPosition)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	project.Position = maxPosition + 1

	stmt, err := db.Prepare("INSERT INTO projects (title, position) VALUES (?, ?)")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	result, err := stmt.Exec(project.Title, project.Position)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the ID of the inserted project
	id, err := result.LastInsertId()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the created project with its ID
	var createdProject Project
	err = db.QueryRow("SELECT id, title, position, created_at FROM projects WHERE id = ?", id).
		Scan(&createdProject.ID, &createdProject.Title, &createdProject.Position, &createdProject.CreatedAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(createdProject)
}

func updateProject(w http.ResponseWriter, r *http.Request) {
	// Extract project ID from URL
	idStr := strings.TrimPrefix(r.URL.Path, "/api/projects/")

	if idStr == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	// Parse ID
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Decode request body
	var project Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update project
	stmt, err := db.Prepare("UPDATE projects SET title = ? WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(project.Title, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if project was found and updated
	var updatedProject Project
	err = db.QueryRow("SELECT id, title, position, created_at FROM projects WHERE id = ?", id).
		Scan(&updatedProject.ID, &updatedProject.Title, &updatedProject.Position, &updatedProject.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Project not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedProject)
}

func deleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	// Delete todos first
	stmt, err := db.Prepare("DELETE FROM todos WHERE project_id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Then delete the project
	stmt, err = db.Prepare("DELETE FROM projects WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	result, err := stmt.Exec(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getTodos(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, completed, created_at, completed_at, due_date, recurrence_interval, recurrence_unit, project_id 
		FROM todos 
		ORDER BY 
			CASE WHEN completed = 0 THEN 0 ELSE 1 END,  -- Incomplete first
			CASE WHEN completed = 1 THEN completed_at END DESC,  -- Completed items by completed_at DESC
			created_at DESC  -- Then by creation time
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var todo Todo
		if err := rows.Scan(&todo.ID, &todo.Title, &todo.Completed, &todo.CreatedAt, &todo.CompletedAt, &todo.DueDate, &todo.RecurrenceInterval, &todo.RecurrenceUnit, &todo.ProjectID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		todos = append(todos, todo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(todos)
}

func addTodo(w http.ResponseWriter, r *http.Request) {
	var todo Todo
	if err := json.NewDecoder(r.Body).Decode(&todo); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare("INSERT INTO todos (title, completed, project_id, due_date, recurrence_interval, recurrence_unit) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(todo.Title, todo.Completed, todo.ProjectID, todo.DueDate, todo.RecurrenceInterval, todo.RecurrenceUnit); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	var todo Todo
	if err := json.NewDecoder(r.Body).Decode(&todo); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if todo.ID == 0 {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	// Ensure ProjectID is set (needed for recurring insertion)
	if todo.ProjectID == 0 {
		if err := db.QueryRow("SELECT project_id FROM todos WHERE id = ?", todo.ID).Scan(&todo.ProjectID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Handle recurring logic: if marking completed and recurrence set, complete current and create next occurrence
	if todo.Completed && todo.CompletedAt == nil {
		if todo.RecurrenceInterval != nil && todo.RecurrenceUnit != nil {
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// complete current
			now := time.Now()
			todo.CompletedAt = &now
			_, err = tx.Exec("UPDATE todos SET title=?, completed=1, completed_at=?, due_date=?, recurrence_interval=?, recurrence_unit=? WHERE id=?", todo.Title, todo.CompletedAt, todo.DueDate, todo.RecurrenceInterval, todo.RecurrenceUnit, todo.ID)
			if err != nil {
				tx.Rollback()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// compute next due date
			var nextDue string
			if todo.DueDate != nil {
				t, err := time.Parse("2006-01-02", *todo.DueDate)
				if err != nil {
					tx.Rollback()
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				interval := *todo.RecurrenceInterval
				switch *todo.RecurrenceUnit {
				case "day":
					t = t.AddDate(0, 0, interval)
				case "week":
					t = t.AddDate(0, 0, 7*interval)
				case "month":
					t = t.AddDate(0, interval, 0)
				case "year":
					t = t.AddDate(interval, 0, 0)
				}
				nextDue = t.Format("2006-01-02")
			}
			// insert new row
			_, err = tx.Exec("INSERT INTO todos (title, completed, project_id, due_date, recurrence_interval, recurrence_unit) VALUES (?,?,?,?,?,?)", todo.Title, 0, todo.ProjectID, sql.NullString{String: nextDue, Valid: nextDue != ""}, todo.RecurrenceInterval, todo.RecurrenceUnit)
			if err != nil {
				tx.Rollback()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		} else {
			// Non-recurring handling
			now := time.Now()
			todo.CompletedAt = &now
		}
	}
	if !todo.Completed {
		todo.CompletedAt = nil
	}

	// Clear recurrence fields if interval is 0 or unit is empty
	if (todo.RecurrenceInterval != nil && *todo.RecurrenceInterval == 0) ||
		(todo.RecurrenceUnit != nil && *todo.RecurrenceUnit == "") {
		todo.RecurrenceInterval = nil
		todo.RecurrenceUnit = nil
	}

	stmt, err := db.Prepare("UPDATE todos SET title = ?, completed = ?, completed_at = ?, due_date = ?, recurrence_interval = ?, recurrence_unit = ? WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(todo.Title, todo.Completed, todo.CompletedAt, todo.DueDate, todo.RecurrenceInterval, todo.RecurrenceUnit, todo.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteTodo(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	stmt, err := db.Prepare("DELETE FROM todos WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	// Create a new HTTP server
	server := &http.Server{
		Addr:    ":8081",
		Handler: createRouter(),
	}

	fmt.Println("Server running on port 8081")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func createRouter() http.Handler {
	mux := http.NewServeMux()

	// Serve static files
	mux.Handle("/", http.FileServer(http.Dir(".")))

	mux.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getProjects(w, r)
		case http.MethodPost:
			addProject(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getProject(w, r)
		case http.MethodPut:
			updateProject(w, r)
		case http.MethodDelete:
			deleteProject(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/todos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		getTodos(w, r)
	})

	mux.HandleFunc("/api/todo", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			addTodo(w, r)
		case http.MethodPut:
			updateTodo(w, r)
		case http.MethodDelete:
			deleteTodo(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return mux
}

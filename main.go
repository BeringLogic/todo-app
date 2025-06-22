package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
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
	Position           int        `json:"position"`
	ProjectID          int        `json:"project_id"`
}

type Project struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed index.html
var indexHTML string

var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("sqlite3", "./data/todos.db")
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

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create iofs source driver: %v", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
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

	// Get the highest position, defaulting to 0 if no projects exist
	var maxPosition int
	err := db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM projects").Scan(&maxPosition)
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
		SELECT id, title, completed, created_at, completed_at, due_date, recurrence_interval, recurrence_unit, project_id, position 
		FROM todos 
		ORDER BY project_id, completed, position
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	todos := make([]Todo, 0)
	for rows.Next() {
		var todo Todo
		if err := rows.Scan(&todo.ID, &todo.Title, &todo.Completed, &todo.CreatedAt, &todo.CompletedAt, &todo.DueDate, &todo.RecurrenceInterval, &todo.RecurrenceUnit, &todo.ProjectID, &todo.Position); err != nil {
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

	// Determine position so the new todo appears at the top of the active list for its project
	var minPos sql.NullInt64
	if err := db.QueryRow("SELECT MIN(position) FROM todos WHERE project_id = ? AND completed = 0", todo.ProjectID).Scan(&minPos); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if minPos.Valid {
		todo.Position = int(minPos.Int64) - 1
	} else {
		todo.Position = 0
	}

	stmt, err := db.Prepare("INSERT INTO todos (title, completed, project_id, due_date, recurrence_interval, recurrence_unit, position) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(todo.Title, todo.Completed, todo.ProjectID, todo.DueDate, todo.RecurrenceInterval, todo.RecurrenceUnit, todo.Position); err != nil {
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

	// Fetch previous completion state to detect transition
	var prevCompleted bool
	var prevCompletedAt sql.NullTime
	if err := db.QueryRow("SELECT completed, completed_at FROM todos WHERE id = ?", todo.ID).Scan(&prevCompleted, &prevCompletedAt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If transitioning from completed to active (uncomplete), move todo to top of active list
	if prevCompleted && !todo.Completed {
		var minPos sql.NullInt64
		if err := db.QueryRow("SELECT MIN(position) FROM todos WHERE project_id = ? AND completed = 0", todo.ProjectID).Scan(&minPos); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newPos := 0
		if minPos.Valid {
			newPos = int(minPos.Int64) - 1
		}
		todo.Position = newPos
	}

	// If transitioning from active to completed
	if !prevCompleted && todo.Completed {
		// move todo to top of completed list
		var minPosCompleted sql.NullInt64
		if err := db.QueryRow("SELECT MIN(position) FROM todos WHERE project_id = ? AND completed = 1", todo.ProjectID).Scan(&minPosCompleted); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if minPosCompleted.Valid {
			todo.Position = int(minPosCompleted.Int64) - 1
		} else {
			todo.Position = 0
		}

		// Handle recurring logic
		if todo.RecurrenceInterval != nil && todo.RecurrenceUnit != nil {
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// complete current
			now := time.Now()
			todo.CompletedAt = &now
			_, err = tx.Exec("UPDATE todos SET title=?, completed=1, completed_at=?, due_date=?, recurrence_interval=?, recurrence_unit=?, position=? WHERE id=?", todo.Title, todo.CompletedAt, todo.DueDate, todo.RecurrenceInterval, todo.RecurrenceUnit, todo.Position, todo.ID)
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

	stmt, err := db.Prepare("UPDATE todos SET title = ?, completed = ?, completed_at = ?, due_date = ?, recurrence_interval = ?, recurrence_unit = ?, project_id = ?, position = ? WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(todo.Title, todo.Completed, todo.CompletedAt, todo.DueDate, todo.RecurrenceInterval, todo.RecurrenceUnit, todo.ProjectID, todo.Position, todo.ID); err != nil {
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

func reorderProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ids []int
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stmt, err := tx.Prepare("UPDATE projects SET position = ? WHERE id = ?")
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for pos, id := range ids {
		if _, err := stmt.Exec(pos+1, id); err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
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

	// Serve embedded index.html at root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	})

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

	mux.HandleFunc("/api/todos/reorder", reorderTodos)
	mux.HandleFunc("/api/projects/reorder", reorderProjects)

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

func reorderTodos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ids []int
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stmt, err := tx.Prepare("UPDATE todos SET position = ? WHERE id = ?")
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for pos, id := range ids {
		if _, err := stmt.Exec(pos+1, id); err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

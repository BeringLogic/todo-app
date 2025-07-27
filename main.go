package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apognu/gocal"
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
	DueDate            *time.Time `json:"due_date,omitempty"`
	RecurrenceInterval *int       `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string    `json:"recurrence_unit,omitempty"`
	Position           int        `json:"position"`
	ProjectID          int        `json:"project_id"`
	UID                string     `json:"uid,omitempty"`
}

type Project struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

type IcsSubscription struct {
	ID            int        `json:"id"`
	URL           string     `json:"url"`
	ProjectID     int        `json:"project_id"`
	ProjectName   string     `json:"project_name"`
	LastUpdatedAt *time.Time `json:"last_updated_at"`
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed index.html
var indexHTML string

//go:embed favicon.svg
var faviconSVG []byte

//go:embed style.css
var styleCSS []byte

var db *sql.DB

func init() {
	log.SetFlags(log.LstdFlags)
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

	projects := make([]Project, 0)
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

	stmt, err = db.Prepare("DELETE FROM ics_subscriptions WHERE project_id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
		SELECT id, title, completed, created_at, completed_at, 
		       datetime(due_date) as due_date, 
		       recurrence_interval, recurrence_unit, project_id, position 
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
		var dueDateStr sql.NullString
		if err := rows.Scan(
			&todo.ID,
			&todo.Title,
			&todo.Completed,
			&todo.CreatedAt,
			&todo.CompletedAt,
			&dueDateStr,
			&todo.RecurrenceInterval,
			&todo.RecurrenceUnit,
			&todo.ProjectID,
			&todo.Position,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Parse the due date string into a time.Time pointer
		if dueDateStr.Valid && dueDateStr.String != "" {
			var parsedTime time.Time
			var err error

			// First try parsing as RFC3339 (UTC timestamp with timezone)
			parsedTime, err = time.Parse(time.RFC3339, dueDateStr.String)
			if err != nil {
				// If that fails, try parsing as database datetime format (YYYY-MM-DD HH:MM:SS)
				parsedTime, err = time.ParseInLocation("2006-01-02 15:04:05", dueDateStr.String, time.UTC)
				if err != nil {
					log.Printf("Warning: could not parse due date '%s': %v", dueDateStr.String, err)
					continue
				}
			}
			// Ensure it's in UTC
			todo.DueDate = &parsedTime
		}
		todos = append(todos, todo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(todos)
}

func addTodo(w http.ResponseWriter, r *http.Request) {
	var requestData struct {
		Title              string  `json:"title"`
		Completed          bool    `json:"completed"`
		ProjectID          int     `json:"project_id"`
		DueDate            *string `json:"due_date,omitempty"`
		RecurrenceInterval *int    `json:"recurrence_interval,omitempty"`
		RecurrenceUnit     *string `json:"recurrence_unit,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Shift all existing todos in the same project down by 1 position
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = tx.Exec("UPDATE todos SET position = position + 1 WHERE project_id = ?", requestData.ProjectID)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the due date if provided (expecting UTC timestamp from frontend)
	var dueDate *time.Time
	if requestData.DueDate != nil && *requestData.DueDate != "" {
		parsedTime, err := time.Parse(time.RFC3339, *requestData.DueDate)
		if err != nil {
			tx.Rollback()
			http.Error(w, "invalid date format, expected RFC3339 format (e.g., 2023-01-02T15:04:05Z)", http.StatusBadRequest)
			return
		}
		// Ensure it's in UTC
		parsedTime = parsedTime.UTC()
		dueDate = &parsedTime
	}

	var dueDateInterface interface{}
	if dueDate != nil {
		dueDateInterface = dueDate.Format(time.RFC3339)
	}

	result, err := tx.Exec(
		"INSERT INTO todos (title, completed, project_id, due_date, recurrence_interval, recurrence_unit, position) VALUES (?, ?, ?, datetime(?, 'utc'), ?, ?, 0)",
		requestData.Title,
		requestData.Completed,
		requestData.ProjectID,
		dueDateInterface,
		requestData.RecurrenceInterval,
		requestData.RecurrenceUnit,
	)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the created todo
	createdTodo := Todo{
		ID:                 int(id),
		Title:              requestData.Title,
		Completed:          requestData.Completed,
		ProjectID:          requestData.ProjectID,
		DueDate:            dueDate,
		RecurrenceInterval: requestData.RecurrenceInterval,
		RecurrenceUnit:     requestData.RecurrenceUnit,
		Position:           0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(createdTodo)
}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	// Helper to check if a todo is currently completed
	currentTodoIsCompleted := func(todoID int) bool {
		var completed bool
		err := db.QueryRow("SELECT completed FROM todos WHERE id = ?", todoID).Scan(&completed)
		return err == nil && completed
	}

	// Define a struct to parse the incoming request
	var requestData struct {
		ID                 int     `json:"id"`
		Title              string  `json:"title"`
		Completed          bool    `json:"completed"`
		ProjectID          int     `json:"project_id"`
		DueDate            *string `json:"due_date,omitempty"`
		RecurrenceInterval *int    `json:"recurrence_interval,omitempty"`
		RecurrenceUnit     *string `json:"recurrence_unit,omitempty"`
		Position           int     `json:"position,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if requestData.ID == 0 {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	// Get the current todo to preserve position and project ID if not provided
	var currentTodo struct {
		Position  int
		ProjectID int
		DueDate   sql.NullString
	}

	err := db.QueryRow("SELECT position, project_id, datetime(due_date) as due_date FROM todos WHERE id = ?", requestData.ID).
		Scan(&currentTodo.Position, &currentTodo.ProjectID, &currentTodo.DueDate)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the due date if provided (expecting UTC timestamp from frontend)
	var dueDate *time.Time
	if requestData.DueDate != nil {
		// If the due date is being cleared
		if *requestData.DueDate == "" {
			dueDate = nil
		} else {
			// Parse as RFC3339 (UTC timestamp with timezone)
			parsedTime, err := time.Parse(time.RFC3339, *requestData.DueDate)
			if err != nil {
				http.Error(w, "invalid date format, expected RFC3339 format (e.g., 2023-01-02T15:04:05Z)", http.StatusBadRequest)
				return
			}
			// Ensure it's in UTC
			parsedTime = parsedTime.UTC()
			dueDate = &parsedTime
		}
	} else if currentTodo.DueDate.Valid && currentTodo.DueDate.String != "" {
		// Keep the existing due date if not being updated
		// Parse as UTC timestamp from database
		parsedTime, err := time.Parse(time.RFC3339, currentTodo.DueDate.String)
		if err != nil {
			http.Error(w, fmt.Sprintf("error parsing existing due date: %v", err), http.StatusInternalServerError)
			return
		}
		dueDate = &parsedTime
	}

	// Use current project ID and position if not provided in the request
	projectID := requestData.ProjectID
	if projectID == 0 {
		projectID = currentTodo.ProjectID
	}

	position := requestData.Position
	if position == 0 {
		position = currentTodo.Position
	}

	// Prepare the due date for SQL
	var dueDateInterface interface{}
	if dueDate != nil {
		dueDateInterface = dueDate.Format(time.RFC3339)
	}

	// Begin transaction for atomic position updates
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine if completed status is changing and handle position logic
	var newPosition int
	if requestData.Completed && !currentTodoIsCompleted(requestData.ID) {
		// Moving to completed: shift all completed todos down and set this to top
		row := tx.QueryRow("SELECT MIN(position) FROM todos WHERE project_id = ? AND completed = 1", projectID)
		var minCompleted sql.NullInt64
		if err := row.Scan(&minCompleted); err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if minCompleted.Valid {
			_, err = tx.Exec("UPDATE todos SET position = position + 1 WHERE project_id = ? AND completed = 1", projectID)
			if err != nil {
				tx.Rollback()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			newPosition = int(minCompleted.Int64) - 1
		} else {
			newPosition = 0
		}
	} else if !requestData.Completed && currentTodoIsCompleted(requestData.ID) {
		// Moving to active: shift all active todos down and set this to top
		row := tx.QueryRow("SELECT MIN(position) FROM todos WHERE project_id = ? AND completed = 0", projectID)
		var minActive sql.NullInt64
		if err := row.Scan(&minActive); err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if minActive.Valid {
			_, err = tx.Exec("UPDATE todos SET position = position + 1 WHERE project_id = ? AND completed = 0", projectID)
			if err != nil {
				tx.Rollback()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			newPosition = int(minActive.Int64) - 1
		} else {
			newPosition = 0
		}
	} else {
		// No status change: keep position
		newPosition = position
	}

	// Update the todo in the database
	_, err = tx.Exec(
		"UPDATE todos SET title = ?, completed = ?, project_id = ?, due_date = datetime(?, 'utc'), recurrence_interval = ?, recurrence_unit = ?, position = ? WHERE id = ?",
		requestData.Title,
		requestData.Completed,
		projectID,
		dueDateInterface,
		requestData.RecurrenceInterval,
		requestData.RecurrenceUnit,
		newPosition,
		requestData.ID,
	)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a recurring todo is being completed, generate the next occurrence
	// The next occurrence will keep the same time-of-day as the original due date.
	if requestData.Completed && !currentTodoIsCompleted(requestData.ID) && requestData.RecurrenceInterval != nil && requestData.RecurrenceUnit != nil {
		var baseDue time.Time
		if dueDate != nil {
			baseDue = *dueDate
		} else {
			baseDue = time.Now().UTC()
		}

		// Calculate the next due date, preserving the time of day
		var nextDue time.Time
		switch strings.ToLower(*requestData.RecurrenceUnit) {
		case "day", "days":
			nextDue = baseDue.AddDate(0, 0, *requestData.RecurrenceInterval)
		case "week", "weeks":
			nextDue = baseDue.AddDate(0, 0, 7*(*requestData.RecurrenceInterval))
		case "month", "months":
			nextDue = baseDue.AddDate(0, *requestData.RecurrenceInterval, 0)
		case "year", "years":
			nextDue = baseDue.AddDate(*requestData.RecurrenceInterval, 0, 0)
		default:
			// Unknown unit, skip recurrence
			nextDue = time.Time{}
		}

		// Explicitly preserve hour, minute, second, nanosecond from baseDue (already preserved by AddDate, but this is robust)
		if !nextDue.IsZero() {
			nextDue = time.Date(
				nextDue.Year(), nextDue.Month(), nextDue.Day(),
				baseDue.Hour(), baseDue.Minute(), baseDue.Second(), baseDue.Nanosecond(),
				time.UTC,
			)
			_, err := tx.Exec(
				"INSERT INTO todos (title, completed, created_at, due_date, recurrence_interval, recurrence_unit, project_id, position) VALUES (?, 0, datetime('now', 'utc'), datetime(?, 'utc'), ?, ?, ?, 0)",
				requestData.Title,
				nextDue.Format(time.RFC3339),
				requestData.RecurrenceInterval,
				requestData.RecurrenceUnit,
				projectID,
			)
			if err != nil {
				tx.Rollback()
				http.Error(w, "Failed to create next recurring todo: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated todo
	updatedTodo := Todo{
		ID:                 requestData.ID,
		Title:              requestData.Title,
		Completed:          requestData.Completed,
		ProjectID:          projectID,
		DueDate:            dueDate,
		RecurrenceInterval: requestData.RecurrenceInterval,
		RecurrenceUnit:     requestData.RecurrenceUnit,
		Position:           newPosition,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedTodo)
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

	// Periodically refresh ICS feeds
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			refreshIcsFeeds()
		}
	}()

	log.Println("Server running on port 8081")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func createRouter() http.Handler {
	mux := http.NewServeMux()

	// Serve favicon.svg
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(faviconSVG)
	})

	// Serve style.css
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write(styleCSS)
	})

	// Serve embedded index.html at root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only serve index.html for root path or non-file paths
		if r.URL.Path == "/" || r.URL.Path == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, indexHTML)
		} else {
			http.NotFound(w, r)
		}
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

	mux.HandleFunc("/api/subscribe_ics", subscribeToICSHandler)
	mux.HandleFunc("/api/ics_subscriptions", getICSSubscriptionsHandler)
	mux.HandleFunc("/api/cancel_ics_subscription", cancelICSSubscriptionHandler)

	return mux
}

func getICSSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT s.id, s.url, s.project_id, p.title, s.last_updated_at
		FROM ics_subscriptions s
		JOIN projects p ON s.project_id = p.id
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	subscriptions := make([]IcsSubscription, 0)
	for rows.Next() {
		var sub IcsSubscription
		if err := rows.Scan(&sub.ID, &sub.URL, &sub.ProjectID, &sub.ProjectName, &sub.LastUpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		subscriptions = append(subscriptions, sub)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subscriptions)
}

func cancelICSSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}

	// Get the project_id before deleting the subscription
	var projectID int
	err := db.QueryRow("SELECT project_id FROM ics_subscriptions WHERE id = ?", id).Scan(&projectID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Subscription not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the subscription
	stmt, err := db.Prepare("DELETE FROM ics_subscriptions WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete todos associated with the project
	stmt, err = db.Prepare("DELETE FROM todos WHERE project_id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(projectID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the project
	stmt, err = db.Prepare("DELETE FROM projects WHERE id = ?")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(projectID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

func subscribeToICSHandler(w http.ResponseWriter, r *http.Request) {
	var requestData struct {
		URL         string `json:"url"`
		ProjectName string `json:"project_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if the user is already subscribed to this feed
	var existingSubscriptionID int
	err := db.QueryRow("SELECT id FROM ics_subscriptions WHERE url = ?", requestData.URL).Scan(&existingSubscriptionID)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existingSubscriptionID != 0 {
		http.Error(w, "You are already subscribed to this ICS feed", http.StatusConflict)
		return
	}

	// Find or create the project
	var projectID int
	err = db.QueryRow("SELECT id FROM projects WHERE title = ?", requestData.ProjectName).Scan(&projectID)
	if err == sql.ErrNoRows {
		// Create the project if it doesn't exist
		stmt, err := db.Prepare("INSERT INTO projects (title, position) VALUES (?, (SELECT COALESCE(MAX(position), 0) + 1 FROM projects))")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		result, err := stmt.Exec(requestData.ProjectName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projectID = int(id)
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stmt, err := db.Prepare("INSERT INTO ics_subscriptions (url, project_id) VALUES (?, ?)")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	if _, err := stmt.Exec(requestData.URL, projectID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go refreshIcsFeeds()

	w.WriteHeader(http.StatusCreated)
}

func refreshIcsFeeds() {
	rows, err := db.Query("SELECT id, url, project_id FROM ics_subscriptions")
	if err != nil {
		log.Printf("Error getting ICS subscriptions: %v", err)
		return
	}
	defer rows.Close()

	var subscriptions []IcsSubscription
	for rows.Next() {
		var sub IcsSubscription
		if err := rows.Scan(&sub.ID, &sub.URL, &sub.ProjectID); err != nil {
			log.Printf("Error scanning ICS subscription: %v", err)
			continue
		}
		subscriptions = append(subscriptions, sub)
	}
	rows.Close()

	for _, sub := range subscriptions {
		var maxPosition int
		err := db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM todos WHERE project_id = ?", sub.ProjectID).Scan(&maxPosition)
		if err != nil {
			log.Printf("Error getting max position for project %d: %v", sub.ProjectID, err)
			continue
		}
		positionCounter := maxPosition + 1

		resp, err := http.Get(sub.URL)
		if err != nil {
			log.Printf("Error fetching ICS feed from %s: %v", sub.URL, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Error fetching ICS feed from %s: status code %d, body: %s", sub.URL, resp.StatusCode, string(body))
			continue
		}

		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endOfYear := time.Date(now.Year()+2, time.December, 31, 23, 59, 59, 0, time.UTC)

		cal := gocal.NewParser(resp.Body)
		cal.Start = &startOfDay
		cal.End = &endOfYear
		cal.Parse()

		for _, event := range cal.Events {
			var existingTodoID int
			err := db.QueryRow("SELECT id FROM todos WHERE uid = ?", event.Uid).Scan(&existingTodoID)
			if err != nil && err != sql.ErrNoRows {
				log.Printf("Error checking for existing todo with UID %s: %v", event.Uid, err)
				continue
			}

			if existingTodoID == 0 {
				var dueDate *time.Time
				if event.Start != nil {
					// Heuristic to check for all-day events: time is exactly midnight.
					if event.Start.Hour() == 0 && event.Start.Minute() == 0 && event.Start.Second() == 0 {
						// For all-day events, create a new time in local time using the event's date, then convert to UTC.
						year, month, day := event.Start.Date()
						localTime := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
						utcTime := localTime.UTC()
						dueDate = &utcTime
					} else {
						// For timed events, just convert to UTC
						utcTime := event.Start.UTC()
						dueDate = &utcTime
					}
				}

				// Todo doesn't exist, so create it
				_, err := db.Exec(
					"INSERT INTO todos (title, completed, project_id, due_date, uid, position) VALUES (?, 0, ?, ?, ?, ?)",
					event.Summary,
					sub.ProjectID,
					dueDate,
					event.Uid,
					positionCounter,
				)
				if err != nil {
					log.Printf("Error inserting new todo with UID %s: %v", event.Uid, err)
				} else {
					positionCounter++
				}
			}
		}

		// Update the last_updated_at timestamp
		stmt, err := db.Prepare("UPDATE ics_subscriptions SET last_updated_at = ? WHERE id = ?")
		if err != nil {
			log.Printf("Error preparing statement: %v", err)
			continue
		}

		if _, err := stmt.Exec(time.Now(), sub.ID); err != nil {
			log.Printf("Error updating last_updated_at: %v", err)
		}
		stmt.Close()
	}
}

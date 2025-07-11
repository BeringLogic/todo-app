# Todo List Web App

Vibe Coded todo app, to check out Windsurf and Gemini.

Todo List application built with Go, SQLite, and vanilla JavaScript.

## ✨ Features

- **Task Management**
  - Add, edit, and delete todos
  - Mark todos as complete/incomplete
  - Reorder todos via drag and drop
  - Due dates with visual indicators
  - Recurring tasks with custom intervals (daily, weekly, monthly, yearly)

- **Project Organization**
  - Create and manage multiple projects
  - Drag and drop todos between projects
  - Reorder projects via drag and drop
  - Special "project" listing upcoming tasks

- **User Experience**
  - Clean, modern UI with Catppuccin color themes
  - Light/Dark mode toggle
  - Import from Google Tasks
  - Import from ICS calendar
  - Subscribe to ICS calendar
  - Export/Import database

## 🚀 Quick Start

### Prerequisites

- Go 1.16 or higher
- SQLite3

### Installation

#### From Source

1. Clone the repository:
   ```bash
   git clone https://github.com/BeringLogic/todo-app.git
   cd todo-app
   ```

2. Install dependencies:
   ```bash
   go mod tidy
   ```

3. Run the application:
   ```bash
   go run main.go
   ```

4. Open your browser and navigate to [http://localhost:8081](http://localhost:8081)

#### Docker

1. ```bash
   docker run -p 8081:8081 -v todo-app-data:/data beringlogic/todo-app:latest
   ```

1. Open your browser and navigate to [http://localhost:8081](http://localhost:8081)

## ⌨️ Keyboard Shortcuts

- `Enter` - Submit todo (when in input field)
- `Shift + Enter` - Add new line in todo text
- `Escape` - Clear todo input
- `Click` on todo text - Edit todo
- `Drag` todo - Reorder or move between projects

## 📄 License

MIT

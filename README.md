# Todo List Web App

A simple Todo List application built with Go and SQLite.

## Features

- Add new todos
- Mark todos as completed/incomplete
- Delete todos
- Persistent storage using SQLite
- Clean and modern UI

## Setup

1. Install Go if you haven't already
2. Install SQLite3 driver:
   ```bash
   go mod tidy
   ```
3. Run the application:
   ```bash
   go run main.go
   ```
4. Open your browser and navigate to `http://localhost:8080`

## API Endpoints

- `GET /todos` - Get all todos
- `POST /todo/add` - Add a new todo
- `POST /todo/update` - Update a todo's status
- `POST /todo/delete` - Delete a todo

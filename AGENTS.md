# Agent Guidelines for todo-app

## Build/Test Commands
- **Build**: `go build` or `CGO_ENABLED=1 go build -o todo-app .`
- **Run**: `go run main.go`
- **Dependencies**: `go mod tidy`
- **Single test**: No test framework configured - add tests using Go's testing package
- **Lint**: No linter configured - consider `gofmt -d .` or `go vet`

## Code Style Guidelines

### Go Backend
- **Imports**: Group standard library, third-party, then local imports
- **Naming**: PascalCase for exported types/structs, camelCase for variables/functions
- **Error handling**: Use `http.Error()` for HTTP responses, `log.Fatal()` for startup errors
- **Database**: Use prepared statements, transactions for multi-step operations
- **Types**: Use struct tags for JSON, pointers for optional fields (e.g., `*time.Time`)

### JavaScript Frontend
- **Syntax**: Modern ES6+ with `const`/`let`, arrow functions, template literals
- **Naming**: camelCase for variables/functions, PascalCase for constructors
- **Comments**: JSDoc style for functions, inline comments for complex logic
- **Error handling**: Try/catch blocks, user-friendly error messages

### General
- **Architecture**: REST API with JSON responses, embedded static files
- **Security**: Input validation, prepared statements, no secrets in code
- **Formatting**: Follow `gofmt` for Go, consistent indentation for JS
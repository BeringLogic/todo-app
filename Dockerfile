# syntax=docker/dockerfile:1

############################################
# Build stage
############################################
FROM golang:1.24-alpine AS builder

# Install build dependencies for CGO & SQLite
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go modules manifests first and download deps (leverages Docker layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source
COPY . .

# Build the binary (CGO enabled so that github.com/mattn/go-sqlite3 works)
RUN CGO_ENABLED=1 go build -v -o todo-app .

############################################
# Runtime stage
############################################
FROM alpine:latest AS runner

# Install runtime dependencies for the compiled binary (libsqlite3 & CA certs)
RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /

# Copy compiled binary and application assets
COPY --from=builder /app/todo-app ./app/todo-app

# Create data directory
RUN mkdir /data

# Ensure non-root user can write the database file
RUN chown -R nobody:nobody /data

# Expose the port used by the Go server
EXPOSE 8081
ENV PORT=8081

# Run as non-root for security where possible
USER nobody

ENTRYPOINT ["./app/todo-app"]

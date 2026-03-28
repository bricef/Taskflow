# Default: list available recipes
default:
    @just --list

# Run all tests
test *args:
    go test -count=1 ./... {{args}}

# Run tests with verbose output
test-v *args:
    go test -v -count=1 ./... {{args}}

# Build all binaries
build:
    go build -o taskflow-server ./cmd/taskflow-server
    go build -o taskflow ./cmd/taskflow

# Format all Go files
fmt:
    gofmt -w .

# Check formatting (fails if files need formatting)
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

# Run go vet
vet:
    go vet ./...

# Tidy module dependencies
tidy:
    go mod tidy

# Run all checks (fmt, vet, test)
check: fmt-check vet test

# Run all checks with verbose test output
check-v: fmt-check vet test-v

# Run the server locally
run:
    TASKFLOW_SEED_ADMIN_NAME=admin go run ./cmd/taskflow-server

# Build Docker image
docker-build:
    docker build -t taskflow .

# Run with Docker Compose
docker-up:
    docker compose up -d

# Stop Docker Compose
docker-down:
    docker compose down

# View Docker Compose logs
docker-logs:
    docker compose logs -f

# Generate a test database with realistic content
seed:
    go run ./cmd/taskflow-seed

# Run the server with the test database
run-test: seed
    TASKFLOW_DB_PATH=./taskflow-test.db TASKFLOW_LISTEN_ADDR=:8374 go run ./cmd/taskflow-server

# Run the TUI against the test database
tui-test:
    TASKFLOW_API_KEY=seed-admin-key-for-testing go run ./cmd/taskflow-tui

# Clean build artifacts
clean:
    rm -f taskflow taskflow-server *.db seed-admin-key.txt

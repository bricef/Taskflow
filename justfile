# Docker image repository (override with DOCKER_REPO env var or `just --set DOCKER_REPO ...`)
DOCKER_REPO := env("DOCKER_REPO", "taskflow")

# Default: list available recipes
default:
    @just --list

# Run all tests (unit + integration + QA smoke)
test *args:
    go test -count=1 -race ./... {{args}}
    ./scripts/qa-test.sh

# Run unit and integration tests only (no QA smoke)
test-unit *args:
    go test -count=1 -race ./... {{args}}

# Run tests with verbose output
test-v *args:
    go test -v -count=1 -race ./... {{args}}

# Version from git (tag or short sha)
VERSION := `git describe --tags --always --dirty 2>/dev/null || echo dev`
LDFLAGS := "-X github.com/bricef/taskflow/internal/version.Version=" + VERSION

# Build all binaries
build:
    go build -ldflags '{{LDFLAGS}}' -o taskflow-server ./cmd/taskflow-server
    go build -ldflags '{{LDFLAGS}}' -o taskflow ./cmd/taskflow
    go build -ldflags '{{LDFLAGS}}' -o taskflow-mcp ./cmd/taskflow-mcp

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

# Build Docker image with BuildKit cache (used in CI)
docker-build-cached:
    docker buildx build --load \
      --cache-from type=gha --cache-to type=gha,mode=max \
      -t taskflow .

# Push Docker image to registry
docker-push tag="latest":
    docker tag taskflow {{DOCKER_REPO}}:{{tag}}
    docker push {{DOCKER_REPO}}:{{tag}}

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
    rm -f taskflow taskflow-server taskflow-mcp *.db *.db-wal *.db-shm seed-admin-key.txt

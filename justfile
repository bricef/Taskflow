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
    go build ./...

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

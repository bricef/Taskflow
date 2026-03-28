# TaskFlow

TaskFlow is a single-user task tracker designed for fluid collaboration between a human operator and multiple AI agents. It provides a durable server as the single source of truth, with tasks organized on kanban boards that have explicitly configured workflow state machines. Any actor -- human or AI -- can create, advance, review, and manage tasks, with a full audit trail recording every action with actor attribution and timestamps.

## Quick Start

```bash
# Start the server (creates DB and seed admin on first run)
TASKFLOW_SEED_ADMIN_NAME=admin ./taskflow-server

# Use the CLI (reads API key from env)
export TASKFLOW_API_KEY=$(cat seed-admin-key.txt)
taskflow board create --slug my-board --name "My Board" --workflow '...'
taskflow task create my-board --title "Fix auth bug" --priority high
taskflow task list my-board
taskflow task transition my-board 1 --transition start --comment "On it"
```

## Documentation

- **[HTTP API Reference](docs/http-api.md)** — all endpoints, authentication, error handling, configuration
- **[CLI Reference](docs/cli.md)** — all commands, flags, output formats
- **[OpenAPI Spec](http://localhost:8374/openapi.json)** — machine-readable, auto-generated from operation definitions

## Status

**Increments 1–4 are complete.** The server and CLI are functional for full task management: actors, boards, tasks, comments, dependencies, attachments, webhooks, and audit. 127 tests passing.

See [docs/](docs/) for the PRD and implementation plans.

## Architecture

Operations are defined once in `model.Operations()` and consumed by three layers:

```
model.Operations()              ← canonical domain operations (action, path, role, input/output)
      │              │              │
      ▼              ▼              ▼
internal/http    internal/cli    openapi.json
Derives:         Derives:        Derives:
 HTTP method      command tree    endpoint docs
 routes           flags/args      schemas
 handlers         HTTP calls      parameters
```

The codebase separates business logic from storage:

```
taskflow.TaskFlow  (Go interface for compile-time safety)
      │
service.Service    (business logic: validation, audit, orchestration)
      │
repo.Store         (storage-agnostic CRUD interfaces + Transactor)
      │
sqlite.Store       (SQLite via sqlx, generic struct mapper)
```

To swap the storage backend, implement `repo.Store` in a new package — nothing in `model/`, `service/`, or `taskflow/` changes.

## Project Structure

```
internal/
├── model/              Domain types and operation definitions
│   ├── operations.go       Canonical operation list (action, path, role, input/output)
│   ├── actor.go            Actor, ActorType, Role
│   ├── board.go            Board, slug validation
│   ├── task.go             Task, Priority, TaskFilter, TaskSort, TransitionTaskParams
│   ├── comment.go          Comment
│   ├── dependency.go       Dependency, DependencyType
│   ├── attachment.go       Attachment, RefType
│   ├── webhook.go          Webhook
│   ├── audit.go            AuditEntry, AuditAction
│   ├── workflow.go         Workflow definition, WorkflowHealthIssue
│   ├── optional.go         Optional[T] with JSON marshaling
│   └── errors.go           ValidationError, NotFoundError, ConflictError
│
├── taskflow/           Go interface for all business operations
├── service/            Business logic (storage-agnostic)
├── repo/               Storage-agnostic repository interfaces
├── sqlite/             SQLite implementation (sqlx, generic mapper)
├── workflow/           Workflow engine (state machine, JSON Schema validation)
├── http/               HTTP server (derived routes, middleware, OpenAPI generation)
├── cli/                CLI client (derived commands, HTTP calls)
└── testutil/           Test helpers

cmd/
├── taskflow-server/    Server binary
└── taskflow/           CLI binary

migrations/             Embedded SQL migrations
```

## Key Design Decisions

- **Operations as data** — `model.Operations()` defines every operation once; HTTP routes, CLI commands, and OpenAPI docs are all derived from it
- **`Optional[T]`** for partial updates — cleanly distinguishes "not provided" from "set to nil" with JSON marshaling support
- **Typed actions** — `model.Action` enum with constants; HTTP method and status are derived by the transport layer
- **Attachments are references** — all attachments (including files) are typed references; file storage is infrastructure, not domain
- **Generic struct mapper** — `toModel`/`fromModel` use reflection with a cached dispatch table; field mismatches panic at startup
- **Workflow engine** — pure state machine with JSON Schema validation; workflows are validated on board creation

## Development

Requires Go 1.23+ and [just](https://github.com/casey/just).

```
just test       # run tests
just test-v     # run tests (verbose)
just build      # build all packages
just fmt        # format code
just vet        # go vet
just check      # fmt-check + vet + test
```

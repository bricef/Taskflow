# TaskFlow

TaskFlow is a single-user task tracker designed for fluid collaboration between a human operator and multiple AI agents. It provides a durable server as the single source of truth, with tasks organized on kanban boards that have explicitly configured workflow state machines. Any actor -- human or AI -- can create, advance, review, and manage tasks, with a full audit trail recording every action with actor attribution and timestamps.

The system exposes multiple interface surfaces -- a CLI for scripting and automation, a TUI for interactive visual management, and an MCP server for AI agent integration -- all derived mechanically from a shared operation registry. Each operation is defined once as a typed Go struct, and the framework handles auth, RBAC, input parsing, validation, output formatting, and audit logging uniformly across all interfaces. The server is built with Go and SQLite (WAL mode + FTS5), deployed as a single binary or Docker container.

## Status

**Increment 1 (Schema, Models & Database Layer) is complete.** This provides the full data model, SQLite schema with FTS5 full-text search, migration runner, and a tested service layer with audit logging. There is no HTTP server or CLI yet -- those come in later increments.

See [docs/plans/](docs/plans/) for the implementation plan.

## Architecture

The codebase separates business logic from storage so the backend can be swapped without touching domain code:

```
internal/
├── model/          Domain types, enums, validation, error types
│   ├── actor.go        Actor, ActorType, Role
│   ├── board.go        Board, slug validation
│   ├── task.go         Task, Priority, TaskFilter, TaskSort
│   ├── comment.go      Comment
│   ├── dependency.go   Dependency, DepType
│   ├── file.go         File
│   ├── attachment.go   Attachment, RefType
│   ├── webhook.go      Webhook
│   ├── audit.go        AuditEntry, AuditAction
│   ├── workflow.go     Workflow definition (data type only)
│   └── errors.go       ValidationError, NotFoundError, ConflictError
│
├── repo/           Repository interfaces (defined by the domain)
│   └── repo.go         Thin CRUD interfaces + Transactor + Tx
│
├── service/        Business logic (storage-agnostic)
│   ├── service.go      Service struct, audit helper
│   ├── actors.go       Validation, CRUD orchestration
│   ├── boards.go       Validation, soft-delete, task reassignment
│   ├── tasks.go        Validation, sequential numbering, change tracking
│   ├── comments.go     Validation, task existence check
│   ├── dependencies.go Validation, self-dep prevention
│   ├── files.go        Validation, referential integrity
│   ├── attachments.go  Validation, file-or-ref enforcement
│   ├── webhooks.go     Validation, CRUD
│   └── audit.go        Audit query pass-through
│
├── sqlite/         SQLite implementation of repo interfaces
│   ├── store.go        Store struct, InTransaction, interface assertion
│   ├── migrate.go      Embedded migration runner
│   ├── helpers.go      Time parsing, error helpers
│   ├── actors.go       Pure SQL CRUD
│   ├── boards.go       Pure SQL CRUD + task num allocation
│   ├── tasks.go        Pure SQL CRUD + FTS5 search
│   ├── comments.go     Pure SQL CRUD
│   ├── dependencies.go Pure SQL CRUD
│   ├── files.go        Pure SQL CRUD
│   ├── attachments.go  Pure SQL CRUD
│   ├── webhooks.go     Pure SQL CRUD
│   └── audit.go        Audit insert + queries
│
└── testutil/       Test helpers
    └── testutil.go     NewTestService, SeedActor, SeedBoard, SeedTask

migrations/
├── embed.go                  go:embed for SQL files
├── 001_initial_schema.sql    9 tables with constraints and indexes
└── 002_fts5.sql              FTS5 virtual table + sync triggers
```

To swap the storage backend (e.g., PostgreSQL), implement the `repo.Store` interface in a new package and wire it in at startup. Nothing in `model/` or `service/` changes.

## Data Model

- **Actors** -- humans or AI agents, identified by name, with roles (admin/member/read_only)
- **Boards** -- kanban boards with slug identifiers and embedded workflow definitions
- **Tasks** -- identified by `board-slug/N`, with priority, tags, assignee, due date
- **Comments** -- attributed to actors, chronologically ordered
- **Dependencies** -- `depends_on` or `relates_to` links, can cross boards
- **Files** -- uploaded file metadata (storage is separate)
- **Attachments** -- link tasks to files or external references (URLs, git branches, etc.)
- **Webhooks** -- event subscriptions, optionally scoped to a board
- **Audit Log** -- append-only record of every mutation with actor attribution

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

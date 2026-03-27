# TaskFlow

TaskFlow is a single-user task tracker designed for fluid collaboration between a human operator and multiple AI agents. It provides a durable server as the single source of truth, with tasks organized on kanban boards that have explicitly configured workflow state machines. Any actor -- human or AI -- can create, advance, review, and manage tasks, with a full audit trail recording every action with actor attribution and timestamps.

The system exposes multiple interface surfaces -- a CLI for scripting and automation, a TUI for interactive visual management, and an MCP server for AI agent integration -- all derived mechanically from a shared operation registry. Each operation is defined once as a typed Go struct, and the framework handles auth, RBAC, input parsing, validation, output formatting, and audit logging uniformly across all interfaces. The server is built with Go and SQLite (WAL mode + FTS5), deployed as a single binary or Docker container.

## Status

**Increment 1 is complete.** This provides the domain model, service layer, SQLite storage, and 74 passing tests. There is no HTTP server or CLI yet -- those come in later increments.

See [docs/](docs/) for the PRD and phase plans.

## Architecture

The codebase has a clean three-layer architecture that separates domain logic from storage. To swap the storage backend (e.g., PostgreSQL), implement `repo.Store` in a new package -- nothing in `model/`, `service/`, or `taskflow/` changes.

```
                    ┌─────────────────────┐
                    │  taskflow.TaskFlow   │  Canonical interface (35 operations)
                    │  (interface)         │  Consumers depend on this, not on concrete types
                    └──────────┬──────────┘
                               │ implements
                    ┌──────────▼──────────┐
                    │  service.Service     │  Business logic: validation, audit recording,
                    │                     │  task numbering, change tracking, reassignment
                    └──────────┬──────────┘
                               │ calls
                    ┌──────────▼──────────┐
                    │  repo.Store          │  Storage-agnostic interfaces + Transactor/Tx
                    │  (interface)         │  Service passes opaque Tx through, never imports a driver
                    └──────────┬──────────┘
                               │ implements
                    ┌──────────▼──────────┐
                    │  sqlite.Store        │  SQLite via sqlx, generic struct mapper,
                    │                     │  Scanner/Valuer types for row↔model conversion
                    └─────────────────────┘
```

## Project Structure

```
internal/
├── model/              Domain types — the shared language of the system
│   ├── actor.go            Actor, ActorType (human/ai_agent), Role (admin/member/read_only)
│   ├── board.go            Board, slug validation, workflow as JSON blob
│   ├── task.go             Task, Priority, TaskFilter, TaskSort
│   ├── comment.go          Comment (attributed to an actor, not necessarily the task creator)
│   ├── dependency.go       Dependency, DependencyType (depends_on/relates_to)
│   ├── attachment.go       Attachment — a typed reference (URL, file, git branch, commit, PR)
│   ├── webhook.go          Webhook (event subscriptions, optionally board-scoped)
│   ├── audit.go            AuditEntry, AuditAction (16 action types)
│   ├── workflow.go         Workflow state machine definition (data type; engine is Increment 2)
│   ├── optional.go         Optional[T] — distinguishes "not provided" from "provided nil"
│   └── errors.go           ValidationError, NotFoundError, ConflictError
│
├── taskflow/           The canonical interface — what the system can do
│   └── taskflow.go         TaskFlow interface: 35 business operations grouped by entity
│
├── repo/               Storage contract — what the service layer needs from persistence
│   └── repo.go             CRUD interfaces per entity + Transactor/Tx for transactions
│
├── service/            Business logic — storage-agnostic, owns all domain rules
│   ├── service.go          Service struct, New() returns taskflow.TaskFlow, audit helper
│   ├── actors.go           Validation, role checking
│   ├── boards.go           Validation, soft-delete, task reassignment with related data migration
│   ├── tasks.go            Validation, sequential numbering, change tracking for audit
│   ├── comments.go         Validation, task existence check
│   ├── dependencies.go     Validation, self-dependency prevention
│   ├── attachments.go      Validation (ref_type + reference required)
│   ├── webhooks.go         Validation
│   └── audit.go            Audit query pass-through
│
├── sqlite/             SQLite implementation of repo.Store
│   ├── store.go            *sqlx.DB, InTransaction, interface satisfaction check
│   ├── migrate.go          Embedded SQL migration runner with schema_version tracking
│   ├── mapper.go           Generic toModel/fromModel via reflection + dispatch table
│   ├── types.go            SQLiteBool, Timestamp, NullTimestamp, StringList, JSONRaw
│   ├── helpers.go          insertRow, queryBuilder, error helpers
│   ├── actors.go           actorRow + CRUD
│   ├── boards.go           boardRow + CRUD + AllocateTaskNum
│   ├── tasks.go            taskRow + CRUD + FTS5 search + dynamic filters
│   ├── comments.go         commentRow + CRUD
│   ├── dependencies.go     dependencyRow + CRUD + bidirectional ref updates
│   ├── attachments.go      attachmentRow + CRUD
│   ├── webhooks.go         webhookRow + CRUD
│   └── audit.go            auditRow + insert + queries
│
└── testutil/           Test infrastructure
    └── testutil.go         NewTestService (returns taskflow.TaskFlow), seed helpers

migrations/
├── embed.go                //go:embed *.sql
├── 001_initial_schema.sql  8 tables with constraints and indexes
└── 002_fts5.sql            FTS5 virtual table + sync triggers
```

## Data Model

- **Actors** -- humans or AI agents, identified by name, with roles (admin/member/read_only)
- **Boards** -- kanban boards with slug identifiers and embedded workflow state machines
- **Tasks** -- identified by `board-slug/N` (sequential per board), with priority, tags, assignee, due date
- **Comments** -- on tasks, attributed to actors, chronologically ordered
- **Dependencies** -- `depends_on` or `relates_to` links between tasks, can cross boards
- **Attachments** -- typed references linking tasks to URLs, files, git branches, commits, or PRs
- **Webhooks** -- event subscriptions with HMAC signing, optionally scoped to a board
- **Audit Log** -- append-only record of every mutation with actor attribution

## Key Design Decisions

- **`Optional[T]`** for partial updates — cleanly distinguishes "not provided" from "set to nil" without pointer hacks
- **Attachments are references** — all attachments (including files) are typed references. File storage is an infrastructure concern outside the domain.
- **Generic struct mapper** — `toModel`/`fromModel` use reflection with a cached dispatch table to convert between DB row types and model types. Field name mismatches panic at first use, not at query time.
- **Scanner/Valuer types** (`SQLiteBool`, `Timestamp`, `StringList`, etc.) — contained in the sqlite package so model types stay clean (`bool`, `time.Time`, `[]string`)

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

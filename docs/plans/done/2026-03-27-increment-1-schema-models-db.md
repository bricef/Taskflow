# Increment 1: Schema, Models & Database Layer

## Context

TaskFlow is a greenfield Go project — no code exists yet. Increment 1 builds the foundation: domain types, SQLite schema, and a tested data access layer. Everything else (workflow engine, operation framework, HTTP/CLI) builds on top of this.

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| SQLite driver | `modernc.org/sqlite` (pure Go) | No CGo, single binary, FTS5 included |
| Migrations | Numbered `.sql` files, embedded via `//go:embed`, up-only | Simple, portable, no external tool |
| Store interface | Composed sub-interfaces per entity | Focused files, single injection point |
| Audit recording | Store writes audit entries within the same transaction as mutations | Atomic consistency |
| Validation | Validate methods on model params, called by Store before SQL | Testable independently, Store never writes invalid data |
| Tags | JSON array in TEXT column, `json_each()` for filtering | Simple, no junction table needed |
| Nullable updates | `*Type` pointers + `ClearX bool` for nullable fields | Unambiguous, no double-pointers |
| Timestamps | ISO 8601 strings in SQLite, `time.Time` in Go | Explicit parsing, no driver magic |

## Implementation Order

### Step 1: Project bootstrap
- `go mod init github.com/bricef/taskflow` + `go get modernc.org/sqlite`

### Step 2: Model types (`internal/model/`)
Files (in order): `errors.go`, `actor.go`, `board.go`, `task.go`, `comment.go`, `dependency.go`, `file.go`, `attachment.go`, `webhook.go`, `audit.go`, `workflow.go`

Each file defines: struct, create/update params, enums with validators.

### Step 3: Migrations (`migrations/`)
- `embed.go` — `//go:embed *.sql`
- `001_initial_schema.sql` — all 9 tables with constraints and indexes
- `002_fts5.sql` — FTS5 virtual table + sync triggers

### Step 4: Audit package (`internal/audit/`)
- `recorder.go` — `Recorder` interface, `NopRecorder`

### Step 5: DB layer (`internal/db/`)
- `store.go` — Store interface (composed sub-interfaces)
- `migrate.go` — migration runner using `schema_version` table
- `sqlite.go` — `NewSQLiteStore()`, pragmas, WAL, `recordAudit()` helper

### Step 6: Test utilities (`internal/testutil/`)
- `testutil.go` — `NewTestStore()`, `SeedActor()`, `SeedBoard()`, `SeedTask()`

### Step 7: Entity implementations + tests (in FK dependency order)
1. `actors.go` + `actors_test.go`
2. `boards.go` + `boards_test.go`
3. `tasks.go` + `tasks_test.go` (most complex: sequential numbering, dynamic filters, FTS5)
4. `comments.go` + `comments_test.go`
5. `dependencies.go` + `dependencies_test.go`
6. `files.go` + `files_test.go`
7. `attachments.go` + `attachments_test.go`
8. `webhooks.go` + `webhooks_test.go`
9. `audit.go` + `audit_test.go` (read queries)
10. `search.go` (FTS5, tested in tasks_test.go)

## Project Structure

```
taskflow/
├── go.mod
├── go.sum
├── internal/
│   ├── model/
│   │   ├── errors.go         # NotFoundError, ValidationError, ConflictError
│   │   ├── actor.go          # Actor, ActorType, Role enums, CreateActorParams
│   │   ├── board.go          # Board, slug validation, CreateBoardParams
│   │   ├── task.go           # Task, Priority enum, TaskFilter, TaskSort
│   │   ├── comment.go        # Comment, CreateCommentParams
│   │   ├── dependency.go     # Dependency, DepType enum
│   │   ├── file.go           # File, CreateFileParams
│   │   ├── attachment.go     # Attachment, RefType enum, validation
│   │   ├── webhook.go        # Webhook, CreateWebhookParams
│   │   ├── audit.go          # AuditEntry, AuditAction enum
│   │   └── workflow.go       # Workflow JSON struct (data only, engine is Increment 2)
│   ├── audit/
│   │   └── recorder.go       # Recorder interface, NopRecorder
│   ├── db/
│   │   ├── store.go          # Store interface (composed sub-interfaces)
│   │   ├── migrate.go        # Migration runner
│   │   ├── sqlite.go         # SQLiteStore constructor, pragmas, recordAudit()
│   │   ├── actors.go         # ActorStore implementation
│   │   ├── boards.go         # BoardStore implementation (incl. ReassignTasks)
│   │   ├── tasks.go          # TaskStore implementation (sequential nums, filters, FTS5)
│   │   ├── comments.go       # CommentStore implementation
│   │   ├── dependencies.go   # DependencyStore implementation
│   │   ├── files.go          # FileStore implementation
│   │   ├── attachments.go    # AttachmentStore implementation
│   │   ├── webhooks.go       # WebhookStore implementation
│   │   ├── audit.go          # AuditStore (read-only queries)
│   │   ├── search.go         # FTS5 search queries
│   │   └── *_test.go         # One test file per entity
│   └── testutil/
│       └── testutil.go       # NewTestStore(), SeedActor(), SeedBoard(), SeedTask()
├── migrations/
│   ├── embed.go              # //go:embed *.sql
│   ├── 001_initial_schema.sql
│   └── 002_fts5.sql
```

## Schema Highlights

- Tasks PK: `(board_slug, num)` — num auto-incremented via `boards.next_task_num`
- FTS5 uses SQLite implicit `rowid` — do NOT use `WITHOUT ROWID` on tasks table
- Attachments have CHECK constraint: exactly one of `file_id` or `(ref_type, value)` must be set
- Dependencies have UNIQUE constraint on `(task_board, task_num, depends_on_board, depends_on_num, dep_type)`
- Audit log is append-only — no UPDATE/DELETE exposed
- Timestamps stored as ISO 8601 strings, parsed to `time.Time` in Go
- Tags stored as JSON arrays, filtered with `json_each()`

## Verification

```bash
go test -v -count=1 ./internal/...
```

All ~68 acceptance tests from the phase 1 plan must pass (actors: 11, boards: 11, tasks: 13, comments: 4, dependencies: 7, files: 4, attachments: 6, webhooks: 7, audit: 5).

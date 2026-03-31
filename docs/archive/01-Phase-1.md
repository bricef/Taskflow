# TaskFlow — Phase 1 Implementation Plan

**Foundation: Server + CLI**

*Version: 2.1*
*Date: 2026-03-18*
*Ref: prd-taskflow-v1.2.md*

---

## Approach

Phase 1 is built around a central design principle: **define each operation once, derive all interfaces mechanically.** An operation framework handles the repetitive wiring — HTTP routing, CLI command generation, auth, RBAC, audit logging, input parsing, output formatting — so that adding a new capability means writing a struct, a business logic function, and a CLI format function.

Every increment follows a **test-first** discipline: acceptance tests are written before implementation, and the increment is complete when all tests pass.

Tests are a mix of:

- **Unit tests** — pure functions (workflow engine, struct tag parsing, framework internals).
- **Integration tests** — tests that register operations, then call them via HTTP (`httptest`) and CLI, verifying the full pipeline.
- **Operation tests** — each operation is tested at the Execute level (business logic) and at the HTTP/CLI level (end-to-end through the framework).


## Architecture: Operation Framework

### The Problem

Every operation in TaskFlow follows the same pattern:

1. Authenticate the actor from the API key
2. Check role permissions
3. Parse structured input (from HTTP request, CLI flags, or MCP tool params)
4. Validate the input
5. Execute business logic against the store
6. Record an audit entry
7. Format structured output (JSON for HTTP, table/text for CLI, structured data for MCP)

Without a framework, this pipeline is re-implemented for every endpoint — once as an HTTP handler, once as a CLI command, and again as an MCP tool. ~45 operations x 3 interfaces = ~135 pieces of glue code.

### The Solution

Each operation is defined once as a Go struct:

```go
type Operation[In, Out any] struct {
    Name        string        // "create_task"
    Resource    string        // "tasks"
    MinRole     Role          // member
    HTTP        HTTPBinding
    CLI         CLIBinding
    Execute     func(ctx context.Context, s *Store, actor Actor, in In) (Out, error)
    AuditAction string        // "created"
    AuditDetail func(In, Out) any
}
```

Input structs carry binding metadata via struct tags:

```go
type CreateTaskInput struct {
    BoardSlug   string   `path:"slug" cli:"arg:0:board" required:"true"`
    Title       string   `json:"title" cli:"arg:1:title" required:"true"`
    Priority    string   `json:"priority" cli:"flag:priority" default:"none"`
    Assignee    string   `json:"assignee" cli:"flag:assignee"`
    Tags        []string `json:"tags" cli:"flag:tags" separator:","`
    DueDate     string   `json:"due_date" cli:"flag:due"`
    Description string   `json:"description" cli:"flag:desc"`
}
```

A **registry** holds all operations. Adapters consume the registry:

- **HTTP adapter** — generates Chi routes, auth middleware, RBAC checks, JSON parsing/serialisation.
- **CLI adapter** — generates Cobra commands, flag/arg parsing, output formatting.
- **MCP adapter** (Phase 3) — generates tool definitions and handlers.

### Edge Cases and Escape Hatches

Not everything fits the standard operation pattern. The framework provides escape hatches:

| Case | Approach |
|------|----------|
| File upload (multipart input) | Custom `InputParser` override on the operation |
| File download (binary output) | Custom `OutputWriter` override on the operation |
| SSE events endpoint | Registered as a raw HTTP handler, outside the operation registry |
| Health check | Raw handler, no auth |
| Dashboard HTML | Raw handler, served as static file |
| Transition with optional comment | Execute function creates the comment internally — no special framework support needed |

### Framework Internals

```
┌──────────────────────────────────────────────────┐
│                 Operation Registry                │
│  []Operation[In, Out]                            │
├──────────────────────────────────────────────────┤
│                                                  │
│  ┌────────────────┐  ┌────────────────┐          │
│  │ Struct Tag      │  │ Validation     │          │
│  │ Parser          │  │ Engine         │          │
│  └────────────────┘  └────────────────┘          │
│                                                  │
│  ┌────────────────┐  ┌────────────────┐          │
│  │ Audit           │  │ RBAC           │          │
│  │ Recorder        │  │ Enforcer       │          │
│  └────────────────┘  └────────────────┘          │
│                                                  │
├──────────┬──────────┬──────────┬─────────────────┤
│ HTTP     │ CLI      │ MCP      │ Future adapters  │
│ Adapter  │ Adapter  │ Adapter  │                  │
│          │          │ (Ph. 3)  │                  │
└──────────┴──────────┴──────────┴─────────────────┘
```


## Project Structure

```
taskflow/
├── cmd/
│   ├── taskflow-server/        # Server binary (thin shell: config + registry.MountHTTP)
│   └── taskflow/               # CLI binary (thin shell: config + registry.BuildCLI)
├── internal/
│   ├── framework/              # The operation framework
│   │   ├── operation.go        # Operation type, Registry
│   │   ├── tags.go             # Struct tag parser
│   │   ├── validate.go         # Input validation from tags
│   │   ├── http.go             # HTTP adapter (Chi route generation, generic handler)
│   │   ├── cli.go              # CLI adapter (Cobra command generation)
│   │   ├── auth.go             # Auth middleware (API key → actor)
│   │   ├── rbac.go             # RBAC enforcement
│   │   ├── audit.go            # Audit recording integration
│   │   └── errors.go           # Error types, HTTP status mapping
│   ├── db/                     # SQLite schema, migrations, queries
│   ├── model/                  # Domain types
│   ├── workflow/               # State machine engine
│   ├── search/                 # FTS5 integration
│   └── ops/                    # All operation definitions
│       ├── actors.go           # Actor CRUD operations
│       ├── boards.go           # Board CRUD + workflow operations
│       ├── tasks.go            # Task CRUD + transition operations
│       ├── comments.go         # Comment operations
│       ├── dependencies.go     # Dependency operations
│       ├── files.go            # File upload/download operations
│       ├── attachments.go      # Attachment operations
│       ├── audit.go            # Audit query operations
│       ├── webhooks.go         # Webhook CRUD operations
│       ├── admin.go            # Backup, health operations
│       ├── reports.go          # Reporting operations
│       ├── tags.go             # Tag listing operation
│       └── registry.go         # Registers all operations
├── migrations/
├── testutil/
├── Dockerfile
├── go.mod
└── go.sum
```


## Increment 1: Schema, Models & Database Layer

**Goal:** SQLite database with migrations, core tables, and a tested data access layer.

No HTTP server yet — this is the foundation everything else builds on.

### 1.1 Acceptance Tests

```
=== Actors ===
TEST: create actor with name, display_name, type, role → row inserted, api_key_hash stored
TEST: create actor with role=admin → role stored correctly
TEST: create actor with role=member → role stored correctly
TEST: create actor with role=read_only → role stored correctly
TEST: create actor with invalid role → validation error
TEST: create actor with duplicate name → error (unique constraint)
TEST: list actors → returns all actors ordered by name, includes role
TEST: update actor role → role changed
TEST: update actor display_name → display_name changed
TEST: deactivate actor → active=false, still queryable
TEST: create actor with empty name → validation error
TEST: create actor with invalid type → validation error

=== Boards ===
TEST: create board with slug, name, workflow JSON → row inserted, next_task_num=1
TEST: create board with duplicate slug → error
TEST: create board with invalid slug (uppercase, spaces, special chars) → validation error
TEST: create board with slug under 2 chars or over 32 chars → validation error
TEST: update board name → name changed, slug unchanged
TEST: soft-delete board → deleted=true, excluded from default list
TEST: list boards → excludes soft-deleted boards
TEST: list boards with include_deleted → includes soft-deleted boards
TEST: reassign tasks from board A to board B → tasks get new numbers on B, removed from A
TEST: reassign tasks → only non-terminal-state tasks moved by default
TEST: reassign tasks with state filter → only specified states moved

=== Tasks ===
TEST: create task on board → assigned next sequential number, state=initial_state
TEST: create second task → number increments (board/1, board/2)
TEST: create task with all optional fields (priority, tags, assignee, due_date, description) → stored correctly
TEST: create task with invalid priority → validation error
TEST: create task with non-existent assignee → error (FK violation)
TEST: create task on non-existent board → error
TEST: update task fields → updated_at changes, fields updated
TEST: soft-delete task → deleted=true, excluded from default list
TEST: list tasks → excludes deleted and terminal-state tasks by default
TEST: list tasks with include_closed=true → includes terminal-state tasks
TEST: list tasks with include_deleted=true → includes soft-deleted tasks
TEST: filter tasks by state → only matching tasks returned
TEST: filter tasks by assignee → only matching tasks returned
TEST: filter tasks by priority → only matching tasks returned
TEST: filter tasks by tag → only matching tasks returned
TEST: sort tasks by created_at, updated_at, priority, due_date → correct ordering

=== Comments ===
TEST: add comment to task → comment stored with actor and timestamp
TEST: list comments for task → returned in chronological order
TEST: edit comment → body updated, updated_at set
TEST: add comment to non-existent task → error

=== Dependencies ===
TEST: add dependency (task A depends_on task B) → stored correctly
TEST: add dependency across boards → stored correctly
TEST: add relates_to dependency → stored correctly
TEST: list dependencies for task → returns both directions (depends_on and depended_on_by)
TEST: remove dependency → deleted
TEST: add duplicate dependency → error
TEST: add self-dependency → error

=== Files ===
TEST: store file metadata → row inserted with id, filename, size, mime_type
TEST: list files → returns all file records
TEST: delete file that has no attachments → deleted
TEST: delete file that is attached to a task → error (referential integrity)

=== Attachments ===
TEST: attach file to task → attachment created with file_id set, ref_type null
TEST: attach external reference → attachment created with ref_type and value set, file_id null
TEST: attachment with both file_id and ref_type → validation error
TEST: attachment with neither file_id nor ref_type → validation error
TEST: list attachments for task → returns all attachments with file metadata populated
TEST: remove attachment → deleted (underlying file not affected)

=== Webhooks ===
TEST: create webhook with url, events, optional board_slug → row inserted
TEST: create webhook with specific board scope → board_slug stored
TEST: create webhook with null board_slug → global scope
TEST: list webhooks → returns all webhooks
TEST: update webhook url → url changed
TEST: update webhook events → events changed
TEST: update webhook active flag → active toggled
TEST: delete webhook → removed

=== Audit Log ===
TEST: creating a task inserts audit entry (action=created, actor, timestamp, detail)
TEST: updating a task inserts audit entry with changed fields in detail
TEST: audit entries are append-only (no update/delete operations exposed)
TEST: query audit log by task → returns entries in chronological order
TEST: query audit log by board → returns all entries for board's tasks
```

### 1.2 Implementation

- Define Go types in `internal/model/` for all domain objects (including Role enum with `admin`, `member`, `read_only`).
- Write SQL migrations for all 10 tables: actors, boards, tasks, comments, dependencies, files, attachments, audit_log, webhooks, and FTS5 virtual table.
- Implement `internal/db/` package with a `Store` interface and SQLite implementation.
  - Each entity gets its own file.
  - Slug validation as a reusable function.
  - Tag storage as JSON array in SQLite.
  - Board reassignment logic.
- Implement `internal/audit/` — a `Recorder` that the store calls on every mutation.
- All tests use a fresh in-memory SQLite database per test case.

### 1.3 Estimated Effort: 3–4 dev-days

### 1.4 Done When

All acceptance tests pass. **You can now** interact with the full data model programmatically via Go code, with all validation and audit logging in place.

---

## Increment 2: Workflow Engine

**Goal:** A self-contained state machine engine that validates workflow definitions and enforces transitions.

### 2.1 Acceptance Tests

```
=== Workflow Parsing & Validation ===
TEST: parse valid workflow JSON → returns Workflow struct with states, transitions, initial, terminal
TEST: parse workflow with fromAll → expands to transitions from every non-terminal state to target
TEST: parse workflow with toAll → expands to transitions from source to every other state
TEST: parse workflow with both fromAll and toAll → both expand correctly, no duplicates
TEST: workflow with no initial_state → validation error
TEST: workflow with initial_state not in states list → validation error
TEST: workflow with terminal_state not in states list → validation error
TEST: workflow with transition referencing unknown state → validation error
TEST: workflow with no path from initial to any terminal state → validation error (unreachable terminal)
TEST: workflow with duplicate transition name → validation error
TEST: workflow with duplicate state names → validation error
TEST: empty states list → validation error

=== Transition Enforcement ===
TEST: transition task from valid state via valid transition → state updated
TEST: transition task via invalid transition name → error listing available transitions
TEST: transition task from wrong state for that transition → error with current state and available transitions
TEST: transition task already in terminal state → error (no transitions available)
TEST: transition with optional comment → comment stored and audit logged

=== Workflow Health ===
TEST: all tasks in valid states → health check returns clean
TEST: workflow edited to remove a state → health check reports orphaned tasks in that state
TEST: workflow edited to remove outbound transitions from a state → health check reports stuck tasks

=== Default Workflow ===
TEST: default workflow has states: backlog, in_progress, review, done, cancelled
TEST: default workflow initial_state=backlog, terminal_states=[done, cancelled]
TEST: default workflow allows: backlog→in_progress→review→done, review→in_progress (reject)
TEST: default workflow has fromAll→cancelled
TEST: transition through full default workflow happy path → works
```

### 2.2 Implementation

- Implement `internal/workflow/` package.
  - `Parse(json) → Workflow` — validates and expands `fromAll`/`toAll`.
  - `Workflow.Validate() → error` — structural validation.
  - `Workflow.AvailableTransitions(currentState) → []Transition`.
  - `Workflow.ExecuteTransition(currentState, transitionName) → newState, error`.
  - `Workflow.HealthCheck(taskStates []string) → []HealthIssue`.
- Define the default workflow as a Go constant.
- Pure functions, no database dependency — fully unit testable.

### 2.3 Integration With DB Layer

- Wire workflow validation into board creation (store rejects boards with invalid workflows).
- Wire transition enforcement into task state changes (store calls workflow engine before updating state).
- Update audit logging: transitions record `from`, `to`, `transition_name`, and optional `comment`.

### 2.4 Acceptance Tests (Integration)

```
TEST: create board with invalid workflow → rejected at DB layer
TEST: create board with --default-workflow → board created with default workflow
TEST: create task on board → task starts in initial_state
TEST: transition task via API-style call through store → state updated, audit logged
TEST: transition task with invalid transition via store → error, state unchanged, no audit entry
TEST: update board workflow → workflow replaced, audit logged
TEST: update board workflow to invalid definition → rejected, original preserved
TEST: health check on board with orphaned tasks → reports issues
```

### 2.5 Estimated Effort: 1.5–2 dev-days

### 2.6 Done When

All acceptance tests pass. **You can now** create boards with validated workflows, transition tasks through enforced state machines, and check workflow health — all via Go code.

---

## Increment 3: Operation Framework

**Goal:** The generic operation framework — struct tag parsing, HTTP adapter, CLI adapter, auth, RBAC, audit integration, error handling. Tested with a small set of stub operations.

### 3.1 Acceptance Tests

The framework is tested by registering simplified **test operations** (stubs, not real business logic) and verifying the full pipeline.

```
=== Struct Tag Parser ===
TEST: parse struct with path, json, cli tags → correct field bindings extracted
TEST: parse required fields → validation enforces presence
TEST: parse default values → defaults applied when field absent
TEST: parse separator tag → slice fields split correctly
TEST: parse nested struct → error (not supported, keep flat)
TEST: parse struct with no tags → error (no bindings)

=== HTTP Adapter ===
TEST: register operation → Chi route created at correct method + path
TEST: request with valid JSON body → input struct populated correctly
TEST: request with path parameters → extracted into tagged fields
TEST: request with query parameters → extracted into tagged fields
TEST: request with missing required field → 400 with field name in error
TEST: request with invalid field value → 400 with validation error
TEST: successful operation → 200/201 with JSON output
TEST: operation returning error → appropriate HTTP status code
TEST: error response body → consistent {"error", "message", "detail"} format

=== CLI Adapter ===
TEST: register operation → Cobra command created with correct name and parent
TEST: command with positional args → extracted into tagged fields
TEST: command with flags → extracted into tagged fields
TEST: command with missing required arg → error with usage hint
TEST: command with default flag value → default applied
TEST: successful operation → formatted output printed to stdout
TEST: operation returning error → error printed to stderr with exit code

=== Auth Integration ===
TEST: HTTP request with valid API key → actor resolved, injected into context
TEST: HTTP request with no auth header → 401
TEST: HTTP request with invalid key → 401
TEST: HTTP request with deactivated actor → 403
TEST: CLI reads API key from config → passed to HTTP client

=== RBAC Integration ===
TEST: operation with MinRole=admin, called by admin → allowed
TEST: operation with MinRole=admin, called by member → 403
TEST: operation with MinRole=admin, called by read_only → 403
TEST: operation with MinRole=member, called by member → allowed
TEST: operation with MinRole=member, called by read_only → 403
TEST: operation with MinRole=read_only, called by read_only → allowed
TEST: 403 response includes required role and actor's current role

=== Audit Integration ===
TEST: operation with AuditAction set → audit entry created after successful Execute
TEST: operation that fails → no audit entry created
TEST: audit entry includes correct actor, action, timestamp, detail
TEST: operation with no AuditAction (queries) → no audit entry

=== Error Handling ===
TEST: NotFoundError → HTTP 404, CLI "not found" message
TEST: ConflictError → HTTP 409, CLI "already exists" message
TEST: ValidationError → HTTP 400, CLI "invalid input" message with field details
TEST: ForbiddenError → HTTP 403, CLI "permission denied" message
TEST: unexpected error → HTTP 500, CLI generic error with suggestion to retry

=== Custom Overrides ===
TEST: operation with custom InputParser → parser used instead of struct tags
TEST: operation with custom OutputWriter → writer used instead of JSON serialisation
TEST: raw HTTP handler registered alongside operations → both accessible
```

### 3.2 Implementation

- `internal/framework/operation.go` — Operation type definition, Registry, registration methods.
- `internal/framework/tags.go` — Struct tag parser using `reflect`.
- `internal/framework/validate.go` — Validates required fields, enum values, string patterns.
- `internal/framework/http.go` — HTTP adapter: `MountHTTP(router chi.Router)`, generic handler.
- `internal/framework/cli.go` — CLI adapter: `BuildCLI() *cobra.Command`, generic command runner. CLI commands call the server via HTTP (thin API client), not Execute directly.
- `internal/framework/auth.go` — Auth middleware: API key lookup, actor resolution.
- `internal/framework/rbac.go` — Role checking against operation's MinRole.
- `internal/framework/audit.go` — Post-Execute audit recording.
- `internal/framework/errors.go` — Error types with HTTP status mapping.
- `cmd/taskflow-server/main.go` — Minimal server: load config, init DB, seed admin bootstrap, build registry, mount HTTP, start listening.
- `cmd/taskflow/main.go` — Minimal CLI: load config, build registry, build CLI, run.

Server config env vars:
- `TASKFLOW_DB_PATH` (default: `./taskflow.db`)
- `TASKFLOW_LISTEN_ADDR` (default: `:8374`)
- `TASKFLOW_BACKUP_PATH` (default: `./backups`)
- `TASKFLOW_FILE_STORAGE_PATH` (default: `./files`)
- `TASKFLOW_SEED_ADMIN_NAME`
- `TASKFLOW_SEED_ADMIN_DISPLAY_NAME`

### 3.3 Estimated Effort: 3–4 dev-days

### 3.4 Done When

All acceptance tests pass using stub operations. **You can now** define an operation with an input struct, an Execute function, and struct tags, and get a working HTTP endpoint and CLI command for free, with auth, RBAC, validation, audit, and error handling.

---

## Increment 4: Core Operations

**Goal:** Define the core operations (actors, boards, tasks, comments, audit queries) using the framework. After this increment, TaskFlow is usable for basic task management via both HTTP and CLI.

### 4.1 Operations to Define

| # | Operation | Resource | MinRole | HTTP | CLI |
|---|-----------|----------|---------|------|-----|
| 1 | `create_actor` | actors | admin | POST /actors | taskflow actor create |
| 2 | `list_actors` | actors | read_only | GET /actors | taskflow actor list |
| 3 | `update_actor` | actors | admin | PATCH /actors/{name} | taskflow actor update |
| 4 | `deactivate_actor` | actors | admin | PATCH /actors/{name} | taskflow actor deactivate |
| 5 | `create_board` | boards | member | POST /boards | taskflow board create |
| 6 | `list_boards` | boards | read_only | GET /boards | taskflow board list |
| 7 | `get_board` | boards | read_only | GET /boards/{slug} | taskflow board show |
| 8 | `update_board` | boards | member | PATCH /boards/{slug} | taskflow board rename |
| 9 | `delete_board` | boards | admin | DELETE /boards/{slug} | taskflow board delete |
| 10 | `reassign_board` | boards | admin | POST /boards/{slug}/reassign | taskflow board reassign |
| 11 | `get_workflow` | boards | read_only | GET /boards/{slug}/workflow | taskflow board workflow show |
| 12 | `set_workflow` | boards | member | PUT /boards/{slug}/workflow | taskflow board workflow set |
| 13 | `check_workflow_health` | boards | read_only | GET /boards/{slug}/workflow/health | taskflow board workflow health |
| 14 | `create_task` | tasks | member | POST /boards/{slug}/tasks | taskflow task create |
| 15 | `list_tasks` | tasks | read_only | GET /boards/{slug}/tasks | taskflow task list |
| 16 | `get_task` | tasks | read_only | GET /boards/{slug}/tasks/{num} | taskflow task show |
| 17 | `update_task` | tasks | member | PATCH /boards/{slug}/tasks/{num} | taskflow task update |
| 18 | `transition_task` | tasks | member | POST .../transition | taskflow task transition |
| 19 | `delete_task` | tasks | member | DELETE /boards/{slug}/tasks/{num} | taskflow task delete |
| 20 | `add_comment` | comments | member | POST .../comments | taskflow comment add |
| 21 | `list_comments` | comments | read_only | GET .../comments | taskflow comment list |
| 22 | `edit_comment` | comments | member | PATCH /comments/{id} | — |
| 23 | `get_task_audit` | audit | read_only | GET .../audit | taskflow audit {ref} |
| 24 | `get_board_audit` | audit | read_only | GET /boards/{slug}/audit | taskflow audit --board |

Note: `list_tasks` (operation 15) includes full-text search via the `q` query parameter. There is no separate search operation — search is a filter on the list endpoint, consistent with PRD §6.3.

### 4.2 Acceptance Tests

```
=== Actor Operations ===
TEST: create actor with name, type, role → returns actor + API key (key shown once)
TEST: create actor with duplicate name → 409
TEST: create actor with invalid role → 400
TEST: list actors → returns all actors with roles, no key hashes
TEST: update actor role → role changed
TEST: update actor display_name → display_name changed
TEST: deactivate actor → active=false
TEST: non-admin creating actor → 403

=== Board Operations ===
TEST: create board with slug, name, workflow → board created, num=1
TEST: create board with default_workflow=true → default workflow applied
TEST: create board with invalid slug → 400
TEST: create board with invalid workflow → 400
TEST: create board with duplicate slug → 409
TEST: create board as member → 201 (allowed)
TEST: create board as read_only → 403
TEST: list boards → excludes deleted, includes open task counts
TEST: get board → includes full workflow definition
TEST: update board name → name changed, slug immutable
TEST: delete board → soft-deleted
TEST: delete board as member → 403
TEST: reassign board tasks → tasks moved with new numbers, audit logged on both boards
TEST: get workflow → returns workflow definition
TEST: set workflow → validated and replaced
TEST: set invalid workflow → 400, original preserved
TEST: workflow health on clean board → no issues
TEST: workflow health with orphaned tasks → issues reported

=== Task Operations ===
TEST: create task → sequential number, initial state, audit logged
TEST: create task with all optional fields → stored correctly
TEST: list tasks → default excludes closed and deleted
TEST: list tasks with filters (state, assignee, priority, tag) → correct filtering
TEST: list tasks with q parameter → full-text search results
TEST: list tasks with q + filters → intersection
TEST: list tasks with sort + order → correct ordering
TEST: list tasks with include_closed=true → includes terminal-state tasks
TEST: get task → full detail including comments and deps
TEST: update task → fields changed, audit logged with old/new values
TEST: transition task → state changed per workflow, audit logged
TEST: transition with comment → comment created alongside
TEST: transition with invalid transition name → 400 with available transitions
TEST: delete task → soft-deleted, audit logged

=== Comment Operations ===
TEST: add comment → stored with actor attribution
TEST: list comments → chronological order
TEST: edit comment → body updated, audit entry for comment_edited

=== Audit Operations ===
TEST: get task audit → entries for that task in chronological order
TEST: get board audit → entries for all tasks on board
TEST: audit entries have correct actor attribution

=== CLI End-to-End ===
TEST: taskflow task create my-board "Fix bug" --priority=high → prints ref
TEST: taskflow task list my-board --state=backlog → tabular output
TEST: taskflow task list my-board --search="auth" → search results
TEST: taskflow task show my-board/1 → formatted detail view
TEST: taskflow task transition my-board/1 start --comment="On it" → confirmation
TEST: taskflow board create x --name="X" --default-workflow → board created
TEST: taskflow actor create bot --type=ai_agent --role=member → prints key
TEST: default_board config → bare number resolves correctly
TEST: commands with read_only key → appropriate errors for write operations
TEST: destructive commands → confirmation prompt, skippable with --yes
```

### 4.3 Implementation

For each operation: define input/output structs with struct tags in `internal/ops/<resource>.go`, write the Execute function, write the CLIFormat function, register in `internal/ops/registry.go`. The framework does the rest.

**Seed admin bootstrap** is in `cmd/taskflow-server/main.go` (startup concern, not an operation).

### 4.4 Estimated Effort: 3–4 dev-days

### 4.5 Done When

All acceptance tests pass. **You can now** manage actors, boards, tasks, and comments via both HTTP API and CLI, with full auth, RBAC, audit, workflow enforcement, and full-text search.

---

## Increment 5: Extended Operations

**Goal:** Define the remaining operations: dependencies, files, attachments, webhooks, admin, reports, tags. After this increment, every endpoint in the PRD is functional.

### 5.1 Operations to Define

| # | Operation | Resource | MinRole | HTTP | CLI |
|---|-----------|----------|---------|------|-----|
| 25 | `add_dependency` | deps | member | POST .../dependencies | taskflow dep add |
| 26 | `list_dependencies` | deps | read_only | GET .../dependencies | taskflow dep list |
| 27 | `remove_dependency` | deps | member | DELETE /dependencies/{id} | taskflow dep remove |
| 28 | `upload_file` | files | member | PUT /files | taskflow file upload |
| 29 | `download_file` | files | read_only | GET /files/{id} | — |
| 30 | `get_file_meta` | files | read_only | GET /files/{id}/meta | — |
| 31 | `list_files` | files | read_only | — | taskflow file list |
| 32 | `delete_file` | files | member | DELETE /files/{id} | taskflow file delete |
| 33 | `attach_file` | attachments | member | POST .../attachments | taskflow attach file |
| 34 | `attach_reference` | attachments | member | POST .../attachments | taskflow attach ref |
| 35 | `list_attachments` | attachments | read_only | GET .../attachments | taskflow attach list |
| 36 | `remove_attachment` | attachments | member | DELETE /attachments/{id} | taskflow attach remove |
| 37 | `create_webhook` | webhooks | admin | POST /webhooks | taskflow webhook create |
| 38 | `list_webhooks` | webhooks | admin | GET /webhooks | taskflow webhook list |
| 39 | `update_webhook` | webhooks | admin | PATCH /webhooks/{id} | taskflow webhook update |
| 40 | `delete_webhook` | webhooks | admin | DELETE /webhooks/{id} | taskflow webhook delete |
| 41 | `trigger_backup` | admin | admin | POST /admin/backup | taskflow admin backup |
| 42 | `report_cfd` | reports | read_only | GET .../reports/cfd | taskflow report cfd |
| 43 | `report_cycle_time` | reports | read_only | GET .../reports/cycle-time | taskflow report cycle-time |
| 44 | `report_throughput` | reports | read_only | GET .../reports/throughput | taskflow report throughput |
| 45 | `report_actor_activity` | reports | read_only | GET .../reports/actor-activity | taskflow report actor-activity |
| 46 | `report_board_summary` | reports | read_only | GET /reports/board-summary | taskflow report board-summary |
| 47 | `list_tags` | tags | read_only | GET /boards/{slug}/tags | taskflow tag list |

**Note on webhooks:** Webhook CRUD is fully functional in Phase 1. Webhook **dispatch** (actually sending POST requests when events occur) is implemented in Phase 4 when the internal event bus lands. Registering webhooks in Phase 1 is not wasted — they'll start firing as soon as dispatch is enabled.

**Note on special cases:**
- `upload_file` uses a custom InputParser (multipart form data).
- `download_file` uses a custom OutputWriter (binary stream with Content-Disposition header).
- `attach_file` and `attach_reference` map to the same HTTP endpoint but are distinct operations with different input structs.
- Report operations are query-only (no audit logging). Their Execute functions replay the audit log to compute metrics.
- System-wide report variants (no board param) are handled by making the board parameter optional on the same operation.

### 5.2 Acceptance Tests

```
=== Dependency Operations ===
TEST: add depends_on dependency → stored correctly
TEST: add relates_to dependency → stored correctly
TEST: add cross-board dependency → stored correctly
TEST: add self-dependency → 400
TEST: add duplicate dependency → 409
TEST: list dependencies → shows both directions
TEST: remove dependency → deleted, audit logged
TEST: read_only adding dependency → 403

=== File Operations ===
TEST: upload file → stored on disk, metadata in DB, returns id + filename
TEST: upload preserves original filename
TEST: download file → correct content, Content-Type, filename header
TEST: get file metadata → metadata without body
TEST: delete file with no attachments → deleted from disk and DB
TEST: delete file with attachments → 400
TEST: upload empty file → 400
TEST: read_only uploading → 403
TEST: read_only downloading → 200 (allowed)

=== Attachment Operations ===
TEST: attach file by id → attachment created
TEST: attach external reference → attachment created
TEST: attach with both file_id and ref_type → 400
TEST: attach with neither → 400
TEST: list attachments → includes file metadata for file-type
TEST: remove attachment → deleted, file not affected
TEST: read_only attaching → 403

=== Webhook Operations ===
TEST: create webhook → stored with URL, events, optional board scope
TEST: list webhooks → all webhooks
TEST: update webhook URL → URL changed
TEST: update webhook events → events changed
TEST: update webhook active flag → toggled
TEST: delete webhook → removed
TEST: member creating webhook → 403
TEST: member listing webhooks → 403

=== Admin Operations ===
TEST: trigger backup (admin) → backup file created with timestamp
TEST: trigger backup (member) → 403
TEST: trigger backup → DB operational during and after

=== Report Operations ===
TEST: CFD report → returns state counts per time bucket
TEST: cycle time report → returns per-task timing with summary stats
TEST: throughput report → returns completion counts per bucket
TEST: actor activity → returns per-actor breakdown
TEST: board summary → returns all boards with open counts and metrics
TEST: system-wide reports (no board) → aggregates across boards
TEST: date range parameters → filters correctly
TEST: bucket parameter → groups correctly

=== Tag Operations ===
TEST: list tags → all tags in use on board with counts
TEST: list tags on empty board → empty list

=== CLI End-to-End ===
TEST: taskflow dep add my-board/1 --on my-board/2 → created
TEST: taskflow file upload ./test.pdf → prints file ID
TEST: taskflow attach file my-board/1 17 → attached
TEST: taskflow attach ref my-board/1 --type=git_branch --value=feat/x → attached
TEST: taskflow webhook create --url=... --events=task.created → created
TEST: taskflow webhook update 1 --active=false → updated
TEST: taskflow webhook list → tabular output
TEST: taskflow admin backup → confirmation
TEST: taskflow report cfd my-board → tabular/chart output
TEST: taskflow report board-summary → overview table
TEST: taskflow tag list my-board → tag counts
```

### 5.3 Estimated Effort: 3–4 dev-days

### 5.4 Done When

All acceptance tests pass. **You can now** use every operation from PRD §6.1–6.13 via both HTTP and CLI, with proper role enforcement throughout.

---

## Increment 6: Containerisation & Deployment

**Goal:** Dockerfile, seed admin bootstrap, container setup, deployment documentation.

### 6.1 Acceptance Tests

```
=== Container Build ===
TEST: docker build succeeds → image contains taskflow-server and taskflow binaries
TEST: docker run with volume → server starts, responds to health check

=== Seed Admin Bootstrap ===
TEST: first start with TASKFLOW_SEED_ADMIN_NAME set → admin actor created
TEST: first start → API key written to /data/seed-admin-key.txt
TEST: first start → server log includes message with key file path
TEST: first start → seed admin has role=admin, type=human, active=true
TEST: second start with same config → no new actor created (idempotent)
TEST: second start → no new key file written
TEST: start without TASKFLOW_SEED_ADMIN_NAME and no actors → warning logged
TEST: start with TASKFLOW_SEED_ADMIN_NAME and actors already exist → seed config ignored
TEST: API key from seed file authenticates successfully → 200 on protected endpoint
TEST: seed admin can create additional actors via API

=== Persistence ===
TEST: server creates DB on first run if none exists
TEST: server opens existing DB on subsequent runs → data persists
TEST: file uploads persist across container restarts (volume mount)
TEST: backup endpoint creates file in /data/backups/

=== Integration ===
TEST: container with Traefik labels → accessible via configured hostname (manual/integration test)
```

### 6.2 Implementation

- Multi-stage Dockerfile: Go builder → minimal Alpine runtime.
- Include both `taskflow-server` and `taskflow` CLI in the image.
- Health check endpoint: `GET /health` → 200 (raw handler, no auth).
- Seed admin bootstrap logic in server startup.
- `docker-compose.yml` with Traefik labels and seed admin env vars.
- `DEPLOY.md` with step-by-step setup instructions.

### 6.3 Estimated Effort: 1–1.5 dev-days

### 6.4 Done When

All tests pass. **You can now** deploy TaskFlow as a container on a VPS behind Traefik, with a bootstrapped admin account.

---

## Phase 1 Summary

| Increment | What it delivers | Dev-days | Depends on |
|-----------|-----------------|----------|------------|
| 1. Schema & DB | Data model with RBAC, validation, audit | 3–4 | — |
| 2. Workflow engine | State machine engine | 1.5–2 | 1 |
| 3. Operation framework | Generic HTTP/CLI infrastructure | 3–4 | — |
| 4. Core operations (24) | Actors, boards, tasks, comments, audit | 3–4 | 1, 2, 3 |
| 5. Extended operations (23) | Deps, files, attachments, webhooks, admin, reports, tags | 3–4 | 4 |
| 6. Containerisation | Docker, bootstrap, deployment | 1–1.5 | 4+ |
| **Total** | **47 operations, full HTTP API + CLI** | **~16–19** | |

### Critical Path

```
Increment 1 ──→ Increment 2 ──→┐
                                ├──→ Increment 4 ──→ Increment 5 ──→ Increment 6
Increment 3 ───────────────────┘
```

Increments 1+2 (data layer) and 3 (framework) are developed **in parallel**. They converge at increment 4.

At ~2–2.5 dev-days per week (evenings/weekends): **~6–8 weeks**.

### Operation Count Verification

Total operations across increments 4 and 5: **47**

- Actors: 4 (create, list, update, deactivate)
- Boards: 9 (create, list, get, update, delete, reassign, get_workflow, set_workflow, check_health)
- Tasks: 6 (create, list, get, update, transition, delete)
- Comments: 3 (add, list, edit)
- Audit: 2 (get_task_audit, get_board_audit)
- Dependencies: 3 (add, list, remove)
- Files: 5 (upload, download, get_meta, list, delete)
- Attachments: 4 (attach_file, attach_reference, list, remove)
- Webhooks: 4 (create, list, update, delete)
- Admin: 1 (trigger_backup)
- Reports: 5 (cfd, cycle_time, throughput, actor_activity, board_summary)
- Tags: 1 (list)

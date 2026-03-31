# TaskFlow

TaskFlow is a single-user task tracker designed for fluid collaboration between a human operator and multiple AI agents. It provides a durable server as the single source of truth, with tasks organized on kanban boards that have explicitly configured workflow state machines. Any actor -- human or AI -- can create, advance, review, and manage tasks, with a full audit trail recording every action with actor attribution and timestamps.

## Quick Start

```bash
# Option 1: Docker Compose (recommended)
just docker-up
docker compose exec taskflow cat /data/seed-admin-key.txt

# Option 2: Run locally
just build
just run
cat seed-admin-key.txt

# Use the CLI
export TASKFLOW_API_KEY=$(cat seed-admin-key.txt)
taskflow board create --slug my-board --name "My Board"
taskflow task create my-board --title "Fix auth bug" --priority high
taskflow task list my-board
taskflow task transition my-board 1 --transition start --comment "On it"

# Or use the TUI
taskflow-tui
```

## Documentation

- **[HTTP API Reference](docs/http-api.md)** — all endpoints, authentication, error handling, configuration
- **[CLI Reference](docs/cli.md)** — all commands, flags, output formats
- **[OpenAPI Spec](http://localhost:8374/openapi.json)** — machine-readable, auto-generated from operation definitions
- **[Claude Code Skill](SKILL.md)** — AI agent guide for using TaskFlow via the CLI
- **[Manual QA Checklist](TESTING.md)** — endpoint-by-endpoint verification guide

## Status

**Phases 1, 2, and 4 complete. Phase 3 (MCP server) remaining.**

### Phase 1 — Server + CLI
- 40 domain endpoints: 17 Resources (read-only) + 23 Operations (mutations), each with an explicit Name
- HTTP API with auth (SHA-256 keys), RBAC (admin/member/read_only), idempotency keys, and batch operations
- CLI derived from Resource/Operation names (`<resource>_<action>` convention)
- OpenAPI 3.1 spec auto-generated with operationIds from Names
- Convenience endpoints: cross-board search, cross-board task list, SSE, batch
- Docker deployment with seed admin bootstrap
- Automated test suite: unit tests, golden tests (OpenAPI spec + CLI command tree), integration tests, and 45-check QA smoke test

### Phase 2 — Event Bus + SSE + TUI
- In-process event bus with ring-buffered subscriptions (256 events per subscriber)
- Before/after task snapshots on all events — consumers can diff state without refetching
- SSE endpoint with board-scoped event streaming, heartbeat, and reconnection
- Interactive TUI (`taskflow-tui`) with four tabs:
  - **Board** — kanban view with column scrolling, priority badges, assignee display
  - **List** — sortable table (cycle sort with `s`, reverse with `S`)
  - **Workflow** — layered graph visualisation with Unicode connectors
  - **Events** — live event stream with side-by-side detail pane
- Task detail overlay with dependency tree, comments, attachments, and audit trail
- Inline actions: transition (`t`), assign (`a`), comment (`c`) — from kanban, list, or detail views
- SSE live updates across kanban, list, and detail views
- Board creation and archival from TUI
- Toggleable help (`?`), done-state toggle (`d`)
- Activity simulator (`taskflow-sim`) for testing SSE updates

### Phase 3 — AI Integration (MCP Server)
- Not yet started — see [PRD](docs/00-Taskflow-PRD.md) section 10

### Phase 4 — Notifications, Dashboard & Polish
- Webhook dispatch with HMAC-SHA256 signatures, retry (3 attempts with backoff), and delivery logging
- Delivery status API: `GET /webhooks/{id}/deliveries`
- HTML dashboard at `/dashboard` with system stats, board overview, and RBAC-aware views
- Live board view at `/dashboard/board/{slug}` with kanban and SSE event stream
- Archived board semantics (mutations blocked, comments allowed)

See [docs/](docs/) for the PRD, phase plans, API reference, and CLI reference.

## TUI

The TUI (`taskflow-tui`) connects to a running server via HTTP API and SSE. It uses the same configuration as the CLI (`TASKFLOW_URL`, `TASKFLOW_API_KEY`, or config file).

```bash
# Start with default settings
taskflow-tui

# Or specify a board directly
taskflow-tui platform
```

**Tabs** — cycle with `tab`:
| Tab | Description |
|-----|-------------|
| Board | Kanban columns by workflow state, navigate with `h`/`j`/`k`/`l` |
| List | Sortable table, `s` to cycle sort column, `S` to reverse |
| Workflow | Visual graph of states and transitions |
| Events | Live SSE event stream with detail pane |

**Actions** — available from Board, List, and Detail views:
| Key | Action |
|-----|--------|
| `enter` | Open task detail |
| `t` | Transition task |
| `a` | Assign task |
| `c` | Add comment (detail view) |
| `d` | Toggle done/terminal tasks |
| `?` | Toggle full help |
| `esc` | Back / close overlay |
| `q` | Quit |

Changes from other sessions (CLI, API, other TUI instances) appear live via SSE.

## Architecture

The domain surface is split into **Resources** (read-only) and **Operations** (mutations), each with an explicit `Name` used as the canonical identifier across all clients:

```
model.Resources()  model.Operations()
 (14 read-only)     (23 mutations)
      │                    │
      ├──────────┬─────────┤
      ▼          ▼         ▼
internal/http  internal/cli  openapi.json
Derives:       Derives:      Derives:
 GET routes     command tree   endpoint docs
 handler map    flags/args     schemas
 status codes   HTTP calls     operationIds
```

The transport layer (`internal/transport`) maps domain actions to HTTP semantics (method, status code). Both the HTTP server and CLI import from it.

The codebase separates business logic from storage:

```
taskflow.TaskFlow  (Go interface for compile-time safety)
      │
service.Service    (business logic: validation, audit, orchestration)
      │                    │
repo.Store              eventbus.Bus
      │                    │
sqlite.Store           SSE endpoint ──→ TUI (live updates)
```

Events carry before/after task snapshots, so consumers can diff state without refetching. The TUI is a pure HTTP+SSE client — it imports no server internals.

To swap the storage backend, implement `repo.Store` in a new package — nothing in `model/`, `service/`, or `taskflow/` changes.

## Project Structure

```
internal/
├── model/              Domain types, resources, and operation definitions
│   ├── operations.go       Resources (read-only) and Operations (mutations) with explicit Names
│   ├── views.go            Composite view types (BoardDetail, BoardOverview, SystemStats)
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
├── transport/          Domain-to-HTTP mapping (MethodForAction, StatusForAction)
├── http/               HTTP server (derived routes, middleware, OpenAPI generation)
├── cli/                CLI client (derived commands, HTTP calls)
├── eventbus/           In-process pub/sub with ring-buffered subscriptions
├── tui/                Interactive terminal UI (Bubble Tea)
│   ├── app.go              Root model, view routing, SSE event handling
│   ├── kanban.go           Kanban board with column scrolling
│   ├── listview.go         Sortable table view
│   ├── workflowview.go     Workflow graph visualisation
│   ├── graphrender.go      Layered DAG renderer with Unicode connectors
│   ├── eventlog.go         Live event stream with detail pane
│   ├── detail.go           Task detail overlay with comment input
│   ├── transition.go       Inline transition picker
│   ├── assign.go           Inline assignee picker
│   ├── selector.go         Board selector with fuzzy filter
│   ├── sse.go              SSE client with reconnection/backoff
│   ├── client.go           HTTP API client for TUI
│   └── keymap.go           Context-specific key bindings
└── testutil/           Test helpers

cmd/
├── taskflow-server/    Server binary
├── taskflow/           CLI binary
├── taskflow-tui/       TUI binary
├── taskflow-seed/      Test data generator
└── taskflow-sim/       Activity simulator for SSE testing

migrations/             Embedded SQL migrations
```

## Key Design Decisions

- **Resources and Operations as data** — `model.Resources()` and `model.Operations()` define the full domain surface; HTTP routes, CLI commands, and OpenAPI docs are all derived from explicit Names
- **`Optional[T]`** for partial updates — cleanly distinguishes "not provided" from "set to nil" with JSON marshaling support
- **Typed actions** — `model.Action` enum with constants; HTTP method and status are derived by the transport layer
- **Attachments are references** — all attachments (including files) are typed references; file storage is infrastructure, not domain
- **Generic struct mapper** — `toModel`/`fromModel` use reflection with a cached dispatch table; field mismatches panic at startup
- **Workflow engine** — pure state machine with JSON Schema validation; workflows are validated on board creation

## Development

Requires Go 1.25+ and [just](https://github.com/casey/just).

```
just check          # fmt-check + vet + test (full suite)
just test           # unit + integration + QA smoke test (45 endpoint checks)
just test-unit      # unit + integration tests only (no server startup)
just build          # build server + CLI binaries
just run            # start the server locally
just fmt            # format code
just seed           # generate test database

just docker-build   # build Docker image
just docker-up      # start with Docker Compose
just docker-down    # stop
just docker-logs    # follow logs
just clean          # remove build artifacts
```

Set `TASKFLOW_DEV_MODE=true` to disable all rate limiting (useful for testing and development). See [TESTING.md](TESTING.md) for the full manual QA checklist.

### Testing with the simulator

The activity simulator generates realistic board activity for testing SSE live updates:

```bash
# Terminal 1: server with test database
just seed && just run

# Terminal 2: simulator
go run ./cmd/taskflow-sim --board platform

# Terminal 3: TUI
TASKFLOW_API_KEY=seed-admin-key-for-testing taskflow-tui
```

The simulator performs a weighted mix of creates, transitions, assignments, and comments every 2-8 seconds, acting as multiple actors.

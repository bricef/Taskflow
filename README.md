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

## Status

**Phase 1 complete. Phase 2 substantially complete.**

### Phase 1 — Server + CLI
- 37 domain operations (actors, boards, tasks, workflows, comments, dependencies, attachments, webhooks, audit, tags)
- HTTP API with auth (SHA-256 keys), RBAC (admin/member/read_only), idempotency keys, and batch operations
- CLI derived from the same operation definitions, with Viper-based config (flags + env + config file)
- OpenAPI 3.1 spec auto-generated at startup
- Convenience endpoints: board detail, system stats, cross-board search
- Docker deployment with seed admin bootstrap
- 140 tests passing across 4 test suites

### Phase 2 — Event Bus + SSE + TUI
- In-process event bus with ring-buffered subscriptions (256 events per subscriber)
- Before/after task snapshots on all events — consumers can diff state without refetching
- SSE endpoint with board-scoped event streaming, heartbeat, and reconnection
- Interactive TUI (`taskflow-tui`) with four tabs:
  - **Board** — kanban view with column scrolling, priority badges, assignee display
  - **List** — sortable table (cycle sort with `s`, reverse with `S`)
  - **Workflow** — layered graph visualisation with Unicode connectors
  - **Events** — live event stream with side-by-side detail pane
- Task detail overlay with comments, dependencies, attachments, and audit trail
- Inline actions: transition (`t`), assign (`a`), comment (`c`) — from kanban, list, or detail views
- SSE live updates across kanban, list, and detail views
- Toggleable help (`?`), done-state toggle (`d`)
- Activity simulator (`taskflow-sim`) for testing SSE updates

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

Operations are defined once in `model.Operations()` and consumed by multiple layers:

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

- **Operations as data** — `model.Operations()` defines every operation once; HTTP routes, CLI commands, and OpenAPI docs are all derived from it
- **`Optional[T]`** for partial updates — cleanly distinguishes "not provided" from "set to nil" with JSON marshaling support
- **Typed actions** — `model.Action` enum with constants; HTTP method and status are derived by the transport layer
- **Attachments are references** — all attachments (including files) are typed references; file storage is infrastructure, not domain
- **Generic struct mapper** — `toModel`/`fromModel` use reflection with a cached dispatch table; field mismatches panic at startup
- **Workflow engine** — pure state machine with JSON Schema validation; workflows are validated on board creation

## Development

Requires Go 1.25+ and [just](https://github.com/casey/just).

```
just check          # fmt-check + vet + test
just test           # run tests
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

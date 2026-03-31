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
- **[TUI Reference](docs/tui.md)** — interactive terminal UI: views, keybindings, live updates
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
- Global and per-board SSE endpoints with heartbeat and reconnection
- Interactive TUI (`taskflow-tui`) — see **[TUI Reference](docs/tui.md)** for full details
- Activity simulator (`taskflow-sim`) for testing live updates

### Phase 3 — AI Integration (MCP Server)
- Not yet started — see [implementation plan](docs/plans/active/2026-03-31-mcp-server.md)

### Phase 4 — Notifications, Dashboard & Polish
- Webhook dispatch with HMAC-SHA256 signatures, retry (3 attempts with backoff), and delivery logging
- Delivery status API: `GET /webhooks/{id}/deliveries`
- HTML dashboard at `/dashboard` with system stats, board overview, and RBAC-aware views
- Live board view at `/dashboard/board/{slug}` with kanban and SSE event stream
- Archived board semantics (mutations blocked, comments allowed)

See [docs/](docs/) for the PRD, phase plans, API reference, and CLI reference.

## Architecture

The domain surface is split into **Resources** (read-only) and **Operations** (mutations), each with an explicit `Name` used as the canonical identifier across all clients:

```
model.Resources()  model.Operations()
 (18 read-only)     (23 mutations)
      │                    │
      ├──────────┬─────────┤
      ▼          ▼         ▼
internal/http  openapi.json  internal/httpclient
 Server side:   Derives:       Client side:
 GET routes      schemas        GetOne, GetMany
 handler map     operationIds   Exec, ExecNoResult
 status codes    parameters     Subscribe (events)
```

The server derives HTTP routes from the model. Clients (CLI, TUI, simulator, MCP) use `httpclient` which accepts model types directly — no manual URL building.

```
taskflow.TaskFlow  (Go interface — compile-time safety)
      │
service.Service    (business logic: validation, audit, orchestration)
      │                    │
repo.Store              eventbus.Bus
      │                    │
sqlite.Store           SSE ──→ httpclient.Subscribe ──→ TUI, MCP
```

Events carry before/after task snapshots so consumers can diff state without refetching. All clients are pure HTTP consumers — they import no server internals.

## Project Structure

```
internal/
├── model/              Domain types, resources, and operation definitions
│   ├── operations.go       Resources and Operations with explicit Names
│   ├── registry.go         Exported named references (ResTaskList, OpTaskCreate, etc.)
│   ├── query.go            Query param derivation from struct tags, path substitution
│   ├── views.go            Composite view types (BoardDetail, BoardOverview, SystemStats)
│   └── ...                 Domain types: Actor, Board, Task, Comment, Dependency, etc.
│
├── taskflow/           Go interface for all business operations
├── service/            Business logic (storage-agnostic)
├── repo/               Storage-agnostic repository interfaces
├── sqlite/             SQLite implementation (sqlx, generic mapper)
├── workflow/           Workflow engine (state machine, JSON Schema validation)
├── transport/          Domain-to-HTTP mapping (MethodForAction, StatusForAction)
├── httpclient/         Domain-aware HTTP client (GetOne, GetMany, Exec, Subscribe)
├── http/               HTTP server (derived routes, middleware, OpenAPI generation)
├── cli/                CLI client (derived commands from model)
├── eventbus/           In-process pub/sub with ring-buffered subscriptions
├── tui/                Interactive terminal UI (Bubble Tea)
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

- **Resources and Operations as data** — `model.Resources()` and `model.Operations()` define the full domain surface; HTTP routes, CLI commands, OpenAPI docs, and MCP tools are all derived from explicit Names
- **Domain-aware client** — `httpclient.GetOne/GetMany/Exec/ExecNoResult` accept model types directly; consumers (TUI, CLI, simulator, MCP) never build URLs manually
- **`Optional[T]`** for partial updates — cleanly distinguishes "not provided" from "set to nil" with JSON marshaling support
- **Typed actions** — `model.Action` enum with constants; HTTP method and status are derived by the transport layer
- **Query params from struct tags** — filter/sort structs use `query` tags; params are derived for OpenAPI, CLI flags, and client query strings
- **Attachments are references** — all attachments (including files) are typed references; file storage is infrastructure, not domain
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

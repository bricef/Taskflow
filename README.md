# TaskFlow

TaskFlow is a task tracker designed for fluid collaboration between humans and AI agents. It provides a durable server as the single source of truth, with tasks organized on kanban boards that have explicitly configured workflow state machines. Any actor — human or AI — can create, advance, review, and manage tasks, with a full audit trail recording every action with actor attribution and timestamps.

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

- **[Architecture](ARCHITECTURE.md)** — package structure, dependency flow, design decisions, event system
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

Operations are defined once in `model.Resources()` and `model.Operations()` and derived into HTTP routes, CLI commands, OpenAPI specs, and the shared `httpclient`. All clients (CLI, TUI, simulator) are pure HTTP consumers — they import no server internals.

See **[ARCHITECTURE.md](ARCHITECTURE.md)** for the full architectural reference: dependency flow, package responsibilities, event system, query param derivation, and design rationale.

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

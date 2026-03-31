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

## Features

- HTTP API with auth (SHA-256 keys), RBAC, idempotency keys, and batch operations
- 41 domain endpoints (18 Resources + 23 Operations) auto-derived from the model
- OpenAPI 3.1 spec auto-generated at startup
- CLI with commands derived from the same model
- Interactive TUI with kanban, list, workflow graph, and live event stream — see **[TUI Reference](docs/tui.md)**
- Real-time event streaming (SSE) with before/after task snapshots
- Webhook dispatch with HMAC-SHA256 signatures, retry, and delivery logging
- HTML dashboard at `/dashboard`
- Docker deployment with seed admin bootstrap

**In progress:** [MCP server](docs/plans/active/2026-03-31-mcp-server.md) for AI agent integration.

See [docs/](docs/) for API, CLI, and TUI reference.

## Roles

Every actor (human or AI) has a role that determines what they can do:

| Action | `admin` | `member` | `read_only` |
|--------|:-------:|:--------:|:-----------:|
| Manage actors (create, update roles) | ✅ | ❌ | ❌ |
| Manage webhooks | ✅ | ❌ | ❌ |
| Delete/reassign boards | ✅ | ❌ | ❌ |
| View system stats | ✅ | ❌ | ❌ |
| Create boards | ✅ | ✅ | ❌ |
| Update boards and workflows | ✅ | ✅ | ❌ |
| Create/update/transition/delete tasks | ✅ | ✅ | ❌ |
| Add comments, dependencies, attachments | ✅ | ✅ | ❌ |
| Read all data (boards, tasks, audit, etc.) | ✅ | ✅ | ✅ |

Requests that exceed the actor's role receive a `403 Forbidden` response. Each Resource and Operation in the model declares its minimum required role.

## Workflows

Each board has a workflow — a state machine that defines how tasks move through stages. Workflows are specified as JSON when creating a board. If omitted, a default workflow is used.

```
backlog → in_progress → review → done
              ↑            │
              └────────────┘ (reject)
         from any → cancelled
```

```json
{
  "states": ["backlog", "in_progress", "review", "done", "cancelled"],
  "initial_state": "backlog",
  "terminal_states": ["done", "cancelled"],
  "transitions": [
    {"from": "backlog", "to": "in_progress", "name": "start"},
    {"from": "in_progress", "to": "review", "name": "submit"},
    {"from": "review", "to": "done", "name": "approve"},
    {"from": "review", "to": "in_progress", "name": "reject"}
  ],
  "from_all": [
    {"to": "cancelled", "name": "cancel"}
  ]
}
```

| Field | Description |
|-------|-------------|
| `states` | All valid states |
| `initial_state` | State assigned to new tasks |
| `terminal_states` | End states — tasks here are considered closed |
| `transitions` | Explicit state-to-state transitions with named actions |
| `from_all` | Transitions reachable from every non-terminal state |
| `to_all` | Transitions from a specific state to every other state |

Tasks are moved between states by name (e.g. `--transition start`), not by target state. Use `taskflow workflow get <board>` or the TUI's Workflow tab to see available transitions.

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

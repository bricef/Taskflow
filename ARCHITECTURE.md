# Architecture

This document describes the current architecture of TaskFlow for developers working on the codebase.

## Overview

TaskFlow is a task tracker with kanban boards and workflow state machines. The server is the single source of truth; all clients (CLI, TUI, MCP, simulator) are pure HTTP consumers that import no server internals.

```
┌──────────────────────────────────────────────────────────┐
│                    TaskFlow Server                        │
│                                                          │
│  taskflow.TaskFlow  (Go interface — compile-time safety) │
│        │                                                 │
│  service.Service    (business logic, audit, validation)  │
│        │                    │                            │
│  repo.Store              eventbus.Bus                    │
│        │                    │                            │
│  sqlite.Store            SSE endpoints                   │
│                                                          │
│  HTTP routes derived from model.Resources/Operations     │
│  OpenAPI spec auto-generated at startup                  │
└──────────────────────────────────────────────────────────┘
        ▲                         ▲
        │ HTTP + JSON             │ SSE (Server-Sent Events)
        │                         │
┌───────┴─────────────────────────┴────────────────────────┐
│                   httpclient.Client                       │
│                                                          │
│  GetOne, GetMany  — typed resource reads                 │
│  Exec, ExecNoResult — typed operation calls              │
│  Subscribe — global event stream with reconnect          │
│                                                          │
│  Accepts model.Resource and model.Operation directly     │
│  Handles path substitution, query params, auth           │
└──────────────────────────────────────────────────────────┘
        ▲           ▲           ▲           ▲
        │           │           │           │
      CLI         TUI         MCP      Simulator
```

## Define once, derive everywhere

The domain surface is defined in two functions:

- `model.Resources()` — 18 read-only endpoints (list boards, get task, etc.)
- `model.Operations()` — 23 mutations (create task, transition, delete board, etc.)

Each entry has an explicit `Name` (e.g. `task_list`, `board_create`) that serves as the canonical identifier across all consumers. From these definitions, the system derives:

| Consumer | What's derived |
|----------|---------------|
| HTTP server | Routes, handler mapping, status codes |
| OpenAPI spec | Paths, methods, parameters, schemas, operationIds |
| CLI | Command tree (`<resource>_<action>` → group + subcommand), flags |
| MCP server | Resources (taskflow:// URIs), tools (input schemas), descriptions |
| httpclient | Path substitution, HTTP method, query string building |

Named references are exported as package-level variables (`model.ResTaskList`, `model.OpTaskCreate`, etc.) so consumers reference domain types directly without string lookups.

## Package structure

```
internal/
├── model/              Domain types and the operation registry
├── taskflow/           Go interface for all business operations
├── service/            Business logic (storage-agnostic)
├── repo/               Storage-agnostic repository interfaces
├── sqlite/             SQLite implementation
├── workflow/           Workflow engine (state machine with JSON definition)
├── transport/          Maps domain actions to HTTP semantics (method, status code)
├── httpclient/         Domain-aware HTTP client for all consumers
├── http/               HTTP server (routes, middleware, OpenAPI generation)
├── mcp/                MCP server (tools + resources derived from model, notifications)
├── cli/                CLI (commands derived from model)
├── eventbus/           In-process pub/sub with ring-buffered subscriptions
├── tui/                Interactive terminal UI (Bubble Tea)
└── testutil/           Test helpers

cmd/
├── taskflow-server/    Server binary
├── taskflow/           CLI binary
├── taskflow-tui/       TUI binary
├── taskflow-mcp/       MCP server binary (stdio transport)
├── taskflow-seed/      Test data generator
└── taskflow-sim/       Activity simulator
```

## Dependency flow

Dependencies flow inward. No package imports from a layer above it.

```
model          — zero dependencies (pure domain types)
  ↑
taskflow       — depends on model (interface definition)
  ↑
repo           — depends on model (storage interface)
  ↑
service        — depends on model, repo, taskflow, eventbus, workflow
  ↑
sqlite         — depends on model, repo (storage implementation)
transport      — depends on model (HTTP method/status mapping)
httpclient     — depends on model, transport, eventbus
http           — depends on model, transport, service, taskflow, eventbus
mcp            — depends on model, httpclient, eventbus
cli            — depends on model, httpclient
tui            — depends on model, httpclient, eventbus
```

The `httpclient` package has no dependency on the `http` server package. Clients and server are completely decoupled — they communicate only through HTTP and SSE.

## Resources and Operations

### Resource

A read-only domain endpoint. Always served via GET, returns 200.

```go
type Resource struct {
    Name    string       // e.g. "task_list"
    Path    string       // e.g. "/boards/{slug}/tasks"
    Summary string
    MinRole Role         // defaults to RoleReadOnly
    Output  any          // zero-value for schema generation
    Filter  any          // struct with `query` tags for filter params
    Sort    any          // struct with `query` tags for sort params
}
```

### Operation

A domain mutation. HTTP method and status code are derived from the Action.

```go
type Operation struct {
    Name    string       // e.g. "task_create"
    Action  Action       // create, update, delete, transition, etc.
    Path    string
    Summary string
    MinRole Role         // defaults to RoleMember
    Input   any          // zero-value for schema generation
    Output  any
}
```

### Query parameter derivation

Filter and sort structs use `query` tags to declare their query parameters:

```go
type TaskFilter struct {
    BoardSlug      string              // path param, no tag — not a query param
    State          *string   `query:"state,Filter by workflow state"`
    Assignee       *string   `query:"assignee,Filter by assignee name"`
    Priority       *Priority `query:"priority,Filter by priority"`
    IncludeClosed  bool      `query:"include_closed,Include tasks in terminal states"`
}
```

`model.QueryParamsFrom(filter)` derives the parameter schema (for OpenAPI and CLI flags). `model.BuildQueryString(filter)` builds a URL query string from a populated struct (for the httpclient). Both use the same tags — the schema and the runtime behaviour are always in sync.

## HTTP server

Routes are derived from the model at startup:

1. `model.Resources()` → each becomes a GET route
2. `model.Operations()` → each becomes a route with method from `transport.MethodForAction`
3. Handlers are registered in a `map[string]handler` keyed by Name — no positional coupling

The server also hosts convenience endpoints not in the model:
- SSE event streams (`/boards/{slug}/events`, `/events`)
- Batch operations (`/batch`)
- Dashboard (`/dashboard`)
- Health check (`/health`)
- OpenAPI spec (`/openapi.json`)

### OpenAPI generation

The OpenAPI 3.1 spec is generated at startup from the route list. Each route contributes:
- Path and method
- `operationId` from Name
- Path parameters (inferred from `{param}` in path)
- Query parameters (derived from Filter/Sort struct tags)
- Request body schema (from Input struct's json tags)
- Response schema (from Output type)

## httpclient

The shared HTTP client provides domain-aware access for all consumers. Key design points:

- **Constructor**: `httpclient.New(baseURL, apiKey)` — stores context internally
- **Generic functions** (package-level, due to Go's lack of generic methods):
  - `GetOne[T](client, resource, params, filter)` — single typed result
  - `GetMany[T](client, resource, params, filter)` — typed slice
  - `Exec[T](client, operation, params, body)` — typed mutation result
  - `ExecNoResult(client, operation, params, body)` — no-content mutations
- **Event subscription**: `client.Subscribe(ctx, opts)` returns an `EventStream` with channels for events, errors, and connection status. Reconnects with exponential backoff.
- **Path substitution**: `model.SubstitutePath` replaces `{param}` placeholders
- **Query strings**: `model.BuildQueryString` serializes filter structs via `query` tags, or accepts `map[string]string` for CLI flag values

## Event system

### Event bus

In-process pub/sub (`eventbus.EventBus`). The service layer publishes events after successful mutations. Subscribers receive events on buffered channels (256-entry ring buffer — publishing never blocks).

### Event structure

Every event carries before/after task snapshots:

```go
type Event struct {
    Type      string        // e.g. "task.transitioned"
    Timestamp time.Time
    Actor     ActorRef
    Board     BoardRef
    Before    *TaskSnapshot // nil for creates
    After     *TaskSnapshot // nil for deletes
    Detail    any
}
```

Consumers can diff state without refetching. For creates, `Before` is nil. For deletes, `After` is nil.

### SSE endpoints

- `GET /boards/{slug}/events` — board-scoped stream
- `GET /events` — global stream with optional `?boards=` and `?assignee=` filters

Both include heartbeats and support reconnection via `?token=` for auth (since EventSource can't set headers).

### Client-side event handling

The TUI subscribes to the global event stream at startup and routes events into per-board ring buffers. Switching boards swaps the active buffer — no reconnection needed, and event history is preserved.

## Workflow engine

Each board has a workflow defined as a JSON state machine:

```json
{
  "states": ["backlog", "in_progress", "review", "done", "cancelled"],
  "initial_state": "backlog",
  "terminal_states": ["done", "cancelled"],
  "transitions": [
    {"from": "backlog", "to": "in_progress", "name": "start"},
    {"from": "in_progress", "to": "review", "name": "submit"}
  ],
  "from_all": [{"to": "cancelled", "name": "cancel"}]
}
```

The workflow engine validates transitions, enforces terminal states, and provides health checks (detecting tasks orphaned in states with no outgoing transitions). Workflows are validated on board creation and replacement.

## Versioning

All binaries embed a version string from `git describe --tags --always` at build time via ldflags. The `internal/version.Version` variable defaults to `"dev"` if not set.

- Server adds `X-TaskFlow-Version` to all response headers
- `/health` includes `version` in the JSON response
- MCP reports the version during capability negotiation
- The httpclient checks the server version on the first request and warns on stderr if versions differ or the header is missing

## Authentication and RBAC

API keys are SHA-256 hashed and stored with actor records. Creating an actor via the API generates a random key and returns it once in the response. Keys can be rotated with `PATCH /actors/{name}/rotate-key` — the old key is immediately invalidated. Three roles:

| Role | Permissions |
|------|------------|
| `admin` | All operations including actor/webhook management and board deletion |
| `member` | Create/update/transition/delete tasks, comments, deps, attachments |
| `read_only` | Read all data, no mutations |

Each Resource and Operation declares a `MinRole`. The HTTP middleware checks the actor's role against the endpoint's requirement.

## Archived boards

When a board is soft-deleted, mutations are blocked (403 Forbidden) except for adding comments (append-only). Read operations continue to work. Boards can be listed with `include_deleted=true`.

## Testing

The test suite has three layers:

1. **Unit tests** — `inferPathParams`, `MethodForAction`, `StatusForAction`, operation invariants
2. **Golden tests** — full OpenAPI spec and CLI command tree snapshots; any change produces a diff
3. **Integration tests** — HTTP server tests with in-memory SQLite, CLI tests against httptest server
4. **QA smoke test** — `scripts/qa-test.sh` runs 45 automated checks against a live server: all resource endpoints, mutations, audit, convenience endpoints, OpenAPI spec, dashboard, and CLI commands

Run everything with `just test`. Run without the smoke test with `just test-unit`.

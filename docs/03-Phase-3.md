# TaskFlow — Phase 3 Implementation Plan

**AI Integration: MCP Server**

*Version: 1.1*
*Date: 2026-03-31*
*Ref: prd-taskflow-v1.2.md, phase 1-2 implementation*

---

## Approach

Phase 3 adds an MCP (Model Context Protocol) server that exposes TaskFlow operations as tools and resources. This enables AI agents (Claude Code, Aider, Cursor, etc.) to participate as full task management peers.

The MCP handlers are implemented in a **shared `internal/mcp` package** and exposed via two transports:

1. **Stdio binary** (`cmd/taskflow-mcp`) — for clients that launch local processes (Claude Code, Aider). One instance per agent, each with its own API key.
2. **HTTP endpoint** (`/mcp` on the TaskFlow server) — for clients that support HTTP+SSE transport. Multi-tenant, auth via Bearer token per request.

Both transports share the same handler code. The stdio binary acts as an HTTP client to the remote server (same as TUI/CLI). The embedded endpoint calls the service layer directly.

The implementation uses the **official Go MCP SDK** (`github.com/modelcontextprotocol/go-sdk`).

## Design Decisions

| Decision | Resolution | Rationale |
|----------|-----------|-----------|
| SDK | Official `go-sdk` | Maintained by MCP + Google, supports spec 2025-11-25, both stdio and HTTP transports built-in |
| Transports | Stdio + HTTP | Stdio for maximum client compatibility today. HTTP for future-facing integration and simpler deployment (no binary distribution). |
| Shared handlers | `internal/mcp` package | Tool and resource handlers are transport-agnostic. Stdio binary and HTTP endpoint both consume the same package. |
| Auth (stdio) | Per-instance API key via env var | Passed when launching the binary. Agent identity comes from the API key. |
| Auth (HTTP) | Bearer token per request | Same API keys as the HTTP API. The server's auth middleware extracts the actor. |
| Tools vs Resources | Both | Idiomatic MCP. Read-only data (boards, tasks, workflows, audit) as resources. Mutations (create, update, transition, delete) as tools. |
| Tool derivation | Manual mapping from Operations() | Operations define the domain surface. Tools map to operations but with MCP-appropriate naming, descriptions, and input schemas. Some tools combine operations (e.g. get_task returns task + comments + deps). |
| Notifications | Dual mechanism | 1) Standard MCP notifications via `notifications/resources/list_changed` when SSE events arrive. 2) Piggyback `_notifications` array on tool responses for clients that don't support MCP notifications. Broadens compatibility. |
| SSE subscription | Background goroutine | MCP server subscribes to the SSE stream. Events from other actors are queued as pending notifications. Reconnects silently. |
| Reporting tools | Deferred | The PRD lists CFD, cycle time, throughput tools. These require new API endpoints. Deferred — raw audit data is already accessible via resources. |

## Architecture

```
Stdio transport (one instance per agent):

┌──────────────┐     stdio      ┌─────────────┐     HTTP/SSE     ┌──────────────┐
│  AI Agent    │ ◄────────────► │ taskflow-mcp │ ────────────────► │ TaskFlow     │
│ (Claude Code)│     JSON-RPC   │  (per-agent) │   API + Events   │ Server       │
└──────────────┘                └─────────────┘                  └──────────────┘
                                  uses internal/mcp handlers
                                  + HTTP client


HTTP transport (embedded in server):

┌──────────────┐    HTTP+SSE    ┌──────────────────────────────────┐
│  AI Agent    │ ◄────────────► │ TaskFlow Server                  │
│ (any client) │    JSON-RPC    │  /mcp endpoint                   │
└──────────────┘                │  uses internal/mcp handlers      │
                                │  + direct service calls          │
                                └──────────────────────────────────┘
```

## Handler Interface

The `internal/mcp` package defines a backend interface that both transports implement:

```go
// Backend provides data access for MCP handlers.
// The stdio binary implements this with HTTP calls.
// The embedded endpoint implements this with direct service calls.
type Backend interface {
    ListBoards(ctx context.Context) ([]model.Board, error)
    GetBoard(ctx context.Context, slug string) (model.Board, error)
    ListTasks(ctx context.Context, filter model.TaskFilter) ([]model.Task, error)
    GetTask(ctx context.Context, slug string, num int) (model.Task, error)
    // ... all operations needed by tools and resources
}
```

This keeps the MCP handlers independent of whether they're running in-process or over HTTP.

## MCP Resources (read-only)

Resources expose data that agents can read. All URIs use the `taskflow://` scheme (see NOTES.md for full scheme definition). Changes are signalled via `notifications/resources/list_changed`.

| Resource URI | Description |
|-------------|-------------|
| `taskflow://boards` | List of all boards |
| `taskflow://boards/{slug}` | Board detail with workflow and task counts by state |
| `taskflow://boards/{slug}/tasks` | All tasks on a board |
| `taskflow://boards/{slug}/tasks/{num}` | Full task detail: metadata, comments, dependencies, attachments, audit |
| `taskflow://boards/{slug}/workflow` | Workflow definition (states, transitions) |
| `taskflow://boards/{slug}/audit` | Board-level audit trail |
| `taskflow://boards/{slug}/tags` | Tags in use on a board |
| `taskflow://actors` | List of actors |
| `taskflow://search?q={query}` | Cross-board task search |
| `taskflow://stats` | System stats (admin only) |

## MCP Tools (mutations)

Tools expose actions that change state. Each tool maps to one or more domain operations. Transition errors preserve available transitions in the error detail for agent self-correction.

| Tool | Description | Maps to |
|------|-------------|---------|
| `create_board` | Create a new board with optional workflow | POST /boards |
| `update_board` | Update board name/description | PATCH /boards/{slug} |
| `delete_board` | Archive (soft-delete) a board | DELETE /boards/{slug} |
| `create_task` | Create a task on a board | POST /boards/{slug}/tasks |
| `update_task` | Update task fields | PATCH /boards/{slug}/tasks/{num} |
| `transition_task` | Execute a workflow transition | POST /boards/{slug}/tasks/{num}/transition |
| `delete_task` | Soft-delete a task | DELETE /boards/{slug}/tasks/{num} |
| `add_comment` | Add a comment to a task | POST /boards/{slug}/tasks/{num}/comments |
| `update_comment` | Edit a comment | PUT /comments/{id} |
| `add_dependency` | Link two tasks | POST /boards/{slug}/tasks/{num}/dependencies |
| `remove_dependency` | Remove a dependency | DELETE /dependencies/{id} |
| `add_attachment` | Attach a reference to a task | POST /boards/{slug}/tasks/{num}/attachments |
| `remove_attachment` | Remove an attachment | DELETE /attachments/{id} |
| `batch` | Execute up to 50 operations in a single call | POST /batch |

Admin-only tools (role checked by the API/service layer):

| Tool | Description |
|------|-------------|
| `create_actor` | Create an actor |
| `update_actor` | Update actor details |
| `create_webhook` | Register a webhook |
| `update_webhook` | Update a webhook |
| `delete_webhook` | Remove a webhook |
| `reassign_tasks` | Move tasks between boards |

## Notification System

The MCP server subscribes to the **global SSE endpoint** (`GET /events`) with `?assignee=@me` to receive events relevant to the agent's tasks across all boards. This requires a new global SSE endpoint as a prerequisite (see Increment 0).

### Standard MCP notifications

When SSE events arrive from other actors, the MCP server sends `notifications/resources/list_changed` to signal that resource data has changed. The agent can re-read affected resources.

### Piggyback notifications

For clients that don't support MCP notifications, pending events are included in tool response metadata as a `_notifications` array:

```json
{
  "result": { ... },
  "_notifications": [
    {"event": "task.transitioned", "board": "platform", "task": "platform/3", "actor": "alice", "timestamp": "..."}
  ]
}
```

Notifications are cleared after delivery. If the queue exceeds 50 entries, older entries are summarized.

## Increments

### Increment 0: Prerequisites (1 day)

API features needed before MCP work:

- **Global SSE endpoint** (`GET /events`) with optional `?boards=` and `?assignee=` filters. Supports `@me` for assignee. Complements the existing per-board `GET /boards/{slug}/events`.
- **Cross-board "my tasks"** — `GET /tasks?assignee=@me` or similar. Returns tasks assigned to the caller across all boards.
- **Board overview with task counts** — enrich board detail / board list responses with task counts by state.

### Increment 1: Shared package + stdio binary scaffold (1 day)

- `internal/mcp/` — shared package with Backend interface
- `internal/mcp/httpbackend/` — Backend implementation using HTTP client (for stdio binary)
- `cmd/taskflow-mcp/main.go` — stdio binary entry point
- MCP server setup with official SDK (stdio transport)
- Capability negotiation (tools + resources)
- Configuration via env vars (`TASKFLOW_SERVER_URL`, `TASKFLOW_API_KEY`)
- Extract shared HTTP client from TUI into a common package

### Increment 2: Resources (1 day)

- Implement all resource handlers in `internal/mcp/`
- `taskflow://` URI scheme with template matching
- JSON response formatting
- Full task detail aggregation (task + comments + deps + attachments + audit)
- Test with stdio binary against running server

### Increment 3: Tools (1-2 days)

- Implement all tool handlers in `internal/mcp/`
- Input schema definitions
- Error mapping (API errors → MCP tool errors, preserving transition context)
- Batch tool wrapping `POST /batch`
- Test with stdio binary against running server

### Increment 4: HTTP transport + notifications (1-2 days)

- `internal/mcp/servicebackend/` — Backend implementation using direct service calls
- Mount `/mcp` endpoint on the TaskFlow server (HTTP+SSE transport)
- Auth via existing Bearer token middleware
- SSE subscription via global `/events?assignee=@me` endpoint
- Event queue with 50-entry cap
- Standard MCP `notifications/resources/list_changed` on SSE events
- Piggyback `_notifications` on tool responses
- Reconnection with backoff

### Increment 5: Documentation + testing (1 day)

- Agent setup guide (Claude Code, Aider, Cursor)
- Example MCP config files for both transports
- Integration tests with mock MCP client
- Update README, CLI skill doc, and dashboard docs

**Total estimate: 6-8 days**

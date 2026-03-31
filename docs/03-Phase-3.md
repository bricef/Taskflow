# TaskFlow — Phase 3 Implementation Plan

**AI Integration: MCP Server**

*Version: 1.0*
*Date: 2026-03-31*
*Ref: prd-taskflow-v1.2.md, phase 1-2 implementation*

---

## Approach

Phase 3 adds an MCP (Model Context Protocol) server that exposes TaskFlow operations as tools and resources over the stdio transport. This enables AI agents (Claude Code, Aider, Cursor, etc.) to participate as full task management peers.

The MCP server is a **separate binary** (`taskflow-mcp`) that acts as an HTTP client to the TaskFlow server — the same architecture as the TUI and CLI. One instance runs per AI agent, each configured with its own API key for actor attribution.

The implementation uses the **official Go MCP SDK** (`github.com/modelcontextprotocol/go-sdk`).

## Design Decisions

| Decision | Resolution | Rationale |
|----------|-----------|-----------|
| SDK | Official `go-sdk` | Maintained by MCP + Google, supports spec 2025-11-25, stdio transport built-in |
| Transport | stdio | Standard for CLI-launched MCP servers (Claude Code, Aider). No HTTP server needed in the MCP binary. |
| Auth | Per-instance API key via env var | One MCP server per agent. API key passed to all HTTP calls. Clean actor attribution. |
| Tools vs Resources | Both | Idiomatic MCP. Read-only data (boards, tasks, workflows, audit) as resources. Mutations (create, update, transition, delete) as tools. |
| Tool derivation | Manual mapping from Operations() | Operations define the domain surface. Tools map to operations but with MCP-appropriate naming, descriptions, and input schemas. Some tools combine operations (e.g. get_task returns task + comments + deps). |
| Notifications | Dual mechanism | 1) Standard MCP notifications via `notifications/resources/list_changed` when SSE events arrive. 2) Piggyback `_notifications` array on tool responses for clients that don't support MCP notifications. |
| SSE subscription | Background goroutine | MCP server subscribes to the board SSE stream. Events from other actors are queued as pending notifications. Reconnects silently. |
| Reporting tools | Deferred | The PRD lists CFD, cycle time, throughput tools. These require new API endpoints. Deferred — raw audit data is already accessible. |

## Architecture

```
┌──────────────┐     stdio      ┌─────────────┐     HTTP/SSE     ┌──────────────┐
│  AI Agent    │ ◄────────────► │ taskflow-mcp │ ────────────────► │ TaskFlow     │
│ (Claude Code │     JSON-RPC   │              │   API + Events   │ Server       │
│  Aider, etc) │                │  per-agent   │                  │              │
└──────────────┘                └─────────────┘                  └──────────────┘
```

## MCP Resources (read-only)

Resources expose data that agents can read. Changes are signalled via `notifications/resources/list_changed`.

| Resource URI | Description |
|-------------|-------------|
| `boards` | List of all boards |
| `boards/{slug}` | Board detail with workflow |
| `boards/{slug}/tasks` | All tasks on a board (supports query params) |
| `boards/{slug}/tasks/{num}` | Full task detail: metadata, comments, dependencies, attachments, audit |
| `boards/{slug}/workflow` | Workflow definition (states, transitions) |
| `boards/{slug}/audit` | Board-level audit trail |
| `boards/{slug}/tags` | Tags in use on a board |
| `actors` | List of actors |
| `search?q={query}` | Cross-board task search |

## MCP Tools (mutations)

Tools expose actions that change state. Each tool maps to one or more domain operations.

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
| `add_dependency` | Link two tasks | POST /boards/{slug}/tasks/{num}/dependencies |
| `remove_dependency` | Remove a dependency | DELETE /dependencies/{id} |
| `add_attachment` | Attach a reference to a task | POST /boards/{slug}/tasks/{num}/attachments |
| `remove_attachment` | Remove an attachment | DELETE /attachments/{id} |

Admin-only tools (role checked by API):

| Tool | Description |
|------|-------------|
| `create_actor` | Create an actor |
| `update_actor` | Update actor details |
| `create_webhook` | Register a webhook |
| `update_webhook` | Update a webhook |
| `delete_webhook` | Remove a webhook |
| `reassign_tasks` | Move tasks between boards |

## Notification System

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

### Increment 1: Binary scaffold + tool framework (1 day)

- `cmd/taskflow-mcp/main.go` — binary entry point
- HTTP client (reuse from TUI or shared package)
- MCP server setup with official SDK (stdio transport)
- Capability negotiation (tools + resources)
- Configuration via env vars (`TASKFLOW_SERVER_URL`, `TASKFLOW_API_KEY`)

### Increment 2: Resources (1 day)

- Implement all resource handlers (boards, tasks, workflow, audit, tags, search)
- Resource URI routing
- JSON response formatting

### Increment 3: Tools (1-2 days)

- Implement all tool handlers (create/update/delete for tasks, boards, comments, deps, attachments)
- Input schema generation from Go structs
- Error mapping (API errors → MCP tool errors)

### Increment 4: Notifications (1 day)

- SSE subscription goroutine (reuse SSE client from TUI)
- Event queue with 50-entry cap
- Standard MCP `notifications/resources/list_changed` on SSE events
- Piggyback `_notifications` on tool responses
- Reconnection with backoff

### Increment 5: Documentation + testing (1 day)

- Agent setup guide (Claude Code, Aider)
- Example MCP config files
- Integration tests with mock MCP client
- Update README and CLI skill doc

**Total estimate: 5-6 days**

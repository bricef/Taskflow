# MCP Server Reference

The MCP server (`taskflow-mcp`) exposes TaskFlow as tools and resources over the Model Context Protocol. AI agents (Claude Code, Aider, Cursor, etc.) can create, update, transition, and query tasks through their native MCP integration.

## Setup

### Prerequisites

A running TaskFlow server and an API key for the agent's actor.

### Claude Code

Add to your Claude Code MCP config (`.claude/settings.json` or project settings):

```json
{
  "mcpServers": {
    "taskflow": {
      "command": "taskflow-mcp",
      "env": {
        "TASKFLOW_URL": "http://localhost:8374",
        "TASKFLOW_API_KEY": "your-agent-api-key"
      }
    }
  }
}
```

### Other MCP Clients

Any MCP client that supports stdio transport can use `taskflow-mcp`:

```bash
TASKFLOW_URL=http://localhost:8374 TASKFLOW_API_KEY=your-key taskflow-mcp
```

The binary communicates via JSON-RPC over stdin/stdout. Logs go to stderr.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `TASKFLOW_URL` | `http://localhost:8374` | TaskFlow server URL |
| `TASKFLOW_API_KEY` | (required) | API key for authentication and actor identity |

Each agent instance should use its own API key so actions are attributed to the correct actor in the audit trail.

## Resources

Resources are read-only data endpoints. All URIs use the `taskflow://` scheme.

| Resource | URI | Description |
|----------|-----|-------------|
| `actor_list` | `taskflow://actors` | List all actors |
| `actor_get` | `taskflow://actors/{name}` | Get an actor |
| `board_list` | `taskflow://boards` | List all boards |
| `board_get` | `taskflow://boards/{slug}` | Get a board |
| `board_detail` | `taskflow://boards/{slug}/detail` | Full board dump (tasks, comments, deps, audit) |
| `board_overview` | `taskflow://boards/{slug}/overview` | Board with task counts by state |
| `workflow_get` | `taskflow://boards/{slug}/workflow` | Workflow definition |
| `task_list` | `taskflow://boards/{slug}/tasks` | List tasks on a board |
| `task_get` | `taskflow://boards/{slug}/tasks/{num}` | Get a task |
| `task_search` | `taskflow://tasks` | Search tasks across all boards |
| `tag_list` | `taskflow://boards/{slug}/tags` | Tags in use on a board |
| `comment_list` | `taskflow://boards/{slug}/tasks/{num}/comments` | Comments on a task |
| `dependency_list` | `taskflow://boards/{slug}/tasks/{num}/dependencies` | Task dependencies |
| `attachment_list` | `taskflow://boards/{slug}/tasks/{num}/attachments` | Task attachments |
| `admin_stats` | `taskflow://admin/stats` | System-wide statistics (admin only) |
| `webhook_list` | `taskflow://webhooks` | List webhooks (admin only) |
| `webhook_get` | `taskflow://webhooks/{id}` | Get a webhook (admin only) |
| `delivery_list` | `taskflow://webhooks/{id}/deliveries` | Webhook delivery log (admin only) |

## Tools

Tools are mutations that change state. Path parameters and body fields are passed together as tool input.

### Task Management

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `task_create` | Create a task | `slug`, `title`, `priority`, `assignee`, `tags` |
| `task_update` | Update a task | `slug`, `num`, plus fields to change |
| `task_transition` | Move a task to a new state | `slug`, `num`, `transition` |
| `task_delete` | Soft-delete a task | `slug`, `num` |

### Board Management

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `board_create` | Create a board | `slug`, `name`, `workflow` (optional) |
| `board_update` | Update board name/description | `slug`, fields to change |
| `board_delete` | Archive a board (admin) | `slug` |
| `board_reassign` | Move tasks between boards (admin) | `slug`, `target_board` |

### Workflow

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `workflow_set` | Replace a board's workflow | `slug`, workflow JSON |
| `workflow_health` | Check for orphaned tasks | `slug` |

### Comments, Dependencies, Attachments

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `comment_create` | Add a comment | `slug`, `num`, `body` |
| `comment_update` | Edit a comment | `id`, fields to change |
| `dependency_create` | Link two tasks | `slug`, `num`, `depends_on_board`, `depends_on_num`, `dep_type` |
| `dependency_delete` | Remove a link | `id` |
| `attachment_create` | Attach a reference | `slug`, `num`, `ref_type`, `reference`, `label` |
| `attachment_delete` | Remove an attachment | `id` |

### Audit

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `task_audit` | Get audit log for a task | `slug`, `num` |
| `board_audit` | Get audit log for a board | `slug` |

### Admin

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `actor_create` | Create an actor (admin) | `name`, `type`, `role` |
| `actor_update` | Update an actor (admin) | `name`, fields to change |
| `webhook_create` | Create a webhook (admin) | `url`, `events`, `secret` |
| `webhook_update` | Update a webhook (admin) | `id`, fields to change |
| `webhook_delete` | Delete a webhook (admin) | `id` |

## Workflow Tips for Agents

1. **Check available transitions** before transitioning: read the `workflow_get` resource for the board, then match the task's current state to available transitions.

2. **Use `@me` for self-assignment**: when creating or updating tasks, use `@me` as the assignee value.

3. **Transition by name, not state**: pass the transition name (e.g. `start`, `submit`, `approve`), not the target state.

4. **Board detail for full context**: use `board_detail` to get a complete snapshot of a board including all tasks with their comments, dependencies, and audit.

5. **Task search across boards**: use `task_search` to find tasks across all boards by keyword.

## Architecture

The MCP server is a thin adapter over `internal/httpclient`. Resources and tools are auto-derived from `model.Resources()` and `model.Operations()` — the same definitions that drive the HTTP API, CLI, and OpenAPI spec. No manual tool registration is needed.

```
AI Agent ←→ taskflow-mcp (stdio) ←→ httpclient ←→ TaskFlow Server
```

Each agent instance runs as a separate process with its own API key. The agent's identity comes from the API key — all actions are attributed to the corresponding actor in the audit trail.

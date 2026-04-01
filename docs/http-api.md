# TaskFlow HTTP API

The TaskFlow server exposes a RESTful JSON API. All endpoints (except `/health` and `/openapi.json`) require authentication via a Bearer token.

## Authentication

Include the API key in every request:

```
Authorization: Bearer <api-key>
```

API keys are SHA-256 hashed before storage. The seed admin key is written to `seed-admin-key.txt` on first server start (see [Getting Started](../README.md#getting-started)).

## OpenAPI Spec

A machine-readable OpenAPI 3.1 spec is generated at startup from the operation definitions and served at:

```
GET /openapi.json
```

No authentication required.

## Idempotency

Mutating requests (POST, PATCH, PUT, DELETE) support idempotency keys for safe retries:

```
Idempotency-Key: <unique-string>
```

If a request with the same key has been seen before, the server returns the cached response without re-executing the operation.

- GET requests are never cached, even with the header present.
- The cache is bounded by total memory (default 50 MB). Oldest entries are evicted when the budget is exceeded.
- Keys are scoped to the server process — they reset on restart.

## Error Responses

All errors return a consistent JSON format:

```json
{
  "error": "validation_error",
  "message": "validation error: title: must not be empty",
  "detail": {"field": "title"}
}
```

| Status | Meaning |
|--------|---------|
| 400 | Validation error — invalid input |
| 401 | Unauthorized — missing or invalid API key |
| 403 | Forbidden — insufficient role |
| 404 | Not found |
| 409 | Conflict — duplicate or referential integrity violation |
| 500 | Internal server error |

## `@me` Actor Alias

Any field that accepts an actor name (`assignee` in task create/update, `assignee` query filter) supports the special value `@me`, which resolves to the authenticated actor's name.

```bash
# Create a task assigned to myself
curl -X POST /boards/my-board/tasks \
  -H "Authorization: Bearer $KEY" \
  -d '{"title": "My task", "assignee": "@me"}'

# List my tasks
curl "/boards/my-board/tasks?assignee=@me" -H "Authorization: Bearer $KEY"
```

## Endpoints

### Actors

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/actors` | Create an actor (returns API key once) | admin |
| GET | `/actors` | List all actors | read_only |
| GET | `/actors/{name}` | Get an actor by name | read_only |
| PATCH | `/actors/{name}/rotate-key` | Rotate API key (returns new key once) | admin |
| PATCH | `/actors/{name}` | Update an actor | admin |

### Self

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/me` | Get the authenticated actor's own record | read_only |

### Boards

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards` | Create a board | member |
| GET | `/boards` | List boards | read_only |
| GET | `/boards/{slug}` | Get a board | read_only |
| PATCH | `/boards/{slug}` | Update a board | member |
| DELETE | `/boards/{slug}` | Delete a board (soft-delete) | admin |
| POST | `/boards/{slug}/reassign` | Reassign tasks to another board | admin |
| GET | `/boards/{slug}/detail` | Complete board with all nested data | read_only |
| GET | `/boards/{slug}/overview` | Board with task counts by state | read_only |

Query params for list: `include_deleted` (boolean).

### Workflows

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/workflow` | Get the board's workflow definition | read_only |
| PUT | `/boards/{slug}/workflow` | Replace the board's workflow | member |
| POST | `/boards/{slug}/workflow/health` | Check workflow health | member |

### Tasks

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards/{slug}/tasks` | Create a task | member |
| GET | `/boards/{slug}/tasks` | List tasks (with filters and search) | read_only |
| GET | `/boards/{slug}/tasks/{num}` | Get a task | read_only |
| GET | `/boards/{slug}/tasks/{num}/detail` | Full task detail (comments, deps, attachments, audit) | read_only |
| PATCH | `/boards/{slug}/tasks/{num}` | Update a task | member |
| POST | `/boards/{slug}/tasks/{num}/transition` | Transition a task to a new state | member |
| DELETE | `/boards/{slug}/tasks/{num}` | Delete a task (soft-delete) | member |

Query params for list: `state`, `assignee`, `priority`, `tag`, `q` (full-text search), `include_closed`, `include_deleted`, `sort` (created_at/updated_at/priority/due_date), `order` (asc/desc).

### Cross-Board Tasks

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/tasks` | Search/filter tasks across all boards | read_only |

Same query params as task list above.

### Comments

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards/{slug}/tasks/{num}/comments` | Add a comment to a task | member |
| GET | `/boards/{slug}/tasks/{num}/comments` | List comments on a task | read_only |
| PATCH | `/comments/{id}` | Edit a comment | member |

### Dependencies

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards/{slug}/tasks/{num}/dependencies` | Add a dependency | member |
| GET | `/boards/{slug}/tasks/{num}/dependencies` | List dependencies for a task | read_only |
| DELETE | `/dependencies/{id}` | Remove a dependency | member |

### Attachments

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards/{slug}/tasks/{num}/attachments` | Add an attachment | member |
| GET | `/boards/{slug}/tasks/{num}/attachments` | List attachments on a task | read_only |
| DELETE | `/attachments/{id}` | Remove an attachment | member |

### Audit

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/tasks/{num}/audit` | Get audit log for a task | read_only |
| GET | `/boards/{slug}/audit` | Get audit log for a board | read_only |

### Tags

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/tags` | List tags in use on a board with counts | read_only |

### Admin

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/admin/stats` | System-wide statistics | admin |

### Webhooks

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/webhooks` | Create a webhook | admin |
| GET | `/webhooks` | List webhooks | admin |
| GET | `/webhooks/{id}` | Get a webhook | admin |
| PATCH | `/webhooks/{id}` | Update a webhook | admin |
| DELETE | `/webhooks/{id}` | Delete a webhook | admin |
| GET | `/webhooks/{id}/deliveries` | List webhook delivery attempts | admin |

### Event Streaming (SSE)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/boards/{slug}/events` | Board-scoped event stream |
| GET | `/events` | Global event stream (optional `?boards=`, `?assignee=` filters) |

SSE endpoints accept `?token=<api-key>` for auth (since EventSource can't set headers).

### Batch Operations

`POST /batch` accepts an array of operations and executes them sequentially:

```json
{
  "operations": [
    {"method": "POST", "path": "/boards/my-board/tasks", "body": {"title": "Task 1"}},
    {"method": "POST", "path": "/boards/my-board/tasks", "body": {"title": "Task 2"}}
  ]
}
```

Response:

```json
{
  "results": [
    {"status": 201, "body": {"board_slug": "my-board", "num": 1, ...}},
    {"status": 201, "body": {"board_slug": "my-board", "num": 2, ...}}
  ]
}
```

Max 50 operations per batch. Each operation can include an optional `idempotency_key`.

### Public Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (returns `{"status": "ok"}`) |
| GET | `/openapi.json` | OpenAPI 3.1 spec |
| GET | `/dashboard` | HTML dashboard |
| GET | `/dashboard/board/{slug}` | Board detail dashboard |

## Server Configuration

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `TASKFLOW_DB_PATH` | `./taskflow.db` | SQLite database path |
| `TASKFLOW_LISTEN_ADDR` | `:8374` | Listen address |
| `TASKFLOW_SEED_ADMIN_NAME` | — | Name for the seed admin actor (created on first start if no actors exist) |
| `TASKFLOW_SEED_ADMIN_DISPLAY_NAME` | same as name | Display name for the seed admin |
| `TASKFLOW_SEED_KEY_FILE` | `./seed-admin-key.txt` | Path to write the seed admin API key |
| `TASKFLOW_DEV_MODE` | `false` | Set to `true` to disable all rate limiting |
| `TASKFLOW_RATE_LIMIT` | `50` | Per-key requests per second (0 = default, -1 = disable) |
| `TASKFLOW_MAX_BODY_BYTES` | `1048576` | Maximum request body size in bytes |
| `TASKFLOW_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `TASKFLOW_WRITE_TIMEOUT` | `60s` | HTTP write timeout |
| `TASKFLOW_IDLE_TIMEOUT` | `120s` | HTTP idle timeout |

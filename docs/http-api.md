# TaskFlow HTTP API

The TaskFlow server exposes a RESTful JSON API. All endpoints (except `/health` and `/openapi.json`) require authentication via a Bearer token.

## Authentication

Include the API key in every request:

```
Authorization: Bearer <api-key>
```

API keys are SHA-256 hashed before storage. The seed admin key is written to `seed-admin-key.txt` on first server start.

## OpenAPI Spec

A machine-readable OpenAPI 3.1 spec is generated at startup from the operation definitions and served at:

```
GET /openapi.json
```

No authentication required.

## Idempotency

Mutating requests (POST, PATCH, PUT, DELETE) support idempotency keys for safe retries. Include the header:

```
Idempotency-Key: <unique-string>
```

If a request with the same key has been seen before, the server returns the cached response (same status code and body) without re-executing the operation. This prevents duplicate creates when a client retries after a timeout.

- GET requests are never cached, even with the header present.
- The cache is bounded by total memory (default 50 MB). Oldest entries are evicted when the budget is exceeded.
- Keys are scoped to the server process — they reset on restart.
- Use a UUID or similar unique string per logical operation.

Example:

```bash
# First request — creates the task
curl -X POST /boards/my-board/tasks \
  -H "Authorization: Bearer $KEY" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{"title": "Fix bug", "priority": "high"}'

# Retry with same key — returns cached 201, no duplicate task created
curl -X POST /boards/my-board/tasks \
  -H "Authorization: Bearer $KEY" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{"title": "Fix bug", "priority": "high"}'
```

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

Any field that accepts an actor name (`assignee` in task create/update, `assignee` query filter) supports the special value `@me`, which resolves to the authenticated actor's name. This avoids requiring the client to know its own identity.

```bash
# Create a task assigned to myself
curl -X POST /boards/my-board/tasks \
  -H "Authorization: Bearer $KEY" \
  -d '{"title": "My task", "priority": "none", "assignee": "@me"}'

# List my tasks
curl "/boards/my-board/tasks?assignee=@me" -H "Authorization: Bearer $KEY"
```

## Role-Based Access Control

Each operation requires a minimum role. Roles form a hierarchy: `admin > member > read_only`.

## Endpoints

### Actors

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/actors` | Create an actor | admin |
| GET | `/actors` | List all actors | read_only |
| GET | `/actors/{name}` | Get an actor by name | read_only |
| PATCH | `/actors/{name}` | Update an actor | admin |

### Boards

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards` | Create a board | member |
| GET | `/boards` | List boards | read_only |
| GET | `/boards/{slug}` | Get a board | read_only |
| PATCH | `/boards/{slug}` | Update a board | member |
| DELETE | `/boards/{slug}` | Delete a board (soft-delete) | admin |
| POST | `/boards/{slug}/reassign` | Reassign tasks to another board | admin |

Query params for list: `include_deleted` (boolean).

### Workflows

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/workflow` | Get the board's workflow definition | read_only |
| PUT | `/boards/{slug}/workflow` | Replace the board's workflow | member |
| GET | `/boards/{slug}/workflow/health` | Check workflow health | read_only |

### Tasks

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/boards/{slug}/tasks` | Create a task | member |
| GET | `/boards/{slug}/tasks` | List tasks (with filters and search) | read_only |
| GET | `/boards/{slug}/tasks/{num}` | Get a task | read_only |
| PATCH | `/boards/{slug}/tasks/{num}` | Update a task | member |
| POST | `/boards/{slug}/tasks/{num}/transition` | Transition a task to a new state | member |
| DELETE | `/boards/{slug}/tasks/{num}` | Delete a task (soft-delete) | member |

Query params for list: `state`, `assignee`, `priority`, `tag`, `q` (full-text search), `include_closed`, `include_deleted`, `sort` (created_at/updated_at/priority/due_date), `order` (asc/desc).

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

### Webhooks

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| POST | `/webhooks` | Create a webhook | admin |
| GET | `/webhooks` | List webhooks | admin |
| GET | `/webhooks/{id}` | Get a webhook | admin |
| PATCH | `/webhooks/{id}` | Update a webhook | admin |
| DELETE | `/webhooks/{id}` | Delete a webhook | admin |

### Audit

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/tasks/{num}/audit` | Get audit log for a task | read_only |
| GET | `/boards/{slug}/audit` | Get audit log for a board | read_only |

### Tags

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/tags` | List tags in use on a board with counts | read_only |

### Convenience Endpoints

These are aggregate views and utilities, not domain operations.

| Method | Path | Description | Min Role |
|--------|------|-------------|----------|
| GET | `/boards/{slug}/detail` | Complete board with all tasks, comments, attachments, dependencies, and audit | read_only |
| GET | `/admin/stats` | System-wide statistics (actor/board/task counts, activity by actor) | any |
| GET | `/search?q=<query>` | Cross-board full-text search (supports `&state=`, `&assignee=`, `&priority=` filters) | any |
| POST | `/batch` | Execute multiple operations in a single request (max 50) | any |

### Batch Operations

`POST /batch` accepts an array of operations and executes them sequentially. Each operation is replayed through the router with full middleware (auth inherited, RBAC, idempotency). Partial failures don't stop execution — each result has its own status code.

```json
{
  "operations": [
    {"method": "POST", "path": "/boards/my-board/tasks", "body": {"title": "Task 1", "priority": "high"}},
    {"method": "POST", "path": "/boards/my-board/tasks", "body": {"title": "Task 2", "priority": "none"}},
    {"method": "GET", "path": "/boards/my-board/tasks"}
  ]
}
```

Response:

```json
{
  "results": [
    {"status": 201, "body": {"board_slug": "my-board", "num": 1, ...}},
    {"status": 201, "body": {"board_slug": "my-board", "num": 2, ...}},
    {"status": 200, "body": [{...}, {...}]}
  ]
}
```

Each operation can include an optional `idempotency_key` field for per-operation idempotency.

## Server Configuration

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `TASKFLOW_DB_PATH` | `./taskflow.db` | SQLite database path |
| `TASKFLOW_LISTEN_ADDR` | `:8374` | Listen address |
| `TASKFLOW_SEED_ADMIN_NAME` | — | Name for the seed admin actor (created on first start) |
| `TASKFLOW_SEED_ADMIN_DISPLAY_NAME` | same as name | Display name for the seed admin |
| `TASKFLOW_SEED_KEY_FILE` | `./seed-admin-key.txt` | Path to write the seed admin API key |

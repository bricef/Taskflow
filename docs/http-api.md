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

## Server Configuration

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `TASKFLOW_DB_PATH` | `./taskflow.db` | SQLite database path |
| `TASKFLOW_LISTEN_ADDR` | `:8374` | Listen address |
| `TASKFLOW_SEED_ADMIN_NAME` | — | Name for the seed admin actor (created on first start) |
| `TASKFLOW_SEED_ADMIN_DISPLAY_NAME` | same as name | Display name for the seed admin |
| `TASKFLOW_SEED_KEY_FILE` | `./seed-admin-key.txt` | Path to write the seed admin API key |

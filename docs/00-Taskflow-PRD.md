# TaskFlow — PRD

**Human/AI Collaborative Task Tracker**

*Version: 1.2*
*Date: 2026-03-18*

---

## 1. Overview

TaskFlow is a single-user task tracker designed for fluid collaboration between a human operator and multiple AI agents. It provides a durable, always-on server as the source of truth, with multiple interface surfaces: a CLI for scripting and automation, a TUI for interactive visual management, and an MCP server for AI agent integration.

The core abstraction is a **kanban board** with explicitly configured workflow state machines. Any actor — human or AI — can create, advance, review, and manage tasks. A full audit trail records every action with actor attribution and timestamps.


## 2. Goals and Non-Goals

### Goals

- **Durable, cross-machine access.** A single running server holds all state. Any client on any machine can connect with an API key.
- **First-class AI participation.** AI agents are peers, not second-class citizens. They get the same CRUD capabilities as the human operator (subject to role permissions).
- **Auditable collaboration.** Every mutation is attributed to a named actor. You can always answer "who did this and when?"
- **Configurable workflows.** Boards define explicit state machines. Transitions are declared, not implicit.
- **Multiple interfaces.** CLI for power use and scripting, TUI for interactive overview, MCP for agent tooling.

### Non-Goals (for now)

- Multi-user / multi-tenant support.
- Real-time collaboration (websockets, live cursors, etc.).
- A web UI or graphical GUI.
- Parent/child task hierarchies (epics, subtasks). Tasks are flat within a board.
- Large-scale file storage. Files are stored on the server filesystem, but TaskFlow is not a file hosting service. Practical limits apply based on disk space.


## 3. Architecture

### Operation Framework

TaskFlow is built around a **"define once, derive interfaces"** pattern. Each system capability is defined as a typed **operation** — an input struct, an execute function, and metadata (permissions, audit config, HTTP/CLI bindings). Interface surfaces (HTTP API, CLI, MCP) are automatically derived from a shared operation registry via generic adapters.

This means adding a new capability requires writing one struct and one function. The framework handles auth, RBAC, input parsing, validation, output formatting, audit logging, and error handling for all interfaces uniformly.

Edge cases (file upload/download, SSE streaming, health checks) use escape hatches: custom input parsers, custom output writers, or raw HTTP handlers registered alongside the operation registry.

### System Diagram

```
┌──────────────┐   ┌──────────────┐   ┌──────────────────────────┐
│   CLI (Go)   │   │   TUI (Go)   │   │  MCP Server (per agent)  │
│   cobra-cli  │   │  bubbletea   │   │  Go, stdio transport     │
└──────┬───────┘   └──────┬───────┘   └──────┬───────────────────┘
       │                  │                   │
       └──────────┬───────┴───────────────────┘
                  │  REST API (JSON/HTTP)
                  ▼
        ┌─────────────────────────────┐
        │      TaskFlow Server        │
        │  Operation Registry + Chi   │
        │  SQLite (WAL) + FTS5        │
        └────────────┬────────────────┘
                     │
           ┌─────────┴─────────┐
           │                   │
      ┌────▼─────┐    ┌────────▼────────┐
      │ SQLite   │    │ Webhook Dispatch │
      │ Database │    │   (async POST)   │
      └──────────┘    └─────────────────┘
```

### Technology Choices

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Server | Go + Chi router | Single binary, minimal deps, strong stdlib |
| Core pattern | Operation registry with generic adapters | Define once, derive HTTP/CLI/MCP interfaces |
| Database | SQLite (WAL mode) + FTS5 | Single-user, durable, portable, zero-ops, built-in full-text search |
| CLI | Go + Cobra (auto-generated from registry) | Same language, commands derived from operations |
| TUI | Go + Bubble Tea | Rich terminal UIs, same ecosystem |
| MCP Server | Go (stdio transport, derived from registry) | Near-zero effort to add via adapter |
| Auth | API key per actor (Bearer token) + RBAC | Simple, auditable, role-based permissions |
| Deployment | Docker container on VPS | Portable, reproducible, TLS via external Traefik |


## 4. Identification Scheme

TaskFlow uses a simple, human-friendly identification scheme throughout.

**Boards** are identified by an immutable **slug**: lowercase alphanumeric plus hyphens (e.g. `my-board`, `infra`). The slug is set at creation and cannot be changed. Boards also have an editable **name** for display purposes (e.g. "Infrastructure & DevOps").

**Tasks** are identified by `board-slug/N` where N is an auto-incrementing integer scoped to the board. Examples: `my-board/1`, `infra/45`.

- The CLI accepts `board-slug/N` anywhere a task is needed. If `default_board` is set in config, bare numbers are resolved against it (e.g. `42` → `my-board/42`).
- Cross-references in comments and descriptions use the same format: "see `infra/45`".
- The composite `(board_slug, sequence_num)` is the primary key for tasks in the database. No ULIDs or UUIDs.

**Actors** are identified by a unique **name** (e.g. `brice`, `claude-code`, `aider`).


## 5. Data Model

### 5.1 Actors

An **actor** represents any entity (human or AI agent) that can interact with the system. Actors have a **role** that determines their permissions.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string (PK) | Unique identifier (e.g. "brice", "claude-code") |
| `display_name` | string | Human-friendly label |
| `type` | enum | `human` \| `ai_agent` |
| `role` | enum | `admin` \| `member` \| `read_only` |
| `api_key_hash` | string | bcrypt hash of the actor's API key |
| `created_at` | timestamp | |
| `active` | bool | Soft-disable without deleting |

- The actor set is small. Actor management (create, deactivate, role changes) is restricted to `admin` actors.
- Every API request is authenticated via `Authorization: Bearer <api_key>`.

**Role permissions:**

| Action | `admin` | `member` | `read_only` |
|--------|---------|----------|-------------|
| Create/deactivate actors | ✅ | ❌ | ❌ |
| Change actor roles | ✅ | ❌ | ❌ |
| Manage webhooks | ✅ | ❌ | ❌ |
| Trigger backup | ✅ | ❌ | ❌ |
| Delete/reassign boards | ✅ | ❌ | ❌ |
| Create boards | ✅ | ✅ | ❌ |
| Update board name/workflow | ✅ | ✅ | ❌ |
| Create/update/transition/delete tasks | ✅ | ✅ | ❌ |
| Add comments, dependencies, attachments | ✅ | ✅ | ❌ |
| Upload/delete files | ✅ | ✅ | ❌ |
| List/query boards, tasks, comments, audit | ✅ | ✅ | ✅ |
| Read reports | ✅ | ✅ | ✅ |
| Download files | ✅ | ✅ | ✅ |

The permission model is enforced in the operation framework's RBAC layer. Requests that exceed the actor's role receive a `403 Forbidden` response with a clear error message.

### 5.2 Boards

A **board** is a named container of tasks with an associated workflow definition.

| Field | Type | Description |
|-------|------|-------------|
| `slug` | string (PK) | Immutable identifier, used in task IDs |
| `name` | string | Editable display name |
| `description` | string | Optional |
| `workflow` | JSON (Workflow) | The state machine definition |
| `next_task_num` | integer | Auto-increment counter for task IDs |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |
| `deleted` | bool | Soft-delete flag, default false |

Slug validation: `^[a-z0-9][a-z0-9-]*[a-z0-9]$`, max 32 characters.

Boards support soft deletion. Before deleting a board, open tasks can optionally be reassigned to another board (they receive new task numbers on the target board). Soft-deleted boards and their tasks are excluded from default queries but remain in the database and audit trail. The slug of a deleted board is not reclaimed.

### 5.3 Workflow Definition (embedded JSON)

A workflow is a directed graph of states with named transitions.

```json
{
  "states": ["backlog", "ready", "in_progress", "review", "done", "cancelled"],
  "initial_state": "backlog",
  "terminal_states": ["done", "cancelled"],
  "transitions": [
    { "from": "backlog",     "to": "ready",       "name": "triage" },
    { "from": "ready",       "to": "in_progress", "name": "start" },
    { "from": "in_progress", "to": "review",      "name": "submit" },
    { "from": "review",      "to": "in_progress", "name": "reject" },
    { "from": "review",      "to": "done",        "name": "approve" }
  ],
  "from_all": [
    { "to": "cancelled", "name": "cancel" }
  ],
  "to_all": [
    { "from": "backlog", "name": "escalate" }
  ]
}
```

- `from_all` expands to: every state (except the target and terminal states) → target.
- `to_all` expands to: source → every state (except the source).
- Transitions are **enforced** — attempting an invalid transition returns an error.
- Workflows can be edited in-place on a board. The API exposes a "workflow health" endpoint that reports tasks in states that no longer exist or have no outbound transitions. No explicit workflow versioning; the audit log records workflow changes.

**Default workflow template.** The server ships with a built-in default workflow, usable via `--default-workflow` on board creation:

```
backlog → in_progress → review → done
                ↑          │
                └──────────┘ (reject)
           fromAll → cancelled
```

States: `backlog`, `in_progress`, `review`, `done`, `cancelled`. Terminal states: `done`, `cancelled`. The `cancelled` state is reachable from all non-terminal states via `fromAll`.

### 5.4 Tasks

| Field | Type | Description |
|-------|------|-------------|
| `board_slug` | string (PK, FK) | Board this task belongs to |
| `num` | integer (PK) | Auto-incrementing number within board |
| `title` | string | Short summary |
| `description` | string | Markdown body, long-form |
| `state` | string | Current workflow state |
| `priority` | enum | `critical` \| `high` \| `medium` \| `low` \| `none` |
| `tags` | []string | Freeform labels |
| `assignee` | string? | FK to actor name (nullable = unassigned) |
| `due_date` | date? | Optional deadline |
| `created_by` | string | FK to actor name |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |
| `deleted` | bool | Soft-delete flag, default false |

- Tasks in terminal states (`done`, `cancelled`) and soft-deleted tasks are excluded from default queries. Both are retrievable with explicit flags (`--include-closed`, `--include-deleted`).
- Title and description are indexed with SQLite FTS5 for full-text search.
- Tags are freeform strings. No pre-defined tag sets — a `tag list` helper command shows tags currently in use across a board for discoverability.
- Time tracking (actuals) is derived from the audit log by computing time-in-state from transition timestamps. No separate `time_spent` field is needed.

### 5.5 Dependencies

Informational links between tasks. Not enforced in workflow transitions.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer (PK) | Auto-increment |
| `task_board` | string | Board slug of the task that has the dependency |
| `task_num` | integer | Task number of the task that has the dependency |
| `depends_on_board` | string | Board slug of the task it depends on |
| `depends_on_num` | integer | Task number of the task it depends on |
| `dep_type` | enum | `depends_on` \| `relates_to` |
| `created_by` | string | FK to actor |
| `created_at` | timestamp | |

Dependencies can cross boards (e.g. `web/12` depends on `infra/45`).

### 5.6 Files

Files are stored and managed by the server on the filesystem. They are first-class entities, independent of tasks — a file is uploaded once and can be attached to one or more tasks.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer (PK) | Auto-increment |
| `filename` | string | Original filename, preserved |
| `size_bytes` | integer | File size |
| `mime_type` | string | Detected MIME type |
| `created_by` | string | FK to actor |
| `created_at` | timestamp | |

Files are stored on disk at `<storage_dir>/<id>/<original_filename>`. The ID directory avoids filename collisions while preserving the original name. The storage directory is configurable (default: `/data/files/` in the container) and lives on the same volume as the database.

### 5.7 Attachments

Attachments link tasks to either **stored files** or **external references**. Each attachment is one or the other.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer (PK) | Auto-increment |
| `board_slug` | string (FK) | |
| `task_num` | integer (FK) | |
| `file_id` | integer? (FK) | FK to file (for stored file attachments) |
| `ref_type` | enum? | `url` \| `git_commit` \| `git_branch` \| `git_pr` (for external references) |
| `value` | string? | The reference value (for external references) |
| `label` | string | Human-readable label |
| `created_by` | string | FK to actor |
| `created_at` | timestamp | |

Exactly one of (`file_id`) or (`ref_type` + `value`) must be set. An attachment either points to a managed file or an external reference, never both.

### 5.8 Audit Log

Every mutation is recorded. This table is **append-only** — entries are never modified or deleted.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer (PK) | Auto-increment |
| `board_slug` | string | |
| `task_num` | integer? | Nullable for board-level events |
| `actor` | string | Who did it |
| `action` | enum | `created` \| `transitioned` \| `updated` \| `commented` \| `comment_edited` \| `assigned` \| `deleted` \| `dependency_added` \| `dependency_removed` \| `file_uploaded` \| `file_deleted` \| `attachment_added` \| `attachment_removed` \| `workflow_changed` \| `board_deleted` \| `tasks_reassigned` |
| `detail` | JSON | Action-specific payload |
| `timestamp` | timestamp | |

Example detail payloads:

```json
// transitioned
{"from": "review", "to": "done", "transition": "approve", "comment": "Looks good."}

// updated
{"fields": {"priority": {"old": "medium", "new": "high"}, "title": {"old": "Fix bug", "new": "Fix auth bug"}}}

// comment_edited
{"comment_id": 7, "old_body": "Needs rework", "new_body": "Needs rework on the auth middleware specifically"}

// workflow_changed
{"board": "my-board", "old_states": ["backlog", "done"], "new_states": ["backlog", "review", "done"]}

// tasks_reassigned
{"from_board": "old-board", "to_board": "my-board", "task_count": 5}
```

### 5.9 Comments

First-class comments on tasks, attributed to actors.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer (PK) | Auto-increment |
| `board_slug` | string (FK) | |
| `task_num` | integer (FK) | |
| `actor` | string | Who wrote it |
| `body` | string | Markdown content |
| `created_at` | timestamp | |
| `updated_at` | timestamp? | If edited |

### 5.10 Webhooks

Webhook registrations for event notifications.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer (PK) | Auto-increment |
| `url` | string | Destination URL for POST requests |
| `events` | []string | List of event types to subscribe to (see §7.1) |
| `board_slug` | string? | Optional board scope (null = all boards) |
| `secret` | string | Shared secret for HMAC-SHA256 signature |
| `active` | bool | Enable/disable without deleting |
| `created_by` | string | FK to actor |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

Webhook management (create, update, delete) is restricted to `admin` actors. Webhook dispatch (actually sending POST requests when events occur) is implemented in Phase 4; the data model and CRUD operations are available from Phase 1.


## 6. API Design

RESTful JSON API over HTTP. All endpoints require `Authorization: Bearer <api_key>`.

**Error responses** use a consistent JSON format:

```json
{
  "error": "invalid_transition",
  "message": "No transition 'approve' from state 'backlog'",
  "detail": {
    "task": "my-board/3",
    "current_state": "backlog",
    "requested_transition": "approve",
    "available_transitions": ["triage", "cancel"]
  }
}
```

HTTP status codes follow standard conventions: 400 for validation errors, 401 for auth failures, 403 for insufficient permissions, 404 for missing resources, 409 for conflicts (e.g. duplicate slug).

### 6.1 Actors

| Method | Path | Description |
|--------|------|-------------|
| POST | `/actors` | Register a new actor, returns API key (shown once). **Admin only.** |
| GET | `/actors` | List all actors |
| PATCH | `/actors/{name}` | Update actor (display name, role, active status). **Admin only.** |

### 6.2 Boards

| Method | Path | Description |
|--------|------|-------------|
| POST | `/boards` | Create board with slug, name, and workflow |
| GET | `/boards` | List boards |
| GET | `/boards/{slug}` | Get board detail including workflow |
| PATCH | `/boards/{slug}` | Update board (name, description only — slug is immutable) |
| DELETE | `/boards/{slug}` | Soft-delete board. **Admin only.** |
| POST | `/boards/{slug}/reassign` | Move open tasks to another board before deletion. **Admin only.** |
| GET | `/boards/{slug}/workflow` | Get workflow definition for a board |
| PUT | `/boards/{slug}/workflow` | Replace workflow definition |
| GET | `/boards/{slug}/workflow/health` | Check for orphaned tasks |

**Reassign request body:**

```json
{
  "target_board": "other-board",
  "include_states": ["backlog", "in_progress", "review"]
}
```

Tasks are assigned new sequential numbers on the target board. The audit trail records the reassignment on both the source and target boards. If `include_states` is omitted, all non-terminal-state tasks are reassigned.

### 6.3 Tasks

| Method | Path | Description |
|--------|------|-------------|
| POST | `/boards/{slug}/tasks` | Create task (returns assigned number) |
| GET | `/boards/{slug}/tasks` | List/filter tasks |
| GET | `/boards/{slug}/tasks/{num}` | Get task detail |
| PATCH | `/boards/{slug}/tasks/{num}` | Update task fields |
| POST | `/boards/{slug}/tasks/{num}/transition` | Execute a named transition |
| DELETE | `/boards/{slug}/tasks/{num}` | Soft-delete task |

**Query parameters for task listing:**

- `state` — filter by state(s), comma-separated
- `assignee` — filter by actor name or `unassigned`
- `priority` — filter by priority
- `tag` — filter by tag(s)
- `q` — full-text search on title + description (FTS5)
- `sort` — `created_at` \| `updated_at` \| `priority` \| `due_date`
- `order` — `asc` \| `desc`
- `include_closed` — include tasks in terminal states (default: false)
- `include_deleted` — include soft-deleted tasks (default: false)

**Transition request body:**

```json
{
  "transition": "approve",
  "comment": "Looks good, merging."
}
```

An optional comment can accompany any transition, recorded in both the audit log and as a comment.

### 6.4 Comments

| Method | Path | Description |
|--------|------|-------------|
| POST | `/boards/{slug}/tasks/{num}/comments` | Add comment |
| GET | `/boards/{slug}/tasks/{num}/comments` | List comments |
| PATCH | `/comments/{id}` | Edit comment |

### 6.5 Dependencies

| Method | Path | Description |
|--------|------|-------------|
| POST | `/boards/{slug}/tasks/{num}/dependencies` | Add dependency |
| GET | `/boards/{slug}/tasks/{num}/dependencies` | List dependencies |
| DELETE | `/dependencies/{id}` | Remove dependency |

**Dependency request body:**

```json
{
  "depends_on": "infra/45",
  "type": "depends_on"
}
```

The `depends_on` field accepts the standard `board-slug/num` format. Reading: "this task depends on `infra/45`".

### 6.6 Files

| Method | Path | Description |
|--------|------|-------------|
| PUT | `/files` | Upload a file (multipart form data), returns file metadata with ID |
| GET | `/files/{id}` | Download a file |
| GET | `/files/{id}/meta` | Get file metadata without downloading |
| DELETE | `/files/{id}` | Delete a stored file (fails if still attached to tasks) |

**Upload response:**

```json
{
  "id": 17,
  "filename": "report.pdf",
  "size_bytes": 204800,
  "mime_type": "application/pdf",
  "created_by": "claude-code",
  "created_at": "2026-03-17T14:32:00Z"
}
```

### 6.7 Attachments

| Method | Path | Description |
|--------|------|-------------|
| POST | `/boards/{slug}/tasks/{num}/attachments` | Attach a file or external reference |
| GET | `/boards/{slug}/tasks/{num}/attachments` | List attachments |
| DELETE | `/attachments/{id}` | Remove attachment (does not delete the underlying file) |

**Attach a stored file:**

```json
{
  "file_id": 17,
  "label": "Final report"
}
```

**Attach an external reference:**

```json
{
  "ref_type": "git_branch",
  "value": "feature/auth",
  "label": "Working branch"
}
```

### 6.8 Audit Log

| Method | Path | Description |
|--------|------|-------------|
| GET | `/boards/{slug}/tasks/{num}/audit` | Audit log for a task |
| GET | `/boards/{slug}/audit` | Audit log for a board |

### 6.9 Events (SSE)

Server-Sent Events endpoint for real-time updates. One stream per board.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/boards/{slug}/events` | SSE stream of board events |

The event stream emits the same event types used by webhooks (§7.1). Each SSE message has the event type as the SSE `event` field and the JSON payload as `data`.

**Example SSE stream:**

```
event: task.transitioned
data: {"task":{"ref":"my-board/7","title":"Refactor auth","state":"review","previous_state":"in_progress"},"actor":{"name":"claude-code","type":"ai_agent"},"detail":{"transition":"submit","comment":"Ready for review."}}

event: task.commented
data: {"task":{"ref":"my-board/3","title":"Fix auth bug"},"actor":{"name":"brice","type":"human"},"detail":{"body":"Looks like this also affects the session handler."}}
```

Clients must authenticate via `Authorization: Bearer <api_key>` header (or `?token=<api_key>` query parameter for browser-based clients). The server sends a heartbeat comment (`:ping`) every 30 seconds to keep the connection alive.

### 6.10 Webhooks

| Method | Path | Description |
|--------|------|-------------|
| POST | `/webhooks` | Register webhook. **Admin only.** |
| GET | `/webhooks` | List webhooks. **Admin only.** |
| PATCH | `/webhooks/{id}` | Update webhook. **Admin only.** |
| DELETE | `/webhooks/{id}` | Remove webhook. **Admin only.** |

**Webhook registration body:**

```json
{
  "url": "https://ntfy.sh/my-taskflow",
  "events": ["task.transitioned", "task.created", "task.commented"],
  "board_slug": null,
  "secret": "my-hmac-secret",
  "active": true
}
```

If `board_slug` is null, the webhook fires for all boards.

### 6.11 Admin

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/backup` | Trigger a SQLite backup to configured path. **Admin only.** |

### 6.12 Reports

All report endpoints derive data from the audit log. No additional storage is required.

**Board-scoped reports:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/boards/{slug}/reports/cfd` | Cumulative flow diagram data |
| GET | `/boards/{slug}/reports/cycle-time` | Cycle time per completed task |
| GET | `/boards/{slug}/reports/throughput` | Task completion rate over time |
| GET | `/boards/{slug}/reports/actor-activity` | Activity breakdown per actor |

**System-wide reports:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/reports/throughput` | Task completion rate across all boards |
| GET | `/reports/cycle-time` | Cycle time across all boards |
| GET | `/reports/actor-activity` | Activity breakdown per actor across all boards |
| GET | `/reports/board-summary` | Overview of all boards: open task counts, recent activity, health |

System-wide CFDs are not supported because boards have different workflow states. The system-wide reports instead focus on metrics that are comparable across boards: throughput (tasks reaching any terminal state), cycle time (time from first non-initial state to terminal state), and actor activity (transitions, comments, uploads per actor).

**Common query parameters:**

- `from` — start date (ISO date, default: 30 days ago)
- `to` — end date (ISO date, default: today)
- `bucket` — time granularity: `day` \| `week` \| `month` (default: `day`)

**CFD response example:**

```json
{
  "board": "my-board",
  "from": "2026-02-15",
  "to": "2026-03-17",
  "bucket": "day",
  "states": ["backlog", "in_progress", "review", "done", "cancelled"],
  "series": [
    {
      "date": "2026-02-15",
      "counts": {"backlog": 5, "in_progress": 2, "review": 1, "done": 0, "cancelled": 0}
    },
    {
      "date": "2026-02-16",
      "counts": {"backlog": 4, "in_progress": 3, "review": 1, "done": 1, "cancelled": 0}
    }
  ]
}
```

**Cycle time response example:**

```json
{
  "board": "my-board",
  "from": "2026-02-15",
  "to": "2026-03-17",
  "tasks": [
    {
      "ref": "my-board/3",
      "title": "Fix auth bug",
      "started_at": "2026-02-20T10:00:00Z",
      "completed_at": "2026-02-22T14:30:00Z",
      "cycle_time_hours": 52.5,
      "state_durations": {
        "in_progress": 36.0,
        "review": 16.5
      }
    }
  ],
  "summary": {
    "mean_hours": 48.2,
    "median_hours": 42.0,
    "p85_hours": 72.0,
    "count": 12
  }
}
```

**Throughput response example:**

```json
{
  "from": "2026-02-15",
  "to": "2026-03-17",
  "bucket": "week",
  "series": [
    {"period": "2026-02-10", "completed": 3},
    {"period": "2026-02-17", "completed": 5},
    {"period": "2026-02-24", "completed": 4}
  ]
}
```

**Actor activity response example:**

```json
{
  "from": "2026-02-15",
  "to": "2026-03-17",
  "actors": [
    {
      "name": "claude-code",
      "type": "ai_agent",
      "transitions": 24,
      "comments": 18,
      "tasks_created": 5,
      "files_uploaded": 3
    },
    {
      "name": "brice",
      "type": "human",
      "transitions": 31,
      "comments": 12,
      "tasks_created": 15,
      "files_uploaded": 1
    }
  ]
}
```

**Board summary response example (system-wide):**

```json
{
  "boards": [
    {
      "slug": "my-board",
      "name": "My Board",
      "open_tasks": 8,
      "tasks_completed_last_7d": 3,
      "mean_cycle_time_hours": 48.2,
      "oldest_open_task": {"ref": "my-board/2", "title": "Legacy cleanup", "age_days": 14}
    }
  ]
}
```

### 6.13 Tags

| Method | Path | Description |
|--------|------|-------------|
| GET | `/boards/{slug}/tags` | List all tags in use on a board with counts |

### 6.14 Dashboard

The server serves a minimal built-in HTML dashboard at `/dashboard` for visualizing report data.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/dashboard` | HTML page — system-wide reporting dashboard |
| GET | `/dashboard?board={slug}` | Dashboard filtered to a specific board |

The dashboard is a single static HTML file with Chart.js, served directly by the server. The HTML page itself is served **without authentication**. The page's JavaScript fetches data from the reporting API endpoints (§6.12), which **do require authentication** — the API key is passed via query parameter or stored in a browser cookie. Features:

- **Board view:** CFD chart, cycle time histogram, throughput bar chart, actor activity breakdown.
- **System view (no board param):** cross-board throughput, cycle time comparison, actor activity, board summary table.
- Board selector dropdown for switching between views.
- Date range picker and bucket granularity controls.

The dashboard is a convenience surface, not the primary interface. All data is available via the reporting API for custom tooling or export.


## 7. Webhook System

Webhooks are the notification primitive. The server dispatches HTTP POST requests to registered URLs when events occur.

### 7.1 Events

- `task.created`
- `task.updated`
- `task.transitioned`
- `task.commented`
- `task.assigned`
- `task.deleted`
- `dependency.added`
- `dependency.removed`
- `attachment.added`
- `attachment.removed`

### 7.2 Payload Format

```json
{
  "event": "task.transitioned",
  "timestamp": "2026-03-17T14:32:00Z",
  "actor": {
    "name": "claude-code",
    "type": "ai_agent"
  },
  "board": {
    "slug": "my-board",
    "name": "My Board"
  },
  "task": {
    "ref": "my-board/7",
    "title": "Refactor auth module",
    "state": "review",
    "previous_state": "in_progress"
  },
  "detail": {
    "transition": "submit",
    "comment": "Refactored to use middleware pattern. Ready for review."
  }
}
```

### 7.3 Delivery

- Async dispatch (non-blocking to the API response).
- Retry on failure: 3 attempts with exponential backoff (5s, 30s, 180s).
- Webhook delivery log accessible via API for debugging.
- HMAC-SHA256 signature header (`X-TaskFlow-Signature`) using the per-webhook secret.
- Configurable per-board or global scope.


## 8. Interface Surfaces

### 8.1 CLI

The CLI is the primary human interface for scripting and quick interactions. Built with Cobra, with commands auto-generated from the operation registry.

**Command structure:**

```
taskflow actor create <n> --type=human|ai_agent --role=admin|member|read_only [--display-name="..."]
taskflow actor list
taskflow actor update <n> [--display-name="..."] [--role=admin|member|read_only]
taskflow actor deactivate <n>

taskflow board create <slug> --name="..." --workflow=<file.json>
taskflow board create <slug> --name="..." --default-workflow
taskflow board list
taskflow board show <slug>
taskflow board rename <slug> --name="New Name"
taskflow board delete <slug>
taskflow board reassign <slug> --to=<target_slug> [--states=backlog,in_progress,...]
taskflow board workflow show <slug>
taskflow board workflow set <slug> <file.json>
taskflow board workflow health <slug>

taskflow task create <board> <title> [--priority=...] [--assignee=...] [--tags=...] [--due=...] [--desc="..."]
taskflow task list [board] [--state=...] [--assignee=...] [--tag=...] [--priority=...] [--include-closed] [--include-deleted] [--search=...]
taskflow task show <board/num>
taskflow task update <board/num> [--title=...] [--priority=...] [--assignee=...] [--tags=...] [--due=...]
taskflow task transition <board/num> <transition_name> [--comment="..."]
taskflow task delete <board/num>

taskflow comment add <board/num> <body>
taskflow comment list <board/num>

taskflow dep add <board/num> --on <target_board/num> [--type=depends_on|relates_to]
taskflow dep list <board/num>
taskflow dep remove <dep_id>

taskflow file upload <local_path> [--label=...]
taskflow file list
taskflow file delete <file_id>

taskflow attach file <board/num> <file_id> [--label=...]
taskflow attach ref <board/num> --type=url|git_commit|git_branch|git_pr --value=<ref> [--label=...]
taskflow attach list <board/num>
taskflow attach remove <attachment_id>

taskflow tag list [board]

taskflow webhook create --url=<url> --events=<events> [--board=<slug>] [--secret=<secret>]
taskflow webhook list
taskflow webhook update <id> [--url=...] [--events=...] [--board=...] [--active=true|false]
taskflow webhook delete <id>

taskflow audit <board/num>
taskflow audit --board=<slug>

taskflow report cfd <board> [--from=...] [--to=...] [--bucket=day|week|month]
taskflow report cycle-time [board] [--from=...] [--to=...]
taskflow report throughput [board] [--from=...] [--to=...] [--bucket=day|week|month]
taskflow report actor-activity [board] [--from=...] [--to=...]
taskflow report board-summary

taskflow admin backup
```

Note: `taskflow task search` from earlier drafts has been replaced by the `--search` flag on `taskflow task list`, since search is a filter parameter on the list endpoint rather than a separate operation.

**Configuration** stored in `~/.config/taskflow/config.toml`:

```toml
server_url = "https://my-server:8374"
api_key = "tf_..."
default_board = "my-board"
```

### 8.2 TUI

An interactive terminal UI built with Bubble Tea. Two primary views, switchable with `Tab`.

**Board View** — Visual kanban columns:

```
┌─ Backlog ──────┬─ In Progress ──┬─ Review ────────┬─ Done ──────────┐
│                │                │                 │                 │
│ [M] #3 Fix     │ [H] #5 Refact  │ [C] #7 Add      │ [L] #1 Update   │
│     @brice     │     @claude    │     @aider      │     @brice      │
│     #api       │     #api       │     #api        │     #docs       │
│                │                │                 │                 │
│ [L] #4 Plan v2 │                │                 │                 │
│     —          │                │                 │                 │
└────────────────┴────────────────┴─────────────────┴─────────────────┘
```

- Priority shown as `[C]ritical [H]igh [M]edium [L]ow`.
- Navigate with arrow keys / vim keys (`h j k l`).
- `Enter` to open task detail pane. `t` to transition. `a` to assign. `c` to comment.
- Filter bar at top for state, assignee, tag, priority.
- Tasks in terminal states hidden by default; toggle with `d` (done).

**List View** — Filterable, sortable table:

```
 #     State        Priority  Title              Assignee      Tags       Due
 5     in_progress  high      Refactor auth      @claude       api        2026-03-20
 3     backlog      medium    Fix auth bug       @brice        api        —
 7     review       critical  Add tests          @aider        api        —
```

- Sort by selecting column headers.
- Same filter bar as board view.

**Task Detail Pane** — Shown on `Enter` from either view:

- Full title, description (rendered markdown), metadata.
- Comments in chronological order.
- Dependencies and attachments listed.
- Recent audit log entries.
- Inline actions: transition, assign, comment, edit.

**Live updates:** The TUI subscribes to the board's SSE stream (`/boards/{slug}/events`) for real-time updates. When an event arrives (e.g. an AI agent transitions a task), the board view updates immediately without manual refresh. The TUI falls back to polling (configurable interval, default 30s) if the SSE connection drops, and automatically reconnects.

### 8.3 MCP Server

A thin Go binary exposing TaskFlow operations as MCP tools over stdio transport. **One instance per AI agent**, each configured with that agent's API key for clean actor attribution. Tools are auto-generated from the same operation registry as the HTTP API and CLI.

**Configuration** (environment variables or config file):

```
TASKFLOW_SERVER_URL=https://my-server:8374
TASKFLOW_API_KEY=tf_...
```

**Tools exposed:**

| Tool | Description |
|------|-------------|
| `list_boards` | List all boards with summary |
| `get_board` | Get board details and workflow definition |
| `create_board` | Create a new board with workflow |
| `update_board` | Update board name/description |
| `delete_board` | Soft-delete a board. **Admin only.** |
| `reassign_tasks` | Move tasks from one board to another. **Admin only.** |
| `get_workflow` | Get workflow definition for a board |
| `update_workflow` | Replace a board's workflow |
| `check_workflow_health` | Check for orphaned tasks |
| `list_tasks` | Query tasks with filters (state, assignee, tag, priority, search) |
| `get_task` | Get full task detail including comments, deps, attachments, recent audit |
| `create_task` | Create a task on a board |
| `update_task` | Update task fields |
| `transition_task` | Execute a workflow transition with optional comment |
| `delete_task` | Soft-delete a task |
| `add_comment` | Add a comment to a task |
| `add_dependency` | Link two tasks |
| `remove_dependency` | Remove a dependency |
| `upload_file` | Upload a file to the server, returns file ID |
| `download_file` | Download a stored file |
| `attach_file` | Attach a stored file to a task |
| `attach_reference` | Attach an external reference (URL, git ref) to a task |
| `list_attachments` | List attachments on a task |
| `remove_attachment` | Remove an attachment from a task |
| `get_audit_log` | Retrieve audit trail for a task or board |
| `list_tags` | List tags in use on a board |
| `get_cfd` | Cumulative flow diagram data for a board |
| `get_cycle_time` | Cycle time report (board-scoped or system-wide) |
| `get_throughput` | Throughput report (board-scoped or system-wide) |
| `get_actor_activity` | Actor activity report (board-scoped or system-wide) |
| `get_board_summary` | System-wide board overview |
| `create_webhook` | Register a webhook. **Admin only.** |
| `list_webhooks` | List registered webhooks. **Admin only.** |
| `update_webhook` | Update a webhook. **Admin only.** |
| `delete_webhook` | Remove a webhook. **Admin only.** |

**Example agent MCP config (Claude Code):**

```json
{
  "mcpServers": {
    "taskflow": {
      "command": "taskflow-mcp",
      "env": {
        "TASKFLOW_SERVER_URL": "https://my-server:8374",
        "TASKFLOW_API_KEY": "tf_claude_code_key_here"
      }
    }
  }
}
```

**Notification hints:**

The MCP server subscribes to the SSE event stream in the background. When events arrive from *other* actors (not the agent using this MCP instance), they are queued as pending notifications. On the agent's next tool call, any pending notifications are included in the response as a `_notifications` array:

```json
{
  "tasks": ["..."],
  "_notifications": [
    {
      "event": "task.transitioned",
      "summary": "brice moved my-board/7 to 'review'",
      "timestamp": "2026-03-17T14:32:00Z"
    },
    {
      "event": "task.commented",
      "summary": "brice commented on my-board/3: 'Looks like this also affects the session handler.'",
      "timestamp": "2026-03-17T14:33:15Z"
    }
  ]
}
```

This piggybacks change awareness onto the agent's natural tool-call rhythm. The agent sees what changed since its last interaction and can choose whether to act on it. Notifications are cleared after being delivered. If the queue exceeds 50 entries, older notifications are summarized (e.g. "12 additional events on my-board").

The MCP server silently reconnects if the SSE stream drops. If SSE is unavailable, no notifications are included — tool responses work normally without them.


## 9. Deployment

### 9.1 Container

The server is packaged as a Docker image.

```dockerfile
FROM golang:1.22-alpine AS builder
# ... build steps ...

FROM alpine:3.19
COPY --from=builder /app/taskflow-server /usr/local/bin/
VOLUME /data
EXPOSE 8374
ENTRYPOINT ["taskflow-server", "--db=/data/taskflow.db"]
```

The SQLite database, uploaded files, and backup files all live on the mounted volume (`/data`).

### 9.2 Seed Admin Bootstrap

On first start, the server checks whether any actors exist. If none exist and a seed admin is configured, it automatically creates the admin actor and writes the API key to a file on disk.

**Configuration (environment variable overrides config file):**

| Env Variable | Config Key | Description |
|-------------|------------|-------------|
| `TASKFLOW_SEED_ADMIN_NAME` | `seed_admin.name` | Actor name for seed admin (e.g. "brice") |
| `TASKFLOW_SEED_ADMIN_DISPLAY_NAME` | `seed_admin.display_name` | Optional display name |

**Bootstrap behaviour:**

1. Server starts, checks actors table.
2. If no actors exist and seed admin is configured:
   - Creates actor with `role=admin`, `type=human`, `active=true`.
   - Generates a random API key (`tf_<random>`).
   - Writes the API key to `/data/seed-admin-key.txt`.
   - Logs: `Seed admin "brice" created. API key written to /data/seed-admin-key.txt — save this key and delete the file.`
3. If actors already exist, the seed config is ignored (idempotent — safe to leave configured).
4. If no seed admin is configured and no actors exist, the server starts but logs a warning: `No actors configured. Use TASKFLOW_SEED_ADMIN_NAME to bootstrap.`

After reading the key, the operator should save it to their CLI config (`~/.config/taskflow/config.toml`) and delete the file.

### 9.3 VPS Deployment

Recommended setup:

- Docker container running the TaskFlow server on an internal network.
- TLS termination and routing handled by an existing external Traefik instance.
- Volume mount for `/data` to persistent storage.
- The container exposes port 8374 on the Docker network only (not published to host).

Example `docker-compose.yml`:

```yaml
services:
  taskflow:
    image: taskflow-server:latest
    volumes:
      - taskflow-data:/data
    environment:
      - TASKFLOW_BACKUP_PATH=/data/backups
      - TASKFLOW_SEED_ADMIN_NAME=brice
      - TASKFLOW_SEED_ADMIN_DISPLAY_NAME=Brice
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.taskflow.rule=Host(`taskflow.example.com`)"
      - "traefik.http.routers.taskflow.entrypoints=websecure"
      - "traefik.http.routers.taskflow.tls.certresolver=letsencrypt"
      - "traefik.http.services.taskflow.loadbalancer.server.port=8374"
    networks:
      - traefik
    restart: unless-stopped

networks:
  traefik:
    external: true

volumes:
  taskflow-data:
```

### 9.4 Backup

- `POST /admin/backup` triggers a SQLite online backup to `TASKFLOW_BACKUP_PATH`.
- Recommended: cron job calling `curl -X POST https://my-server/admin/backup -H "Authorization: Bearer ..."` daily.
- Backups are timestamped files (e.g. `taskflow-2026-03-17T02:00:00.db`).
- Retention policy is external (a simple script to prune old backups).


## 10. Build Phases

### Phase 1 — Foundation

**Deliverable:** Working server + CLI. Usable for task management immediately.

- Operation framework: generic HTTP/CLI adapters, struct tag parsing, RBAC enforcement, audit integration.
- Server: SQLite schema with FTS5, seed admin bootstrap.
- All operations: actors, boards, tasks, comments, dependencies, files, attachments, webhooks (CRUD only, dispatch is Phase 4), reports, tags, admin.
- Workflow engine: state machine validation, transition enforcement, `fromAll`/`toAll` expansion.
- Audit logging on all mutations.
- Full-text search on task title and description.
- CLI: full command set from §8.1, auto-generated from operation registry.
- Configuration file support.
- Dockerfile and deployment docs.

### Phase 2 — Visibility & Live Updates

**Deliverable:** TUI for interactive management, backed by real-time SSE.

- SSE event stream endpoint (`/boards/{slug}/events`).
- Internal event bus: mutations emit events consumed by SSE and (later) webhooks.
- Board view (kanban columns).
- List view (sortable/filterable table).
- Task detail pane with comments and audit trail.
- Inline actions: transition, assign, comment, filter.
- Live updates via SSE subscription with polling fallback.

### Phase 3 — AI Integration

**Deliverable:** MCP server enabling AI agents as full task participants.

- MCP adapter for operation registry — near-zero effort given the framework.
- All tools from §8.3, auto-generated.
- Per-agent instance configuration.
- SSE subscription in MCP server for notification hints on tool responses.
- Agent setup documentation (Claude Code, Aider, etc.).
- Integration testing with at least one agent framework.

### Phase 4 — Notifications, Dashboard & Polish

**Deliverable:** Webhook dispatch, dashboard, and accumulated refinements.

- Webhook dispatch system (consuming the internal event bus from Phase 2).
- Retry logic and delivery logging.
- HMAC-SHA256 signatures.
- Built-in HTML dashboard at `/dashboard`.
- Bulk operations endpoint (`POST /boards/{slug}/tasks/bulk`).
- Webhook delivery status API.


## 11. Design Decisions Log

All open questions have been resolved. Key decisions for reference:

| Decision | Resolution |
|----------|-----------|
| Architecture | Operation framework: define operations once, derive HTTP/CLI/MCP interfaces via generic adapters. |
| Tag management | Freeform strings, no pre-defined sets. `taskflow tag list` for discoverability. |
| Time tracking | All time data derived from audit log transition timestamps. No estimate or actuals fields on tasks. |
| Default workflows | Ship a built-in default template; `--default-workflow` flag on board creation. |
| Task IDs | `board-slug/N` with auto-incrementing integers per board. No ULIDs. |
| Board slugs | Immutable once created. Display name is editable. |
| Workflow versioning | No explicit versioning. In-place edits with health check endpoint. Audit log records changes. |
| Task deletion | Soft delete. Terminal workflow states (`done`, `cancelled`) are separate from deletion. |
| Board deletion | Soft delete with optional task reassignment to another board. Slug not reclaimed. |
| Dependencies | Informational only, not enforced. Expressed as "X depends on Y". Cross-board supported. |
| Attachments | Dual model: server-managed file storage + external references (URL, git refs). |
| Pagination | Deferred. Add when needed. |
| API versioning | No `/v1/` prefix. Single-user tool; version if/when breaking changes arise. |
| Notifications | Webhooks as the primitive. No built-in channels. |
| Real-time updates | SSE per-board endpoint. TUI subscribes directly; MCP uses notification hints. |
| MCP change awareness | Notification hints piggybacked on tool responses (pending events from other actors). |
| Reporting | API endpoints deriving all metrics from audit log. No additional storage. Included in Phase 1. |
| Dashboard | Minimal built-in HTML page at `/dashboard` consuming reporting API. Phase 4. |
| System-wide reporting | Throughput, cycle time, actor activity, board summary. No system-wide CFD (different state sets). |
| MCP auth | One MCP server instance per AI agent, each with its own API key. |
| Networking | Containerised deployment on VPS. TLS via external Traefik instance. |
| Backup | Server-side backup endpoint using SQLite online backup. Scheduling is external (cron). |
| Permissions | Role-based (admin / member / read_only). Admin for actor management, webhooks, backup, board deletion. Members can create boards and do all task work. Read-only for queries and reports. |
| Bootstrap | Seed admin created on first start from env var / config file. API key written to `/data/seed-admin-key.txt`. |

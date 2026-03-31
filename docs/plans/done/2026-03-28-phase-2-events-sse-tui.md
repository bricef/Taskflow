# Phase 2: Event Bus + SSE + TUI

## Context

Phase 1 delivered a complete HTTP API and CLI. All operations are request/response — clients don't know about changes until they poll. Phase 2 adds real-time event propagation and an interactive terminal UI.

Three deliverables:
1. **Event bus** — in-process pub/sub wired into the service layer
2. **SSE endpoint** — streams board-scoped events to HTTP clients
3. **TUI** — interactive Bubble Tea application consuming the API + SSE

## How This Maps to Our Architecture

The original Phase 2 plan assumed an "operation framework" with post-Execute hooks. We don't have that — we have a service layer that owns all business logic. The event bus integrates at the service layer:

```
Service method succeeds
  → Record audit entry (existing, via repo)
  → Emit event to bus (new)
  → Return result
```

The bus is injected into `service.Service` at construction. No framework hooks needed.

## Increment 1: Event Bus

**Goal:** An in-process pub/sub with ring-buffered subscriptions. The service layer emits events after every successful mutation.

### Package: `internal/eventbus/`

```go
// event.go
type Event struct {
    Type      string    `json:"event"`
    Timestamp time.Time `json:"timestamp"`
    Actor     ActorRef  `json:"actor"`
    Board     BoardRef  `json:"board"`
    Task      *TaskRef  `json:"task,omitempty"`
    Detail    any       `json:"detail,omitempty"`
}

// Event type constants
const (
    EventTaskCreated      = "task.created"
    EventTaskUpdated      = "task.updated"
    EventTaskTransitioned = "task.transitioned"
    EventTaskDeleted      = "task.deleted"
    EventTaskAssigned     = "task.assigned"
    EventTaskCommented    = "task.commented"
    EventDependencyAdded  = "dependency.added"
    EventDependencyRemoved= "dependency.removed"
    EventAttachmentAdded  = "attachment.added"
    EventAttachmentRemoved= "attachment.removed"
)

// bus.go
type EventBus struct { ... }
func New() *EventBus
func (b *EventBus) Subscribe() *Subscription
func (b *EventBus) Publish(event Event)

type Subscription struct {
    C      <-chan Event  // buffered, 256 capacity
    cancel func()
}
func (s *Subscription) Cancel()
```

### Service Integration

Add `EventBus` to `service.Service`:

```go
type Service struct {
    store repo.Store
    bus   *eventbus.EventBus  // nil = no events (tests can omit)
}

func New(store repo.Store, opts ...Option) taskflow.TaskFlow
```

Each mutating service method emits after success:

```go
func (s *Service) TransitionTask(ctx context.Context, params model.TransitionTaskParams) (model.Task, error) {
    // ... existing logic ...

    s.emit(eventbus.Event{
        Type: eventbus.EventTaskTransitioned,
        Board: eventbus.BoardRef{Slug: params.BoardSlug},
        Task: &eventbus.TaskRef{Ref: fmt.Sprintf("%s/%d", task.BoardSlug, task.Num), ...},
        Detail: map[string]any{"from": oldState, "to": newState, "transition": params.TransitionName},
    })
    return updated, nil
}
```

The `emit` helper is nil-safe — if no bus is configured (e.g. in tests), it's a no-op.

### Tests

- Bus core: publish/subscribe, multiple subscribers, cancel, ring buffer overflow
- Concurrency: -race safe for concurrent publish/subscribe/cancel
- Service integration: each mutating operation emits the correct event type
- No event on failed operations
- No event on read operations

### Estimated Effort: 1.5 dev-days

---

## Increment 2: SSE Endpoint

**Goal:** `GET /boards/{slug}/events` streams board-scoped events via Server-Sent Events.

### Handler: `internal/http/sse.go`

Registered alongside the convenience endpoints (auth required, not a domain operation):

```go
r.Get("/boards/{slug}/events", s.sseHandler)
```

The handler:
1. Authenticates (same auth middleware)
2. Validates board exists
3. Subscribes to the event bus
4. Filters events to the requested board
5. Writes SSE format: `event: <type>\ndata: <json>\n\n`
6. Sends `:ping\n` heartbeat every 30s
7. Cleans up subscription on client disconnect

Also supports `?token=<key>` query param auth for EventSource clients that can't set headers.

### Tests (using httptest)

- Connection lifecycle: auth, board validation, clean disconnect
- Event delivery: mutation on board → SSE client receives event
- Filtering: event on different board → not received
- Heartbeat: received within 30s on idle connection
- Multiple concurrent SSE clients
- Race safety

### Estimated Effort: 1 dev-day

---

## Increment 3: TUI Core + Board View

**Goal:** `taskflow-tui` binary with board selector, kanban view, and filter bar. Read-only — no mutations or live updates yet.

### Package: `internal/tui/`

```
internal/tui/
├── app.go              Root Bubble Tea model, view routing
├── client.go           HTTP API client (reuses our existing endpoints)
├── theme.go            Lipgloss styles (priority colours, borders, etc.)
├── keymap.go           Key bindings
├── views/
│   ├── selector.go     Board selector (list with fuzzy filter)
│   ├── board.go        Kanban columns
│   ├── list.go         Sortable table (Increment 4)
│   ├── detail.go       Task detail pane (Increment 4)
│   └── filter.go       Filter bar (field:value syntax)
└── components/
    ├── card.go          Task card ([H] #5 Title @actor tag)
    └── column.go        Kanban column with header + scrollable cards
```

### Binary: `cmd/taskflow-tui/main.go`

Shares config with CLI via Viper (same `~/.config/taskflow/config.yaml`).

```
taskflow-tui [--url=URL] [--api-key=KEY] [--board=SLUG]
```

### API Client

The TUI communicates exclusively via the HTTP API — no internal server imports. It needs:
- `ListBoards`, `GetBoard` (board selector)
- `ListTasks` (board/list views)
- `GetWorkflow` (column order, available transitions)
- `ListActors` (assign picker, Increment 5)
- `ListTags` (filter tab-completion)

All of these endpoints already exist.

### Board View

One column per non-terminal workflow state. Tasks as cards with priority badge, number, title, assignee. Navigation with arrow keys. Terminal states toggled with `d`.

### Filter Bar

`f` opens filter input. Syntax: `state:backlog assignee:brice priority:high tag:api`. Free text for FTS. Applied client-side to in-memory task list (FTS forwarded to server).

### Tests

- App lifecycle: start, connect, quit
- Board selector: list, navigate, select, fuzzy filter
- Board view: columns match workflow, cards in correct columns, navigation
- Filter bar: parse, apply, tab-completion

### Dependencies

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/lipgloss`
- `github.com/charmbracelet/bubbles` (viewport, textarea, list)

### Estimated Effort: 3–4 dev-days

---

## Increment 4: List View + Task Detail Pane

**Goal:** Sortable table view and task detail overlay. Completes the read-only TUI.

### List View

Tab switches between board view and list view. Sortable by clicking column headers with ←→. Default sort: priority desc, then task number asc.

### Detail Pane

Enter on a task opens a scrollable overlay showing: title, state, priority, assignee, tags, due date, description, dependencies, attachments, comments (chronological), and recent audit entries.

Uses `GET /boards/{slug}/tasks/{num}` + audit endpoint. Or the board detail endpoint for complete data.

### Tests

- View switching (Tab)
- Table sorting and navigation
- Detail pane content sections
- Detail pane open/close (Enter/Esc)

### Estimated Effort: 2–3 dev-days

---

## Increment 5: Inline Actions + Live Updates

**Goal:** Mutations from TUI + SSE-driven real-time updates. The TUI becomes fully interactive.

### Inline Actions

- `t` — transition picker (available transitions from workflow, optional comment)
- `a` — assignee picker (list of active actors)
- `c` — comment composer (multi-line input, Ctrl+D to submit)

All actions call the existing HTTP API endpoints.

### SSE Client (TUI side)

Background goroutine connects to `GET /boards/{slug}/events`, parses SSE events, feeds them into Bubble Tea as `Msg` values. The Update handler applies events to the in-memory task list.

Reconnection: exponential backoff (1s → 30s max). After 3 SSE failures, falls back to polling `ListTasks` every 30s. Continues SSE attempts in background.

Status indicator in footer: `●  live` / `◌ reconnecting...` / `⟳ polling` / `✕ disconnected`

### Tests

- Transition: picker, execute, error handling
- Assign: picker, execute, unassign
- Comment: composer, submit, cancel
- SSE: receive events, apply to state, reconnect
- Board switch: old SSE closed, new opened

### Estimated Effort: 2–3 dev-days

---

## Summary

| Increment | Delivers | Effort | Depends on |
|-----------|----------|--------|------------|
| 1. Event Bus | In-process pub/sub wired to service layer | 1.5 days | Phase 1 |
| 2. SSE Endpoint | Real-time event streaming per board | 1 day | 1 |
| 3. TUI Core + Board View | App shell, board selector, kanban, filter | 3–4 days | Phase 1 |
| 4. List View + Detail | Sortable table, task detail pane | 2–3 days | 3 |
| 5. Actions + Live Updates | Mutations from TUI, SSE updates | 2–3 days | 2, 4 |
| **Total** | **Interactive TUI with real-time updates** | **~10–12 days** | |

### Critical Path

```
Increment 1 ──→ Increment 2 ──────────────────────┐
                                                    ├──→ Increment 5
Increment 3 ──→ Increment 4 ──────────────────────┘
```

Increments 1→2 (event infrastructure) and 3→4 (TUI read-only) can be developed in parallel. They converge at Increment 5.

### Key Differences from Original Phase 2 Plan

| Aspect | Original Plan | Updated Plan |
|--------|--------------|--------------|
| Event emission | Framework post-Execute hook | Service layer `emit()` method |
| Event bus injection | Into framework at startup | Into `service.New()` via options |
| Package location | `internal/eventbus/`, `internal/sse/` | Same |
| Config format | `config.toml` | `config.yaml` (Viper, shared with CLI) |
| TUI API client | Custom client | Reuses existing HTTP API endpoints |
| Architecture refs | `internal/framework/`, `internal/ops/` | `internal/service/`, `internal/http/` |
| Auth for SSE | Custom, `?token=` query param | Same auth middleware + `?token=` fallback |

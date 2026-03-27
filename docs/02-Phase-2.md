# TaskFlow — Phase 2 Implementation Plan

**Visibility & Live Updates: Event Bus + SSE + TUI**

*Version: 1.0*
*Date: 2026-03-18*
*Ref: prd-taskflow-v1.2.md, phase1-implementation-v2.1.md*

---

## Approach

Phase 2 adds two capabilities on top of the Phase 1 foundation: **real-time event propagation** inside the server, and an **interactive terminal UI** that consumes it.

The server gains an internal event bus — an in-process pub/sub system that every mutation feeds into. An SSE endpoint subscribes to the bus and streams events to clients. This bus is the foundation for SSE now and webhook dispatch in Phase 4, so the design accommodates both consumers from the start.

The TUI is a separate binary (`taskflow-tui`) built with Bubble Tea. It communicates with the server exclusively via the HTTP API and SSE stream — the same interfaces available to any external client. It supports multi-board navigation, two primary views (kanban board and sortable list), a task detail pane, and inline mutation actions.

Every increment follows the same **test-first** discipline established in Phase 1: acceptance tests are written before implementation, and the increment is complete when all tests pass.


## Design Decisions

| Decision | Resolution | Rationale |
|----------|-----------|-----------|
| TUI entry point | Separate binary (`taskflow-tui`) | Clean separation from CLI concerns. The CLI is stateless and scriptable; the TUI is stateful and interactive. Different binary keeps each focused. Shares config file (`~/.config/taskflow/config.toml`). |
| Event bus implementation | Channel-based with ring buffer | Bounded memory per subscriber (256 events), non-blocking publish (drop oldest on slow consumer), graceful degradation. Mutation path never blocks. Maps naturally to MCP notification hints (Phase 3) which already assume bounded queuing. |
| Ring buffer size | 256 per subscriber | Generous for a single-user system. Only hit if a consumer completely stalls. Missed events on reconnect are recovered by refetching board state. |
| Multi-board TUI | Board switcher within TUI | Avoids restarting the TUI to change boards. SSE subscriptions managed per active board. Board selector accessible from any view. |
| TUI ↔ Server communication | HTTP API + SSE only | TUI is a pure external client — no internal imports from server packages beyond shared model types. Same API surface as any other client. |
| SSE reconnection | Client-side exponential backoff with polling fallback | TUI attempts SSE reconnect on disconnect (1s, 2s, 4s, 8s, max 30s). Falls back to polling (configurable, default 30s) if SSE is unavailable. Auto-resumes SSE on success. |
| View state on board switch | Fresh fetch | Switching boards fetches board state and tasks from the API. No persistent cache across boards. SSE subscription switched to new board. |
| Filter/sort persistence | In-memory, per session | Filters and sort settings are held in TUI memory. Reset on restart. No persistence to config file. |


## Architecture: Event Bus

### The Problem

Phase 1 operations are request/response — a client mutates, gets a result, and that's it. Other clients don't know anything changed until they query. Phase 2 needs three consumers to react to mutations in real-time: SSE streams (now), MCP notification hints (Phase 3), and webhook dispatch (Phase 4).

### The Solution

An **EventBus** sits inside the server process. Operations emit events after successful mutations. Subscribers register interest and receive events on a dedicated channel.

```
┌─────────────────────────────────────────────────────┐
│                  Operation Execute                   │
│  (create_task, transition_task, add_comment, ...)   │
└──────────────────────┬──────────────────────────────┘
                       │ emit(Event)
                       ▼
              ┌────────────────┐
              │   Event Bus    │
              │                │
              │  subscribers:  │
              │  []RingBuffer  │
              └───┬────┬───┬───┘
                  │    │   │
         ┌────────┘    │   └────────┐
         ▼             ▼            ▼
   ┌──────────┐  ┌──────────┐  ┌──────────┐
   │ SSE sub  │  │ MCP sub  │  │ Webhook  │
   │ (Ph. 2)  │  │ (Ph. 3)  │  │ (Ph. 4)  │
   └──────────┘  └──────────┘  └──────────┘
```

### Event Type

```go
type Event struct {
    Type      string    // "task.created", "task.transitioned", etc.
    Timestamp time.Time
    Actor     ActorRef  // {Name, Type}
    Board     BoardRef  // {Slug, Name}
    Task      *TaskRef  // {Ref, Title, State, PreviousState} — nil for board-level events
    Detail    any       // Event-specific payload, serialised as JSON
}

type ActorRef struct {
    Name string `json:"name"`
    Type string `json:"type"`
}

type BoardRef struct {
    Name string `json:"name"`
    Slug string `json:"slug"`
}

type TaskRef struct {
    Ref           string `json:"ref"`            // "my-board/7"
    Title         string `json:"title"`
    State         string `json:"state"`
    PreviousState string `json:"previous_state,omitempty"`
}
```

This matches the webhook payload format from PRD §7.2 exactly, so the same event struct serves SSE, MCP hints, and webhook dispatch without transformation.

### Ring Buffer Mechanics

```go
type Subscription struct {
    ch     chan Event    // buffered channel, size = ring buffer capacity
    cancel func()       // unsubscribe and drain
}

// Publish: non-blocking send to all subscribers.
// If a subscriber's channel is full, drop the oldest event.
func (bus *EventBus) Publish(event Event) {
    for _, sub := range bus.subscribers {
        select {
        case sub.ch <- event:
        default:
            // Buffer full — drain oldest, then send
            <-sub.ch
            sub.ch <- event
        }
    }
}
```

- Buffer capacity: 256 per subscriber.
- `Subscribe()` returns a `Subscription` with a receive channel and a cancel function.
- `cancel()` removes the subscriber from the bus and drains the channel.
- The bus is protected by a `sync.RWMutex` — `Publish` takes a read lock (concurrent publishes are fine), `Subscribe`/`cancel` take a write lock.
- Zero subscribers = zero cost. Events are discarded if nobody is listening.


### Integration With Operation Framework

Events are emitted **after** a successful operation, alongside the existing audit recording. The framework's post-Execute hook is extended:

```
Execute succeeds
  → Record audit entry (existing)
  → Emit event to bus (new)
  → Return response to client
```

The event bus is injected into the framework at server startup, alongside the store and audit recorder. Operations that don't produce events (queries, reports) simply don't emit.

A mapping function on each operation converts the operation's input + output into an `Event`. This is analogous to the existing `AuditDetail` function:

```go
type Operation[In, Out any] struct {
    // ... existing fields ...
    EmitEvent   func(actor Actor, in In, out Out) *Event  // nil = no event
}
```

Operations that produce events define this function. Operations that don't (queries) leave it nil. The framework checks for nil before emitting.


## Architecture: SSE Endpoint

The SSE endpoint is a raw HTTP handler registered alongside the operation registry (same escape-hatch pattern as the health check and dashboard from Phase 1).

```
GET /boards/{slug}/events
Authorization: Bearer <api_key>  (or ?token=<api_key>)
Accept: text/event-stream
```

**Lifecycle:**

1. Authenticate the request (same auth middleware as operations).
2. Validate the board slug exists.
3. Subscribe to the event bus.
4. Filter: only forward events matching the requested board slug.
5. Stream events as SSE, with `event:` and `data:` fields.
6. Send `:ping` heartbeat comment every 30 seconds.
7. On client disconnect: cancel subscription, clean up.

**SSE format:**

```
event: task.transitioned
data: {"event":"task.transitioned","timestamp":"2026-03-17T14:32:00Z","actor":{"name":"claude-code","type":"ai_agent"},"board":{"slug":"my-board","name":"My Board"},"task":{"ref":"my-board/7","title":"Refactor auth","state":"review","previous_state":"in_progress"},"detail":{"transition":"submit","comment":"Ready for review."}}

```

Each event is a single `data:` line with the full JSON payload. Events are separated by a blank line per the SSE spec. The `event:` field matches the event type, allowing `EventSource.addEventListener` filtering on the client side.


## Architecture: TUI

### Binary and Configuration

`taskflow-tui` is a separate binary in `cmd/taskflow-tui/`. It reads the same config file as the CLI (`~/.config/taskflow/config.toml`) and accepts the same `--server` and `--api-key` flags for overrides.

```
taskflow-tui [--server=URL] [--api-key=KEY] [--board=SLUG]
```

If `--board` is provided, the TUI opens directly to that board. Otherwise it opens to the board selector.

### Project Structure (additions to existing tree)

```
taskflow/
├── cmd/
│   ├── taskflow-server/
│   ├── taskflow/
│   └── taskflow-tui/           # TUI binary
│       └── main.go
├── internal/
│   ├── eventbus/               # Event bus (server-side)
│   │   ├── bus.go              # EventBus, Subscription, ring buffer
│   │   └── event.go            # Event type, ActorRef, BoardRef, TaskRef
│   ├── sse/                    # SSE handler (server-side)
│   │   └── handler.go          # SSE endpoint
│   ├── tui/                    # TUI application
│   │   ├── app.go              # Root Bubble Tea model, view router
│   │   ├── client.go           # HTTP API client (shared with CLI or thin wrapper)
│   │   ├── sse.go              # SSE client (connect, parse, reconnect)
│   │   ├── keymap.go           # Key bindings
│   │   ├── theme.go            # Colours, styles
│   │   ├── views/
│   │   │   ├── selector.go     # Board selector
│   │   │   ├── board.go        # Kanban board view
│   │   │   ├── list.go         # List/table view
│   │   │   ├── detail.go       # Task detail pane
│   │   │   └── filter.go       # Filter bar (shared across views)
│   │   ├── components/
│   │   │   ├── card.go         # Task card (used in board view)
│   │   │   ├── column.go       # Kanban column
│   │   │   ├── table.go        # Sortable table
│   │   │   ├── input.go        # Text input (comments, assignments)
│   │   │   └── confirm.go      # Confirmation dialog
│   │   └── actions/
│   │       ├── transition.go   # Transition picker
│   │       ├── assign.go       # Assignee picker
│   │       └── comment.go      # Comment composer
│   ├── framework/
│   ├── db/
│   ├── model/
│   ├── workflow/
│   ├── search/
│   └── ops/
```

### Application Model

The TUI has a layered model structure:

```
App (root model)
├── activeView: selector | board | list
├── BoardSelector (choose / switch boards)
├── BoardView (kanban columns)
├── ListView (sortable table)
├── DetailPane (overlay, shown on Enter from any view)
├── FilterBar (shared, docked at top)
├── ActionOverlay (transition / assign / comment dialogs)
└── SSEClient (background, feeds Msg into Bubble Tea)
```

**View switching:**

- `Tab` cycles between board view and list view (when a board is selected).
- `b` opens the board selector from any view.
- `Enter` opens the detail pane as an overlay on the current view.
- `Esc` closes overlays (detail pane, action dialogs) and returns to the underlying view.

**Data flow:**

1. Selecting a board fetches `GET /boards/{slug}` (workflow) and `GET /boards/{slug}/tasks` (tasks).
2. The TUI holds the full task list in memory and applies filters/sorts client-side.
3. SSE events arrive as Bubble Tea `Msg` values and update the in-memory task list.
4. Inline actions (`t`, `a`, `c`) open an overlay, collect input, POST to the API, and the SSE echo confirms the change.

### Board Selector

Shown on startup (unless `--board` flag used) and when pressing `b`.

```
┌─ TaskFlow ────────────────────────────────────────────┐
│                                                       │
│  Select a board:                                      │
│                                                       │
│  > my-board        My Board             8 open tasks  │
│    infra           Infrastructure       3 open tasks  │
│    docs            Documentation        12 open tasks │
│                                                       │
│  ↑↓ navigate  Enter select  q quit                    │
└───────────────────────────────────────────────────────┘
```

Fetches `GET /boards` on open. Shows slug, display name, and open task count. Filterable by typing (fuzzy match on slug/name).

### Board View (Kanban)

One column per non-terminal workflow state by default. Terminal states (`done`, `cancelled`) hidden unless toggled with `d`.

```
┌─ my-board: My Board ──── backlog ─────── Filter: (none) ──────────────┐
│                                                                        │
│ ┌─ Backlog ──────┬─ In Progress ──┬─ Review ────────┬─ Done ────────┐ │
│ │                │                │                 │  (hidden)     │ │
│ │ [M] #3 Fix     │ [H] #5 Refact  │ [C] #7 Add      │               │ │
│ │     @brice     │     @claude    │     @aider      │               │ │
│ │     api        │     api        │     api         │               │ │
│ │                │                │                 │               │ │
│ │ [L] #4 Plan v2 │                │                 │               │ │
│ │     —          │                │                 │               │ │
│ └────────────────┴────────────────┴─────────────────┴───────────────┘ │
│                                                                        │
│ ←→ columns  ↑↓ tasks  Enter detail  t transition  a assign  c comment │
│ Tab list view  b boards  f filter  d toggle done  / search  q quit    │
└────────────────────────────────────────────────────────────────────────┘
```

**Navigation:** Arrow keys or `h`/`l` move between columns. `j`/`k` move between tasks within a column. The selected task is visually highlighted.

**Task cards** show: priority badge `[C/H/M/L]`, task number, truncated title, assignee (or `—`), and first tag. Cards are ordered within columns by priority (critical first), then by task number.

**Column sizing:** Columns share terminal width equally. If the terminal is too narrow, columns scroll horizontally.

### List View

```
┌─ my-board: My Board ──── list ─────── Filter: state=backlog ──────────┐
│                                                                        │
│  #     State        Pri   Title              Assignee   Tags     Due   │
│  ─────────────────────────────────────────────────────────────────────  │
│  7     review       crit  Add tests          @aider     api      —     │
│  5     in_progress  high  Refactor auth      @claude    api      3/20  │
│> 3     backlog      med   Fix auth bug       @brice     api      —     │
│  4     backlog      low   Plan v2            —          —        —     │
│                                                                        │
│ ↑↓ navigate  Enter detail  ←→ sort column  Tab board view  f filter   │
└────────────────────────────────────────────────────────────────────────┘
```

- Sort by selecting column headers with `←`/`→` and toggling direction with `Enter` (or `s`).
- Default sort: priority descending, then task number ascending.
- Same filter bar as board view.

### Filter Bar

Activated with `f`. A modal input row at the top of the screen.

```
Filter: state:backlog,in_progress assignee:brice priority:high tag:api
```

Syntax: `field:value` pairs, space-separated. Comma-separated values for multi-match. Supports:

- `state:<states>` — comma-separated state names
- `assignee:<name>` or `assignee:unassigned`
- `priority:<priorities>` — comma-separated
- `tag:<tags>` — comma-separated
- Free text (no `field:` prefix) — full-text search via `q` parameter

The filter bar supports tab-completion for state names (from the board's workflow), actor names, and tags (from `GET /boards/{slug}/tags`).

Pressing `f` focuses the filter bar for editing. `Enter` applies. `Esc` cancels changes. Filters are applied client-side against the in-memory task list (which was fetched with no server-side filters, so the full set is available). Full-text search is the exception — it's forwarded to the server since FTS5 runs server-side.

### Task Detail Pane

Opened with `Enter` from either view. Displayed as an overlay covering roughly the right two-thirds of the screen (or full screen on narrow terminals).

```
┌─ my-board/7: Add integration tests ─────────────────────────────────┐
│                                                                      │
│ State: review (via submit)    Priority: critical    Assignee: @aider │
│ Tags: api, testing            Due: —                Created: 3/15    │
│                                                                      │
│ ── Description ──────────────────────────────────────────────────── │
│ Add integration tests for the auth middleware. Cover:                 │
│ - Token validation                                                   │
│ - Role enforcement                                                   │
│ - Deactivated actor rejection                                        │
│                                                                      │
│ ── Dependencies ─────────────────────────────────────────────────── │
│ depends_on: my-board/5 Refactor auth (in_progress)                   │
│                                                                      │
│ ── Attachments ──────────────────────────────────────────────────── │
│ [file] test-plan.md (14.2 KB)                                        │
│ [git_branch] feature/auth-tests                                      │
│                                                                      │
│ ── Comments (3) ─────────────────────────────────────────────────── │
│ brice (3/15 10:00): Should also cover session handler edge cases.    │
│ aider (3/16 14:30): Added. 12 test cases covering sessions.         │
│ brice (3/16 15:00): Looks good, transitioning to review.            │
│                                                                      │
│ ── Recent Audit ─────────────────────────────────────────────────── │
│ 3/15 09:50  brice      created                                       │
│ 3/15 10:00  brice      commented                                     │
│ 3/16 14:00  aider      transitioned: backlog → in_progress (start)   │
│ 3/16 14:30  aider      commented                                     │
│ 3/16 15:00  brice      transitioned: in_progress → review (submit)   │
│                                                                      │
│ t transition  a assign  c comment  Esc close                         │
└──────────────────────────────────────────────────────────────────────┘
```

Data is fetched via `GET /boards/{slug}/tasks/{num}` (which returns comments, dependencies, and attachments inline in Phase 1's `get_task` operation), supplemented by `GET /boards/{slug}/tasks/{num}/audit` for the audit trail.

### Inline Actions

All inline actions open as small overlays on top of the current view.

**Transition (`t`):** Shows available transitions for the selected task's current state.

```
┌─ Transition my-board/7 ──────────┐
│                                   │
│  Current state: in_progress       │
│                                   │
│  > submit    → review             │
│    cancel    → cancelled          │
│                                   │
│  Comment (optional):              │
│  ┌─────────────────────────────┐  │
│  │ Ready for review.           │  │
│  └─────────────────────────────┘  │
│                                   │
│  Enter confirm  Esc cancel        │
└───────────────────────────────────┘
```

Fetches available transitions from the workflow (already in memory from the board fetch). POSTs to `/boards/{slug}/tasks/{num}/transition`.

**Assign (`a`):** Shows a list of active actors.

```
┌─ Assign my-board/7 ──────────────┐
│                                   │
│  > brice       (human)            │
│    claude-code (ai_agent)         │
│    aider       (ai_agent)         │
│    — unassign                     │
│                                   │
│  Enter confirm  Esc cancel        │
└───────────────────────────────────┘
```

Fetches actor list from `GET /actors`. PATCHes to `/boards/{slug}/tasks/{num}`.

**Comment (`c`):** Multi-line text input.

```
┌─ Comment on my-board/7 ──────────┐
│                                   │
│  ┌─────────────────────────────┐  │
│  │ Looks good. One small nit   │  │
│  │ on the error handling in    │  │
│  │ test case 7.                │  │
│  └─────────────────────────────┘  │
│                                   │
│  Ctrl+D submit  Esc cancel        │
└───────────────────────────────────┘
```

POSTs to `/boards/{slug}/tasks/{num}/comments`.

### SSE Client (TUI Side)

A background goroutine manages the SSE connection and feeds events into the Bubble Tea event loop as `Msg` values.

**Lifecycle:**

1. On board select: connect to `GET /boards/{slug}/events?token=<api_key>`.
2. Parse incoming SSE lines into `Event` structs.
3. Send each event into the Bubble Tea program via `Program.Send(EventMsg{...})`.
4. On disconnect: retry with exponential backoff (1s, 2s, 4s, 8s, max 30s).
5. After 3 failed SSE attempts: fall back to polling `GET /boards/{slug}/tasks` every 30s (configurable).
6. Continue attempting SSE reconnection in background. On success, stop polling.
7. On board switch: close old connection, open new one.

**Applying events:** The TUI's `Update` method handles `EventMsg`:

- `task.created` → add task to in-memory list, re-render.
- `task.updated` → update fields on matching task.
- `task.transitioned` → update state on matching task, move card between columns.
- `task.deleted` → remove task from in-memory list (or mark deleted).
- `task.commented` → increment comment count badge; if detail pane is open for that task, append comment.
- `task.assigned` → update assignee on matching task.
- `dependency.*`, `attachment.*` → if detail pane is open for that task, refresh detail.

Events originating from the TUI's own actions (same actor) are still applied — the server is the source of truth, and the SSE echo confirms the mutation succeeded.


## Increment 1: Internal Event Bus

**Goal:** An in-process pub/sub system with ring-buffered subscriptions, wired into the operation framework so that every mutation emits a typed event.

### 1.1 Acceptance Tests

```
=== EventBus Core ===
TEST: publish with no subscribers → no error, no panic
TEST: subscribe → returns Subscription with receive channel
TEST: publish event → subscriber receives event on channel
TEST: publish event → multiple subscribers each receive a copy
TEST: cancel subscription → subscriber removed, channel drained
TEST: cancel subscription → subsequent publishes don't send to cancelled sub
TEST: double cancel → no panic

=== Ring Buffer Behaviour ===
TEST: publish 256 events to a non-consuming subscriber → channel full, no block
TEST: publish 257th event → oldest event dropped, newest delivered
TEST: slow consumer catches up → receives most recent 256 events in order
TEST: fast consumer → receives all events in order, no drops

=== Concurrency ===
TEST: concurrent publishes from multiple goroutines → no race (run with -race)
TEST: concurrent subscribe/cancel during publish → no race
TEST: publish does not block even with slow subscribers

=== Event Types ===
TEST: Event struct serialises to JSON matching PRD §7.2 payload format
TEST: all 10 event types from PRD §7.1 have corresponding constants
TEST: Event with nil Task (board-level event) → serialises without task field

=== Framework Integration ===
TEST: operation with EmitEvent defined → event emitted after successful Execute
TEST: operation with EmitEvent defined, Execute fails → no event emitted
TEST: operation with EmitEvent=nil (query) → no event emitted
TEST: emitted event has correct actor, board, timestamp from operation context
TEST: audit entry AND event both recorded on successful mutation
TEST: create_task → emits task.created with correct task ref and state
TEST: transition_task → emits task.transitioned with from/to states and transition name
TEST: update_task → emits task.updated with changed fields
TEST: add_comment → emits task.commented with comment body
TEST: delete_task → emits task.deleted
TEST: transition_task with comment → emits task.transitioned (comment in detail)
TEST: update_task assignee change → emits task.assigned (not task.updated)
TEST: add_dependency → emits dependency.added
TEST: remove_dependency → emits dependency.removed
TEST: attach_file → emits attachment.added
TEST: attach_reference → emits attachment.added
TEST: remove_attachment → emits attachment.removed
```

### 1.2 Implementation

- Implement `internal/eventbus/event.go` — Event type, ref types (ActorRef, BoardRef, TaskRef), event type constants for all 10 PRD event types.
- Implement `internal/eventbus/bus.go` — EventBus struct, Subscribe/Publish/cancel, ring buffer logic with `sync.RWMutex`.
- Extend `internal/framework/operation.go` — add `EmitEvent` field to Operation type.
- Extend framework post-Execute hook — after audit recording, check for EmitEvent, build event, publish to bus.
- Inject EventBus into server startup alongside Store and audit Recorder.
- Add `EmitEvent` functions to all mutating operations in `internal/ops/` (actors, boards, tasks, comments, dependencies, files, attachments). Non-mutating operations (queries, reports, list/get) leave EmitEvent nil.
- Special case: `update_task` where the assignee field changes emits `task.assigned` instead of `task.updated`. If both assignee and other fields change in a single PATCH, emit both events.

### 1.3 Estimated Effort: 1.5–2 dev-days

### 1.4 Done When

All acceptance tests pass. **You can now** subscribe to the event bus in Go code and receive typed events for every mutation, with correct payloads matching the PRD webhook format.

---

## Increment 2: SSE Endpoint

**Goal:** A streaming HTTP endpoint that subscribes to the event bus and delivers events as Server-Sent Events, scoped per board.

### 2.1 Acceptance Tests

```
=== SSE Connection ===
TEST: GET /boards/{slug}/events with Accept: text/event-stream → 200, Content-Type: text/event-stream
TEST: request with valid Bearer token → connection established
TEST: request with ?token=<key> query param → connection established
TEST: request with no auth → 401
TEST: request with invalid key → 401
TEST: request with deactivated actor → 403
TEST: request for non-existent board → 404
TEST: response includes Cache-Control: no-cache header
TEST: response includes Connection: keep-alive header

=== Event Streaming ===
TEST: create task on board → SSE client receives task.created event
TEST: transition task on board → SSE client receives task.transitioned event
TEST: update task on board → SSE client receives task.updated event
TEST: add comment on board → SSE client receives task.commented event
TEST: event on different board → SSE client does NOT receive it
TEST: SSE event format: "event: <type>\ndata: <json>\n\n"
TEST: SSE data field is valid JSON matching PRD §7.2 format
TEST: multiple SSE clients on same board → all receive the event

=== Heartbeat ===
TEST: idle connection receives ":ping\n" comment within 30 seconds
TEST: heartbeat does not interfere with event delivery
TEST: heartbeat resets after each real event

=== Connection Lifecycle ===
TEST: client disconnects → subscription cleaned up (no goroutine leak)
TEST: server shutdown → SSE connections closed gracefully
TEST: rapid connect/disconnect → no race conditions (run with -race)
TEST: many concurrent SSE connections (50+) → all receive events, no deadlock

=== Filtering ===
TEST: event for board "my-board" → only delivered to /boards/my-board/events subscribers
TEST: board-level events (workflow_changed) → delivered to board subscribers
```

### 2.2 Implementation

- Implement `internal/sse/handler.go` — raw HTTP handler function.
  - Auth: reuse framework's auth middleware (extract actor from Bearer token or `?token=` query param).
  - Subscribe to EventBus on connection open.
  - Filter loop: read from subscription channel, skip events where `event.Board.Slug != requestedSlug`.
  - Write SSE-formatted lines to `http.ResponseWriter`, flush after each event.
  - Heartbeat: 30-second ticker, write `:ping\n` comment.
  - Context cancellation: select on `r.Context().Done()` to detect client disconnect.
  - Cancel subscription on disconnect.
- Register handler in `cmd/taskflow-server/main.go` as a raw Chi route: `r.Get("/boards/{slug}/events", sseHandler)`.
- Set response headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`.
- Use `http.Flusher` interface to flush after each write.

### 2.3 Estimated Effort: 1–1.5 dev-days

### 2.4 Done When

All acceptance tests pass. **You can now** open an SSE connection to a board and receive real-time events for every mutation on that board.

---

## Increment 3: TUI Core + Board View

**Goal:** The Bubble Tea application shell with board selector, kanban board view, filter bar, and shared HTTP API client. After this increment, you can browse boards and tasks visually.

### 3.1 Acceptance Tests

```
=== Application Shell ===
TEST: taskflow-tui starts → connects to configured server
TEST: taskflow-tui with --server and --api-key flags → overrides config
TEST: taskflow-tui with invalid server URL → error message and exit
TEST: taskflow-tui with invalid API key → error message and exit
TEST: q from any top-level view → exits cleanly
TEST: Ctrl+C → exits cleanly

=== Board Selector ===
TEST: startup without --board flag → shows board selector
TEST: board selector lists all boards with slug, name, open task count
TEST: board selector excludes deleted boards
TEST: ↑↓ (or j/k) navigates board list
TEST: Enter on board → switches to board view, fetches tasks
TEST: typing text → filters boards by fuzzy match on slug/name
TEST: Esc clears filter text
TEST: b from board/list view → returns to board selector
TEST: board selector with --board flag → skips selector, opens board directly
TEST: empty board list → shows message "No boards found"

=== Board View (Kanban) ===
TEST: selecting a board → shows kanban columns matching workflow states
TEST: column order matches workflow state order
TEST: terminal-state columns hidden by default
TEST: d toggles terminal-state column visibility
TEST: tasks displayed as cards in correct state columns
TEST: card shows: priority badge, task number, truncated title
TEST: card shows: assignee or "—" if unassigned
TEST: card shows: first tag if present
TEST: cards within column ordered by priority (critical first), then number
TEST: ←→ (or h/l) navigates between columns
TEST: ↑↓ (or j/k) navigates between tasks within column
TEST: selected task visually highlighted
TEST: column with no tasks → shows empty column with state name
TEST: more tasks than visible height → column scrolls vertically
TEST: more columns than terminal width → columns scroll horizontally
TEST: terminal resize → layout recalculates

=== Filter Bar ===
TEST: f opens filter bar for editing
TEST: typing "state:backlog" → filters tasks to backlog state only
TEST: typing "assignee:brice" → filters to tasks assigned to brice
TEST: typing "priority:high,critical" → filters to high and critical
TEST: typing "tag:api" → filters to tasks with api tag
TEST: free text without field: prefix → triggers full-text search via API
TEST: multiple filters → intersection applied
TEST: Enter applies filter, returns focus to main view
TEST: Esc cancels filter editing, reverts to previous filter
TEST: active filter displayed at top of screen
TEST: clearing all filter text → shows all tasks (no filter)
TEST: tab-completion in filter bar → completes state names from workflow
TEST: tab-completion → completes actor names
TEST: tab-completion → completes tag names

=== HTTP API Client ===
TEST: client sends Authorization header with configured API key
TEST: client parses JSON responses into model types
TEST: client surfaces HTTP error responses as typed errors
TEST: client handles connection refused → clear error message
TEST: client handles timeout → clear error message
```

### 3.2 Implementation

- `cmd/taskflow-tui/main.go` — Load config (reuse CLI config parser), parse flags, initialise API client, run Bubble Tea program.
- `internal/tui/client.go` — HTTP API client. Methods for all needed endpoints: `ListBoards`, `GetBoard`, `ListTasks`, `GetTask`, `ListActors`, `TransitionTask`, `UpdateTask`, `AddComment`, `ListTags`, `GetTaskAudit`. Thin wrapper over `net/http` with JSON (de)serialisation and auth header injection.
- `internal/tui/theme.go` — Lipgloss styles: colours for priority levels, states, selected items, borders, headers.
- `internal/tui/keymap.go` — Bubble Tea key bindings, configurable.
- `internal/tui/app.go` — Root model. Holds active view state, board data, API client ref. Routes `Init`, `Update`, `View` to active sub-model.
- `internal/tui/views/selector.go` — Board selector model. List with fuzzy filter.
- `internal/tui/views/board.go` — Kanban view model. Holds columns (one per state), selected column/row index, terminal-state visibility toggle.
- `internal/tui/views/filter.go` — Filter bar model. Parses `field:value` syntax, applies to in-memory task list, tab-completion.
- `internal/tui/components/card.go` — Task card renderer. Formats priority badge, number, title, assignee, tag into a fixed-width block.
- `internal/tui/components/column.go` — Kanban column renderer. Header with state name + count, scrollable card list.

### 3.3 Estimated Effort: 3–4 dev-days

### 3.4 Done When

All acceptance tests pass. **You can now** launch `taskflow-tui`, select a board, see tasks laid out in kanban columns, navigate with keyboard, and filter tasks. No mutations or live updates yet.

---

## Increment 4: List View + Task Detail Pane

**Goal:** The sortable list view and the task detail overlay, completing the read-only TUI experience.

### 4.1 Acceptance Tests

```
=== List View ===
TEST: Tab from board view → switches to list view
TEST: Tab from list view → switches to board view
TEST: list view shows tasks in tabular format: #, state, priority, title, assignee, tags, due
TEST: list view respects same filters as board view
TEST: ↑↓ (or j/k) navigates rows
TEST: selected row visually highlighted
TEST: ←→ selects sort column (visual indicator on column header)
TEST: s (or Enter on column header) toggles sort direction on selected column
TEST: default sort: priority desc, then task number asc
TEST: sort by due date → tasks with no due date sorted last
TEST: sort by state → ordered by workflow state order, not alphabetical
TEST: terminal-state tasks hidden by default, shown with d toggle
TEST: long titles truncated with ellipsis
TEST: priority displayed as short label: crit, high, med, low, none
TEST: due date displayed as M/DD or YYYY-MM-DD if not current year
TEST: empty list → shows "No tasks match current filters"

=== Task Detail Pane ===
TEST: Enter on selected task in board view → opens detail pane
TEST: Enter on selected task in list view → opens detail pane
TEST: detail pane shows: title, state (with transition name), priority, assignee, tags, due date, created date
TEST: detail pane shows: full description (markdown rendered to plain text)
TEST: detail pane shows: dependencies with target task state
TEST: detail pane shows: attachments with type indicator ([file] or [ref_type])
TEST: detail pane shows: comments in chronological order with actor and timestamp
TEST: detail pane shows: recent audit entries (last 20) in chronological order
TEST: detail pane scrollable with ↑↓ (or j/k)
TEST: Esc closes detail pane, returns to previous view
TEST: detail pane renders as overlay on right two-thirds of screen
TEST: narrow terminal (< 80 cols) → detail pane renders full width
TEST: detail pane for task with no comments → shows "No comments"
TEST: detail pane for task with no dependencies → section omitted
TEST: detail pane for task with no attachments → section omitted
TEST: cross-board dependency → shows full ref (other-board/45)
```

### 4.2 Implementation

- `internal/tui/views/list.go` — List view model. Holds sortable table state: selected column, sort direction, column widths. Shares the same filtered task set as the board view.
- `internal/tui/components/table.go` — Generic sortable table component. Column headers with sort indicators (`▲`/`▼`), row selection, truncation with ellipsis, dynamic column widths based on terminal size.
- `internal/tui/views/detail.go` — Task detail pane model. Fetches full task data (`GetTask`) and audit log (`GetTaskAudit`) on open. Renders sections: header/metadata, description, dependencies, attachments, comments, audit. Scrollable viewport.
- Extend `internal/tui/app.go` — View switching logic: Tab cycles board↔list, Enter opens detail, Esc closes detail. Detail pane as an overlay flag on the root model.
- Sort comparison functions for each column type: string, priority (custom order), state (workflow order), date, integer.

### 4.3 Estimated Effort: 2–3 dev-days

### 4.4 Done When

All acceptance tests pass. **You can now** browse tasks in both kanban and list views, sort and filter, and inspect full task details. The read-only TUI experience is complete.

---

## Increment 5: Inline Actions + Live Updates

**Goal:** Mutation actions from within the TUI and real-time SSE-driven updates. After this increment, the TUI is a fully interactive task management interface.

### 5.1 Acceptance Tests

```
=== Transition Action ===
TEST: t on selected task → opens transition picker showing available transitions
TEST: transition picker shows: transition name → target state for each option
TEST: ↑↓ selects transition
TEST: Enter with no comment → executes transition, overlay closes
TEST: Enter with comment → executes transition with comment, overlay closes
TEST: successful transition → task moves to new column in board view (via SSE or API confirmation)
TEST: failed transition (e.g. 400 invalid) → error displayed, overlay stays open
TEST: Esc → cancels, no transition executed
TEST: t on task in terminal state → shows "No transitions available" message
TEST: transition picker includes optional comment input field

=== Assign Action ===
TEST: a on selected task → opens assignee picker listing active actors
TEST: assignee picker shows current assignee highlighted
TEST: includes "— unassign" option
TEST: Enter on actor → task updated with new assignee
TEST: Enter on "— unassign" → assignee cleared
TEST: successful assignment → task card updated
TEST: Esc → cancels, no assignment change

=== Comment Action ===
TEST: c on selected task → opens comment composer
TEST: comment composer is multi-line text input
TEST: Ctrl+D submits comment
TEST: Esc cancels (with confirmation if text entered)
TEST: successful comment → if detail pane open, comment appears
TEST: empty comment → submit disabled / no-op

=== Action Availability ===
TEST: t, a, c available from board view on selected task
TEST: t, a, c available from list view on selected task
TEST: t, a, c available from detail pane for displayed task
TEST: read_only actor → t, a, c show "permission denied" message

=== SSE Live Updates ===
TEST: task created by another client → appears in board/list view without refresh
TEST: task transitioned by another client → moves between columns in board view
TEST: task updated by another client → card/row reflects new values
TEST: task deleted by another client → removed from view
TEST: task assigned by another client → card/row shows new assignee
TEST: comment added by another client → if detail pane open, comment appended
TEST: event from own actions → also applied (server is source of truth)
TEST: rapid events → all applied in order, no visual glitch
TEST: SSE connection established → status indicator in footer shows "live"

=== SSE Reconnection ===
TEST: SSE connection drops → automatic reconnect attempt
TEST: reconnect backoff: 1s, 2s, 4s, 8s, capped at 30s
TEST: on reconnect → full task list refetched to sync missed events
TEST: reconnect succeeds → status indicator returns to "live"
TEST: 3 consecutive SSE failures → falls back to polling (30s interval)
TEST: status indicator shows "polling" during fallback
TEST: SSE reconnect succeeds after fallback → resumes SSE, stops polling
TEST: status indicator shows "disconnected" during backoff attempts

=== Board Switching with Live Updates ===
TEST: switch board → old SSE connection closed
TEST: switch board → new SSE connection opened for new board
TEST: events from old board → not received after switch
```

### 5.2 Implementation

- `internal/tui/actions/transition.go` — Transition picker model. Fetches available transitions from workflow (in memory). List selection + optional comment input. Calls `client.TransitionTask(...)` on confirm.
- `internal/tui/actions/assign.go` — Assignee picker model. Fetches actor list on open. List selection. Calls `client.UpdateTask(...)` with new assignee on confirm.
- `internal/tui/actions/comment.go` — Comment composer model. Multi-line `textarea` (Bubble Tea textarea component). Calls `client.AddComment(...)` on Ctrl+D.
- `internal/tui/components/input.go` — Shared text input component wrapping `bubbles/textarea`.
- `internal/tui/components/confirm.go` — Confirmation dialog component (used for discard-unsaved-comment prompt).
- `internal/tui/sse.go` — SSE client.
  - `Connect(ctx, boardSlug)` → opens HTTP connection, returns event channel.
  - Line parser: reads `event:` and `data:` lines, assembles into `Event` structs.
  - Reconnection manager: exponential backoff, max 30s, refetch on reconnect.
  - Polling fallback: after 3 SSE failures, polls `ListTasks` every 30s; continues SSE attempts.
  - Integrates with Bubble Tea via `Program.Send(msg)` from a background goroutine.
- Extend `internal/tui/app.go`:
  - SSE lifecycle management: start on board select, stop on board switch/quit.
  - `Update` handlers for SSE event messages: apply to in-memory task list.
  - Status indicator in footer: "live" | "reconnecting..." | "polling" | "disconnected".
  - Action overlay routing: `t`/`a`/`c` key handlers that open the appropriate action model.
  - Permission checking: if current actor is read_only, show error instead of action overlays.

### 5.3 Estimated Effort: 2–3 dev-days

### 5.4 Done When

All acceptance tests pass. **You can now** transition tasks, assign them, and add comments directly from the TUI, and see changes from other clients appear in real time.

---

## Phase 2 Summary

| Increment | What it delivers | Dev-days | Depends on |
|-----------|-----------------|----------|------------|
| 1. Event Bus | In-process pub/sub, wired to all operations | 1.5–2 | Phase 1 |
| 2. SSE Endpoint | Real-time event streaming per board | 1–1.5 | 1 |
| 3. TUI Core + Board View | App shell, board selector, kanban view, filter bar | 3–4 | Phase 1 (API) |
| 4. List View + Detail | Sortable table, task detail pane | 2–3 | 3 |
| 5. Inline Actions + Live Updates | Mutations from TUI, SSE-driven updates | 2–3 | 2, 4 |
| **Total** | **Full interactive TUI with real-time updates** | **~10–13.5** | |

### Critical Path

```
Increment 1 ──→ Increment 2 ──────────────────────┐
                                                    ├──→ Increment 5
Increment 3 ──→ Increment 4 ──────────────────────┘
```

Increments 1→2 (server-side event infrastructure) and 3→4 (TUI read-only experience) are developed **in parallel**. They converge at increment 5, which wires the TUI to the SSE stream and adds mutation actions.

At ~2–2.5 dev-days per week: **~4–6 weeks**.

### Phase 2 Exit Criteria

- All 10 event types emitted by the server on corresponding mutations.
- SSE endpoint streams events per board with auth, heartbeat, and clean lifecycle.
- `taskflow-tui` binary with multi-board navigation, kanban and list views, filtering and sorting.
- Task detail pane with full information, comments, dependencies, attachments, and audit trail.
- Inline transition, assign, and comment actions.
- Live updates via SSE with automatic reconnection and polling fallback.
- Status indicator shows connection state.
- All acceptance tests passing.

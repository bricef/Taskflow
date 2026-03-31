## Archived Board Semantics

When a board is archived (soft-deleted), the following operations are **blocked** with a 403 Forbidden:
- Create, update, transition, or delete tasks
- Create or delete dependencies
- Create or delete attachments

The following operations remain **allowed**:
- Reading boards, tasks, audit, comments, dependencies, attachments
- Adding comments (append-only)
- Listing boards (with `include_deleted=true` or TUI `a` toggle)

## Known Bugs

- [ ] **TUI resize**: Resizing the terminal (especially making it larger) causes layout issues. The kanban columns, event log, and viewport don't reliably recalculate their dimensions. Needs a deeper investigation into how Bubble Tea's value-receiver model interacts with viewport sizing.
- [ ] **space below help**: We still have a line of deadspace at the bottom. Lt's fix that.

## Architecture decisions

### MCP Resource URI Scheme

MCP resources use the `taskflow://` URI scheme, mapping directly to the API path structure:

```
taskflow://boards                          — list all boards
taskflow://boards/{slug}                   — board detail with task counts by state
taskflow://boards/{slug}/tasks             — task list (supports query params)
taskflow://boards/{slug}/tasks/{num}       — full task detail (comments, deps, attachments, audit)
taskflow://boards/{slug}/workflow          — workflow definition
taskflow://boards/{slug}/audit             — board audit trail
taskflow://boards/{slug}/tags              — tags in use
taskflow://actors                          — actor list
taskflow://search?q={query}                — cross-board search
taskflow://stats                           — system stats (admin)
```

### Global SSE Endpoint

`GET /events` — global event stream with optional filtering:
- `?boards=platform,product` — filter to specific boards
- `?assignee=alice` or `?assignee=@me` — filter to events on tasks assigned to a user

This complements the existing per-board `GET /boards/{slug}/events` endpoint. The TUI currently uses per-board SSE; it should be revisited to use the global endpoint once available.

## Cross-surface improvements (from MCP review)

These features were identified during MCP planning and should be integrated into the TUI, CLI, and dashboard as well:

- [ ] **"My tasks" view**: Cross-board task list filtered by assignee (`@me`). Already implemented as API feature for MCP. Integrate into TUI (new tab or filter), CLI (`taskflow task list --assignee @me`), and dashboard.
- [ ] **Board overview with task counts**: Board detail endpoint should include task counts by state. Use in dashboard board cards, TUI board selector, CLI board get.
- [ ] **Transition error context in CLI/TUI**: When a transition fails, show available transitions in the error message (API already returns them).
- [x] **TUI SSE architecture**: TUI now uses the global `/events` endpoint with per-board ring buffers. Events are captured from startup; switching boards preserves history. No reconnection on board switch.

## Planned Dashboard Features

- [ ] **Cumulative flow diagram (CFD)**: Stacked area chart showing task counts per state over time, derived from audit log.
- [ ] **Cycle time report**: Time from first non-initial state to terminal state, per task. Histogram and averages.
- [ ] **Throughput report**: Tasks completed per time period (day/week/month).
- [ ] **Actor activity charts**: Visual breakdown of who's doing what over time.
- [ ] **Board summary**: System-wide overview with per-board health indicators.

## Planned TUI Features

- [x] **List view**: Sortable table as an alternative to kanban (tab to switch). Columns: num, title, state, priority, assignee, due date.
- [ ] **Filter bar**: Filter tasks by assignee, priority, tags, state — applies to both kanban and list views.
- [ ] **Dependency tree**: Show a visual dependency tree in the task detail view.
- [x] **Workflow visualisation**: A board tab showing the workflow graph (states and transitions).
- [ ] **Mouse Support**: using https://github.com/lrstanley/bubblezone to manage interactivitys

---

### Q: this system might be used for someone (something?) like you in the future. what would you like to see out of it? any design decisions that would make your life easier?

Great question. Here's what I'd want as an AI agent using TaskFlow:

- [ ] MCP server interface. The PRD mentions it but it's not built yet. For me, an MCP tool interface would be the most natural way to interact — I could create tasks, transition them, and check board state without constructing HTTP requests. The operation definitions in model.Operations() could derive MCP tool definitions the same way they derive HTTP routes and CLI commands.

- [x] Structured error responses with actionable context. The current error format is good ({"error": "invalid_transition", "message": "..."}) but when a transition fails, I'd love the response to include the available transitions so I can self-correct without a second round-trip:

  {"error": "invalid_transition", "message": "...", "detail": {"available": ["start", "cancel"]}}

- [x] Batch operations. If I'm triaging a backlog, I'd want to create 10 tasks or transition 5 tasks in a single call rather than 10/5 sequential requests. A POST /batch endpoint accepting an array of operations would be huge.

- [x] Webhook/event notifications. Webhook dispatcher subscribes to the event bus and delivers matching events as signed HTTP POST requests (HMAC-SHA256). Supports board-scoped and global webhooks, event type filtering, and active/inactive toggle.

- [x] Task assignment to self. The API requires knowing my actor name to set assignee. It would be nice if the authenticated actor's name were available as a special value like "@me" so I don't have to discover my own identity first.

- [x] Search across boards. The current FTS is board-scoped. If I'm working across multiple boards, I'd want GET /tasks?q=auth+bug without knowing which board to look in.

- [x] Idempotency keys. If my HTTP call times out, I don't know if the task was created or not. An Idempotency-Key header that deduplicates creates would let me safely retry.

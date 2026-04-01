# TODO

Outstanding tasks and known issues. See [docs/plans/active/](docs/plans/active/) for larger planned work.

## Bugs

- [ ] **TUI resize**: resizing the terminal (especially making it larger) causes layout issues — kanban columns, event log, and viewport don't reliably recalculate dimensions
- [ ] **TUI space below help**: a line of dead space at the bottom of the terminal

## TUI

- [ ] Filter bar: filter tasks by assignee, priority, tags, state — applies to both kanban and list views
- [ ] Board overview in TUI board selector: show task counts by state when selecting a board
- [ ] Mouse support (using [bubblezone](https://github.com/lrstanley/bubblezone))

## MCP

- [ ] Integration test with a real MCP client (Claude Code) — pipe tests pass but protocol-level issues may only surface with a real agent
- [ ] Rate limiting awareness — MCP agents making rapid tool calls could overwhelm the server; consider backpressure or advisory rate info in tool responses

## API

- [ ] Due date filtering: `--overdue` filter and sort support (field exists on Task but isn't queryable)
- [ ] Due date visual indicator in TUI kanban (e.g. red highlight for past-due tasks)
- [ ] Backup/restore: `taskflow admin backup` command (SQLite makes this easy but no tooling exists)

## Deployment

- [ ] Base path prefix support (`TASKFLOW_BASE_PATH`) — currently the server must be hosted at the root of a domain; hosting at a subpath (e.g. `/taskflow`) requires code changes to route registration, OpenAPI generation, and dashboard links

## Admin

- [ ] Webhook management from TUI (admin users currently need CLI or API)

## Done

- [x] Task edit form: full edit overlay (title, description, priority, assignee, tags, due date) with dependency and attachment management
- [x] Dependency tree visualisation in task detail view with parent/child/related indentation
- [x] "My tasks" mode in TUI: cross-board view of tasks assigned to current user, with live updates
- [x] Shared task table component with styled cells (coloured priorities, @me assignee)
- [x] Comment prompt on transition/assign with auto-generated summary line
- [x] Richer audit detail rendering showing field-level changes in task detail view
- [x] Board selector split into "Quick Access" and "Boards" sections
- [x] Consistent keyboard conventions across all text inputs (enter submit, ctrl+j newline, esc cancel)
- [x] Page scrolling (PgUp/PgDn/Ctrl-U/Ctrl-D/Home/End) in workflow, event log, and detail views
- [x] Searchable selectors for transition and assignment overlays
- [x] @me shortcut in assignment overlay and styled @me display across kanban, list, and detail views
- [x] "Take" action (T) for instant self-assignment from kanban/list/detail
- [x] Terminal-state tasks hidden by default in board list and My Tasks views (d toggles)
- [x] Default sort by priority in list views
- [x] MCP server with auto-derived tools, resources, notifications, whoami, task_ref
- [x] TUI global event stream with per-board ring buffers
- [x] Structured error responses with available transitions on failure
- [x] Transition error context — error messages include available transitions in all clients
- [x] Batch operations (POST /batch)
- [x] Webhook dispatch with HMAC-SHA256 signatures
- [x] @me actor alias for self-assignment
- [x] Cross-board task search (GET /tasks) with @me filter support
- [x] Idempotency keys for safe retries
- [x] List view with sortable columns
- [x] Workflow graph visualisation
- [x] Board overview endpoint (board_overview Resource)

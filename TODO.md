# TODO

Outstanding tasks and known issues. See [docs/plans/active/](docs/plans/active/) for larger planned work.

## Bugs

- [ ] **TUI resize**: resizing the terminal (especially making it larger) causes layout issues — kanban columns, event log, and viewport don't reliably recalculate dimensions
- [ ] **TUI space below help**: a line of dead space at the bottom of the terminal

## TUI

- [ ] Filter bar: filter tasks by assignee, priority, tags, state — applies to both kanban and list views
- [ ] Dependency tree visualisation in task detail view
- [ ] Mouse support (using [bubblezone](https://github.com/lrstanley/bubblezone))

See also [TUI improvements plan](docs/plans/active/2026-03-31-tui-improvements.md) for page scrolling, searchable selectors, @me shortcut, and take action.

## Cross-surface

- [ ] "My tasks" view: integrate cross-board `@me` filter into TUI (new tab or filter mode) and dashboard
- [ ] Board overview in TUI board selector: show task counts by state when selecting a board
- [ ] Transition error context in CLI/TUI: show available transitions in error messages (API already returns them)

## Done

- [x] MCP server with auto-derived tools, resources, notifications, whoami, task_ref
- [x] TUI global event stream with per-board ring buffers
- [x] Structured error responses with available transitions on failure
- [x] Batch operations (POST /batch)
- [x] Webhook dispatch with HMAC-SHA256 signatures
- [x] @me actor alias for self-assignment
- [x] Cross-board task search (GET /tasks)
- [x] Idempotency keys for safe retries
- [x] List view with sortable columns
- [x] Workflow graph visualisation

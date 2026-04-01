# TODO

Outstanding tasks and known issues. See [docs/plans/active/](docs/plans/active/) for larger planned work.

## Bugs

- [ ] **TUI resize**: resizing the terminal (especially making it larger) causes layout issues — kanban columns, event log, and viewport don't reliably recalculate dimensions
- [ ] **TUI space below help**: a line of dead space at the bottom of the terminal

## TUI

- [ ] Filter bar: filter tasks by assignee, priority, tags, state — applies to both kanban and list views
- [ ] "My tasks" mode in TUI: dedicated view or filter for `@me` tasks (API and CLI already support it)
- [ ] Board overview in TUI board selector: show task counts by state when selecting a board
- [ ] Dependency tree visualisation in task detail view
- [ ] Mouse support (using [bubblezone](https://github.com/lrstanley/bubblezone))

See also [TUI improvements plan](docs/plans/active/2026-03-31-tui-improvements.md) for page scrolling, searchable selectors, @me shortcut, and take action.

## MCP

- [ ] Integration test with a real MCP client (Claude Code) — pipe tests pass but protocol-level issues may only surface with a real agent
- [ ] Rate limiting awareness — MCP agents making rapid tool calls could overwhelm the server; consider backpressure or advisory rate info in tool responses

## API

- [ ] Due date filtering: `--overdue` filter and sort support (field exists on Task but isn't queryable)
- [ ] Due date visual indicator in TUI kanban (e.g. red highlight for past-due tasks)
- [ ] Backup/restore: `taskflow admin backup` command (SQLite makes this easy but no tooling exists)

## Admin

- [ ] Webhook management from TUI (admin users currently need CLI or API)

## Done

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

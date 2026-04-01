---
name: taskflow
description: Manage tasks on TaskFlow boards. Use the MCP server (preferred) or CLI to create, update, transition, list, and comment on tasks.
---

You have access to TaskFlow, a task tracker with kanban boards and workflow state machines. Every action is attributed to your actor identity in the audit trail.

## Integration Options

**MCP server (preferred):** If TaskFlow is configured as an MCP server, use the MCP tools and resources directly. They provide typed inputs, rich descriptions, and notification piggyback. See the tools list for available operations.

**CLI fallback:** If MCP is not available, use the `taskflow` CLI. Add `--json` when you need to parse output programmatically.

Both use the same API key for authentication and actor identity.

## Quick Reference

### Read data

```bash
# Boards
taskflow board list
taskflow board detail <slug>              # full board dump
taskflow board overview <slug>            # task counts by state

# Tasks
taskflow task list <slug>                 # all tasks on a board
taskflow task detail <slug> <num>         # task with comments, deps, attachments, audit
taskflow task search --q "auth bug"       # search across all boards

# Workflow
taskflow workflow get <slug>              # see states and transitions
```

### Manage tasks

```bash
# Create (priority: critical, high, medium, low, or omit)
taskflow task create <slug> --title "Fix auth bug" --priority high --assignee @me

# Transition (use transition NAME, not state name)
taskflow task transition <slug> <num> --transition start --comment "On it"

# Update
taskflow task update <slug> <num> --priority critical --assignee alice

# Comment
taskflow comment create <slug> <num> --body "Ready for review"

# Delete (soft-delete)
taskflow task delete <slug> <num>
```

### Boards

```bash
# Create (omit --workflow for the default backlog→in_progress→review→done workflow)
taskflow board create --slug my-board --name "My Board"

# Archive
taskflow board delete <slug>
```

## Key Concepts

- **Workflows**: each board has a state machine defining how tasks move. Always check `taskflow workflow get <slug>` before transitioning if you're unsure which transitions are valid.
- **Transitions by name**: pass the transition name (e.g. `start`, `submit`, `approve`), not the target state. On failure, the error message lists available transitions.
- **`@me`**: use as assignee to assign tasks to yourself. Works in create, update, and search filters.
- **Task references**: tasks are identified by `<board-slug> <num>` (CLI) or `board/num` shorthand (MCP `task_ref`).
- **Priorities**: `critical`, `high`, `medium`, `low`, or omit for none.
- **Audit trail**: every action is recorded with actor attribution and timestamp.

## Tips

- Use `task detail` instead of `task get` — it includes comments, dependencies, attachments, and audit in one request.
- Use `task search --assignee @me` to find all your tasks across boards.
- Comments are the primary way to communicate progress and context on tasks.
- When transitioning fails, read the error — it tells you which transitions are available.

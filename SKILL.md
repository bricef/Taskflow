---
name: taskflow
description: Manage tasks on TaskFlow boards via the CLI. Use when asked to create, update, transition, list, or comment on tasks.
---

You have access to the `taskflow` CLI which manages tasks on kanban boards with workflow state machines.

## Prerequisites

The CLI needs a running TaskFlow server. Configuration is resolved in this order:
1. Flags: `--url`, `--api-key`
2. Environment: `TASKFLOW_URL`, `TASKFLOW_API_KEY`
3. Config file: `~/.config/taskflow/config.yaml`
4. Default: `http://localhost:8374`

Always use `--json` when you need to parse output programmatically.

## Common Workflows

### Discover what's available

```bash
# List all boards
taskflow board list

# See a board's workflow (available states and transitions)
taskflow workflow get <board-slug>

# List tasks on a board
taskflow task list <board-slug>

# List tasks with filters
taskflow task list <board-slug> --state in_progress --assignee alice
taskflow task list <board-slug> --priority high --include_closed

# Search across all boards
taskflow search --q "auth bug"

# Get full board detail (tasks, comments, deps, audit)
taskflow board detail <board-slug>
```

### Create and manage tasks

```bash
# Create a task (priority: critical, high, medium, low, or omit for none)
taskflow task create <board-slug> --title "Fix the auth bug" --priority high --tags "security,urgent"

# Update a task
taskflow task update <board-slug> <num> --title "New title" --priority medium --assignee alice

# Transition a task through the workflow (e.g. start, submit, approve, reject, cancel)
taskflow task transition <board-slug> <num> --transition start --comment "Working on this now"

# Delete a task (soft-delete)
taskflow task delete <board-slug> <num>
```

### Comments

```bash
# Add a comment
taskflow comment create <board-slug> <num> --body "This needs review"

# List comments on a task
taskflow comment list <board-slug> <num>
```

### Dependencies

```bash
# Create a dependency (task 3 depends on task 1)
taskflow dependency create <board-slug> 3 --depends_on_board <board-slug> --depends_on_num 1 --dep_type depends_on

# List dependencies
taskflow dependency list <board-slug> <num>
```

### Boards

```bash
# Create a board (default workflow if --workflow omitted)
taskflow board create --slug my-board --name "My Board"

# Create with custom workflow
taskflow board create --slug my-board --name "My Board" --workflow '{"states":["todo","doing","done"],"initial_state":"todo","terminal_states":["done"],"transitions":[{"from":"todo","to":"doing","name":"start"},{"from":"doing","to":"done","name":"finish"}]}'

# Archive a board (soft-delete, comments still allowed)
taskflow board delete <board-slug>
```

### Audit trail

```bash
# Task audit log
taskflow task audit <board-slug> <num>

# Board audit log
taskflow board audit <board-slug>
```

### Views

```bash
# Board overview (task counts by state)
taskflow board overview <board-slug>

# System-wide statistics (admin only)
taskflow admin stats
```

## Key Concepts

- **Workflow**: Each board has a state machine. Use `taskflow workflow get <slug>` to see available states and transitions before transitioning tasks.
- **Archived boards**: Archived boards are read-only except for comments. Use `taskflow board list --include_deleted` to see them.
- **Priorities**: `critical`, `high`, `medium`, `low`, or omit for none.
- **Task IDs**: Tasks are numbered per-board. Reference as `<board-slug> <num>`.
- **Actor**: Your identity is determined by your API key. Use `@me` as assignee to assign to yourself.

## Tips

- When asked to check task status, use `taskflow task get <slug> <num> --json` and parse the JSON.
- When transitioning, first check available transitions with `taskflow workflow get <slug>` if unsure which transition names are valid.
- Use `taskflow task list <slug> --json` for programmatic processing of task lists.
- Comments are the primary way to communicate progress on tasks.

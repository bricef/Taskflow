# TUI Reference

The TUI (`taskflow-tui`) is an interactive terminal interface for TaskFlow. It connects to a running server via the HTTP API and live event stream.

## Getting Started

```bash
# Start with default settings
taskflow-tui

# Or specify a board directly
taskflow-tui platform
```

Configuration is resolved in the same order as the CLI:
1. Environment: `TASKFLOW_URL`, `TASKFLOW_API_KEY`
2. Config file: `~/.config/taskflow/config.yaml`
3. Default: `http://localhost:8374`

## Views

The TUI has two modes: a **board selector** for picking which board to view, and a **board view** with four tabs.

### Board Selector

The initial view lists all boards. Type to filter, `enter` to select.

| Key | Action |
|-----|--------|
| `enter` | Select board |
| `n` | Create new board |
| `a` | Toggle archived boards |
| `x` | Archive selected board |
| type | Filter by name |
| `esc` | Clear filter |

### Board View

Once a board is selected, the view has four tabs (cycle with `tab` / `shift+tab`):

| Tab | Description |
|-----|-------------|
| **Board** | Kanban columns by workflow state |
| **List** | Sortable task table |
| **Workflow** | Visual graph of states and transitions |
| **Events** | Live event stream with detail pane |

#### Board (Kanban)

Navigate columns and tasks with vim keys:

| Key | Action |
|-----|--------|
| `h` / `l` | Move between columns |
| `j` / `k` | Move between tasks in a column |
| `d` | Toggle done/terminal state columns |
| `enter` | Open task detail |

#### List

A sortable table of all tasks:

| Key | Action |
|-----|--------|
| `j` / `k` | Move between rows |
| `s` | Cycle sort column (num, title, state, priority, assignee) |
| `S` | Reverse sort order |
| `d` | Toggle done/terminal tasks |
| `enter` | Open task detail |

#### Workflow

A visual graph of the board's workflow state machine, showing states and transitions with Unicode connectors. Read-only.

#### Events

A live stream of domain events with a side-by-side detail pane. Events are buffered per-board from startup — switching boards preserves event history.

| Key | Action |
|-----|--------|
| `j` / `k` | Select event |
| `enter` | Open task detail for the selected event's task |

## Actions

These actions are available from the Board, List, and Detail views:

| Key | Action | Context |
|-----|--------|---------|
| `t` | Transition task | Board, List, Detail |
| `a` | Assign task | Board, List, Detail |
| `c` | Add comment | Detail only |
| `enter` | Open task detail | Board, List, Events |
| `esc` | Close overlay / go back | Everywhere |
| `?` | Toggle help | Everywhere |
| `q` | Quit | Everywhere |

### Task Detail

The detail overlay shows full task information: metadata, comments, dependency tree, attachments, and audit trail. From here you can transition (`t`), assign (`a`), or add a comment (`c`).

### Transition

A picker showing available transitions for the current task's workflow state. Navigate with `j`/`k`, confirm with `enter`, cancel with `esc`.

### Assign

A picker showing all active actors. Select an actor to assign the task, or choose "(unassign)" to clear the assignment. Navigate with `j`/`k`, confirm with `enter`.

## Live Updates

The TUI subscribes to the global event stream at startup. Changes from other sessions (CLI, API, other TUI instances, AI agents) appear live across all views:

- **Kanban**: tasks move between columns
- **List**: table updates with new/changed tasks
- **Detail**: refreshes if the viewed task is affected
- **Events**: new entries appear in the event log

Events are captured for all boards from the moment the TUI starts. Switching between boards preserves each board's event history.

## Architecture

The TUI is a pure client — it imports no server internals. All data access goes through `internal/httpclient`:

- **Resources** (`httpclient.GetOne`, `httpclient.GetMany`) for reading boards, tasks, workflows, etc.
- **Operations** (`httpclient.Exec`, `httpclient.ExecNoResult`) for mutations
- **Events** (`httpclient.Subscribe`) for the live event stream

The TUI uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the terminal UI framework and [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling.

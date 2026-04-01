package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

// Shared column definitions used by both the board list view and My Tasks view.
var (
	colNum = taskColumn{
		Key: "num", Title: "#", Width: 5, Visible: true,
		Render: func(t model.Task) string { return fmt.Sprintf("%d", t.Num) },
		Value:  func(t model.Task) string { return fmt.Sprintf("%05d", t.Num) },
	}
	colTitle = taskColumn{
		Key: "title", Title: "Title", Width: 0, Visible: true, // flex
		Render: func(t model.Task) string { return t.Title },
		Value:  func(t model.Task) string { return strings.ToLower(t.Title) },
	}
	colState = taskColumn{
		Key: "state", Title: "State", Width: 14, Visible: true,
		Render: func(t model.Task) string { return t.State },
		Value:  func(t model.Task) string { return t.State },
	}
	colPriority = taskColumn{
		Key: "priority", Title: "Priority", Width: 10, Visible: true,
		Render: func(t model.Task) string { return string(t.Priority) },
		Value:  func(t model.Task) string { return fmt.Sprintf("%d", priorityRank(t.Priority)) },
		Style: func(s string) string {
			if st, ok := priorityStyle[model.Priority(s)]; ok {
				return st.Render(s)
			}
			return s
		},
	}
	meStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
)

// newColAssignee returns an assignee column that renders the current user as "@me".
func newColAssignee(currentUserName string) taskColumn {
	return taskColumn{
		Key: "assignee", Title: "Assignee", Width: 12, Visible: true,
		Render: func(t model.Task) string {
			if t.Assignee == nil {
				return "—"
			}
			if currentUserName != "" && *t.Assignee == currentUserName {
				return "@me"
			}
			return *t.Assignee
		},
		Value: func(t model.Task) string {
			if t.Assignee != nil {
				return *t.Assignee
			}
			return "zzz"
		},
		Style: func(s string) string {
			if s == "@me" {
				return meStyle.Render(s)
			}
			return s
		},
	}
}

// taskColumn defines a column in the task table.
type taskColumn struct {
	Key     string
	Title   string
	Width   int  // fixed width; 0 means flex (takes remaining space)
	Visible bool // toggled at runtime
	// Render returns the plain display text for this column given a task.
	Render func(model.Task) string
	// Value returns a plain sortable string for this column.
	Value func(model.Task) string
	// Style optionally applies ANSI styling to the cell text after truncation.
	// If nil, the cell is rendered as plain text.
	Style func(string) string
}

// taskTable is a manually-rendered, sortable, scrollable task list with
// styled cells and toggleable column visibility.
type taskTable struct {
	columns  []taskColumn
	tasks    []model.Task
	filtered []model.Task
	cursor   int
	sortCol  int // index into visible columns
	sortAsc  bool
	ready    bool

	// isTerminal is called during rebuild to filter terminal-state tasks.
	// Provided by the consumer (board list view uses workflow, my-tasks uses cached workflows).
	isTerminal func(model.Task) bool
	showDone   bool
}

func newTaskTable(columns []taskColumn, defaultSortCol int) taskTable {
	return taskTable{
		columns: columns,
		sortCol: defaultSortCol,
		sortAsc: true,
	}
}

// visibleColumns returns only columns with Visible == true.
func (t *taskTable) visibleColumns() []taskColumn {
	var cols []taskColumn
	for _, c := range t.columns {
		if c.Visible {
			cols = append(cols, c)
		}
	}
	return cols
}

func (t *taskTable) load(tasks []model.Task) {
	t.tasks = tasks
	t.ready = true
	t.rebuild()
}

func (t *taskTable) updateTask(task model.Task) {
	for i, existing := range t.tasks {
		if existing.BoardSlug == task.BoardSlug && existing.Num == task.Num {
			t.tasks[i] = task
			t.rebuild()
			return
		}
	}
	t.tasks = append(t.tasks, task)
	t.rebuild()
}

func (t *taskTable) removeTask(boardSlug string, num int) {
	for i, existing := range t.tasks {
		if existing.BoardSlug == boardSlug && existing.Num == num {
			t.tasks = append(t.tasks[:i], t.tasks[i+1:]...)
			t.rebuild()
			return
		}
	}
}

func (t *taskTable) selectedTask() *model.Task {
	if t.cursor >= 0 && t.cursor < len(t.filtered) {
		f := t.filtered[t.cursor]
		for i := range t.tasks {
			if t.tasks[i].BoardSlug == f.BoardSlug && t.tasks[i].Num == f.Num {
				return &t.tasks[i]
			}
		}
	}
	return nil
}

func (t *taskTable) rebuild() {
	t.filtered = nil
	for _, task := range t.tasks {
		if task.Deleted {
			continue
		}
		if !t.showDone && t.isTerminal != nil && t.isTerminal(task) {
			continue
		}
		t.filtered = append(t.filtered, task)
	}
	t.sortFiltered()
	if t.cursor >= len(t.filtered) {
		t.cursor = len(t.filtered) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

func (t *taskTable) sortFiltered() {
	vis := t.visibleColumns()
	if t.sortCol < 0 || t.sortCol >= len(vis) {
		return
	}
	col := vis[t.sortCol]
	if col.Value == nil {
		return
	}
	sort.SliceStable(t.filtered, func(i, j int) bool {
		less := col.Value(t.filtered[i]) < col.Value(t.filtered[j])
		if !t.sortAsc {
			less = !less
		}
		return less
	})
}

func (t *taskTable) cycleSort() {
	vis := t.visibleColumns()
	t.sortCol = (t.sortCol + 1) % len(vis)
	t.sortAsc = true
	t.rebuild()
}

func (t *taskTable) toggleSortDir() {
	t.sortAsc = !t.sortAsc
	t.rebuild()
}

func (t *taskTable) toggleDone() {
	t.showDone = !t.showDone
	t.rebuild()
}

func (t *taskTable) update(msg tea.KeyMsg) {
	switch msg.String() {
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
		}
	case "down", "j":
		if t.cursor < len(t.filtered)-1 {
			t.cursor++
		}
	case "s":
		t.cycleSort()
	case "S":
		t.toggleSortDir()
	case "d":
		t.toggleDone()
	}
}

var (
	ttHeaderStyle = lipgloss.NewStyle().Bold(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("241")).
			BorderBottom(true)
	ttSelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236"))
)

func (t *taskTable) colHeader(col taskColumn, idx int) string {
	name := col.Title
	if idx == t.sortCol {
		if t.sortAsc {
			return name + " ▲"
		}
		return name + " ▼"
	}
	return name
}

func (t taskTable) view(width, height int) string {
	if !t.ready {
		return dimStyle.Render("Loading...") + "\n"
	}
	if len(t.filtered) == 0 {
		return dimStyle.Render("No tasks to show.") + "\n"
	}

	vis := t.visibleColumns()
	widths := t.computeWidths(vis, width)

	// Header.
	var headerParts []string
	for i, col := range vis {
		headerParts = append(headerParts, fmt.Sprintf("%-*s", widths[i], truncate(t.colHeader(col, i), widths[i])))
	}
	var b strings.Builder
	b.WriteString(ttHeaderStyle.Width(width).Render(strings.Join(headerParts, " ")) + "\n")

	// Visible rows with scrolling window.
	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}
	start := 0
	if t.cursor >= listHeight {
		start = t.cursor - listHeight + 1
	}
	end := start + listHeight
	if end > len(t.filtered) {
		end = len(t.filtered)
	}

	for i := start; i < end; i++ {
		task := t.filtered[i]
		var parts []string
		for ci, col := range vis {
			w := widths[ci]
			text := truncate(col.Render(task), w)
			if col.Style != nil {
				styled := col.Style(text)
				pad := w - lipgloss.Width(styled)
				if pad > 0 {
					styled += strings.Repeat(" ", pad)
				}
				parts = append(parts, styled)
			} else {
				parts = append(parts, fmt.Sprintf("%-*s", w, text))
			}
		}
		row := strings.Join(parts, " ")
		if i == t.cursor {
			row = ttSelectedStyle.Width(width).Render(row)
		}
		b.WriteString(row + "\n")
	}

	return b.String()
}

func truncate(s string, maxW int) string {
	if len(s) > maxW {
		return s[:maxW-1] + "…"
	}
	return s
}

// computeWidths distributes available width among columns.
// Fixed-width columns get their Width; flex columns (Width==0) share the remainder.
func (t taskTable) computeWidths(vis []taskColumn, totalWidth int) []int {
	widths := make([]int, len(vis))
	used := len(vis) - 1 // spacing between columns
	flexCount := 0
	for i, col := range vis {
		if col.Width > 0 {
			w := col.Width
			if i == t.sortCol {
				w += 2 // space for sort indicator (e.g. " ▲")
			}
			widths[i] = w
			used += w
		} else {
			flexCount++
		}
	}
	remaining := totalWidth - used
	if remaining < 0 {
		remaining = 0
	}
	if flexCount > 0 {
		each := remaining / flexCount
		if each < 10 {
			each = 10
		}
		for i, col := range vis {
			if col.Width == 0 {
				widths[i] = each
			}
		}
	}
	return widths
}

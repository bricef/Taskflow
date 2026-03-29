package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

type sortField int

const (
	sortNum sortField = iota
	sortTitle
	sortState
	sortPriority
	sortAssignee
	sortFieldCount
)

var sortFieldNames = []string{"#", "Title", "State", "Priority", "Assignee"}

type listViewModel struct {
	tasks    []model.Task
	table    table.Model
	sortBy   sortField
	sortAsc  bool
	ready    bool
	showDone bool
}

func newListView() listViewModel {
	cols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Title", Width: 30},
		{Title: "State", Width: 14},
		{Title: "Priority", Width: 10},
		{Title: "Assignee", Width: 12},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("241")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("236")).
		Bold(false)
	t.SetStyles(s)

	return listViewModel{
		table:   t,
		sortBy:  sortNum,
		sortAsc: true,
	}
}

func (m *listViewModel) load(data boardDataLoaded) {
	if data.err != nil {
		return
	}
	m.tasks = data.tasks
	m.ready = true
	m.rebuild()
}

func (m *listViewModel) updateTask(task model.Task) {
	for i, t := range m.tasks {
		if t.BoardSlug == task.BoardSlug && t.Num == task.Num {
			m.tasks[i] = task
			m.rebuild()
			return
		}
	}
	m.tasks = append(m.tasks, task)
	m.rebuild()
}

func (m *listViewModel) removeTask(boardSlug string, num int) {
	for i, t := range m.tasks {
		if t.BoardSlug == boardSlug && t.Num == num {
			m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
			m.rebuild()
			return
		}
	}
}

func (m *listViewModel) resize(width, height int) {
	m.table.SetWidth(width)
	m.table.SetHeight(height - 2) // account for header + sort indicator

	// Distribute column widths: fixed for #, priority, assignee; rest to title and state.
	numW, prioW, assignW := 5, 10, 12
	stateW := 14
	titleW := width - numW - stateW - prioW - assignW - 10 // padding
	if titleW < 15 {
		titleW = 15
	}
	m.table.SetColumns([]table.Column{
		{Title: m.colHeader(sortNum), Width: numW},
		{Title: m.colHeader(sortTitle), Width: titleW},
		{Title: m.colHeader(sortState), Width: stateW},
		{Title: m.colHeader(sortPriority), Width: prioW},
		{Title: m.colHeader(sortAssignee), Width: assignW},
	})
}

func (m *listViewModel) colHeader(f sortField) string {
	name := sortFieldNames[f]
	if f == m.sortBy {
		if m.sortAsc {
			return name + " ▲"
		}
		return name + " ▼"
	}
	return name
}

func (m *listViewModel) cycleSort() {
	m.sortBy = (m.sortBy + 1) % sortFieldCount
	m.sortAsc = true
	m.rebuild()
}

func (m *listViewModel) toggleSortDir() {
	m.sortAsc = !m.sortAsc
	m.rebuild()
}

func (m *listViewModel) toggleDone() {
	m.showDone = !m.showDone
	m.rebuild()
}

func (m *listViewModel) rebuild() {
	filtered := m.filteredTasks()
	m.sortTasks(filtered)

	rows := make([]table.Row, len(filtered))
	for i, t := range filtered {
		assignee := "—"
		if t.Assignee != nil {
			assignee = *t.Assignee
		}
		rows[i] = table.Row{
			fmt.Sprintf("%d", t.Num),
			t.Title,
			t.State,
			string(t.Priority),
			assignee,
		}
	}
	m.table.SetRows(rows)
}

func (m *listViewModel) filteredTasks() []model.Task {
	var result []model.Task
	for _, t := range m.tasks {
		if t.Deleted {
			continue
		}
		if !m.showDone && isTerminalState(t.State) {
			continue
		}
		result = append(result, t)
	}
	return result
}

func isTerminalState(state string) bool {
	// Match the common terminal states. The kanban view uses workflow.IsTerminal
	// but we don't have the workflow here — these cover the defaults and the
	// product board's custom workflow.
	switch state {
	case "done", "cancelled", "shipped", "wontfix":
		return true
	}
	return false
}

func (m *listViewModel) sortTasks(tasks []model.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		var less bool
		switch m.sortBy {
		case sortNum:
			less = a.Num < b.Num
		case sortTitle:
			less = strings.ToLower(a.Title) < strings.ToLower(b.Title)
		case sortState:
			less = a.State < b.State
		case sortPriority:
			less = priorityRank(a.Priority) < priorityRank(b.Priority)
		case sortAssignee:
			aa, ba := assigneeStr(a), assigneeStr(b)
			less = aa < ba
		}
		if !m.sortAsc {
			less = !less
		}
		return less
	})
}

func assigneeStr(t model.Task) string {
	if t.Assignee != nil {
		return *t.Assignee
	}
	return "zzz" // sort unassigned to end
}

func (m *listViewModel) selectedTask() *model.Task {
	row := m.table.SelectedRow()
	if row == nil {
		return nil
	}
	var num int
	fmt.Sscanf(row[0], "%d", &num)
	for i, t := range m.tasks {
		if t.Num == num {
			return &m.tasks[i]
		}
	}
	return nil
}

func (m *listViewModel) update(msg tea.KeyMsg) {
	switch msg.String() {
	case "s":
		m.cycleSort()
	case "S":
		m.toggleSortDir()
	case "d":
		m.toggleDone()
	default:
		m.table, _ = m.table.Update(msg)
	}
}

func (m listViewModel) view(width, height int) string {
	if !m.ready {
		return dimStyle.Render("Loading...") + "\n"
	}
	m.table.SetWidth(width)
	m.table.SetHeight(height - 1)
	return m.table.View()
}

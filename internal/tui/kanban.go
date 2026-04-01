package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

// kanbanModel is the kanban board tab.
type kanbanModel struct {
	workflow        *workflow.Workflow
	tasks           []model.Task
	columns         []string // state names in workflow order
	colCursor       int      // selected column
	rowCursor       int      // selected task within column
	showDone        bool     // show terminal-state columns
	loading         bool
	err             error
	currentUserName string
}

// boardDataLoaded is sent when workflow + tasks are fetched for the kanban view.
type boardDataLoaded struct {
	workflow *workflow.Workflow
	tasks    []model.Task
	audit    []model.AuditEntry
	err      error
}

func fetchBoardData(client *httpclient.Client, slug string) tea.Cmd {
	return func() tea.Msg {
		p := httpclient.PathParams{"slug": slug}
		wf, err := httpclient.GetOne[workflow.Workflow](client, model.ResWorkflowGet, p, nil)
		if err != nil {
			return boardDataLoaded{err: err}
		}
		tasks, err := httpclient.GetMany[model.Task](client, model.ResTaskList, p, model.TaskFilter{IncludeClosed: true})
		if err != nil {
			return boardDataLoaded{err: err}
		}
		audit, _ := httpclient.Exec[[]model.AuditEntry](client, model.OpBoardAudit, p, nil)
		return boardDataLoaded{workflow: &wf, tasks: tasks, audit: audit}
	}
}

func newKanban(currentUserName string) kanbanModel {
	return kanbanModel{loading: true, currentUserName: currentUserName}
}

func (m *kanbanModel) load(data boardDataLoaded) {
	m.loading = false
	if data.err != nil {
		m.err = data.err
		return
	}
	m.workflow = data.workflow
	m.tasks = data.tasks

	// Build column list from workflow states.
	m.columns = nil
	for _, s := range m.workflow.States {
		if !m.showDone && m.workflow.IsTerminal(s) {
			continue
		}
		m.columns = append(m.columns, s)
	}
	m.colCursor = 0
	m.rowCursor = 0
}

func (m *kanbanModel) updateTask(task model.Task) {
	for i, t := range m.tasks {
		if t.BoardSlug == task.BoardSlug && t.Num == task.Num {
			m.tasks[i] = task
			return
		}
	}
	// New task — append.
	m.tasks = append(m.tasks, task)
}

func (m *kanbanModel) removeTask(boardSlug string, num int) {
	for i, t := range m.tasks {
		if t.BoardSlug == boardSlug && t.Num == num {
			m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
			return
		}
	}
}

func (m kanbanModel) tasksInColumn(state string) []model.Task {
	var result []model.Task
	for _, t := range m.tasks {
		if t.State == state && !t.Deleted {
			result = append(result, t)
		}
	}
	// Sort by priority (critical first), then by num.
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if priorityRank(result[i].Priority) > priorityRank(result[j].Priority) {
				result[i], result[j] = result[j], result[i]
			} else if priorityRank(result[i].Priority) == priorityRank(result[j].Priority) && result[i].Num > result[j].Num {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func (m *kanbanModel) update(msg tea.KeyMsg) {
	if m.loading || m.workflow == nil || len(m.columns) == 0 {
		return
	}
	switch msg.String() {
	case "left", "h":
		if m.colCursor > 0 {
			m.colCursor--
			m.rowCursor = 0
		}
	case "right", "l":
		if m.colCursor < len(m.columns)-1 {
			m.colCursor++
			m.rowCursor = 0
		}
	case "up", "k":
		if m.rowCursor > 0 {
			m.rowCursor--
		}
	case "down", "j":
		tasks := m.tasksInColumn(m.columns[m.colCursor])
		if m.rowCursor < len(tasks)-1 {
			m.rowCursor++
		}
	case "pgup":
		m.rowCursor -= 5
		if m.rowCursor < 0 {
			m.rowCursor = 0
		}
	case "pgdown":
		tasks := m.tasksInColumn(m.columns[m.colCursor])
		m.rowCursor += 5
		if m.rowCursor > len(tasks)-1 {
			m.rowCursor = len(tasks) - 1
		}
		if m.rowCursor < 0 {
			m.rowCursor = 0
		}
	case "home":
		m.rowCursor = 0
	case "end":
		tasks := m.tasksInColumn(m.columns[m.colCursor])
		if len(tasks) > 0 {
			m.rowCursor = len(tasks) - 1
		}
	case "d":
		m.showDone = !m.showDone
		m.columns = nil
		for _, s := range m.workflow.States {
			if !m.showDone && m.workflow.IsTerminal(s) {
				continue
			}
			m.columns = append(m.columns, s)
		}
		if m.colCursor >= len(m.columns) {
			m.colCursor = len(m.columns) - 1
		}
		m.rowCursor = 0
	}
}

func (m kanbanModel) selectedTask() *model.Task {
	if len(m.columns) == 0 {
		return nil
	}
	tasks := m.tasksInColumn(m.columns[m.colCursor])
	if m.rowCursor >= 0 && m.rowCursor < len(tasks) {
		return &tasks[m.rowCursor]
	}
	return nil
}

func cardStyleForWidth(w int, selected bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(w)
	if selected {
		s = s.BorderForeground(lipgloss.Color("39"))
	} else {
		s = s.BorderForeground(lipgloss.Color("241"))
	}
	return s
}

var (
	columnHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Padding(0, 1)
	priorityBadge     = map[model.Priority]string{
		model.PriorityCritical: "C",
		model.PriorityHigh:     "H",
		model.PriorityMedium:   "M",
		model.PriorityLow:      "L",
		model.PriorityNone:     " ",
	}
)

func (m kanbanModel) view(width, height int) string {
	if m.loading {
		return dimStyle.Render("Loading board...") + "\n"
	}
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n"
	}
	if len(m.columns) == 0 {
		return dimStyle.Render("No columns to display.") + "\n"
	}

	numCols := len(m.columns)
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	colWidth := width / numCols
	if colWidth < 20 {
		colWidth = 20
	}
	// Card width accounts for column padding.
	cardWidth := colWidth - 4
	if cardWidth < 16 {
		cardWidth = 16
	}

	// Each card is ~4 lines tall (2 content + 2 border).
	// Subtract 1 for the column header line.
	cardArea := height - 1
	if cardArea < 4 {
		cardArea = 4
	}
	maxVisible := cardArea / 4
	if maxVisible < 1 {
		maxVisible = 1
	}

	var cols []string
	for ci, state := range m.columns {
		tasks := m.tasksInColumn(state)
		isSelectedCol := ci == m.colCursor

		header := columnHeaderStyle.Width(colWidth).Render(fmt.Sprintf("%s (%d)", state, len(tasks)))

		// Compute scroll window for the selected column.
		scrollStart := 0
		if isSelectedCol && m.rowCursor >= maxVisible {
			scrollStart = m.rowCursor - maxVisible + 1
		}
		scrollEnd := scrollStart + maxVisible
		if scrollEnd > len(tasks) {
			scrollEnd = len(tasks)
		}

		var cards []string
		if scrollStart > 0 {
			cards = append(cards, dimStyle.Render(fmt.Sprintf("  ↑ %d more", scrollStart)))
		}
		for ri := scrollStart; ri < scrollEnd; ri++ {
			cards = append(cards, renderCard(tasks[ri], cardWidth, isSelectedCol && ri == m.rowCursor, m.currentUserName))
		}
		if scrollEnd < len(tasks) {
			cards = append(cards, dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(tasks)-scrollEnd)))
		}
		if len(tasks) == 0 {
			cards = append(cards, dimStyle.Width(colWidth).Render("  (empty)"))
		}

		col := lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, cards...)...)
		cols = append(cols, col)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func renderCard(task model.Task, width int, selected bool, currentUserName string) string {
	badge := priorityBadge[task.Priority]
	pStyle := priorityStyle[task.Priority]

	title := task.Title
	maxTitle := width - 8
	if maxTitle > 0 && len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	line1 := fmt.Sprintf("%s #%d %s", pStyle.Render("["+badge+"]"), task.Num, title)

	var line2 string
	switch {
	case task.Assignee == nil:
		line2 = dimStyle.Render("—")
	case currentUserName != "" && *task.Assignee == currentUserName:
		line2 = meStyle.Render("@me")
	default:
		line2 = dimStyle.Render("@" + *task.Assignee)
	}

	return cardStyleForWidth(width, selected).Render(line1 + "\n" + line2)
}

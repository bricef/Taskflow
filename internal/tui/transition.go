package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

var (
	transitionBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)
	transitionSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
)

// transitionModel is an overlay for transitioning a task.
type transitionModel struct {
	boardSlug    string
	taskNum      int
	taskTitle    string
	currentState string
	transitions  []workflow.Transition
	cursor       int
	filter       textinput.Model
	err          string
}

type transitionResult struct {
	err error
}

func newTransition(client *httpclient.Client, boardSlug string, task model.Task) (*transitionModel, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 50
	ti.Width = 30
	ti.Focus()

	m := &transitionModel{
		boardSlug:    boardSlug,
		taskNum:      task.Num,
		taskTitle:    task.Title,
		currentState: task.State,
		filter:       ti,
	}

	// Fetch available transitions.
	return m, func() tea.Msg {
		wf, err := httpclient.GetOne[workflow.Workflow](client, model.ResWorkflowGet, httpclient.PathParams{"slug": boardSlug}, nil)
		if err != nil {
			return transitionsLoaded{err: err}
		}
		return transitionsLoaded{transitions: wf.AvailableTransitions(task.State)}
	}
}

type transitionsLoaded struct {
	transitions []workflow.Transition
	err         error
}

func (m *transitionModel) filteredTransitions() []workflow.Transition {
	query := strings.ToLower(m.filter.Value())
	if query == "" {
		return m.transitions
	}
	var result []workflow.Transition
	for _, t := range m.transitions {
		if strings.Contains(strings.ToLower(t.Name), query) || strings.Contains(strings.ToLower(t.To), query) {
			result = append(result, t)
		}
	}
	return result
}

func (m *transitionModel) update(msg tea.Msg, client *httpclient.Client) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case transitionsLoaded:
		if msg.err != nil {
			m.err = msg.err.Error()
			return false, nil
		}
		m.transitions = msg.transitions
		if len(m.transitions) == 0 {
			m.err = "no transitions available from " + m.currentState
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.filter.Value() != "" {
				m.filter.SetValue("")
				m.cursor = 0
				return false, nil
			}
			return true, nil // close
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return false, nil
		case "down":
			filtered := m.filteredTransitions()
			if m.cursor < len(filtered)-1 {
				m.cursor++
			}
			return false, nil
		case "enter":
			filtered := m.filteredTransitions()
			if m.cursor < len(filtered) {
				tr := filtered[m.cursor]
				return false, executeTransition(client, m.boardSlug, m.taskNum, tr.Name)
			}
			return false, nil
		}

		// Pass remaining keys to the text input for filtering.
		prevValue := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != prevValue {
			m.cursor = 0
		}
		return false, cmd

	case transitionResult:
		if msg.err != nil {
			m.err = msg.err.Error()
			return false, nil
		}
		return true, nil // success — close
	}

	return false, nil
}

func (m transitionModel) view(width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Transition %s/%d", m.boardSlug, m.taskNum)) + "\n")
	b.WriteString(dimStyle.Render(m.taskTitle) + "\n\n")
	b.WriteString(fmt.Sprintf("Current state: %s\n\n", m.currentState))

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}

	if len(m.transitions) == 0 && m.err == "" {
		b.WriteString(dimStyle.Render("Loading...") + "\n")
	}

	if len(m.transitions) > 0 {
		b.WriteString(m.filter.View() + "\n\n")
	}

	filtered := m.filteredTransitions()
	for i, tr := range filtered {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = transitionSelected
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%s → %s", cursor, tr.Name, tr.To)) + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("enter confirm  esc cancel"))

	boxWidth := width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	return transitionBorder.Width(boxWidth).Render(b.String())
}

func executeTransition(client *httpclient.Client, boardSlug string, num int, transition string) tea.Cmd {
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(num)}
		err := httpclient.ExecNoResult(client, model.OpTaskTransition, tp, map[string]string{"transition": transition})
		if err != nil {
			return transitionResult{err: err}
		}
		return transitionResult{}
	}
}

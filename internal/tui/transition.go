package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
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
	actorName    string // current authenticated user
	transitions  []workflow.Transition
	cursor       int
	filter       textinput.Model
	err          string

	// Comment phase: after selecting a transition, prompt for an optional comment.
	commenting bool
	selected   *workflow.Transition
	comment    textarea.Model
}

type transitionResult struct {
	err error
}

func newTransition(client *httpclient.Client, boardSlug string, task model.Task, actorName string) (*transitionModel, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 50
	ti.Width = 30
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "Add a comment (optional)..."
	ta.CharLimit = 500
	ta.MaxWidth = 60
	ta.MaxHeight = 4
	ta.ShowLineNumbers = false

	m := &transitionModel{
		boardSlug:    boardSlug,
		taskNum:      task.Num,
		taskTitle:    task.Title,
		currentState: task.State,
		actorName:    actorName,
		filter:       ti,
		comment:      ta,
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

func (m *transitionModel) enterCommentPhase(tr workflow.Transition) tea.Cmd {
	m.commenting = true
	m.selected = &tr
	m.comment.SetValue("")
	return m.comment.Focus()
}

func (m *transitionModel) execute(client *httpclient.Client) tea.Cmd {
	userComment := strings.TrimSpace(m.comment.Value())
	tr := m.selected
	boardSlug := m.boardSlug
	taskNum := m.taskNum
	taskRef := fmt.Sprintf("%s/%d", boardSlug, taskNum)
	actor := m.actorName
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(taskNum)}
		err := httpclient.ExecNoResult(client, model.OpTaskTransition, tp, map[string]string{"transition": tr.Name})
		if err != nil {
			return transitionResult{err: err}
		}
		summary := fmt.Sprintf("%s transitioned %s to %s", actor, taskRef, tr.To)
		body := summary
		if userComment != "" {
			body = summary + "\n\n" + userComment
		}
		httpclient.Exec[model.Comment](client, model.OpCommentCreate, tp, map[string]string{"body": body})
		return transitionResult{}
	}
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
		if m.commenting {
			switch msg.String() {
			case "ctrl+c":
				return true, nil
			case "esc":
				// Cancel the action entirely.
				m.commenting = false
				m.selected = nil
				return true, nil
			case "enter":
				// Confirm — execute with comment.
				return false, m.execute(client)
			case "ctrl+j":
				// Insert newline in comment.
				m.comment.InsertString("\n")
				return false, nil
			default:
				var cmd tea.Cmd
				m.comment, cmd = m.comment.Update(msg)
				return false, cmd
			}
		}

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
				return false, m.enterCommentPhase(filtered[m.cursor])
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
			m.commenting = false
			m.selected = nil
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

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}

	if m.commenting && m.selected != nil {
		b.WriteString(fmt.Sprintf("%s → %s  %s\n\n",
			m.currentState,
			transitionSelected.Render(m.selected.To),
			dimStyle.Render("["+m.selected.Name+"]")))
		b.WriteString(m.comment.View() + "\n\n")
		b.WriteString(dimStyle.Render("enter confirm  ctrl+j newline  esc cancel"))
	} else {
		b.WriteString(fmt.Sprintf("Current state: %s\n\n", m.currentState))

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
	}

	boxWidth := width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	return transitionBorder.Width(boxWidth).Render(b.String())
}

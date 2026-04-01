package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
)

const assignMeSentinel = "@me"

// assignModel is an overlay for assigning a task to an actor.
type assignModel struct {
	boardSlug       string
	taskNum         int
	taskTitle       string
	currentAssignee string
	currentUser     *model.Actor
	actors          []string // actor names; first entry is "(unassign)", second may be "@me"
	cursor          int
	filter          textinput.Model
	err             string
}

type actorsLoaded struct {
	actors []model.Actor
	err    error
}

type assignResult struct {
	task model.Task
	err  error
}

func newAssign(client *httpclient.Client, boardSlug string, task model.Task, currentUser *model.Actor) (*assignModel, tea.Cmd) {
	current := "unassigned"
	if task.Assignee != nil {
		current = *task.Assignee
	}
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 50
	ti.Width = 30
	ti.Focus()

	m := &assignModel{
		boardSlug:       boardSlug,
		taskNum:         task.Num,
		taskTitle:       task.Title,
		currentAssignee: current,
		currentUser:     currentUser,
		filter:          ti,
	}
	return m, func() tea.Msg {
		actors, err := httpclient.GetMany[model.Actor](client, model.ResActorList, nil, nil)
		return actorsLoaded{actors: actors, err: err}
	}
}

func (m *assignModel) filteredActors() []string {
	query := strings.ToLower(m.filter.Value())
	if query == "" {
		return m.actors
	}
	var result []string
	for _, name := range m.actors {
		display := name
		if name == assignMeSentinel && m.currentUser != nil {
			display = fmt.Sprintf("@me (%s)", m.currentUser.Name)
		}
		if strings.Contains(strings.ToLower(display), query) {
			result = append(result, name)
		}
	}
	return result
}

func (m *assignModel) update(msg tea.Msg, client *httpclient.Client) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case actorsLoaded:
		if msg.err != nil {
			m.err = msg.err.Error()
			return false, nil
		}
		m.actors = []string{"(unassign)"}
		if m.currentUser != nil {
			m.actors = append(m.actors, assignMeSentinel)
		}
		for _, a := range msg.actors {
			if a.Active && (m.currentUser == nil || a.Name != m.currentUser.Name) {
				m.actors = append(m.actors, a.Name)
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.filter.Value() != "" {
				m.filter.SetValue("")
				m.cursor = 0
				return false, nil
			}
			return true, nil
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return false, nil
		case "down":
			filtered := m.filteredActors()
			if m.cursor < len(filtered)-1 {
				m.cursor++
			}
			return false, nil
		case "enter":
			filtered := m.filteredActors()
			if m.cursor < len(filtered) {
				selected := filtered[m.cursor]
				var assignee *string
				if selected != "(unassign)" {
					name := selected
					if name == assignMeSentinel && m.currentUser != nil {
						name = m.currentUser.Name
					}
					assignee = &name
				}
				return false, executeAssign(client, m.boardSlug, m.taskNum, assignee)
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

	case assignResult:
		if msg.err != nil {
			m.err = msg.err.Error()
			return false, nil
		}
		return true, nil
	}

	return false, nil
}

func (m assignModel) view(width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Assign %s/%d", m.boardSlug, m.taskNum)) + "\n")
	b.WriteString(dimStyle.Render(m.taskTitle) + "\n\n")
	b.WriteString(fmt.Sprintf("Currently: %s\n\n", m.currentAssignee))

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}

	if len(m.actors) == 0 && m.err == "" {
		b.WriteString(dimStyle.Render("Loading...") + "\n")
	}

	if len(m.actors) > 0 {
		b.WriteString(m.filter.View() + "\n\n")
	}

	filtered := m.filteredActors()
	for i, name := range filtered {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		}
		display := name
		if name == assignMeSentinel && m.currentUser != nil {
			display = fmt.Sprintf("@me (%s)", m.currentUser.Name)
		}
		b.WriteString(style.Render(cursor+display) + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("enter confirm  esc cancel"))

	boxWidth := width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	return transitionBorder.Width(boxWidth).Render(b.String())
}

func executeAssign(client *httpclient.Client, boardSlug string, num int, assignee *string) tea.Cmd {
	return func() tea.Msg {
		tp := httpclient.PathParams{"slug": boardSlug, "num": fmt.Sprint(num)}
		task, err := httpclient.Exec[model.Task](client, model.OpTaskUpdate, tp, map[string]any{"assignee": assignee})
		return assignResult{task: task, err: err}
	}
}

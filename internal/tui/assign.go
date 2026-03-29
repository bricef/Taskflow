package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

// assignModel is an overlay for assigning a task to an actor.
type assignModel struct {
	boardSlug    string
	taskNum      int
	taskTitle    string
	currentAssignee string
	actors       []string // actor names; first entry is "(unassign)"
	cursor       int
	err          string
}

type actorsLoaded struct {
	actors []model.Actor
	err    error
}

type assignResult struct {
	task model.Task
	err  error
}

func newAssign(client *Client, boardSlug string, task model.Task) (*assignModel, tea.Cmd) {
	current := "unassigned"
	if task.Assignee != nil {
		current = *task.Assignee
	}
	m := &assignModel{
		boardSlug:       boardSlug,
		taskNum:         task.Num,
		taskTitle:       task.Title,
		currentAssignee: current,
	}
	return m, func() tea.Msg {
		actors, err := client.ListActors()
		return actorsLoaded{actors: actors, err: err}
	}
}

func (m *assignModel) update(msg tea.Msg, client *Client, apiKey string) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case actorsLoaded:
		if msg.err != nil {
			m.err = msg.err.Error()
			return false, nil
		}
		m.actors = []string{"(unassign)"}
		for _, a := range msg.actors {
			if a.Active {
				m.actors = append(m.actors, a.Name)
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace":
			return true, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.actors)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.actors) {
				var assignee *string
				if m.cursor > 0 {
					name := m.actors[m.cursor]
					assignee = &name
				}
				return false, executeAssign(client, m.boardSlug, m.taskNum, assignee)
			}
		}

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

	for i, name := range m.actors {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		}
		b.WriteString(style.Render(cursor+name) + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("enter confirm  esc cancel"))

	boxWidth := width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	return transitionBorder.Width(boxWidth).Render(b.String())
}

func executeAssign(client *Client, boardSlug string, num int, assignee *string) tea.Cmd {
	return func() tea.Msg {
		task, err := client.AssignTask(boardSlug, num, assignee)
		return assignResult{task: task, err: err}
	}
}

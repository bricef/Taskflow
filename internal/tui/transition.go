package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	boardSlug   string
	taskNum     int
	taskTitle   string
	currentState string
	transitions []workflow.Transition
	cursor      int
	err         string
}

type transitionResult struct {
	err error
}

func newTransition(client *Client, boardSlug string, task model.Task) (*transitionModel, tea.Cmd) {
	m := &transitionModel{
		boardSlug:    boardSlug,
		taskNum:      task.Num,
		taskTitle:    task.Title,
		currentState: task.State,
	}

	// Fetch available transitions.
	return m, func() tea.Msg {
		wf, err := client.GetWorkflow(boardSlug)
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

func (m *transitionModel) update(msg tea.Msg, client *Client, apiKey string) (bool, tea.Cmd) {
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
		case "esc", "backspace":
			return true, nil // close
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.transitions)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.transitions) {
				tr := m.transitions[m.cursor]
				return false, executeTransition(client, apiKey, m.boardSlug, m.taskNum, tr.Name)
			}
		}

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

	for i, tr := range m.transitions {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = transitionSelected
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%s → %s", cursor, tr.Name, tr.To)) + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("enter confirm  esc cancel"))

	boxWidth := width/2
	if boxWidth < 40 {
		boxWidth = 40
	}
	return transitionBorder.Width(boxWidth).Render(b.String())
}

func executeTransition(client *Client, apiKey, boardSlug string, num int, transition string) tea.Cmd {
	return func() tea.Msg {
		body, _ := json.Marshal(map[string]string{"transition": transition})
		url := fmt.Sprintf("%s/boards/%s/tasks/%d/transition", client.baseURL, boardSlug, num)
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return transitionResult{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errResp map[string]any
			json.NewDecoder(resp.Body).Decode(&errResp)
			msg := fmt.Sprintf("status %d", resp.StatusCode)
			if m, ok := errResp["message"].(string); ok {
				msg = m
			}
			return transitionResult{err: fmt.Errorf("%s", msg)}
		}
		return transitionResult{}
	}
}

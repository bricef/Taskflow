package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

// boardsLoaded is sent when the board list has been fetched.
type boardsLoaded struct {
	boards []model.Board
	err    error
}

func fetchBoards(client *Client) tea.Cmd {
	return func() tea.Msg {
		boards, err := client.ListBoards()
		return boardsLoaded{boards: boards, err: err}
	}
}

// selectorModel is the board selector view.
type selectorModel struct {
	boards  []model.Board
	cursor  int
	filter  string
	err     error
	loading bool
}

func newSelector() selectorModel {
	return selectorModel{loading: true}
}

func (m selectorModel) filteredBoards() []model.Board {
	if m.filter == "" {
		return m.boards
	}
	f := strings.ToLower(m.filter)
	var result []model.Board
	for _, b := range m.boards {
		if strings.Contains(strings.ToLower(b.Slug), f) || strings.Contains(strings.ToLower(b.Name), f) {
			result = append(result, b)
		}
	}
	return result
}

func (m selectorModel) selectedBoard() *model.Board {
	filtered := m.filteredBoards()
	if m.cursor >= 0 && m.cursor < len(filtered) {
		return &filtered[m.cursor]
	}
	return nil
}

func (m selectorModel) update(msg tea.Msg) (selectorModel, *model.Board) {
	switch msg := msg.(type) {
	case boardsLoaded:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.boards = msg.boards
			m.cursor = 0
		}

	case tea.KeyMsg:
		filtered := m.filteredBoards()
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(filtered)-1 {
				m.cursor++
			}
		case "enter":
			if b := m.selectedBoard(); b != nil {
				return m, b
			}
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.cursor = 0
			}
		case "esc":
			m.filter = ""
			m.cursor = 0
		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m.cursor = 0
			}
		}
	}

	return m, nil
}

func (m selectorModel) view(width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("TaskFlow — Select a board") + "\n\n")

	if m.loading {
		b.WriteString(dimStyle.Render("Loading boards...") + "\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n")
		return b.String()
	}

	if m.filter != "" {
		b.WriteString(dimStyle.Render("Filter: ") + m.filter + "\n\n")
	}

	filtered := m.filteredBoards()
	if len(filtered) == 0 {
		if m.filter != "" {
			b.WriteString(dimStyle.Render("No boards match filter.") + "\n")
		} else {
			b.WriteString(dimStyle.Render("No boards found. Create one with: taskflow board create --slug <slug> --name <name>") + "\n")
		}
		return b.String()
	}

	for i, board := range filtered {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = style.Bold(true).Foreground(lipgloss.Color("39"))
		}
		line := fmt.Sprintf("%s%-20s %s", cursor, board.Slug, dimStyle.Render(board.Name))
		b.WriteString(style.Render(line) + "\n")
	}

	return b.String()
}

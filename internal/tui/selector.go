package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

// boardsLoaded is sent when the board list has been fetched.
type boardsLoaded struct {
	boards []model.Board
	err    error
}

// boardCreated is sent when a new board has been created.
type boardCreated struct {
	board model.Board
	err   error
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

	// Create form state.
	creating   bool
	formField  int // 0 = slug, 1 = name
	slugInput  textinput.Model
	nameInput  textinput.Model
	createErr  string
}

func newSelector() selectorModel {
	slug := textinput.New()
	slug.Placeholder = "my-board"
	slug.CharLimit = 32
	slug.Width = 32

	name := textinput.New()
	name.Placeholder = "My Board"
	name.CharLimit = 100
	name.Width = 40

	return selectorModel{
		loading:   true,
		slugInput: slug,
		nameInput: name,
	}
}

func (m *selectorModel) startCreate() tea.Cmd {
	m.creating = true
	m.formField = 0
	m.slugInput.SetValue("")
	m.nameInput.SetValue("")
	m.createErr = ""
	m.slugInput.Focus()
	return m.slugInput.Focus()
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

func (m selectorModel) update(msg tea.Msg, client *Client) (selectorModel, *model.Board, tea.Cmd) {
	switch msg := msg.(type) {
	case boardsLoaded:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.boards = msg.boards
			m.cursor = 0
		}

	case boardCreated:
		if msg.err != nil {
			m.createErr = msg.err.Error()
		} else {
			m.creating = false
			return m, &msg.board, nil
		}

	case tea.KeyMsg:
		if m.creating {
			return m.updateCreateForm(msg, client)
		}

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
				return m, b, nil
			}
		case "n":
			cmd := m.startCreate()
			return m, nil, cmd
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

	return m, nil, nil
}

func (m selectorModel) updateCreateForm(msg tea.KeyMsg, client *Client) (selectorModel, *model.Board, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating = false
		m.createErr = ""
		return m, nil, nil
	case "tab", "down":
		if m.formField == 0 {
			m.formField = 1
			m.slugInput.Blur()
			return m, nil, m.nameInput.Focus()
		}
	case "shift+tab", "up":
		if m.formField == 1 {
			m.formField = 0
			m.nameInput.Blur()
			return m, nil, m.slugInput.Focus()
		}
	case "enter":
		slug := strings.TrimSpace(m.slugInput.Value())
		name := strings.TrimSpace(m.nameInput.Value())
		if slug == "" {
			m.createErr = "Slug is required"
			return m, nil, nil
		}
		if name == "" {
			name = slug
		}
		return m, nil, func() tea.Msg {
			board, err := client.CreateBoard(slug, name)
			return boardCreated{board: board, err: err}
		}
	}

	// Delegate to the active input.
	var cmd tea.Cmd
	if m.formField == 0 {
		m.slugInput, cmd = m.slugInput.Update(msg)
	} else {
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, nil, cmd
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

	if m.creating {
		b.WriteString(detailSectionStyle.Render("New Board") + "\n\n")
		b.WriteString(fmt.Sprintf("  %s %s\n", detailFieldLabel.Render("Slug:"), m.slugInput.View()))
		b.WriteString(fmt.Sprintf("  %s %s\n", detailFieldLabel.Render("Name:"), m.nameInput.View()))
		if m.createErr != "" {
			b.WriteString("\n" + errorStyle.Render("  "+m.createErr) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("  enter submit  tab next field  esc cancel") + "\n")
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
			b.WriteString(dimStyle.Render("No boards found. Press n to create one.") + "\n")
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

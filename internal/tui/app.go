package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

// Config holds the TUI configuration.
type Config struct {
	ServerURL string
	APIKey    string
	BoardSlug string        // if set, skip the selector
	Program   **tea.Program // set after tea.NewProgram; pointer-to-pointer so the model copy shares it
}

type viewMode int

const (
	viewSelector viewMode = iota
	viewBoard
)

type boardTab int

const (
	tabEventLog boardTab = iota
	tabCount             // keep last for cycling
)

var tabNames = []string{"Events"}

// Model is the root Bubble Tea model.
type Model struct {
	cfg    Config
	client *Client
	view   viewMode
	width  int
	height int

	// Board selector
	selector selectorModel

	// Board view (active after selecting a board)
	activeBoard *model.Board
	activeTab   boardTab
	sseStatus   string
	lastError   string
	eventLog    eventLogModel
}

// New creates a new TUI model.
func New(cfg Config) Model {
	client := NewClient(cfg.ServerURL, cfg.APIKey)
	m := Model{
		cfg:       cfg,
		client:    client,
		selector:  newSelector(),
		view:      viewSelector,
		sseStatus: "disconnected",
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.cfg.BoardSlug != "" {
		// Skip selector — go directly to board.
		return func() tea.Msg {
			board, err := m.client.GetBoard(m.cfg.BoardSlug)
			if err != nil {
				return boardsLoaded{err: err}
			}
			return boardSelected{board: board}
		}
	}
	return fetchBoards(m.client)
}

// boardSelected is sent when a board is chosen.
type boardSelected struct {
	board model.Board
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.view == viewSelector {
				return m, tea.Quit
			}
			// From board view, q goes back to selector.
			m.view = viewSelector
			m.activeBoard = nil
			m.sseStatus = "disconnected"
			m.eventLog = eventLogModel{}
			return m, fetchBoards(m.client)
		case "tab":
			if m.view == viewBoard {
				m.activeTab = (m.activeTab + 1) % tabCount
			}
		case "b":
			if m.view == viewBoard {
				m.view = viewSelector
				return m, fetchBoards(m.client)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case boardSelected:
		m.activeBoard = &msg.board
		m.view = viewBoard
		m.activeTab = tabEventLog
		m.sseStatus = "connecting..."
		m.eventLog = eventLogModel{}
		if m.cfg.Program != nil && *m.cfg.Program != nil {
			startSSE(*m.cfg.Program, m.cfg.ServerURL, msg.board.Slug, m.cfg.APIKey)
		}
		return m, nil

	case SSEConnected:
		m.sseStatus = "live"
		m.lastError = ""
		return m, nil

	case SSEEvent:
		m.eventLog.addEvent(msg.Event)
		return m, nil

	case SSEError:
		m.lastError = msg.Err.Error()
		if msg.Permanent {
			m.sseStatus = "error"
		} else {
			m.sseStatus = "reconnecting..."
		}
		return m, nil
	}

	// Delegate to sub-views.
	if m.view == viewSelector {
		var selected *model.Board
		m.selector, selected = m.selector.update(msg)
		if selected != nil {
			return m, func() tea.Msg { return boardSelected{board: *selected} }
		}
	}

	return m, nil
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	eventStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	tabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
)

func (m Model) View() string {
	switch m.view {
	case viewSelector:
		return m.selector.view(m.width)
	case viewBoard:
		return m.boardView()
	}
	return ""
}

func (m Model) boardView() string {
	var b strings.Builder

	// Header with board name and SSE status.
	var status string
	switch m.sseStatus {
	case "live":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Render("● live")
	case "error":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✕ error")
	default:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◌ " + m.sseStatus)
	}

	boardName := m.activeBoard.Slug
	if m.activeBoard.Name != "" {
		boardName = m.activeBoard.Name
	}
	header := fmt.Sprintf("%s  %s", titleStyle.Render("TaskFlow — "+boardName), status)
	b.WriteString(header + "\n")

	// Tabs
	var tabs []string
	for i, name := range tabNames {
		if boardTab(i) == m.activeTab {
			tabs = append(tabs, tabActive.Render("["+name+"]"))
		} else {
			tabs = append(tabs, tabInactive.Render(" "+name+" "))
		}
	}
	b.WriteString(strings.Join(tabs, "") + "\n")

	if m.lastError != "" {
		b.WriteString(errorStyle.Render(m.lastError) + "\n")
	}

	b.WriteString("\n")

	// Tab content
	switch m.activeTab {
	case tabEventLog:
		b.WriteString(m.eventLog.view(m.height))
	}

	// Footer
	b.WriteString("\n" + dimStyle.Render("Tab switch view  b boards  q back  ctrl+c quit") + "\n")
	return b.String()
}

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/viewport"
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
	tabKanban boardTab = iota
	tabEventLog
	tabCount // keep last for cycling
)

var tabNames = []string{"Board", "Events"}

// chromeLines is the total number of lines used by header, tabs, padding, and footer
// that are rendered outside the viewport. Header(1) + Tabs(1) + padding(1) + footer newline(1) + help(1) = 5
const chromeLines = 5

// Model is the root Bubble Tea model.
type Model struct {
	cfg      Config
	client   *Client
	help     help.Model
	viewport viewport.Model
	view     viewMode
	width    int
	height   int

	// Board selector
	selector selectorModel

	// Board view (active after selecting a board)
	activeBoard *model.Board
	activeTab   boardTab
	sseStatus   string
	lastError   string
	eventLog    eventLogModel
	kanban      kanbanModel
}

// New creates a new TUI model.
func New(cfg Config) Model {
	client := NewClient(cfg.ServerURL, cfg.APIKey)
	return Model{
		cfg:       cfg,
		client:    client,
		help:      newHelp(),
		viewport:  viewport.New(80, 20),
		selector:  newSelector(),
		view:      viewSelector,
		sseStatus: "disconnected",
	}
}

func (m Model) Init() tea.Cmd {
	if m.cfg.BoardSlug != "" {
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

func (m *Model) resizeViewport() {
	extra := 0
	if m.lastError != "" {
		extra = 1
	}
	contentHeight := m.height - chromeLines - extra
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.viewport.Width = m.width
	m.viewport.Height = contentHeight
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.view == viewBoard {
				m.view = viewSelector
				m.activeBoard = nil
				m.sseStatus = "disconnected"
				m.eventLog = eventLogModel{}
				return m, fetchBoards(m.client)
			}
		case "tab":
			if m.view == viewBoard {
				m.activeTab = (m.activeTab + 1) % tabCount
				m.viewport.GotoTop()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.resizeViewport()
		return m, nil

	case boardSelected:
		m.activeBoard = &msg.board
		m.view = viewBoard
		m.activeTab = tabKanban
		m.sseStatus = "connecting..."
		m.eventLog = eventLogModel{}
		m.kanban = newKanban()
		m.resizeViewport()
		if m.cfg.Program != nil && *m.cfg.Program != nil {
			startSSE(*m.cfg.Program, m.cfg.ServerURL, msg.board.Slug, m.cfg.APIKey)
		}
		return m, fetchBoardData(m.client, msg.board.Slug)

	case boardDataLoaded:
		m.kanban.load(msg)
		m.eventLog.seedFromAudit(msg.audit)
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
	switch {
	case m.view == viewSelector:
		var selected *model.Board
		m.selector, selected = m.selector.update(msg)
		if selected != nil {
			return m, func() tea.Msg { return boardSelected{board: *selected} }
		}
	case m.view == viewBoard && m.activeTab == tabKanban:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			m.kanban.update(keyMsg)
		}
	case m.view == viewBoard && m.activeTab == tabEventLog:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "up", "k":
				m.eventLog.moveUp()
			case "down", "j":
				m.eventLog.moveDown()
			}
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
		return m.selectorView()
	case viewBoard:
		return m.boardView()
	}
	return ""
}

func (m Model) selectorView() string {
	var b strings.Builder
	b.WriteString(m.selector.view(m.width))
	b.WriteString("\n" + m.help.View(selectorKeyMap))
	return b.String()
}

func (m Model) boardView() string {
	var b strings.Builder

	// Header.
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
	b.WriteString(fmt.Sprintf("%s  %s", titleStyle.Render("TaskFlow — "+boardName), status) + "\n")

	// Tabs.
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

	// Tab content rendered into the viewport.
	var content string
	switch m.activeTab {
	case tabKanban:
		content = m.kanban.view(m.viewport.Width, m.viewport.Height)
	case tabEventLog:
		content = m.eventLog.view(m.viewport.Width, m.viewport.Height)
	}
	m.viewport.SetContent(content)
	b.WriteString(m.viewport.View())
	// Tab-specific help.
	var keyMap help.KeyMap
	switch m.activeTab {
	case tabKanban:
		keyMap = kanbanKeyMap
	case tabEventLog:
		keyMap = eventLogKeyMap
	}
	b.WriteString("\n" + m.help.View(keyMap))
	return b.String()
}

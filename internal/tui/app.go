package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/eventbus"
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

// chromeFixed is the lines used by header, tabs, and footer newline
// (excludes help, which varies by view). Header(1) + Tabs(1) + footer newline(1) = 3
const chromeFixed = 3

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
	detail     *detailModel     // non-nil when detail overlay is open
	transition *transitionModel // non-nil when transition overlay is open
	assign     *assignModel     // non-nil when assign overlay is open
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

func (m *Model) selectedTaskFromContext() *model.Task {
	if m.detail != nil && m.detail.data != nil {
		return &m.detail.data.task
	}
	return m.kanban.selectedTask()
}

func (m *Model) openAssignFromContext() tea.Cmd {
	if m.activeBoard == nil {
		return nil
	}
	task := m.selectedTaskFromContext()
	if task == nil {
		return nil
	}
	am, cmd := newAssign(m.client, m.activeBoard.Slug, *task)
	m.assign = am
	return cmd
}

func (m *Model) openTransitionFromContext() tea.Cmd {
	if m.activeBoard == nil {
		return nil
	}
	task := m.selectedTaskFromContext()
	if task == nil {
		return nil
	}
	tm, cmd := newTransition(m.client, m.activeBoard.Slug, *task)
	m.transition = tm
	return cmd
}

func (m *Model) openDetail() tea.Cmd {
	var boardSlug string
	var num int

	if m.activeBoard != nil {
		boardSlug = m.activeBoard.Slug
	}

	switch m.activeTab {
	case tabKanban:
		if t := m.kanban.selectedTask(); t != nil {
			num = t.Num
		}
	case tabEventLog:
		// Try to get task num from the selected event.
		if m.eventLog.cursor >= 0 && m.eventLog.cursor < len(m.eventLog.entries) {
			entry := m.eventLog.entries[m.eventLog.cursor]
			if entry.event != nil && entry.event.Task != nil {
				// Parse num from ref "board/num".
				ref := entry.event.Task.Ref
				if idx := strings.LastIndex(ref, "/"); idx >= 0 {
					fmt.Sscanf(ref[idx+1:], "%d", &num)
				}
			} else if entry.audit != nil && entry.audit.TaskNum != nil {
				num = *entry.audit.TaskNum
			}
		}
	}

	if boardSlug == "" || num == 0 {
		return nil
	}

	m.detail = &detailModel{loading: true}
	return fetchTaskDetail(m.client, boardSlug, num)
}

func (m *Model) applySSEToKanban(evt eventbus.Event) {
	if evt.Task == nil || m.activeBoard == nil {
		return
	}
	ref := evt.Task.Ref
	boardSlug := m.activeBoard.Slug

	switch evt.Type {
	case eventbus.EventTaskCreated:
		// Add a minimal task from the event data — enough to display a card.
		m.kanban.updateTask(model.Task{
			BoardSlug: boardSlug,
			Num:       parseNumFromRef(ref),
			Title:     evt.Task.Title,
			State:     evt.Task.State,
			Priority:  model.PriorityNone,
		})
	case eventbus.EventTaskTransitioned:
		m.kanban.updateTaskState(parseNumFromRef(ref), evt.Task.State)
	case eventbus.EventTaskUpdated, eventbus.EventTaskAssigned:
		// For updates we don't have the full data — refetch would be ideal
		// but for now just update state if present.
		if evt.Task.State != "" {
			m.kanban.updateTaskState(parseNumFromRef(ref), evt.Task.State)
		}
	case eventbus.EventTaskDeleted:
		m.kanban.removeTask(boardSlug, parseNumFromRef(ref))
	}
}

func parseNumFromRef(ref string) int {
	var num int
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		fmt.Sscanf(ref[idx+1:], "%d", &num)
	}
	return num
}

func (m *Model) resizeViewport() {
	helpHeight := strings.Count(m.help.View(m.activeKeyMap()), "\n") + 1
	contentHeight := m.height - chromeFixed - helpHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.viewport.Width = m.width
	m.viewport.Height = contentHeight
}

func (m Model) activeKeyMap() help.KeyMap {
	if m.detail != nil && m.detail.commenting {
		return commentKeyMap
	}
	if m.detail != nil {
		return detailKeyMap
	}
	switch m.activeTab {
	case tabKanban:
		return kanbanKeyMap
	case tabEventLog:
		return eventLogKeyMap
	}
	return kanbanKeyMap
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// When the comment input is active, all keys go to the textarea.
		if m.detail != nil && m.detail.commenting {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.detail.commenting = false
				m.detail.postErr = ""
				return m, nil
			case "ctrl+d":
				cmd := m.detail.submitComment(m.client, m.cfg.APIKey)
				return m, cmd
			default:
				cmd := m.detail.update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "?":
			m.help.ShowAll = !m.help.ShowAll
			m.resizeViewport()
			return m, nil
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "backspace":
			if m.assign != nil {
				m.assign = nil
				return m, nil
			}
			if m.transition != nil {
				m.transition = nil
				return m, nil
			}
			if m.detail != nil {
				m.detail = nil
				return m, nil
			}
			if m.view == viewBoard {
				m.view = viewSelector
				m.activeBoard = nil
				m.sseStatus = "disconnected"
				m.eventLog = eventLogModel{}
				return m, fetchBoards(m.client)
			}
		case "tab":
			if m.view == viewBoard && m.detail == nil {
				m.activeTab = (m.activeTab + 1) % tabCount
				m.viewport.GotoTop()
			}
		case "t":
			if m.view == viewBoard && m.transition == nil && m.assign == nil {
				return m, m.openTransitionFromContext()
			}
		case "a":
			if m.view == viewBoard && m.assign == nil && m.transition == nil {
				return m, m.openAssignFromContext()
			}
		case "c":
			if m.detail != nil && m.detail.data != nil && !m.detail.commenting {
				m.detail.startComment()
				return m, m.detail.input.Focus()
			}
		case "enter":
			if m.view == viewBoard && m.detail == nil && m.transition == nil && m.assign == nil {
				return m, m.openDetail()
			}
		}

		// When an overlay is open, delegate to it.
		if m.assign != nil {
			closed, cmd := m.assign.update(msg, m.client, m.cfg.APIKey)
			if closed {
				m.assign = nil
			}
			return m, cmd
		}
		if m.transition != nil {
			closed, cmd := m.transition.update(msg, m.client, m.cfg.APIKey)
			if closed {
				m.transition = nil
			}
			return m, cmd
		}
		if m.detail != nil {
			return m, nil
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

	case actorsLoaded, assignResult:
		if m.assign != nil {
			closed, cmd := m.assign.update(msg, m.client, m.cfg.APIKey)
			if closed {
				m.assign = nil
			}
			return m, cmd
		}
		return m, nil

	case transitionsLoaded, transitionResult:
		if m.transition != nil {
			closed, cmd := m.transition.update(msg, m.client, m.cfg.APIKey)
			if closed {
				m.transition = nil
			}
			return m, cmd
		}
		return m, nil

	case commentPosted:
		if m.detail != nil {
			if msg.err != nil {
				m.detail.postErr = msg.err.Error()
			} else {
				m.detail.data.comments = append(m.detail.data.comments, msg.comment)
				m.detail.commenting = false
				m.detail.postErr = ""
			}
		}
		return m, nil

	case taskDetailLoaded:
		if m.detail != nil {
			if msg.err != nil {
				m.detail.err = msg.err
			} else {
				m.detail.data = &msg.data
			}
			m.detail.loading = false
		}
		return m, nil

	case SSEConnected:
		m.sseStatus = "live"
		m.lastError = ""
		return m, nil

	case SSEEvent:
		m.eventLog.addEvent(msg.Event)
		m.applySSEToKanban(msg.Event)
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
	// Recalculate viewport height for current help context.
	m.resizeViewport()

	var b strings.Builder

	// Header: title + SSE status + last error.
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
	if m.lastError != "" {
		header += "  " + errorStyle.Render(m.lastError)
	}
	b.WriteString(header + "\n")

	// Tabs (hidden when an overlay is open).
	if m.detail == nil && m.transition == nil && m.assign == nil {
		var tabs []string
		for i, name := range tabNames {
			if boardTab(i) == m.activeTab {
				tabs = append(tabs, tabActive.Render("["+name+"]"))
			} else {
				tabs = append(tabs, tabInactive.Render(" "+name+" "))
			}
		}
		b.WriteString(strings.Join(tabs, "") + "\n")
	} else {
		b.WriteString("\n")
	}

	// Tab content rendered into the viewport.
	var content string
	switch m.activeTab {
	case tabKanban:
		content = m.kanban.view(m.viewport.Width, m.viewport.Height)
	case tabEventLog:
		content = m.eventLog.view(m.viewport.Width, m.viewport.Height)
	}
	// Overlays replace tab content when open.
	if m.assign != nil {
		content = m.assign.view(m.viewport.Width)
	} else if m.transition != nil {
		content = m.transition.view(m.viewport.Width)
	} else if m.detail != nil {
		content = m.detail.view(m.viewport.Width, m.viewport.Height)
	}

	m.viewport.SetContent(content)
	b.WriteString(m.viewport.View())

	// Context-specific help.
	b.WriteString("\n" + m.help.View(m.activeKeyMap()))
	return b.String()
}

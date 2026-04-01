package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/httpclient"
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
	viewMyTasks
)

type boardTab int

const (
	tabKanban boardTab = iota
	tabList
	tabWorkflow
	tabEventLog
	tabCount // keep last for cycling
)

var tabNames = []string{"Board", "List", "Workflow", "Events"}

// chromeFixed is the lines used by header, tabs, and footer newline
// (excludes help, which varies by view). Header(1) + Tabs(1) + footer newline(1) = 3
const chromeFixed = 3

// Model is the root Bubble Tea model.
type Model struct {
	cfg      Config
	client   *httpclient.Client
	help     help.Model
	viewport viewport.Model
	view     viewMode
	width    int
	height   int

	// Board selector
	selector selectorModel

	// Event stream (connected at startup, shared across boards)
	liveStatus   string
	lastError    string
	eventsCancel func()                    // cancels the event stream
	boardEvents  map[string]*eventLogModel // per-board event buffers

	// Current authenticated user (fetched via WhoAmI on startup)
	currentUser *model.Actor

	// "My Tasks" cross-board view
	myTasks myTasksModel

	// Board view (active after selecting a board)
	activeBoard *model.Board
	activeTab    boardTab
	kanban       kanbanModel
	listView     listViewModel
	workflowView workflowViewModel
	detail       *detailModel     // non-nil when detail overlay is open
	transition   *transitionModel // non-nil when transition overlay is open
	assign       *assignModel     // non-nil when assign overlay is open
}

// New creates a new TUI model.
func New(cfg Config) Model {
	client := httpclient.New(cfg.ServerURL, cfg.APIKey)
	return Model{
		cfg:         cfg,
		client:      client,
		help:        newHelp(),
		viewport:    viewport.New(80, 20),
		selector:    newSelector(),
		view:        viewSelector,
		liveStatus:  "connecting...",
		boardEvents: map[string]*eventLogModel{},
	}
}

// activeEventLog returns the event log for the current board, creating it if needed.
func (m *Model) activeEventLog() *eventLogModel {
	if m.activeBoard == nil {
		return &eventLogModel{}
	}
	slug := m.activeBoard.Slug
	if log, ok := m.boardEvents[slug]; ok {
		return log
	}
	log := &eventLogModel{}
	m.boardEvents[slug] = log
	return log
}

// boardEventLog returns the event log for a board slug, creating it if needed.
func (m *Model) boardEventLog(slug string) *eventLogModel {
	if log, ok := m.boardEvents[slug]; ok {
		return log
	}
	log := &eventLogModel{}
	m.boardEvents[slug] = log
	return log
}

func (m Model) Init() tea.Cmd {
	// Start the global event stream immediately.
	if m.cfg.Program != nil && *m.cfg.Program != nil {
		m.eventsCancel = startLiveEvents(*m.cfg.Program, m.client)
	}

	whoAmI := func() tea.Msg {
		actor, err := m.client.WhoAmI()
		return whoAmILoaded{actor: actor, err: err}
	}

	if m.cfg.BoardSlug != "" {
		return tea.Batch(whoAmI, func() tea.Msg {
			board, err := httpclient.GetOne[model.Board](m.client, model.ResBoardGet, httpclient.PathParams{"slug": m.cfg.BoardSlug}, nil)
			if err != nil {
				return boardsLoaded{err: err}
			}
			return boardSelected{board: board}
		})
	}
	return tea.Batch(whoAmI, fetchBoards(m.client, m.selector.showArchived))
}

// whoAmILoaded is sent when the current user identity is fetched.
type whoAmILoaded struct {
	actor model.Actor
	err   error
}

// myTasksSelected is sent when "My Tasks" is chosen from the selector.
type myTasksSelected struct{}

// myTasksLoaded is sent when cross-board tasks are fetched.
type myTasksLoaded struct {
	tasks []model.Task
	err   error
}

func fetchMyTasks(client *httpclient.Client) tea.Cmd {
	return func() tea.Msg {
		assignee := "@me"
		tasks, err := httpclient.GetMany[model.Task](client, model.ResTaskSearch, nil, model.TaskFilter{Assignee: &assignee, IncludeClosed: true})
		return myTasksLoaded{tasks: tasks, err: err}
	}
}

// boardSelected is sent when a board is chosen.
type boardSelected struct {
	board model.Board
}

func (m *Model) selectedTaskFromContext() *model.Task {
	if m.detail != nil && m.detail.data != nil {
		return &m.detail.data.task
	}
	if m.view == viewMyTasks {
		return m.myTasks.selectedTask()
	}
	switch m.activeTab {
	case tabKanban:
		return m.kanban.selectedTask()
	case tabList:
		return m.listView.selectedTask()
	}
	return nil
}

func (m *Model) activeBoardSlug() string {
	if m.view == viewMyTasks {
		if t := m.selectedTaskFromContext(); t != nil {
			return t.BoardSlug
		}
		return ""
	}
	if m.activeBoard != nil {
		return m.activeBoard.Slug
	}
	return ""
}

func (m *Model) openAssignFromContext() tea.Cmd {
	task := m.selectedTaskFromContext()
	if task == nil {
		return nil
	}
	slug := m.activeBoardSlug()
	if slug == "" {
		return nil
	}
	am, cmd := newAssign(m.client, slug, *task, m.currentUser)
	m.assign = am
	return cmd
}

func (m *Model) takeTask() tea.Cmd {
	if m.currentUser == nil {
		return nil
	}
	task := m.selectedTaskFromContext()
	if task == nil {
		return nil
	}
	slug := m.activeBoardSlug()
	if slug == "" {
		return nil
	}
	name := m.currentUser.Name
	return executeAssign(m.client, slug, task.Num, &name)
}

func (m *Model) openTransitionFromContext() tea.Cmd {
	task := m.selectedTaskFromContext()
	if task == nil {
		return nil
	}
	slug := m.activeBoardSlug()
	if slug == "" {
		return nil
	}
	actorName := ""
	if m.currentUser != nil {
		actorName = m.currentUser.Name
	}
	tm, cmd := newTransition(m.client, slug, *task, actorName)
	m.transition = tm
	return cmd
}

func (m *Model) openDetail() tea.Cmd {
	var boardSlug string
	var num int

	if m.view == viewMyTasks {
		if t := m.myTasks.selectedTask(); t != nil {
			boardSlug = t.BoardSlug
			num = t.Num
		}
	} else {
		if m.activeBoard != nil {
			boardSlug = m.activeBoard.Slug
		}

		switch m.activeTab {
		case tabKanban:
			if t := m.kanban.selectedTask(); t != nil {
				num = t.Num
			}
		case tabList:
			if t := m.listView.selectedTask(); t != nil {
				num = t.Num
			}
		case tabEventLog:
			// Try to get task num from the selected event.
			el := m.activeEventLog()
			if el.cursor >= 0 && el.cursor < len(el.entries) {
				entry := el.entries[el.cursor]
				if entry.event != nil {
					snap := entry.event.After
					if snap == nil {
						snap = entry.event.Before
					}
					if snap != nil {
						num = snap.Num
					}
				} else if entry.audit != nil && entry.audit.TaskNum != nil {
					num = *entry.audit.TaskNum
				}
			}
		}
	}

	if boardSlug == "" || num == 0 {
		return nil
	}

	m.detail = &detailModel{loading: true}
	return fetchTaskDetail(m.client, boardSlug, num)
}

func (m *Model) applyEventToKanban(evt eventbus.Event) {
	if m.activeBoard == nil {
		return
	}
	boardSlug := m.activeBoard.Slug

	switch evt.Type {
	case eventbus.EventTaskCreated:
		if evt.After == nil {
			return
		}
		m.kanban.updateTask(snapshotToTask(boardSlug, evt.After))
	case eventbus.EventTaskTransitioned, eventbus.EventTaskUpdated, eventbus.EventTaskAssigned:
		if evt.After == nil {
			return
		}
		m.kanban.updateTask(snapshotToTask(boardSlug, evt.After))
	case eventbus.EventTaskDeleted:
		if evt.Before == nil {
			return
		}
		m.kanban.removeTask(boardSlug, evt.Before.Num)
	}
}

func (m *Model) refreshDetailIfAffected(evt eventbus.Event) tea.Cmd {
	if m.detail == nil || m.detail.data == nil {
		return nil
	}
	task := m.detail.data.task
	snap := evt.After
	if snap == nil {
		snap = evt.Before
	}
	if snap == nil || snap.Num != task.Num || evt.Board.Slug != task.BoardSlug {
		return nil
	}
	// Refetch the full detail data.
	m.detail.loading = true
	return fetchTaskDetail(m.client, task.BoardSlug, task.Num)
}

func (m *Model) applyEventToList(evt eventbus.Event) {
	if m.activeBoard == nil {
		return
	}
	boardSlug := m.activeBoard.Slug

	switch evt.Type {
	case eventbus.EventTaskCreated, eventbus.EventTaskTransitioned,
		eventbus.EventTaskUpdated, eventbus.EventTaskAssigned:
		if evt.After != nil {
			m.listView.updateTask(snapshotToTask(boardSlug, evt.After))
		}
	case eventbus.EventTaskDeleted:
		if evt.Before != nil {
			m.listView.removeTask(boardSlug, evt.Before.Num)
		}
	}
}

func (m *Model) applyEventToMyTasks(evt eventbus.Event) tea.Cmd {
	me := m.currentUser.Name
	boardSlug := evt.Board.Slug

	isMyTask := func(snap *eventbus.TaskSnapshot) bool {
		return snap != nil && snap.Assignee != nil && *snap.Assignee == me
	}

	switch evt.Type {
	case eventbus.EventTaskDeleted:
		if isMyTask(evt.Before) {
			m.myTasks.removeTask(boardSlug, evt.Before.Num)
		}
	default:
		wasMine := isMyTask(evt.Before)
		isMine := isMyTask(evt.After)
		switch {
		case isMine:
			m.myTasks.updateTask(snapshotToTask(boardSlug, evt.After))
			// Fetch workflow if we haven't seen this board yet.
			if _, ok := m.myTasks.workflows[boardSlug]; !ok {
				return fetchWorkflowsForBoards(m.client, []string{boardSlug})
			}
		case wasMine && !isMine:
			m.myTasks.removeTask(boardSlug, evt.Before.Num)
		}
	}
	return nil
}

func snapshotToTask(boardSlug string, snap *eventbus.TaskSnapshot) model.Task {
	return model.Task{
		BoardSlug: boardSlug,
		Num:       snap.Num,
		Title:     snap.Title,
		State:     snap.State,
		Priority:  model.Priority(snap.Priority),
		Assignee:  snap.Assignee,
	}
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
	if m.view == viewMyTasks {
		return myTasksKeyMap
	}
	switch m.activeTab {
	case tabKanban:
		return kanbanKeyMap
	case tabList:
		return listKeyMap
	case tabWorkflow:
		return workflowKeyMap
	case tabEventLog:
		return eventLogKeyMap
	}
	return kanbanKeyMap
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// When the board create form is active, delegate directly.
		if m.view == viewSelector && m.selector.creating {
			var selected *model.Board
			var cmd tea.Cmd
			m.selector, selected, cmd = m.selector.update(msg, m.client)
			if selected != nil {
				return m, func() tea.Msg { return boardSelected{board: *selected} }
			}
			return m, cmd
		}

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
				cmd := m.detail.submitComment(m.client)
				return m, cmd
			default:
				cmd := m.detail.update(msg)
				return m, cmd
			}
		}

		// When assign or transition overlays are open, delegate all keys
		// to the overlay (except ctrl+c) so the search filter works.
		if m.assign != nil {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			closed, cmd := m.assign.update(msg, m.client)
			if closed {
				m.assign = nil
			}
			return m, cmd
		}
		if m.transition != nil {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			closed, cmd := m.transition.update(msg, m.client)
			if closed {
				m.transition = nil
			}
			return m, cmd
		}

		switch msg.String() {
		case "?":
			m.help.ShowAll = !m.help.ShowAll
			m.resizeViewport()
			return m, nil
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "backspace":
			if m.detail != nil {
				m.detail = nil
				return m, nil
			}
			if m.view == viewBoard || m.view == viewMyTasks {
				m.view = viewSelector
				m.activeBoard = nil
				return m, fetchBoards(m.client, m.selector.showArchived)
			}
		case "tab":
			if m.view == viewBoard && m.detail == nil {
				m.activeTab = (m.activeTab + 1) % tabCount
				m.viewport.GotoTop()
			}
		case "shift+tab":
			if m.view == viewBoard && m.detail == nil {
				m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
				m.viewport.GotoTop()
			}
		case "t":
			if (m.view == viewBoard || m.view == viewMyTasks) && m.transition == nil && m.assign == nil {
				return m, m.openTransitionFromContext()
			}
		case "T":
			if (m.view == viewBoard || m.view == viewMyTasks) && m.assign == nil && m.transition == nil {
				return m, m.takeTask()
			}
		case "a":
			if (m.view == viewBoard || m.view == viewMyTasks) && m.assign == nil && m.transition == nil {
				return m, m.openAssignFromContext()
			}
		case "c":
			if m.detail != nil && m.detail.data != nil && !m.detail.commenting {
				m.detail.startComment()
				return m, m.detail.input.Focus()
			}
		case "enter":
			if (m.view == viewBoard || m.view == viewMyTasks) && m.detail == nil && m.transition == nil && m.assign == nil {
				return m, m.openDetail()
			}
		}

		if m.detail != nil {
			m.detail.ensureViewport(m.viewport.Width, m.viewport.Height)
			switch msg.String() {
			case "down", "j":
				m.detail.scrollDown()
			case "up", "k":
				m.detail.scrollUp()
			case "pgdown", "ctrl+d":
				m.detail.halfPageDown()
			case "pgup", "ctrl+u":
				m.detail.halfPageUp()
			case "home":
				m.detail.gotoTop()
			case "end":
				m.detail.gotoBottom()
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.resizeViewport()
		m.viewport.GotoTop()
		return m, nil

	case whoAmILoaded:
		if msg.err == nil {
			m.currentUser = &msg.actor
		}
		return m, nil

	case myTasksSelected:
		m.view = viewMyTasks
		m.activeBoard = nil
		m.myTasks = newMyTasks()
		m.resizeViewport()
		return m, fetchMyTasks(m.client)

	case myTasksLoaded:
		if msg.err != nil {
			m.lastError = msg.err.Error()
			return m, nil
		}
		m.myTasks.load(msg.tasks)
		if slugs := m.myTasks.missingWorkflowSlugs(); len(slugs) > 0 {
			return m, fetchWorkflowsForBoards(m.client, slugs)
		}
		return m, nil

	case myTasksWorkflowsLoaded:
		for slug, wf := range msg.workflows {
			m.myTasks.workflows[slug] = wf
		}
		m.myTasks.rebuild()
		return m, nil

	case boardSelected:
		m.activeBoard = &msg.board
		m.view = viewBoard
		m.activeTab = tabKanban
		currentName := ""
		if m.currentUser != nil {
			currentName = m.currentUser.Name
		}
		m.kanban = newKanban(currentName)
		m.listView = newListView(currentName)
		m.resizeViewport()
		return m, fetchBoardData(m.client, msg.board.Slug)

	case boardDataLoaded:
		m.kanban.load(msg)
		m.listView.load(msg)
		m.workflowView.load(msg.workflow)
		if m.activeBoard != nil {
			m.boardEventLog(m.activeBoard.Slug).seedFromAudit(msg.audit)
		}
		return m, nil

	case actorsLoaded, assignResult:
		if m.assign != nil {
			closed, cmd := m.assign.update(msg, m.client)
			if closed {
				m.assign = nil
			}
			return m, cmd
		}
		// "Take" action result (no overlay open).
		if result, ok := msg.(assignResult); ok && result.err != nil {
			m.lastError = result.err.Error()
		}
		return m, nil

	case transitionsLoaded, transitionResult:
		if m.transition != nil {
			closed, cmd := m.transition.update(msg, m.client)
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
				m.detail.invalidateContent()
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
			m.detail.invalidateContent()
		}
		return m, nil

	case liveConnected:
		m.liveStatus = "live"
		m.lastError = ""
		return m, nil

	case eventbus.Event:
		m.boardEventLog(msg.Board.Slug).addEvent(msg)
		// Apply to board views if the event is for the active board.
		if m.activeBoard != nil && msg.Board.Slug == m.activeBoard.Slug {
			m.applyEventToKanban(msg)
			m.applyEventToList(msg)
		}
		// Apply to "My Tasks" view based on assignee changes.
		var cmds []tea.Cmd
		if m.view == viewMyTasks && m.currentUser != nil {
			if cmd := m.applyEventToMyTasks(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if cmd := m.refreshDetailIfAffected(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case httpclient.StreamError:
		m.lastError = msg.Err.Error()
		if msg.Permanent {
			m.liveStatus = "error"
		} else {
			m.liveStatus = "reconnecting..."
		}
		return m, nil
	}

	// Delegate to sub-views.
	switch {
	case m.view == viewMyTasks:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			m.myTasks.update(keyMsg)
		}
	case m.view == viewSelector:
		var selected *model.Board
		var cmd tea.Cmd
		m.selector, selected, cmd = m.selector.update(msg, m.client)
		if selected != nil {
			return m, func() tea.Msg { return boardSelected{board: *selected} }
		}
		if cmd != nil {
			return m, cmd
		}
	case m.view == viewBoard && m.activeTab == tabKanban:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			m.kanban.update(keyMsg)
		}
	case m.view == viewBoard && m.activeTab == tabList:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			m.listView.update(keyMsg)
		}
	case m.view == viewBoard && m.activeTab == tabWorkflow:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			m.workflowView.update(keyMsg, m.viewport.Width, m.viewport.Height)
		}
	case m.view == viewBoard && m.activeTab == tabEventLog:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			el := m.activeEventLog()
			switch keyMsg.String() {
			case "up", "k":
				el.moveUp()
			case "down", "j":
				el.moveDown()
			case "pgdown", "ctrl+d":
				el.pageDown(m.viewport.Height / 2)
			case "pgup", "ctrl+u":
				el.pageUp(m.viewport.Height / 2)
			case "home":
				el.gotoTop()
			case "end":
				el.gotoBottom()
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
	tabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Background(lipgloss.Color("236")).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
)

func (m Model) View() string {
	switch m.view {
	case viewSelector:
		return m.selectorView()
	case viewBoard:
		return m.boardView()
	case viewMyTasks:
		return m.myTasksView()
	}
	return ""
}

func (m Model) selectorView() string {
	var b strings.Builder
	b.WriteString(m.selector.view(m.width))
	b.WriteString("\n" + m.help.View(selectorKeyMap))
	return b.String()
}

func (m Model) myTasksView() string {
	m.resizeViewport()
	var b strings.Builder

	// Header.
	var status string
	switch m.liveStatus {
	case "live":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Render("● live")
	case "error":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✕ error")
	default:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◌ " + m.liveStatus)
	}
	header := fmt.Sprintf("%s  %s", titleStyle.Render("TaskFlow — My Tasks"), status)
	if m.lastError != "" {
		header += "  " + errorStyle.Render(m.lastError)
	}
	b.WriteString(header + "\n")
	b.WriteString("\n") // no tabs

	// Content.
	if m.detail != nil && m.assign == nil && m.transition == nil {
		b.WriteString(m.detail.view(m.viewport.Width, m.viewport.Height))
	} else {
		var content string
		if m.assign != nil {
			content = m.assign.view(m.viewport.Width)
		} else if m.transition != nil {
			content = m.transition.view(m.viewport.Width)
		} else {
			content = m.myTasks.view(m.viewport.Width, m.viewport.Height)
		}
		m.viewport.SetContent(content)
		b.WriteString(m.viewport.View())
	}

	b.WriteString("\n" + m.help.View(m.activeKeyMap()))
	return b.String()
}

func (m Model) boardView() string {
	m.resizeViewport()
	var b strings.Builder

	// Header: title + live status + last error.
	var status string
	switch m.liveStatus {
	case "live":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Render("● live")
	case "error":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✕ error")
	default:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◌ " + m.liveStatus)
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
				tabs = append(tabs, tabActive.Render(name))
			} else {
				tabs = append(tabs, tabInactive.Render(name))
			}
		}
		b.WriteString(strings.Join(tabs, "") + "\n")
	} else {
		b.WriteString("\n")
	}

	// Tab content rendered into the viewport.
	// The workflow tab has its own viewport for independent scrolling.
	// Views with their own viewport render directly.
	// Others go through the shared viewport.
	if m.detail != nil && m.assign == nil && m.transition == nil {
		b.WriteString(m.detail.view(m.viewport.Width, m.viewport.Height))
	} else if m.activeTab == tabWorkflow && m.detail == nil && m.transition == nil && m.assign == nil {
		b.WriteString(m.workflowView.view(m.viewport.Width, m.viewport.Height))
	} else {
		var content string
		switch m.activeTab {
		case tabKanban:
			content = m.kanban.view(m.viewport.Width, m.viewport.Height)
		case tabList:
			content = m.listView.view(m.viewport.Width, m.viewport.Height)
		case tabEventLog:
			content = m.activeEventLog().view(m.viewport.Width, m.viewport.Height)
		}
		// Overlays replace tab content.
		if m.assign != nil {
			content = m.assign.view(m.viewport.Width)
		} else if m.transition != nil {
			content = m.transition.view(m.viewport.Width)
		}

		m.viewport.SetContent(content)
		b.WriteString(m.viewport.View())
	}

	// Context-specific help.
	b.WriteString("\n" + m.help.View(m.activeKeyMap()))
	return b.String()
}

package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// selectorKeys defines key bindings for the board selector view.
type selectorKeys struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	New     key.Binding
	Archive key.Binding
	ShowAll key.Binding
	Esc     key.Binding
	Quit    key.Binding
}

func (k selectorKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyHelp, k.Up, k.Down, k.Enter, k.New, k.Archive, k.ShowAll, k.Quit}
}

func (k selectorKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter},
		{k.New, k.Archive, k.ShowAll},
		{k.Esc, k.Quit},
	}
}

var selectorKeyMap = selectorKeys{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	New:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new board")),
	Archive: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "archive")),
	ShowAll: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle archived")),
	Esc:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// Shared board-level keys (present on all tabs).
var (
	keyTab  = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch tab"))
	keyEsc  = key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc/⌫", "boards"))
	keyQuit = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
)

// kanbanKeys defines key bindings for the kanban board tab.
type kanbanKeys struct {
	Left       key.Binding
	Right      key.Binding
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Transition key.Binding
	Assign     key.Binding
	Take       key.Binding
	ToggleD    key.Binding
	Tab        key.Binding
	Esc        key.Binding
	Quit       key.Binding
}

func (k kanbanKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyHelp, k.Left, k.Up, k.Enter, k.Transition, k.Assign, k.Take, k.ToggleD, k.Tab, k.Esc, k.Quit}
}

func (k kanbanKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down},
		{k.Enter, k.Transition, k.Assign, k.Take},
		{k.ToggleD, k.Tab},
		{k.Esc, k.Quit},
	}
}

var kanbanKeyMap = kanbanKeys{
	Left:       key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev column")),
	Right:      key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next column")),
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "prev task")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "next task")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
	Transition: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
	Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign")),
	Take:       key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "take")),
	ToggleD:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle done")),
	Tab:        keyTab,
	Esc:        keyEsc,
	Quit:       keyQuit,
}

// listKeys defines key bindings for the list view tab.
type listKeys struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Sort       key.Binding
	SortDir    key.Binding
	Transition key.Binding
	Assign     key.Binding
	Take       key.Binding
	ToggleD    key.Binding
	Tab        key.Binding
	Esc        key.Binding
	Quit       key.Binding
}

func (k listKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyHelp, k.Up, k.Enter, k.Sort, k.Transition, k.Assign, k.Take, k.Tab, k.Quit}
}

func (k listKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Enter, k.Transition, k.Assign, k.Take},
		{k.Sort, k.SortDir, k.ToggleD},
		{k.Tab, k.Esc, k.Quit},
	}
}

var listKeyMap = listKeys{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "prev task")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "next task")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
	Sort:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "cycle sort")),
	SortDir:    key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "reverse sort")),
	Transition: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
	Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign")),
	Take:       key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "take")),
	ToggleD:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle done")),
	Tab:        keyTab,
	Esc:        keyEsc,
	Quit:       keyQuit,
}

// workflowKeys defines key bindings for the workflow visualisation tab.
type workflowKeys struct {
	Up   key.Binding
	Down key.Binding
	PgUp key.Binding
	PgDn key.Binding
	Home key.Binding
	End  key.Binding
	Tab  key.Binding
	Esc  key.Binding
	Quit key.Binding
}

func (k workflowKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyHelp, k.Up, k.Down, k.PgUp, k.PgDn, k.Tab, k.Esc, k.Quit}
}

func (k workflowKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PgUp, k.PgDn, k.Home, k.End},
		{k.Tab, k.Esc, k.Quit},
	}
}

var workflowKeyMap = workflowKeys{
	Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	PgUp: key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("PgUp", "page up")),
	PgDn: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("PgDn", "page down")),
	Home: key.NewBinding(key.WithKeys("home"), key.WithHelp("Home", "top")),
	End:  key.NewBinding(key.WithKeys("end"), key.WithHelp("End", "bottom")),
	Tab:  keyTab,
	Esc:  keyEsc,
	Quit: keyQuit,
}

// eventLogKeys defines key bindings for the event log tab.
type eventLogKeys struct {
	Up    key.Binding
	Down  key.Binding
	PgUp  key.Binding
	PgDn  key.Binding
	Home  key.Binding
	End   key.Binding
	Enter key.Binding
	Tab   key.Binding
	Esc   key.Binding
	Quit  key.Binding
}

func (k eventLogKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyHelp, k.Up, k.Down, k.PgUp, k.PgDn, k.Enter, k.Tab, k.Esc, k.Quit}
}

func (k eventLogKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PgUp, k.PgDn, k.Home, k.End},
		{k.Enter, k.Tab},
		{k.Esc, k.Quit},
	}
}

var eventLogKeyMap = eventLogKeys{
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	PgUp:  key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("PgUp", "page up")),
	PgDn:  key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("PgDn", "page down")),
	Home:  key.NewBinding(key.WithKeys("home"), key.WithHelp("Home", "top")),
	End:   key.NewBinding(key.WithKeys("end"), key.WithHelp("End", "bottom")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
	Tab:   keyTab,
	Esc:   keyEsc,
	Quit:  keyQuit,
}

// detailKeys defines key bindings for the task detail pane.
type detailKeys struct {
	Up         key.Binding
	Down       key.Binding
	PgUp       key.Binding
	PgDn       key.Binding
	Home       key.Binding
	End        key.Binding
	Comment    key.Binding
	Transition key.Binding
	Assign     key.Binding
	Take       key.Binding
	Esc        key.Binding
	Quit       key.Binding
}

func (k detailKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyHelp, k.Up, k.Down, k.PgUp, k.PgDn, k.Comment, k.Transition, k.Assign, k.Take, k.Esc, k.Quit}
}

func (k detailKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PgUp, k.PgDn, k.Home, k.End},
		{k.Comment, k.Transition, k.Assign, k.Take},
		{k.Esc, k.Quit},
	}
}

var detailKeyMap = detailKeys{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	PgUp:       key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("PgUp", "page up")),
	PgDn:       key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("PgDn", "page down")),
	Home:       key.NewBinding(key.WithKeys("home"), key.WithHelp("Home", "top")),
	End:        key.NewBinding(key.WithKeys("end"), key.WithHelp("End", "bottom")),
	Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
	Transition: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
	Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign")),
	Take:       key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "take")),
	Esc:        key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc/⌫", "close")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// commentKeys defines key bindings shown when the comment textarea is active.
type commentKeys struct {
	Submit key.Binding
	Cancel key.Binding
}

func (k commentKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel}
}

func (k commentKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.Cancel}}
}

var commentKeyMap = commentKeys{
	Submit: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "submit")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

var keyHelp = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))

func newHelp() help.Model {
	h := help.New()
	h.ShortSeparator = "  "
	h.FullSeparator = "    "
	return h
}

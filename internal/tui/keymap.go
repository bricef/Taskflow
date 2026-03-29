package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// selectorKeys defines key bindings for the board selector view.
type selectorKeys struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Esc   key.Binding
	Quit  key.Binding
}

func (k selectorKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Quit}
}

func (k selectorKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter},
		{k.Esc, k.Quit},
	}
}

var selectorKeyMap = selectorKeys{
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Esc:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
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
	ToggleD    key.Binding
	Tab        key.Binding
	Esc        key.Binding
	Quit       key.Binding
}

func (k kanbanKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Up, k.Enter, k.Transition, k.Assign, k.ToggleD, k.Tab, k.Esc, k.Quit}
}

func (k kanbanKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down},
		{k.Enter, k.Transition, k.Assign, k.ToggleD, k.Tab, k.Esc, k.Quit},
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
	ToggleD:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle done")),
	Tab:        keyTab,
	Esc:        keyEsc,
	Quit:       keyQuit,
}

// eventLogKeys defines key bindings for the event log tab.
type eventLogKeys struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Tab   key.Binding
	Esc   key.Binding
	Quit  key.Binding
}

func (k eventLogKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Tab, k.Esc, k.Quit}
}

func (k eventLogKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Tab, k.Esc, k.Quit},
	}
}

var eventLogKeyMap = eventLogKeys{
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
	Tab:   keyTab,
	Esc:   keyEsc,
	Quit:  keyQuit,
}

// detailKeys defines key bindings for the task detail pane.
type detailKeys struct {
	Up         key.Binding
	Down       key.Binding
	Comment    key.Binding
	Transition key.Binding
	Assign     key.Binding
	Esc        key.Binding
	Quit       key.Binding
}

func (k detailKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Comment, k.Transition, k.Assign, k.Esc, k.Quit}
}

func (k detailKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Comment, k.Transition, k.Assign, k.Esc, k.Quit}}
}

var detailKeyMap = detailKeys{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
	Transition: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
	Assign:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assign")),
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

func newHelp() help.Model {
	h := help.New()
	h.ShortSeparator = "  "
	return h
}

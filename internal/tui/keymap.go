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

// boardKeys defines key bindings for the board view.
type boardKeys struct {
	Tab  key.Binding
	Esc  key.Binding
	Quit key.Binding
}

func (k boardKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Esc, k.Quit}
}

func (k boardKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab, k.Esc, k.Quit},
	}
}

var boardKeyMap = boardKeys{
	Tab:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch tab")),
	Esc:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back to boards")),
	Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

func newHelp() help.Model {
	h := help.New()
	h.ShortSeparator = "  "
	return h
}

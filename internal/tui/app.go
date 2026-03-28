package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/eventbus"
)

// Config holds the TUI configuration.
type Config struct {
	ServerURL string
	APIKey    string
	BoardSlug string
}

// Model is the root Bubble Tea model — a live event log.
type Model struct {
	cfg       Config
	events    []string
	status    string
	lastError string
	width     int
	height    int
}

// New creates a new TUI model.
func New(cfg Config) Model {
	return Model{
		cfg:    cfg,
		status: "connecting...",
	}
}

func (m Model) Init() tea.Cmd {
	return nil // SSE is started externally via StartSSE after program creation.
}

// StartSSE begins the background SSE listener. Call this after tea.NewProgram.
func StartSSE(p *tea.Program, cfg Config) {
	startSSE(p, cfg.ServerURL, cfg.BoardSlug, cfg.APIKey)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case SSEConnected:
		m.status = "live"
		m.lastError = ""

	case SSEEvent:
		line := formatEvent(msg.Event)
		m.events = append(m.events, line)
		if len(m.events) > 100 {
			m.events = m.events[len(m.events)-100:]
		}

	case SSEError:
		m.lastError = msg.Err.Error()
		if msg.Permanent {
			m.status = "error"
			// Don't quit — let the user read the error and press q.
			return m, nil
		}
		m.status = "reconnecting..."
	}

	return m, nil
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	eventStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusLive  = lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Render("● live")
	statusRetry = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◌ reconnecting...")
)

func (m Model) View() string {
	var b strings.Builder

	// Header
	var status string
	switch m.status {
	case "live":
		status = statusLive
	case "error":
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✕ error")
	default:
		status = statusRetry
	}
	header := fmt.Sprintf("%s  %s", titleStyle.Render("TaskFlow — "+m.cfg.BoardSlug), status)
	b.WriteString(header + "\n")

	if m.lastError != "" {
		b.WriteString("\n" + errorStyle.Render(m.lastError) + "\n")
	}

	if m.status == "error" {
		b.WriteString("\n" + dimStyle.Render("Press q to quit.") + "\n")
		return b.String()
	}

	b.WriteString(dimStyle.Render("Listening for events... Press q to quit.") + "\n\n")

	if len(m.events) == 0 {
		b.WriteString(dimStyle.Render("No events yet.") + "\n")
	} else {
		// Show events, scrolled to bottom.
		maxLines := m.height - 5
		if maxLines < 1 {
			maxLines = 20
		}
		start := 0
		if len(m.events) > maxLines {
			start = len(m.events) - maxLines
		}
		for _, line := range m.events[start:] {
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

func formatEvent(evt eventbus.Event) string {
	ts := evt.Timestamp.Format("15:04:05")
	actor := evt.Actor.Name
	if actor == "" {
		actor = "system"
	}
	evtType := eventStyle.Render(evt.Type)

	task := ""
	if evt.Task != nil {
		task = fmt.Sprintf(" %s %q", dimStyle.Render(evt.Task.Ref), evt.Task.Title)
		if evt.Task.PreviousState != "" {
			task += fmt.Sprintf(" (%s → %s)", evt.Task.PreviousState, evt.Task.State)
		}
	}

	return fmt.Sprintf("%s  %-12s  %s%s", dimStyle.Render(ts), actor, evtType, task)
}

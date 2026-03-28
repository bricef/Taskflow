package tui

import (
	"fmt"
	"strings"

	"github.com/bricef/taskflow/internal/eventbus"
)

// eventLogModel is a tab view that shows a live stream of events.
type eventLogModel struct {
	events []string
}

func (m *eventLogModel) addEvent(evt eventbus.Event) {
	line := formatEvent(evt)
	m.events = append(m.events, line)
	if len(m.events) > 200 {
		m.events = m.events[len(m.events)-200:]
	}
}

func (m eventLogModel) view(height int) string {
	var b strings.Builder

	if len(m.events) == 0 {
		b.WriteString(dimStyle.Render("No events yet.") + "\n")
		return b.String()
	}

	maxLines := height - 6
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

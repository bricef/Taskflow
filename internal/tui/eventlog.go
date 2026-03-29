package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
)

const recentAuditCount = 5

var (
	selectedLineStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	detailLabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	detailValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	detailBorder      = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("241")).
				Padding(0, 1)
)

type eventEntry struct {
	line  string            // formatted one-liner for the list
	event *eventbus.Event   // live SSE event (nil for audit history)
	audit *model.AuditEntry // audit history entry (nil for live events)
}

// eventLogModel is a tab view that shows a live stream of events.
type eventLogModel struct {
	entries []eventEntry
	cursor  int
}

func (m *eventLogModel) addEvent(evt eventbus.Event) {
	m.entries = append(m.entries, eventEntry{
		line:  formatEvent(evt),
		event: &evt,
	})
	if len(m.entries) > 200 {
		m.entries = m.entries[len(m.entries)-200:]
	}
	m.cursor = len(m.entries) - 1
}

func (m *eventLogModel) moveUp() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *eventLogModel) moveDown() {
	if m.cursor < len(m.entries)-1 {
		m.cursor++
	}
}

func (m *eventLogModel) seedFromAudit(entries []model.AuditEntry) {
	if len(entries) == 0 {
		return
	}
	start := 0
	if len(entries) > recentAuditCount {
		start = len(entries) - recentAuditCount
	}
	for i := start; i < len(entries); i++ {
		e := entries[i]
		m.entries = append(m.entries, eventEntry{
			line:  formatAuditEntry(e),
			audit: &e,
		})
	}
	m.cursor = len(m.entries) - 1
}

func (m eventLogModel) view(width, height int) string {
	if len(m.entries) == 0 {
		return dimStyle.Render("No events yet.") + "\n"
	}

	// Side-by-side: event list (2/3) | detail pane (1/3).
	detailWidth := width / 3
	if detailWidth < 30 {
		detailWidth = 30
	}
	listWidth := width - detailWidth - 3 // 2 for spacer, 1 for safety
	if listWidth < 20 {
		listWidth = 20
	}

	// Render event list with scrolling window around cursor.
	listHeight := height
	var listB strings.Builder
	start := 0
	if m.cursor >= listHeight {
		start = m.cursor - listHeight + 1
	}
	end := start + listHeight
	if end > len(m.entries) {
		end = len(m.entries)
	}

	for i := start; i < end; i++ {
		line := truncate(m.entries[i].line, listWidth-2) // 2 for "▸ " prefix
		if i == m.cursor {
			listB.WriteString(selectedLineStyle.Width(listWidth).Render("▸ " + line))
		} else {
			listB.WriteString(lipgloss.NewStyle().Width(listWidth).Render("  " + line))
		}
		listB.WriteString("\n")
	}

	detail := m.renderDetail(detailWidth, height)

	// Drop the list down one line relative to the detail panel.
	list := "\n" + listB.String()

	return lipgloss.JoinHorizontal(lipgloss.Top, list, "  ", detail)
}

func (m eventLogModel) renderDetail(width, height int) string {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return ""
	}

	entry := m.entries[m.cursor]
	// Label is 9 chars ("Label:" + padding), border+padding is ~6, leave room.
	maxVal := width - 15
	if maxVal < 10 {
		maxVal = 10
	}
	var rows []string

	if entry.event != nil {
		evt := entry.event
		rows = append(rows, detailRow("Event", string(evt.Type), maxVal))
		rows = append(rows, detailRow("Time", evt.Timestamp.Format("2006-01-02 15:04:05"), maxVal))
		rows = append(rows, detailRow("Actor", evt.Actor.Name, maxVal))
		rows = append(rows, detailRow("Board", evt.Board.Slug, maxVal))
		if evt.Task != nil {
			rows = append(rows, detailRow("Task", fmt.Sprintf("%s — %s", evt.Task.Ref, evt.Task.Title), maxVal))
			if evt.Task.PreviousState != "" {
				rows = append(rows, detailRow("State", fmt.Sprintf("%s → %s", evt.Task.PreviousState, evt.Task.State), maxVal))
			} else {
				rows = append(rows, detailRow("State", evt.Task.State, maxVal))
			}
		}
		if evt.Detail != nil {
			if b, err := json.Marshal(evt.Detail); err == nil && string(b) != "null" {
				rows = append(rows, detailRow("Detail", string(b), maxVal))
			}
		}
	} else if entry.audit != nil {
		a := entry.audit
		rows = append(rows, detailRow("Action", string(a.Action), maxVal))
		rows = append(rows, detailRow("Time", a.CreatedAt.Format("2006-01-02 15:04:05"), maxVal))
		rows = append(rows, detailRow("Actor", a.Actor, maxVal))
		rows = append(rows, detailRow("Board", a.BoardSlug, maxVal))
		if a.TaskNum != nil {
			rows = append(rows, detailRow("Task", fmt.Sprintf("%s/%d", a.BoardSlug, *a.TaskNum), maxVal))
		}
		if len(a.Detail) > 0 && string(a.Detail) != "{}" {
			rows = append(rows, detailRow("Detail", string(a.Detail), maxVal))
		}
	}

	if len(rows) == 0 {
		return ""
	}

	content := strings.Join(rows, "\n")
	return detailBorder.Width(width - 4).Render(content)
}

func detailRow(label, value string, maxValueWidth int) string {
	value = truncate(value, maxValueWidth)
	return fmt.Sprintf("%s %s", detailLabelStyle.Width(8).Render(label+":"), detailValueStyle.Render(value))
}

// truncate cuts a string to max visible characters, appending "…" if truncated.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	// Use rune count for proper unicode handling.
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
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

func formatAuditEntry(e model.AuditEntry) string {
	ts := e.CreatedAt.Format("15:04:05")
	action := eventStyle.Render(string(e.Action))
	task := ""
	if e.TaskNum != nil {
		task = dimStyle.Render(fmt.Sprintf(" %s/%d", e.BoardSlug, *e.TaskNum))
	}
	return fmt.Sprintf("%s  %-12s  %s%s  %s", dimStyle.Render(ts), e.Actor, action, task, dimStyle.Render("(history)"))
}

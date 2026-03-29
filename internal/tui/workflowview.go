package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/workflow"
)

var (
	wfStateBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)
	wfInitialBox = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			Padding(0, 1).
			BorderForeground(lipgloss.Color("39"))
	wfTerminalBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			BorderForeground(lipgloss.Color("241")).
			Foreground(lipgloss.Color("241"))
	wfArrowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	wfLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	wfHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
)

type workflowViewModel struct {
	workflow *workflow.Workflow
	ready    bool
}

func (m *workflowViewModel) load(wf *workflow.Workflow) {
	m.workflow = wf
	m.ready = true
}

func (m workflowViewModel) view(width, height int) string {
	if !m.ready || m.workflow == nil {
		return dimStyle.Render("Loading workflow...") + "\n"
	}

	wf := m.workflow
	var b strings.Builder

	// Title and legend.
	b.WriteString(wfHeaderStyle.Render("Workflow") + "\n\n")
	b.WriteString(fmt.Sprintf("  %s initial   %s terminal   %s intermediate\n\n",
		lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1).Render("state"),
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("241")).Foreground(lipgloss.Color("241")).Padding(0, 1).Render("state"),
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render("state"),
	))

	// States section.
	b.WriteString(wfHeaderStyle.Render("States") + "\n")
	var stateBoxes []string
	for _, s := range wf.States {
		style := wfStateBox
		if s == wf.InitialState {
			style = wfInitialBox
		} else if wf.IsTerminal(s) {
			style = wfTerminalBox
		}
		stateBoxes = append(stateBoxes, style.Render(s))
	}
	// Lay out state boxes horizontally, wrapping if needed.
	b.WriteString(wrapBoxes(stateBoxes, width, "  "))
	b.WriteString("\n")

	// Transitions section — grouped by source state.
	b.WriteString(wfHeaderStyle.Render("Transitions") + "\n\n")

	// Group transitions by from state, preserving state order.
	type transGroup struct {
		from  string
		edges []workflow.Transition
	}
	groupMap := map[string]*transGroup{}
	var groups []*transGroup
	for _, t := range wf.Transitions {
		g, ok := groupMap[t.From]
		if !ok {
			g = &transGroup{from: t.From}
			groupMap[t.From] = g
			groups = append(groups, g)
		}
		g.edges = append(g.edges, t)
	}

	arrow := wfArrowStyle.Render("→")
	for _, g := range groups {
		for _, t := range g.edges {
			label := wfLabelStyle.Render(t.Name)
			b.WriteString(fmt.Sprintf("  %s %s %s  %s\n", g.from, arrow, t.To, label))
		}
	}

	// Terminal states (no outgoing transitions).
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Terminal: %s", strings.Join(wf.TerminalStates, ", "))) + "\n")

	return b.String()
}

// wrapBoxes lays out rendered boxes horizontally, wrapping to new lines
// when they would exceed maxWidth.
func wrapBoxes(boxes []string, maxWidth int, sep string) string {
	if len(boxes) == 0 {
		return ""
	}

	var lines []string
	var currentLine []string
	currentWidth := 0
	sepWidth := lipgloss.Width(sep)

	for _, box := range boxes {
		boxWidth := lipgloss.Width(box)
		needed := boxWidth
		if currentWidth > 0 {
			needed += sepWidth
		}
		if currentWidth > 0 && currentWidth+needed > maxWidth {
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Bottom, interleave(currentLine, sep)...))
			currentLine = nil
			currentWidth = 0
		}
		currentLine = append(currentLine, box)
		if currentWidth > 0 {
			currentWidth += sepWidth
		}
		currentWidth += boxWidth
	}
	if len(currentLine) > 0 {
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Bottom, interleave(currentLine, sep)...))
	}

	return strings.Join(lines, "\n")
}

func interleave(items []string, sep string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, item)
	}
	return result
}

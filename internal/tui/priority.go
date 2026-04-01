package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

// priorityStyle maps each priority level to a display style.
var priorityStyle = map[model.Priority]lipgloss.Style{
	model.PriorityCritical: lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
	model.PriorityHigh:     lipgloss.NewStyle().Foreground(lipgloss.Color("208")),
	model.PriorityMedium:   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	model.PriorityLow:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
	model.PriorityNone:     lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
}

// priorityRank returns a numeric rank for sorting (lower = higher priority).
func priorityRank(p model.Priority) int {
	switch p {
	case model.PriorityCritical:
		return 0
	case model.PriorityHigh:
		return 1
	case model.PriorityMedium:
		return 2
	case model.PriorityLow:
		return 3
	default:
		return 4
	}
}

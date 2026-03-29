package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/workflow"
)

// GraphStyles controls the appearance of rendered workflow graphs.
type GraphStyles struct {
	InitialState  lipgloss.Style
	TerminalState lipgloss.Style
	DefaultState  lipgloss.Style
	Arrow         lipgloss.Style
	Label         lipgloss.Style
}

// RenderWorkflowGraph produces a styled string visualising the workflow
// as a layered directed graph that fits within the given width.
func RenderWorkflowGraph(wf *workflow.Workflow, width int, styles GraphStyles) string {
	if wf == nil || len(wf.States) == 0 {
		return ""
	}

	layers := assignLayers(wf)

	boxFor := func(state string) string {
		style := styles.DefaultState
		if state == wf.InitialState {
			style = styles.InitialState
		} else if wf.IsTerminal(state) {
			style = styles.TerminalState
		}
		return style.Render(state)
	}

	// Build a layer index for each state.
	stateLayer := map[string]int{}
	for li, layer := range layers {
		for _, s := range layer {
			stateLayer[s] = li
		}
	}

	arrow := styles.Arrow.Render("→")
	pipe := styles.Arrow.Render("│")

	var b strings.Builder

	for li, layer := range layers {
		// Render state boxes for this layer, centered.
		boxes := make([]string, len(layer))
		for i, s := range layer {
			boxes[i] = boxFor(s)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Bottom, interleave(boxes, "  ")...)
		rowWidth := lipgloss.Width(row)
		pad := (width - rowWidth) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad) + row + "\n")

		// Collect forward edges (to next layer) and other edges from this layer.
		var forward []workflow.Transition
		var back []workflow.Transition

		for _, t := range wf.Transitions {
			if stateLayer[t.From] != li {
				continue
			}
			toLayer := stateLayer[t.To]
			if toLayer == li+1 {
				forward = append(forward, t)
			} else {
				back = append(back, t)
			}
		}

		if len(forward) == 0 && len(back) == 0 {
			continue
		}

		// Render edge labels between layers.
		// For single-state layers with one forward edge, show a simple pipe.
		if len(layer) == 1 && len(forward) == 1 && len(back) == 0 {
			label := styles.Label.Render(forward[0].Name)
			line := fmt.Sprintf("%s %s", pipe, label)
			lineWidth := lipgloss.Width(line)
			linePad := (width - lineWidth) / 2
			if linePad < 0 {
				linePad = 0
			}
			b.WriteString(strings.Repeat(" ", linePad) + line + "\n")
			continue
		}

		// Multiple edges: list them as labeled arrows.
		for _, t := range forward {
			label := styles.Label.Render(t.Name)
			line := fmt.Sprintf("  %s %s %s  %s", t.From, arrow, t.To, label)
			b.WriteString(line + "\n")
		}
		for _, t := range back {
			backArrow := styles.Arrow.Render("↩")
			label := styles.Label.Render(t.Name)
			line := fmt.Sprintf("  %s %s %s  %s", t.From, backArrow, t.To, label)
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

// assignLayers places states into layers via BFS from the initial state.
func assignLayers(wf *workflow.Workflow) [][]string {
	depth := map[string]int{}
	visited := map[string]bool{}

	queue := []string{wf.InitialState}
	visited[wf.InitialState] = true
	depth[wf.InitialState] = 0

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, t := range wf.Transitions {
			if t.From == current && !visited[t.To] {
				visited[t.To] = true
				depth[t.To] = depth[current] + 1
				queue = append(queue, t.To)
			}
		}
	}

	// Place unreached states in the last layer + 1.
	maxDepth := 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}
	for _, s := range wf.States {
		if !visited[s] {
			depth[s] = maxDepth + 1
			visited[s] = true
			if depth[s] > maxDepth {
				maxDepth = depth[s]
			}
		}
	}

	layers := make([][]string, maxDepth+1)
	for _, s := range wf.States {
		d := depth[s]
		layers[d] = append(layers[d], s)
	}

	return layers
}

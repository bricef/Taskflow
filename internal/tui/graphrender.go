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

	// Pre-render all boxes and compute their visible widths.
	type boxInfo struct {
		rendered string
		width    int
	}
	stateBoxes := map[string]boxInfo{}
	for _, s := range wf.States {
		r := boxFor(s)
		stateBoxes[s] = boxInfo{rendered: r, width: lipgloss.Width(r)}
	}

	// Build a layer index for each state.
	stateLayer := map[string]int{}
	for li, layer := range layers {
		for _, s := range layer {
			stateLayer[s] = li
		}
	}

	// Compute center X for each state when its layer is centered.
	gap := 4
	stateCenter := map[string]int{}
	layerRows := make([]string, len(layers))

	for li, layer := range layers {
		// Render the row first so we can measure its actual width.
		boxes := make([]string, len(layer))
		for i, s := range layer {
			boxes[i] = stateBoxes[s].rendered
		}
		gapStr := strings.Repeat(" ", gap)
		row := lipgloss.JoinHorizontal(lipgloss.Bottom, interleave(boxes, gapStr)...)
		rowWidth := lipgloss.Width(row)

		// Center the row; compute the actual left padding.
		leftPad := (width - rowWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}
		row = lipgloss.PlaceHorizontal(width, lipgloss.Center, row)
		layerRows[li] = row

		// Compute centers using the same leftPad.
		x := leftPad
		for i, s := range layer {
			if i > 0 {
				x += gap
			}
			bw := stateBoxes[s].width
			stateCenter[s] = x + bw/2
			x += bw
		}
	}

	// Render layers with connectors between them.
	var b strings.Builder

	for li, row := range layerRows {
		b.WriteString(row + "\n")

		if li >= len(layers)-1 {
			continue
		}

		// Collect edges originating from this layer.
		var forward []workflow.Transition // to next layer
		var back []workflow.Transition    // to same or earlier layer
		var skip []workflow.Transition    // to layer > li+1

		for _, t := range wf.Transitions {
			if stateLayer[t.From] != li {
				continue
			}
			toLayer := stateLayer[t.To]
			switch {
			case toLayer == li+1:
				forward = append(forward, t)
			case toLayer <= li:
				back = append(back, t)
			default:
				skip = append(skip, t)
			}
		}

		// Render forward connectors as plain-text lines using rune buffers.
		// Track previous target positions so later connectors show │ where
		// earlier ones pass through.
		var prevTargets []int
		for _, t := range forward {
			line := drawConnectorWithContinuations(stateCenter[t.From], stateCenter[t.To], t.Name, width, styles, prevTargets)
			b.WriteString(line + "\n")
			prevTargets = append(prevTargets, stateCenter[t.To])
		}

		// Back-edges and skip-edges as labeled text, centered.
		for _, t := range back {
			label := styles.Label.Render(t.Name)
			arrow := styles.Arrow.Render("→")
			line := fmt.Sprintf("%s %s %s  %s", t.From, arrow, t.To, label)
			b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, line) + "\n")
		}
		for _, t := range skip {
			label := styles.Label.Render(t.Name)
			arrow := styles.Arrow.Render("→")
			line := fmt.Sprintf("%s %s %s  %s", t.From, arrow, t.To, label)
			b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, line) + "\n")
		}

		// Draw vertical drops into the next layer's boxes.
		if len(forward) > 0 {
			drops := make([]rune, width)
			for i := range drops {
				drops[i] = ' '
			}
			for _, t := range forward {
				x := stateCenter[t.To]
				if x >= 0 && x < width {
					drops[x] = '│'
				}
			}
			dropLine := strings.TrimRight(string(drops), " ")
			if strings.TrimSpace(dropLine) != "" {
				b.WriteString(dropLine + "\n")
			}
		}
	}

	return b.String()
}

// drawConnectorWithContinuations renders a connector line between fromX and toX,
// placing │ at each position in continuations where a previous connector passes through.
func drawConnectorWithContinuations(fromX, toX int, label string, width int, styles GraphStyles, continuations []int) string {
	if width <= 0 {
		width = 80
	}

	buf := make([]rune, width)
	for i := range buf {
		buf[i] = ' '
	}

	clamp := func(x int) int {
		if x < 0 {
			return 0
		}
		if x >= width {
			return width - 1
		}
		return x
	}

	fromX = clamp(fromX)
	toX = clamp(toX)

	// Place continuation markers first (may be overwritten by the connector).
	for _, cx := range continuations {
		cx = clamp(cx)
		buf[cx] = '│'
	}

	if fromX == toX {
		buf[fromX] = '│'
	} else {
		minX, maxX := fromX, toX
		if minX > maxX {
			minX, maxX = maxX, minX
		}
		for x := minX; x <= maxX; x++ {
			buf[x] = '─'
		}
		if toX > fromX {
			buf[fromX] = '╰'
			buf[toX] = '╮'
		} else {
			buf[fromX] = '╯'
			buf[toX] = '╭'
		}

		// Where a continuation crosses the horizontal line, use ┼.
		for _, cx := range continuations {
			cx = clamp(cx)
			if cx > minX && cx < maxX {
				buf[cx] = '┼'
			}
		}
	}

	line := strings.TrimRight(string(buf), " ")
	styledLabel := styles.Label.Render(label)
	return line + " " + styledLabel
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
		layers[depth[s]] = append(layers[depth[s]], s)
	}

	return layers
}

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
	edges := buildEdges(wf)

	return renderGraph(wf, layers, edges, width, styles)
}

// --- Layer assignment via BFS from initial state ---

func assignLayers(wf *workflow.Workflow) [][]string {
	depth := map[string]int{}
	visited := map[string]bool{}

	// BFS from initial state.
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

	// Any states not reached by BFS (e.g. only reachable via fromAll
	// from themselves) get placed in the last layer + 1.
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
		}
	}

	// Recompute max depth.
	maxDepth = 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}

	// Build layers preserving the original state order within each layer.
	layers := make([][]string, maxDepth+1)
	for _, s := range wf.States {
		d := depth[s]
		layers[d] = append(layers[d], s)
	}

	return layers
}

// edge represents a directed edge with a label.
type edge struct {
	from, to, label string
}

func buildEdges(wf *workflow.Workflow) []edge {
	var edges []edge
	for _, t := range wf.Transitions {
		edges = append(edges, edge{from: t.From, to: t.To, label: t.Name})
	}
	return edges
}

// --- Rendering ---

func renderGraph(wf *workflow.Workflow, layers [][]string, edges []edge, width int, styles GraphStyles) string {
	// Compute box widths: each box is the state name + padding + border.
	// We need to know the rendered width of each state box.
	boxFor := func(state string) string {
		style := styles.DefaultState
		if state == wf.InitialState {
			style = styles.InitialState
		} else if wf.IsTerminal(state) {
			style = styles.TerminalState
		}
		return style.Render(state)
	}

	// Pre-compute the column position (center x) for each state.
	// We'll render top-to-bottom, one layer per section.
	stateCenter := map[string]int{}
	var layerRenderings []string

	for _, layer := range layers {
		boxes := make([]string, len(layer))
		for i, s := range layer {
			boxes[i] = boxFor(s)
		}

		// Calculate total width of boxes + gaps.
		gap := "    "
		gapWidth := lipgloss.Width(gap)
		totalWidth := 0
		for i, box := range boxes {
			if i > 0 {
				totalWidth += gapWidth
			}
			totalWidth += lipgloss.Width(box)
		}

		// Center the layer within the available width.
		leftPad := (width - totalWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}

		// Track center positions.
		x := leftPad
		for i, s := range layer {
			if i > 0 {
				x += gapWidth
			}
			bw := lipgloss.Width(boxes[i])
			stateCenter[s] = x + bw/2
			x += bw
		}

		// Render the layer line.
		padded := strings.Repeat(" ", leftPad) + lipgloss.JoinHorizontal(lipgloss.Bottom, interleave(boxes, gap)...)
		layerRenderings = append(layerRenderings, padded)
	}

	// Build the full graph: layer, then arrows to next layer, then next layer...
	var b strings.Builder
	for li, layerStr := range layerRenderings {
		b.WriteString(layerStr + "\n")

		if li >= len(layers)-1 {
			continue
		}

		// Collect edges from this layer to the next (forward edges).
		// Also collect back-edges and skip-edges.
		currentLayer := layers[li]
		currentSet := toSet(currentLayer)

		var forwardLines []string
		var otherLines []string

		for _, e := range edges {
			if !currentSet[e.from] {
				continue
			}

			fromX := stateCenter[e.from]
			toX := stateCenter[e.to]
			label := styles.Label.Render(e.label)
			arrow := styles.Arrow.Render("→")

			// Determine if this is a forward edge (to next layer),
			// back edge, or skip edge.
			fromLayer := layerOf(layers, e.from)
			toLayer := layerOf(layers, e.to)

			if toLayer == fromLayer+1 {
				// Forward edge to next layer — draw connector.
				line := renderConnector(fromX, toX, arrow, label, width)
				forwardLines = append(forwardLines, line)
			} else {
				// Back/skip edge — render as text annotation.
				dir := "↓"
				if toLayer <= fromLayer {
					dir = styles.Arrow.Render("↩")
				}
				otherLines = append(otherLines, fmt.Sprintf("%s%s %s %s %s",
					strings.Repeat(" ", fromX-1),
					dir, e.from, arrow, e.to+" "+label))
			}
		}

		for _, line := range forwardLines {
			b.WriteString(line + "\n")
		}
		for _, line := range otherLines {
			b.WriteString(line + "\n")
		}
	}

	// Render any edges between states in the same layer or going backwards
	// that weren't covered above (fromAll edges etc).
	return b.String()
}

func renderConnector(fromX, toX int, arrow, label string, width int) string {
	// Draw a line from fromX down to toX with an arrow and label.
	if width <= 0 {
		width = 80
	}

	buf := make([]byte, width)
	for i := range buf {
		buf[i] = ' '
	}

	minX := fromX
	maxX := toX
	if minX > maxX {
		minX, maxX = maxX, minX
	}

	// Clamp to width.
	if maxX >= width {
		maxX = width - 1
	}
	if minX < 0 {
		minX = 0
	}

	// Draw the vertical drop from source.
	if fromX >= 0 && fromX < width {
		buf[fromX] = '|'
	}

	// Build a styled line showing the connection.
	if fromX == toX {
		// Straight down.
		return fmt.Sprintf("%s│ %s", strings.Repeat(" ", fromX), label)
	}

	// Angled connector: draw horizontal line between fromX and toX.
	line := make([]rune, width)
	for i := range line {
		line[i] = ' '
	}

	// Draw horizontal segment.
	for x := minX; x <= maxX; x++ {
		if x >= 0 && x < width {
			line[x] = '─'
		}
	}

	// Corner characters.
	if fromX >= 0 && fromX < width {
		if toX > fromX {
			line[fromX] = '╰'
		} else {
			line[fromX] = '╯'
		}
	}
	if toX >= 0 && toX < width {
		line[toX] = '│'
	}

	// Append label after the line.
	rendered := strings.TrimRight(string(line), " ")
	return rendered + " " + label
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func layerOf(layers [][]string, state string) int {
	for i, layer := range layers {
		for _, s := range layer {
			if s == state {
				return i
			}
		}
	}
	return -1
}

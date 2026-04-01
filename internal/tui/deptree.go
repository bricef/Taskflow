package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bricef/taskflow/internal/model"
)

var (
	depCurrentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	depRefStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	depTypeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// depTreeNode represents a task in the dependency tree.
type depTreeNode struct {
	ref      string // "board/num"
	title    string
	state    string
	current  bool           // is this the task we're viewing
	depType  string         // "depends_on" or "relates_to"
	children []*depTreeNode // tasks this one depends on
}

// taskSummary holds just enough info to display a task in the tree.
type taskSummary struct {
	BoardSlug string
	Num       int
	Title     string
	State     string
}

// buildDepTree constructs a tree rooted at the current task.
// Parents (tasks that depend on the current task) appear above,
// children (tasks the current task depends on) appear below.
func buildDepTree(current model.Task, deps []model.Dependency, related map[string]taskSummary) string {
	if len(deps) == 0 {
		return dimStyle.Render("  No dependencies") + "\n"
	}

	var parents []depTreeNode  // tasks that depend on current
	var children []depTreeNode // tasks that current depends on
	var relatesTo []depTreeNode

	currentRef := fmt.Sprintf("%s/%d", current.BoardSlug, current.Num)

	for _, dep := range deps {
		sourceRef := fmt.Sprintf("%s/%d", dep.BoardSlug, dep.TaskNum)
		targetRef := fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum)

		if dep.DependencyType == model.DependencyTypeRelatesTo {
			// "relates_to" — show the other side
			otherRef := targetRef
			if sourceRef != currentRef {
				otherRef = sourceRef
			}
			sum := related[otherRef]
			relatesTo = append(relatesTo, depTreeNode{
				ref:     otherRef,
				title:   sum.Title,
				state:   sum.State,
				depType: "relates_to",
			})
			continue
		}

		// depends_on: source depends on target
		// source is parent, target is child
		if sourceRef == currentRef {
			// Current task depends on target → target is a child
			sum := related[targetRef]
			children = append(children, depTreeNode{
				ref:     targetRef,
				title:   sum.Title,
				state:   sum.State,
				depType: "depends_on",
			})
		} else {
			// Another task depends on current → that task is a parent
			sum := related[sourceRef]
			parents = append(parents, depTreeNode{
				ref:     sourceRef,
				title:   sum.Title,
				state:   sum.State,
				depType: "depends_on",
			})
		}
	}

	var b strings.Builder

	// Parents (tasks that depend on current task) — outer level.
	for _, p := range parents {
		b.WriteString(renderDepNode("◆ ", p) + "\n")
	}

	// Current task (highlighted) — indented under parents.
	currentTitle := current.Title
	if len(currentTitle) > 50 {
		currentTitle = currentTitle[:47] + "..."
	}
	prefix := ""
	childIndent := ""
	if len(parents) > 0 {
		prefix = "└──"
		childIndent = "   "
	}
	b.WriteString(prefix + depCurrentStyle.Render(fmt.Sprintf("◆ %s: %s", currentRef, currentTitle)) + "  " + dimStyle.Render("["+current.State+"]") + "\n")

	// Children and related — indented under current task.
	allBelow := make([]string, 0, len(children)+len(relatesTo))
	for _, c := range children {
		allBelow = append(allBelow, renderDepNode("◆ ", c))
	}
	for _, r := range relatesTo {
		allBelow = append(allBelow, renderRelated("◆ ", r))
	}
	for i, line := range allBelow {
		connector := "├──"
		if i == len(allBelow)-1 {
			connector = "└──"
		}
		b.WriteString(childIndent + connector + line + "\n")
	}

	return b.String()
}

func renderDepNode(prefix string, node depTreeNode) string {
	title := node.title
	if title == "" {
		title = "(unknown)"
	}
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	ref := depRefStyle.Render(node.ref)
	state := dimStyle.Render("[" + node.state + "]")
	return prefix + fmt.Sprintf("%s: %s  %s", ref, title, state)
}

func renderRelated(prefix string, node depTreeNode) string {
	title := node.title
	if title == "" {
		title = "(unknown)"
	}
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	ref := depRefStyle.Render(node.ref)
	state := dimStyle.Render("[" + node.state + "]")
	label := depTypeStyle.Render("relates to")
	return prefix + fmt.Sprintf("%s %s:  %s  %s", label, ref, title, state)
}

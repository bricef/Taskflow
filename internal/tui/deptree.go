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
	current  bool            // is this the task we're viewing
	depType  string          // "depends_on" or "relates_to"
	children []*depTreeNode  // tasks this one depends on
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

	// Parents (tasks that depend on current task).
	for i, p := range parents {
		prefix := "├── "
		if i == len(parents)-1 && len(children) == 0 && len(relatesTo) == 0 {
			prefix = "├── "
		}
		b.WriteString(renderDepNode(prefix, p) + "\n")
		b.WriteString("│\n")
	}

	// Current task (highlighted).
	currentTitle := current.Title
	if len(currentTitle) > 50 {
		currentTitle = currentTitle[:47] + "..."
	}
	b.WriteString(depCurrentStyle.Render(fmt.Sprintf("◆ %s: %s", currentRef, currentTitle)) + "  " + dimStyle.Render("["+current.State+"]") + "\n")

	// Children (tasks that current depends on).
	for i, c := range children {
		connector := "├── "
		if i == len(children)-1 && len(relatesTo) == 0 {
			connector = "└── "
		}
		b.WriteString(renderDepNode(connector, c) + "\n")
	}

	// Related tasks.
	for i, r := range relatesTo {
		connector := "├── "
		if i == len(relatesTo)-1 {
			connector = "└── "
		}
		b.WriteString(renderRelated(connector, r) + "\n")
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
	return fmt.Sprintf("%s%s: %s  %s", prefix, ref, title, state)
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
	return fmt.Sprintf("%s%s %s: %s  %s", prefix, label, ref, title, state)
}

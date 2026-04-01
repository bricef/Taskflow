package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bricef/taskflow/internal/httpclient"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

// myTasksWorkflowsLoaded carries cached workflows for boards in the My Tasks view.
type myTasksWorkflowsLoaded struct {
	workflows map[string]*workflow.Workflow
}

// myTasksModel shows tasks assigned to the current user across all boards.
type myTasksModel struct {
	table     taskTable
	workflows map[string]*workflow.Workflow // cached per board slug
}

func newMyTasks() myTasksModel {
	return myTasksModel{
		workflows: map[string]*workflow.Workflow{},
		table: newTaskTable([]taskColumn{
			colTaskRef,
			colTitle,
			colState,
			colPriority,
		}, 3), // default sort: priority
	}
}

func (m *myTasksModel) bindTerminalFunc() {
	workflows := m.workflows
	m.table.isTerminal = func(task model.Task) bool {
		if wf, ok := workflows[task.BoardSlug]; ok {
			return wf.IsTerminal(task.State)
		}
		return false
	}
}

// missingWorkflowSlugs returns board slugs present in tasks but not yet cached.
func (m *myTasksModel) missingWorkflowSlugs() []string {
	seen := map[string]bool{}
	var missing []string
	for _, t := range m.table.tasks {
		if _, ok := m.workflows[t.BoardSlug]; !ok && !seen[t.BoardSlug] {
			seen[t.BoardSlug] = true
			missing = append(missing, t.BoardSlug)
		}
	}
	return missing
}

func fetchWorkflowsForBoards(client *httpclient.Client, slugs []string) tea.Cmd {
	return func() tea.Msg {
		result := make(map[string]*workflow.Workflow, len(slugs))
		for _, slug := range slugs {
			wf, err := httpclient.GetOne[workflow.Workflow](client, model.ResWorkflowGet, httpclient.PathParams{"slug": slug}, nil)
			if err == nil {
				result[slug] = &wf
			}
		}
		return myTasksWorkflowsLoaded{workflows: result}
	}
}

// Delegate methods to the embedded table.

func (m *myTasksModel) load(tasks []model.Task) {
	m.bindTerminalFunc()
	m.table.load(tasks)
}
func (m *myTasksModel) updateTask(task model.Task)               { m.table.updateTask(task) }
func (m *myTasksModel) removeTask(boardSlug string, num int)     { m.table.removeTask(boardSlug, num) }
func (m *myTasksModel) selectedTask() *model.Task                { return m.table.selectedTask() }
func (m *myTasksModel) rebuild()                     { m.table.rebuild() }
func (m *myTasksModel) update(msg tea.KeyMsg)        { m.table.update(msg) }
func (m myTasksModel) view(width, height int) string { return m.table.view(width, height) }

// colTaskRef renders the board/num task reference.
var colTaskRef = taskColumn{
	Key:     "task",
	Title:   "Task",
	Width:   20,
	Visible: true,
	Render: func(t model.Task) string {
		return fmt.Sprintf("%s/%d", t.BoardSlug, t.Num)
	},
	Value: func(t model.Task) string {
		return fmt.Sprintf("%s/%d", t.BoardSlug, t.Num)
	},
}

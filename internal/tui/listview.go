package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/workflow"
)

// listViewModel is the list tab within a board view.
type listViewModel struct {
	table    taskTable
	workflow *workflow.Workflow
}

func newListView(currentUserName string) listViewModel {
	return listViewModel{
		table: newTaskTable([]taskColumn{
			colNum,
			colTitle,
			colState,
			colPriority,
			newColAssignee(currentUserName),
		}, 3), // default sort: priority
	}
}

func (m *listViewModel) load(data boardDataLoaded) {
	if data.err != nil {
		return
	}
	m.workflow = data.workflow
	wf := m.workflow
	m.table.isTerminal = func(task model.Task) bool {
		return wf != nil && wf.IsTerminal(task.State)
	}
	m.table.load(data.tasks)
}

func (m *listViewModel) updateTask(task model.Task)           { m.table.updateTask(task) }
func (m *listViewModel) removeTask(boardSlug string, num int) { m.table.removeTask(boardSlug, num) }
func (m *listViewModel) selectedTask() *model.Task            { return m.table.selectedTask() }
func (m *listViewModel) update(msg tea.KeyMsg)                { m.table.update(msg) }
func (m listViewModel) view(width, height int) string         { return m.table.view(width, height) }

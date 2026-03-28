package service

import (
	"context"
	"encoding/json"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
	"github.com/bricef/taskflow/internal/workflow"
)

func (s *Service) CreateBoard(ctx context.Context, params model.CreateBoardParams) (model.Board, error) {
	if err := params.Validate(); err != nil {
		return model.Board{}, err
	}
	if len(params.Workflow) == 0 {
		params.Workflow = workflow.DefaultWorkflowJSON
	}
	if _, err := workflow.Parse(params.Workflow); err != nil {
		return model.Board{}, &model.ValidationError{Field: "workflow", Message: err.Error()}
	}

	var board model.Board
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		board, err = s.store.BoardInsert(ctx, tx, model.Board{
			Slug:        params.Slug,
			Name:        params.Name,
			Description: params.Description,
			Workflow:    params.Workflow,
		})
		return err
	})
	return board, err
}

func (s *Service) GetWorkflow(ctx context.Context, boardSlug string) (*workflow.Workflow, error) {
	board, err := s.store.BoardGet(ctx, boardSlug)
	if err != nil {
		return nil, err
	}
	return workflow.Parse(board.Workflow)
}

func (s *Service) SetWorkflow(ctx context.Context, boardSlug string, workflowJSON json.RawMessage, actor string) error {
	if _, err := workflow.Parse(workflowJSON); err != nil {
		return &model.ValidationError{Field: "workflow", Message: err.Error()}
	}

	board, err := s.store.BoardGet(ctx, boardSlug)
	if err != nil {
		return err
	}

	return s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.BoardSetWorkflow(ctx, tx, boardSlug, workflowJSON); err != nil {
			return err
		}
		return s.audit(ctx, tx, boardSlug, nil, actor, model.AuditActionWorkflowChanged, map[string]any{
			"board": boardSlug, "old_states": extractStates(board.Workflow), "new_states": extractStates(workflowJSON),
		})
	})
}

func (s *Service) CheckWorkflowHealth(ctx context.Context, boardSlug string) ([]workflow.HealthIssue, error) {
	w, err := s.GetWorkflow(ctx, boardSlug)
	if err != nil {
		return nil, err
	}

	tasks, err := s.store.TaskList(ctx, model.TaskFilter{BoardSlug: boardSlug, IncludeDeleted: false}, nil)
	if err != nil {
		return nil, err
	}

	states := make([]string, len(tasks))
	for i, t := range tasks {
		states[i] = t.State
	}

	return w.HealthCheck(states), nil
}

func extractStates(workflowJSON json.RawMessage) []string {
	var raw struct {
		States []string `json:"states"`
	}
	json.Unmarshal(workflowJSON, &raw)
	return raw.States
}

func (s *Service) GetBoard(ctx context.Context, slug string) (model.Board, error) {
	return s.store.BoardGet(ctx, slug)
}

func (s *Service) ListBoards(ctx context.Context, params model.ListBoardsParams) ([]model.Board, error) {
	return s.store.BoardList(ctx, params)
}

func (s *Service) UpdateBoard(ctx context.Context, params model.UpdateBoardParams) (model.Board, error) {
	var board model.Board
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		board, err = s.store.BoardUpdate(ctx, tx, params)
		return err
	})
	return board, err
}

func (s *Service) DeleteBoard(ctx context.Context, slug, actor string) error {
	return s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.BoardSetDeleted(ctx, tx, slug); err != nil {
			return err
		}
		return s.audit(ctx, tx, slug, nil, actor, model.AuditActionBoardDeleted, map[string]string{"board": slug})
	})
}

func (s *Service) ReassignTasks(ctx context.Context, fromSlug, toSlug, actor string, states []string) (int, error) {
	// Verify both boards exist.
	if _, err := s.store.BoardGet(ctx, fromSlug); err != nil {
		return 0, err
	}
	if _, err := s.store.BoardGet(ctx, toSlug); err != nil {
		return 0, err
	}

	// Fetch tasks to move.
	filter := model.TaskFilter{BoardSlug: fromSlug}
	if len(states) > 0 {
		// Filter by specific states — need to list all then filter, or list per state.
		// For simplicity, list all non-deleted from board and filter in Go.
	}
	allTasks, err := s.store.TaskList(ctx, filter, nil)
	if err != nil {
		return 0, err
	}

	var tasksToMove []model.Task
	for _, t := range allTasks {
		if len(states) > 0 {
			match := false
			for _, st := range states {
				if t.State == st {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		tasksToMove = append(tasksToMove, t)
	}

	if len(tasksToMove) == 0 {
		return 0, nil
	}

	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		// Allocate new task numbers on target board.
		numMap := make(map[int]int) // oldNum → newNum
		for _, t := range tasksToMove {
			newNum, err := s.store.BoardAllocateTaskNum(ctx, tx, toSlug)
			if err != nil {
				return err
			}
			numMap[t.Num] = newNum

			// Insert task with new board/num.
			newTask := t
			newTask.BoardSlug = toSlug
			newTask.Num = newNum
			newTask.Tags = t.Tags
			if newTask.Tags == nil {
				newTask.Tags = []string{}
			}
			if _, err := s.store.TaskInsert(ctx, tx, newTask); err != nil {
				return err
			}
		}

		// Migrate related records.
		oldNums := make([]int, 0, len(numMap))
		for oldNum, newNum := range numMap {
			oldNums = append(oldNums, oldNum)
			if err := s.store.CommentUpdateTaskRef(ctx, tx, fromSlug, oldNum, toSlug, newNum); err != nil {
				return err
			}
			if err := s.store.AttachmentUpdateTaskRef(ctx, tx, fromSlug, oldNum, toSlug, newNum); err != nil {
				return err
			}
			if err := s.store.DependencyUpdateTaskRefs(ctx, tx, fromSlug, oldNum, toSlug, newNum); err != nil {
				return err
			}
			if err := s.store.AuditUpdateTaskRef(ctx, tx, fromSlug, oldNum, toSlug, newNum); err != nil {
				return err
			}
		}

		// Delete old task rows.
		if err := s.store.TaskDeleteByBoardAndNums(ctx, tx, fromSlug, oldNums); err != nil {
			return err
		}

		// Record audit entries on both boards.
		count := len(tasksToMove)
		detail := map[string]any{"from_board": fromSlug, "to_board": toSlug, "task_count": count}
		if err := s.audit(ctx, tx, fromSlug, nil, actor, model.AuditActionTasksReassigned, detail); err != nil {
			return err
		}
		return s.audit(ctx, tx, toSlug, nil, actor, model.AuditActionTasksReassigned, detail)
	})

	return len(tasksToMove), err
}

func marshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

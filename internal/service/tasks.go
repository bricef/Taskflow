package service

import (
	"context"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
	"github.com/bricef/taskflow/internal/workflow"
)

func (s *Service) CreateTask(ctx context.Context, params model.CreateTaskParams) (model.Task, error) {
	if err := params.Validate(); err != nil {
		return model.Task{}, err
	}
	if params.Priority == "" {
		params.Priority = model.PriorityNone
	}

	// Derive initial state from the board's workflow.
	board, err := s.store.BoardGet(ctx, params.BoardSlug)
	if err != nil {
		return model.Task{}, err
	}
	w, err := workflow.Parse(board.Workflow)
	if err != nil {
		return model.Task{}, err
	}

	var task model.Task
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		num, err := s.store.BoardAllocateTaskNum(ctx, tx, params.BoardSlug)
		if err != nil {
			return err
		}

		tags := params.Tags
		if tags == nil {
			tags = []string{}
		}

		task, err = s.store.TaskInsert(ctx, tx, model.Task{
			BoardSlug:   params.BoardSlug,
			Num:         num,
			Title:       params.Title,
			Description: params.Description,
			State:       w.InitialState,
			Priority:    params.Priority,
			Tags:        tags,
			Assignee:    params.Assignee,
			DueDate:     params.DueDate,
			CreatedBy:   params.CreatedBy,
		})
		if err != nil {
			return err
		}

		return s.audit(ctx, tx, params.BoardSlug, &num, params.CreatedBy, model.AuditActionCreated, map[string]any{
			"title": params.Title, "state": w.InitialState,
		})
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:  eventbus.EventTaskCreated,
			Actor: actorRef(params.CreatedBy),
			Board: boardRef(params.BoardSlug),
			Task:  taskRef(task),
		})
	}
	return task, err
}

func (s *Service) GetTask(ctx context.Context, boardSlug string, num int) (model.Task, error) {
	return s.store.TaskGet(ctx, boardSlug, num)
}

func (s *Service) ListTasks(ctx context.Context, filter model.TaskFilter, sort *model.TaskSort) ([]model.Task, error) {
	return s.store.TaskList(ctx, filter, sort)
}

func (s *Service) TransitionTask(ctx context.Context, params model.TransitionTaskParams) (model.Task, error) {
	task, err := s.store.TaskGet(ctx, params.BoardSlug, params.Num)
	if err != nil {
		return model.Task{}, err
	}

	board, err := s.store.BoardGet(ctx, params.BoardSlug)
	if err != nil {
		return model.Task{}, err
	}
	w, err := workflow.Parse(board.Workflow)
	if err != nil {
		return model.Task{}, err
	}

	newState, err := w.ExecuteTransition(task.State, params.TransitionName)
	if err != nil {
		available := w.AvailableTransitions(task.State)
		names := make([]string, len(available))
		for i, t := range available {
			names[i] = t.Name
		}
		return model.Task{}, &model.ValidationError{
			Field:   "transition",
			Message: err.Error(),
			Detail: map[string]any{
				"current_state": task.State,
				"available":     names,
			},
		}
	}

	var updated model.Task
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		updated, err = s.store.TaskUpdate(ctx, tx, model.UpdateTaskParams{
			BoardSlug: params.BoardSlug,
			Num:       params.Num,
			State:     model.Set(newState),
		})
		if err != nil {
			return err
		}

		auditDetail := map[string]any{
			"from": task.State, "to": newState, "transition": params.TransitionName,
		}
		if params.Comment != "" {
			auditDetail["comment"] = params.Comment
		}

		if err := s.audit(ctx, tx, params.BoardSlug, &params.Num, params.Actor, model.AuditActionTransitioned, auditDetail); err != nil {
			return err
		}

		// If a comment was provided, also create a comment record.
		if params.Comment != "" {
			_, err := s.store.CommentInsert(ctx, tx, model.Comment{
				BoardSlug: params.BoardSlug,
				TaskNum:   params.Num,
				Actor:     params.Actor,
				Body:      params.Comment,
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err == nil {
		ref := taskRef(updated)
		ref.PreviousState = task.State
		s.emit(eventbus.Event{
			Type:   eventbus.EventTaskTransitioned,
			Actor:  actorRef(params.Actor),
			Board:  boardRef(params.BoardSlug),
			Task:   ref,
			Detail: map[string]any{"from": task.State, "to": updated.State, "transition": params.TransitionName},
		})
	}
	return updated, err
}

func (s *Service) UpdateTask(ctx context.Context, params model.UpdateTaskParams, actor string) (model.Task, error) {
	if params.Priority.Set {
		if err := model.ValidatePriority(params.Priority.Value); err != nil {
			return model.Task{}, err
		}
	}

	old, err := s.store.TaskGet(ctx, params.BoardSlug, params.Num)
	if err != nil {
		return model.Task{}, err
	}

	changes := map[string]any{}
	if params.Title.Set {
		changes["title"] = map[string]any{"old": old.Title, "new": params.Title.Value}
	}
	if params.Description.Set {
		changes["description"] = map[string]any{"old": old.Description, "new": params.Description.Value}
	}
	if params.State.Set {
		changes["state"] = map[string]any{"old": old.State, "new": params.State.Value}
	}
	if params.Priority.Set {
		changes["priority"] = map[string]any{"old": string(old.Priority), "new": string(params.Priority.Value)}
	}
	if params.Tags.Set {
		changes["tags"] = map[string]any{"old": old.Tags, "new": params.Tags.Value}
	}
	if params.Assignee.Set {
		changes["assignee"] = map[string]any{"old": old.Assignee, "new": params.Assignee.Value}
	}
	if params.DueDate.Set {
		changes["due_date"] = map[string]any{"old": old.DueDate, "new": params.DueDate.Value}
	}

	if len(changes) == 0 {
		return old, nil
	}

	var task model.Task
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		task, err = s.store.TaskUpdate(ctx, tx, params)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, params.BoardSlug, &params.Num, actor, model.AuditActionUpdated, map[string]any{"fields": changes})
	})
	if err == nil {
		if params.Assignee.Set {
			s.emit(eventbus.Event{
				Type:   eventbus.EventTaskAssigned,
				Actor:  actorRef(actor),
				Board:  boardRef(params.BoardSlug),
				Task:   taskRef(task),
				Detail: map[string]any{"assignee": params.Assignee.Value},
			})
		} else {
			s.emit(eventbus.Event{
				Type:   eventbus.EventTaskUpdated,
				Actor:  actorRef(actor),
				Board:  boardRef(params.BoardSlug),
				Task:   taskRef(task),
				Detail: map[string]any{"fields": changes},
			})
		}
	}
	return task, err
}

func (s *Service) DeleteTask(ctx context.Context, boardSlug string, num int, actor string) error {
	task, err := s.store.TaskGet(ctx, boardSlug, num)
	if err != nil {
		return err
	}
	if task.Deleted {
		return &model.NotFoundError{Resource: "task"}
	}

	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.TaskSetDeleted(ctx, tx, boardSlug, num); err != nil {
			return err
		}
		return s.audit(ctx, tx, boardSlug, &num, actor, model.AuditActionDeleted, nil)
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:  eventbus.EventTaskDeleted,
			Actor: actorRef(actor),
			Board: boardRef(boardSlug),
			Task:  taskRef(task),
		})
	}
	return err
}

func (s *Service) ListTags(ctx context.Context, boardSlug string) ([]model.TagCount, error) {
	return s.store.TaskListTags(ctx, boardSlug)
}

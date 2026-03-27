package service

import (
	"context"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateTask(ctx context.Context, params model.CreateTaskParams) (model.Task, error) {
	if err := params.Validate(); err != nil {
		return model.Task{}, err
	}

	var task model.Task
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
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
			State:       params.State,
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
			"title": params.Title, "state": params.State,
		})
	})
	return task, err
}

func (s *Service) GetTask(ctx context.Context, boardSlug string, num int) (model.Task, error) {
	return s.store.TaskGet(ctx, boardSlug, num)
}

func (s *Service) ListTasks(ctx context.Context, filter model.TaskFilter, sort *model.TaskSort) ([]model.Task, error) {
	return s.store.TaskList(ctx, filter, sort)
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

	return s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.TaskSetDeleted(ctx, tx, boardSlug, num); err != nil {
			return err
		}
		return s.audit(ctx, tx, boardSlug, &num, actor, model.AuditActionDeleted, nil)
	})
}

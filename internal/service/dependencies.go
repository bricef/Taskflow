package service

import (
	"context"
	"fmt"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateDependency(ctx context.Context, params model.CreateDependencyParams) (model.Dependency, error) {
	if err := params.Validate(); err != nil {
		return model.Dependency{}, err
	}
	if err := s.checkNotArchived(ctx, params.BoardSlug); err != nil {
		return model.Dependency{}, err
	}

	task, err := s.store.TaskGet(ctx, params.BoardSlug, params.TaskNum)
	if err != nil {
		return model.Dependency{}, err
	}

	var dep model.Dependency
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		dep, err = s.store.DependencyInsert(ctx, tx, model.Dependency{
			BoardSlug:      params.BoardSlug,
			TaskNum:        params.TaskNum,
			DependsOnBoard: params.DependsOnBoard,
			DependsOnNum:   params.DependsOnNum,
			DependencyType: params.DependencyType,
			CreatedBy:      params.CreatedBy,
		})
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, params.BoardSlug, &params.TaskNum, params.CreatedBy, model.AuditActionDependencyAdded, map[string]any{
			"depends_on": fmt.Sprintf("%s/%d", params.DependsOnBoard, params.DependsOnNum),
			"dep_type":   string(params.DependencyType),
		})
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:  eventbus.EventDependencyAdded,
			Actor: actorRef(params.CreatedBy),
			Board: boardRef(params.BoardSlug),
			After: taskSnap(task),
			Detail: map[string]any{
				"dependency_id": dep.ID,
				"depends_on":    fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum),
				"type":          string(dep.DependencyType),
			},
		})
	}
	return dep, err
}

func (s *Service) ListDependencies(ctx context.Context, boardSlug string, taskNum int) ([]model.Dependency, error) {
	return s.store.DependencyList(ctx, boardSlug, taskNum)
}

func (s *Service) DeleteDependency(ctx context.Context, id int, actor string) error {
	dep, err := s.store.DependencyGet(ctx, id)
	if err != nil {
		return err
	}
	if err := s.checkNotArchived(ctx, dep.BoardSlug); err != nil {
		return err
	}

	task, err := s.store.TaskGet(ctx, dep.BoardSlug, dep.TaskNum)
	if err != nil {
		return err
	}

	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.DependencyDelete(ctx, tx, id); err != nil {
			return err
		}
		return s.audit(ctx, tx, dep.BoardSlug, &dep.TaskNum, actor, model.AuditActionDependencyRemoved, map[string]any{
			"depends_on": fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum),
			"dep_type":   string(dep.DependencyType),
		})
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:  eventbus.EventDependencyRemoved,
			Actor: actorRef(actor),
			Board: boardRef(dep.BoardSlug),
			After: taskSnap(task),
			Detail: map[string]any{
				"dependency_id": dep.ID,
				"depends_on":    fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum),
				"type":          string(dep.DependencyType),
			},
		})
	}
	return err
}

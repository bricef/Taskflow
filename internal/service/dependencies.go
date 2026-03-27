package service

import (
	"context"
	"fmt"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateDependency(ctx context.Context, params model.CreateDependencyParams) (model.Dependency, error) {
	if err := params.Validate(); err != nil {
		return model.Dependency{}, err
	}

	var dep model.Dependency
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
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

	return s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.DependencyDelete(ctx, tx, id); err != nil {
			return err
		}
		return s.audit(ctx, tx, dep.BoardSlug, &dep.TaskNum, actor, model.AuditActionDependencyRemoved, map[string]any{
			"depends_on": fmt.Sprintf("%s/%d", dep.DependsOnBoard, dep.DependsOnNum),
			"dep_type":   string(dep.DependencyType),
		})
	})
}

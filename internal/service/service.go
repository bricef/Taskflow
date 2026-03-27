// Package service contains the business logic layer. It orchestrates
// validation, audit recording, and transactional operations by calling
// the repo interfaces. The service layer is storage-agnostic.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
	"github.com/bricef/taskflow/internal/taskflow"
)

type Service struct {
	store repo.Store
}

// New creates a new Service backed by the given store.
func New(store repo.Store) taskflow.TaskFlow {
	return &Service{store: store}
}

func (s *Service) audit(ctx context.Context, tx repo.Tx, boardSlug string, taskNum *int, actor string, action model.AuditAction, detail any) error {
	var detailJSON json.RawMessage
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		detailJSON = b
	}
	return s.store.AuditInsert(ctx, tx, model.AuditEntry{
		BoardSlug: boardSlug,
		TaskNum:   taskNum,
		Actor:     actor,
		Action:    action,
		Detail:    detailJSON,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	})
}

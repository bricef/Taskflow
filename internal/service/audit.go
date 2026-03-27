package service

import (
	"context"

	"github.com/bricef/taskflow/internal/model"
)

func (s *Service) QueryAuditByTask(ctx context.Context, boardSlug string, taskNum int) ([]model.AuditEntry, error) {
	return s.store.AuditQueryByTask(ctx, boardSlug, taskNum)
}

func (s *Service) QueryAuditByBoard(ctx context.Context, boardSlug string) ([]model.AuditEntry, error) {
	return s.store.AuditQueryByBoard(ctx, boardSlug)
}

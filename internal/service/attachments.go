package service

import (
	"context"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateAttachment(ctx context.Context, params model.CreateAttachmentParams) (model.Attachment, error) {
	if err := params.Validate(); err != nil {
		return model.Attachment{}, err
	}

	task, err := s.store.TaskGet(ctx, params.BoardSlug, params.TaskNum)
	if err != nil {
		return model.Attachment{}, err
	}

	var att model.Attachment
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		att, err = s.store.AttachmentInsert(ctx, tx, model.Attachment{
			BoardSlug: params.BoardSlug,
			TaskNum:   params.TaskNum,
			RefType:   params.RefType,
			Reference: params.Reference,
			Label:     params.Label,
			CreatedBy: params.CreatedBy,
		})
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, params.BoardSlug, &params.TaskNum, params.CreatedBy, model.AuditActionAttachmentAdded, map[string]any{
			"attachment_id": att.ID,
		})
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:  eventbus.EventAttachmentAdded,
			Actor: actorRef(params.CreatedBy),
			Board: boardRef(params.BoardSlug),
			Task:  taskRef(task),
		})
	}
	return att, err
}

func (s *Service) ListAttachments(ctx context.Context, boardSlug string, taskNum int) ([]model.Attachment, error) {
	return s.store.AttachmentList(ctx, boardSlug, taskNum)
}

func (s *Service) DeleteAttachment(ctx context.Context, id int, actor string) error {
	att, err := s.store.AttachmentGet(ctx, id)
	if err != nil {
		return err
	}

	task, err := s.store.TaskGet(ctx, att.BoardSlug, att.TaskNum)
	if err != nil {
		return err
	}

	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		if err := s.store.AttachmentDelete(ctx, tx, id); err != nil {
			return err
		}
		return s.audit(ctx, tx, att.BoardSlug, &att.TaskNum, actor, model.AuditActionAttachmentRemoved, map[string]any{
			"attachment_id": id,
		})
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:  eventbus.EventAttachmentRemoved,
			Actor: actorRef(actor),
			Board: boardRef(att.BoardSlug),
			Task:  taskRef(task),
		})
	}
	return err
}

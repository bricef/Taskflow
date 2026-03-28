package service

import (
	"context"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateComment(ctx context.Context, params model.CreateCommentParams) (model.Comment, error) {
	if err := params.Validate(); err != nil {
		return model.Comment{}, err
	}

	// Verify task exists.
	task, err := s.store.TaskGet(ctx, params.BoardSlug, params.TaskNum)
	if err != nil {
		return model.Comment{}, err
	}

	var comment model.Comment
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		comment, err = s.store.CommentInsert(ctx, tx, model.Comment{
			BoardSlug: params.BoardSlug,
			TaskNum:   params.TaskNum,
			Actor:     params.Actor,
			Body:      params.Body,
		})
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, params.BoardSlug, &params.TaskNum, params.Actor, model.AuditActionCommented, map[string]any{
			"comment_id": comment.ID,
		})
	})
	if err == nil {
		s.emit(eventbus.Event{
			Type:   eventbus.EventTaskCommented,
			Actor:  actorRef(params.Actor),
			Board:  boardRef(params.BoardSlug),
			Task:   taskRef(task),
			Detail: map[string]any{"body": params.Body},
		})
	}
	return comment, err
}

func (s *Service) ListComments(ctx context.Context, boardSlug string, taskNum int) ([]model.Comment, error) {
	return s.store.CommentList(ctx, boardSlug, taskNum)
}

func (s *Service) UpdateComment(ctx context.Context, params model.UpdateCommentParams, actor string) (model.Comment, error) {
	old, err := s.store.CommentGet(ctx, params.ID)
	if err != nil {
		return model.Comment{}, err
	}

	var comment model.Comment
	err = s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		comment, err = s.store.CommentUpdateBody(ctx, tx, params.ID, params.Body)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, old.BoardSlug, &old.TaskNum, actor, model.AuditActionCommentEdited, map[string]any{
			"comment_id": old.ID, "old_body": old.Body, "new_body": params.Body,
		})
	})
	return comment, err
}

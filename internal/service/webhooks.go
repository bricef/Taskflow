package service

import (
	"context"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateWebhook(ctx context.Context, params model.CreateWebhookParams) (model.Webhook, error) {
	if err := params.Validate(); err != nil {
		return model.Webhook{}, err
	}

	var webhook model.Webhook
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		webhook, err = s.store.WebhookInsert(ctx, tx, model.Webhook{
			URL:       params.URL,
			Events:    params.Events,
			BoardSlug: params.BoardSlug,
			Secret:    params.Secret,
			CreatedBy: params.CreatedBy,
		})
		return err
	})
	return webhook, err
}

func (s *Service) GetWebhook(ctx context.Context, id int) (model.Webhook, error) {
	return s.store.WebhookGet(ctx, id)
}

func (s *Service) ListWebhooks(ctx context.Context) ([]model.Webhook, error) {
	return s.store.WebhookList(ctx)
}

func (s *Service) UpdateWebhook(ctx context.Context, params model.UpdateWebhookParams) (model.Webhook, error) {
	var webhook model.Webhook
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		webhook, err = s.store.WebhookUpdate(ctx, tx, params)
		return err
	})
	return webhook, err
}

func (s *Service) DeleteWebhook(ctx context.Context, id int) error {
	return s.store.InTransaction(ctx, func(tx repo.Tx) error {
		return s.store.WebhookDelete(ctx, tx, id)
	})
}

func (s *Service) ListWebhookDeliveries(ctx context.Context, webhookID int) ([]model.WebhookDelivery, error) {
	// Verify webhook exists.
	if _, err := s.store.WebhookGet(ctx, webhookID); err != nil {
		return nil, err
	}
	return s.store.WebhookDeliveryList(ctx, webhookID)
}

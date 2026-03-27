package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

type webhookRow struct {
	ID        int        `db:"id"`
	URL       string     `db:"url"`
	Events    StringList `db:"events"`
	BoardSlug *string    `db:"board_slug"`
	Secret    string     `db:"secret"`
	Active    SQLiteBool `db:"active"`
	CreatedBy string     `db:"created_by"`
	CreatedAt Timestamp  `db:"created_at"`
	UpdatedAt Timestamp  `db:"updated_at"`
}

func (s *Store) WebhookInsert(ctx context.Context, tx repo.Tx, webhook model.Webhook) (model.Webhook, error) {
	events := StringList(webhook.Events)
	eventsVal, _ := events.Value()
	result, err := asTx(tx).ExecContext(ctx,
		`INSERT INTO webhooks (url, events, board_slug, secret, created_by) VALUES (?, ?, ?, ?, ?)`,
		webhook.URL, eventsVal, webhook.BoardSlug, webhook.Secret, webhook.CreatedBy,
	)
	if err != nil {
		return model.Webhook{}, err
	}
	id, _ := result.LastInsertId()
	return s.webhookGet(ctx, tx, int(id))
}

func (s *Store) WebhookGet(ctx context.Context, id int) (model.Webhook, error) {
	return s.webhookGet(ctx, nil, id)
}

func (s *Store) webhookGet(ctx context.Context, tx repo.Tx, id int) (model.Webhook, error) {
	var r webhookRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM webhooks WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return model.Webhook{}, notFound("webhook", fmt.Sprintf("%d", id))
	}
	if err != nil {
		return model.Webhook{}, err
	}
	return toModel[webhookRow, model.Webhook](r), nil
}

func (s *Store) WebhookList(ctx context.Context) ([]model.Webhook, error) {
	var rows []webhookRow
	err := sqlx.SelectContext(ctx, s.db, &rows, "SELECT * FROM webhooks ORDER BY id")
	if err != nil {
		return nil, err
	}
	return toModelSlice[webhookRow, model.Webhook](rows), nil
}

func (s *Store) WebhookUpdate(ctx context.Context, tx repo.Tx, params model.UpdateWebhookParams) (model.Webhook, error) {
	var setClauses []string
	var args []any

	if params.URL.Set {
		setClauses = append(setClauses, "url = ?")
		args = append(args, params.URL.Value)
	}
	if params.Events.Set {
		events := StringList(params.Events.Value)
		eventsVal, _ := events.Value()
		setClauses = append(setClauses, "events = ?")
		args = append(args, eventsVal)
	}
	if params.Active.Set {
		setClauses = append(setClauses, "active = ?")
		args = append(args, SQLiteBool(params.Active.Value))
	}
	if len(setClauses) == 0 {
		return s.webhookGet(ctx, tx, params.ID)
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, nowUTC())
	args = append(args, params.ID)

	query := fmt.Sprintf("UPDATE webhooks SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	result, err := asTx(tx).ExecContext(ctx, query, args...)
	if err != nil {
		return model.Webhook{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return model.Webhook{}, notFound("webhook", fmt.Sprintf("%d", params.ID))
	}
	return s.webhookGet(ctx, tx, params.ID)
}

func (s *Store) WebhookDelete(ctx context.Context, tx repo.Tx, id int) error {
	result, err := asTx(tx).ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("webhook", fmt.Sprintf("%d", id))
	}
	return nil
}

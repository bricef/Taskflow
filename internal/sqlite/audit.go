package sqlite

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

type auditRow struct {
	ID        int       `db:"id"`
	BoardSlug string    `db:"board_slug"`
	TaskNum   *int      `db:"task_num"`
	Actor     string    `db:"actor"`
	Action    string    `db:"action"`
	Detail    JSONRaw   `db:"detail"`
	CreatedAt Timestamp `db:"timestamp"`
}

func (s *Store) AuditInsert(ctx context.Context, tx repo.Tx, entry model.AuditEntry) error {
	detail := entry.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	_, err := asTx(tx).ExecContext(ctx,
		`INSERT INTO audit_log (board_slug, task_num, actor, action, detail, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.BoardSlug, entry.TaskNum, entry.Actor, string(entry.Action), string(detail), formatTime(entry.CreatedAt),
	)
	return err
}

func (s *Store) AuditQueryByTask(ctx context.Context, boardSlug string, taskNum int) ([]model.AuditEntry, error) {
	var rows []auditRow
	err := sqlx.SelectContext(ctx, s.db, &rows,
		"SELECT * FROM audit_log WHERE board_slug = ? AND task_num = ? ORDER BY timestamp ASC",
		boardSlug, taskNum)
	if err != nil {
		return nil, err
	}
	return toModelSlice[auditRow, model.AuditEntry](rows), nil
}

func (s *Store) AuditQueryByBoard(ctx context.Context, boardSlug string) ([]model.AuditEntry, error) {
	var rows []auditRow
	err := sqlx.SelectContext(ctx, s.db, &rows,
		"SELECT * FROM audit_log WHERE board_slug = ? ORDER BY timestamp ASC",
		boardSlug)
	if err != nil {
		return nil, err
	}
	return toModelSlice[auditRow, model.AuditEntry](rows), nil
}

func (s *Store) AuditUpdateTaskRef(ctx context.Context, tx repo.Tx, oldBoard string, oldNum int, newBoard string, newNum int) error {
	_, err := asTx(tx).ExecContext(ctx,
		`UPDATE audit_log SET board_slug = ?, task_num = ? WHERE board_slug = ? AND task_num = ?`,
		newBoard, newNum, oldBoard, oldNum)
	return err
}

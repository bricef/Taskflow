package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

type attachmentRow struct {
	ID        int       `db:"id"`
	BoardSlug string    `db:"board_slug"`
	TaskNum   int       `db:"task_num"`
	RefType   string    `db:"ref_type"`
	Reference string    `db:"reference"`
	Label     string    `db:"label"`
	CreatedBy string    `db:"created_by"`
	CreatedAt Timestamp `db:"created_at"`
}

func (s *Store) AttachmentInsert(ctx context.Context, tx repo.Tx, att model.Attachment) (model.Attachment, error) {
	result, err := asTx(tx).ExecContext(ctx,
		`INSERT INTO attachments (board_slug, task_num, ref_type, reference, label, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		att.BoardSlug, att.TaskNum, string(att.RefType), att.Reference, att.Label, att.CreatedBy,
	)
	if err != nil {
		return model.Attachment{}, err
	}
	id, _ := result.LastInsertId()
	return s.attachmentGet(ctx, tx, int(id))
}

func (s *Store) AttachmentGet(ctx context.Context, id int) (model.Attachment, error) {
	return s.attachmentGet(ctx, nil, id)
}

func (s *Store) attachmentGet(ctx context.Context, tx repo.Tx, id int) (model.Attachment, error) {
	var r attachmentRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM attachments WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return model.Attachment{}, notFound("attachment", fmt.Sprintf("%d", id))
	}
	if err != nil {
		return model.Attachment{}, err
	}
	return toModel[attachmentRow, model.Attachment](r), nil
}

func (s *Store) AttachmentList(ctx context.Context, boardSlug string, taskNum int) ([]model.Attachment, error) {
	var rows []attachmentRow
	err := sqlx.SelectContext(ctx, s.db, &rows,
		"SELECT * FROM attachments WHERE board_slug = ? AND task_num = ? ORDER BY created_at ASC",
		boardSlug, taskNum)
	if err != nil {
		return nil, err
	}
	return toModelSlice[attachmentRow, model.Attachment](rows), nil
}

func (s *Store) AttachmentDelete(ctx context.Context, tx repo.Tx, id int) error {
	result, err := asTx(tx).ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("attachment", fmt.Sprintf("%d", id))
	}
	return nil
}

func (s *Store) AttachmentUpdateTaskRef(ctx context.Context, tx repo.Tx, oldBoard string, oldNum int, newBoard string, newNum int) error {
	_, err := asTx(tx).ExecContext(ctx,
		`UPDATE attachments SET board_slug = ?, task_num = ? WHERE board_slug = ? AND task_num = ?`,
		newBoard, newNum, oldBoard, oldNum)
	return err
}

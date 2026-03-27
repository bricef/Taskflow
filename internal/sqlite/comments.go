package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

type commentRow struct {
	ID        int           `db:"id"`
	BoardSlug string        `db:"board_slug"`
	TaskNum   int           `db:"task_num"`
	Actor     string        `db:"actor"`
	Body      string        `db:"body"`
	CreatedAt Timestamp     `db:"created_at"`
	UpdatedAt NullTimestamp `db:"updated_at"`
}

func (s *Store) CommentInsert(ctx context.Context, tx repo.Tx, comment model.Comment) (model.Comment, error) {
	result, err := asTx(tx).ExecContext(ctx,
		`INSERT INTO comments (board_slug, task_num, actor, body) VALUES (?, ?, ?, ?)`,
		comment.BoardSlug, comment.TaskNum, comment.Actor, comment.Body,
	)
	if err != nil {
		return model.Comment{}, err
	}
	id, _ := result.LastInsertId()
	return s.commentGet(ctx, tx, int(id))
}

func (s *Store) CommentGet(ctx context.Context, id int) (model.Comment, error) {
	return s.commentGet(ctx, nil, id)
}

func (s *Store) commentGet(ctx context.Context, tx repo.Tx, id int) (model.Comment, error) {
	var r commentRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM comments WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return model.Comment{}, notFound("comment", fmt.Sprintf("%d", id))
	}
	if err != nil {
		return model.Comment{}, err
	}
	return toModel[commentRow, model.Comment](r), nil
}

func (s *Store) CommentList(ctx context.Context, boardSlug string, taskNum int) ([]model.Comment, error) {
	var rows []commentRow
	err := sqlx.SelectContext(ctx, s.db, &rows,
		"SELECT * FROM comments WHERE board_slug = ? AND task_num = ? ORDER BY created_at ASC",
		boardSlug, taskNum)
	if err != nil {
		return nil, err
	}
	return toModelSlice[commentRow, model.Comment](rows), nil
}

func (s *Store) CommentUpdateBody(ctx context.Context, tx repo.Tx, id int, body string) (model.Comment, error) {
	result, err := asTx(tx).ExecContext(ctx,
		`UPDATE comments SET body = ?, updated_at = ? WHERE id = ?`, body, nowUTC(), id)
	if err != nil {
		return model.Comment{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return model.Comment{}, notFound("comment", fmt.Sprintf("%d", id))
	}
	return s.commentGet(ctx, tx, id)
}

func (s *Store) CommentUpdateTaskRef(ctx context.Context, tx repo.Tx, oldBoard string, oldNum int, newBoard string, newNum int) error {
	_, err := asTx(tx).ExecContext(ctx,
		`UPDATE comments SET board_slug = ?, task_num = ? WHERE board_slug = ? AND task_num = ?`,
		newBoard, newNum, oldBoard, oldNum)
	return err
}

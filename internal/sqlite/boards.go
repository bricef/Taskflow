package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

type boardRow struct {
	Slug        string     `db:"slug"`
	Name        string     `db:"name"`
	Description string     `db:"description"`
	Workflow    JSONRaw    `db:"workflow"`
	NextTaskNum int        `db:"next_task_num"`
	CreatedAt   Timestamp  `db:"created_at"`
	UpdatedAt   Timestamp  `db:"updated_at"`
	Deleted     SQLiteBool `db:"deleted"`
}

func (s *Store) BoardInsert(ctx context.Context, tx repo.Tx, board model.Board) (model.Board, error) {
	_, err := asTx(tx).NamedExecContext(ctx,
		`INSERT INTO boards (slug, name, description, workflow) VALUES (:slug, :name, :description, :workflow)`,
		map[string]any{"slug": board.Slug, "name": board.Name, "description": board.Description, "workflow": string(board.Workflow)},
	)
	if err != nil {
		if isConstraintError(err) {
			return model.Board{}, conflict("board", "slug", board.Slug)
		}
		return model.Board{}, err
	}
	return s.boardGet(ctx, tx, board.Slug)
}

func (s *Store) BoardGet(ctx context.Context, slug string) (model.Board, error) {
	return s.boardGet(ctx, nil, slug)
}

func (s *Store) boardGet(ctx context.Context, tx repo.Tx, slug string) (model.Board, error) {
	var r boardRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM boards WHERE slug = ?", slug)
	if err == sql.ErrNoRows {
		return model.Board{}, notFound("board", slug)
	}
	if err != nil {
		return model.Board{}, err
	}
	return toModel[boardRow, model.Board](r), nil
}

func (s *Store) BoardList(ctx context.Context, params model.ListBoardsParams) ([]model.Board, error) {
	query := "SELECT * FROM boards"
	if !params.IncludeDeleted {
		query += " WHERE deleted = 0"
	}
	query += " ORDER BY slug"

	var rows []boardRow
	err := sqlx.SelectContext(ctx, s.db, &rows, query)
	if err != nil {
		return nil, err
	}
	return toModelSlice[boardRow, model.Board](rows), nil
}

func (s *Store) BoardUpdate(ctx context.Context, tx repo.Tx, params model.UpdateBoardParams) (model.Board, error) {
	var setClauses []string
	var args []any

	if params.Name.Set {
		setClauses = append(setClauses, "name = ?")
		args = append(args, params.Name.Value)
	}
	if params.Description.Set {
		setClauses = append(setClauses, "description = ?")
		args = append(args, params.Description.Value)
	}
	if len(setClauses) == 0 {
		return s.boardGet(ctx, tx, params.Slug)
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, nowUTC())
	args = append(args, params.Slug)

	query := fmt.Sprintf("UPDATE boards SET %s WHERE slug = ? AND deleted = 0", strings.Join(setClauses, ", "))
	result, err := asTx(tx).ExecContext(ctx, query, args...)
	if err != nil {
		return model.Board{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return model.Board{}, notFound("board", params.Slug)
	}
	return s.boardGet(ctx, tx, params.Slug)
}

func (s *Store) BoardSetDeleted(ctx context.Context, tx repo.Tx, slug string) error {
	result, err := asTx(tx).ExecContext(ctx,
		`UPDATE boards SET deleted = 1, updated_at = ? WHERE slug = ? AND deleted = 0`, nowUTC(), slug)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("board", slug)
	}
	return nil
}

func (s *Store) BoardSetWorkflow(ctx context.Context, tx repo.Tx, slug string, wf json.RawMessage) error {
	result, err := asTx(tx).ExecContext(ctx,
		`UPDATE boards SET workflow = ?, updated_at = ? WHERE slug = ? AND deleted = 0`,
		string(wf), nowUTC(), slug)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("board", slug)
	}
	return nil
}

func (s *Store) BoardAllocateTaskNum(ctx context.Context, tx repo.Tx, slug string) (int, error) {
	var num int
	err := asTx(tx).QueryRowContext(ctx,
		`SELECT next_task_num FROM boards WHERE slug = ? AND deleted = 0`, slug).Scan(&num)
	if err == sql.ErrNoRows {
		return 0, notFound("board", slug)
	}
	if err != nil {
		return 0, err
	}
	_, err = asTx(tx).ExecContext(ctx, `UPDATE boards SET next_task_num = ? WHERE slug = ?`, num+1, slug)
	return num, err
}

func (s *Store) BoardUpdateNextTaskNum(ctx context.Context, tx repo.Tx, slug string, nextNum int) error {
	_, err := asTx(tx).ExecContext(ctx, `UPDATE boards SET next_task_num = ? WHERE slug = ?`, nextNum, slug)
	return err
}

// Ensure json import is used (for boardRow's Workflow field in BoardInsert).
var _ = json.RawMessage{}

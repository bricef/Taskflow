package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

type dependencyRow struct {
	ID             int       `db:"id"`
	BoardSlug      string    `db:"task_board"`
	TaskNum        int       `db:"task_num"`
	DependsOnBoard string    `db:"depends_on_board"`
	DependsOnNum   int       `db:"depends_on_num"`
	DependencyType string    `db:"dep_type"`
	CreatedBy      string    `db:"created_by"`
	CreatedAt      Timestamp `db:"created_at"`
}

func (s *Store) DependencyInsert(ctx context.Context, tx repo.Tx, dep model.Dependency) (model.Dependency, error) {
	result, err := asTx(tx).ExecContext(ctx,
		`INSERT INTO dependencies (task_board, task_num, depends_on_board, depends_on_num, dep_type, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		dep.BoardSlug, dep.TaskNum, dep.DependsOnBoard, dep.DependsOnNum, string(dep.DependencyType), dep.CreatedBy,
	)
	if err != nil {
		if isConstraintError(err) {
			return model.Dependency{}, conflictMsg("dependency", "duplicate dependency")
		}
		if isForeignKeyError(err) {
			return model.Dependency{}, notFound("task", fmt.Sprintf("%s/%d or %s/%d", dep.BoardSlug, dep.TaskNum, dep.DependsOnBoard, dep.DependsOnNum))
		}
		return model.Dependency{}, err
	}
	id, _ := result.LastInsertId()
	return s.dependencyGet(ctx, tx, int(id))
}

func (s *Store) DependencyGet(ctx context.Context, id int) (model.Dependency, error) {
	return s.dependencyGet(ctx, nil, id)
}

func (s *Store) dependencyGet(ctx context.Context, tx repo.Tx, id int) (model.Dependency, error) {
	var r dependencyRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM dependencies WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return model.Dependency{}, notFound("dependency", fmt.Sprintf("%d", id))
	}
	if err != nil {
		return model.Dependency{}, err
	}
	return toModel[dependencyRow, model.Dependency](r), nil
}

func (s *Store) DependencyList(ctx context.Context, boardSlug string, taskNum int) ([]model.Dependency, error) {
	var rows []dependencyRow
	err := sqlx.SelectContext(ctx, s.db, &rows,
		`SELECT * FROM dependencies
		 WHERE (task_board = ? AND task_num = ?) OR (depends_on_board = ? AND depends_on_num = ?)
		 ORDER BY created_at ASC`,
		boardSlug, taskNum, boardSlug, taskNum)
	if err != nil {
		return nil, err
	}
	return toModelSlice[dependencyRow, model.Dependency](rows), nil
}

func (s *Store) DependencyDelete(ctx context.Context, tx repo.Tx, id int) error {
	result, err := asTx(tx).ExecContext(ctx, `DELETE FROM dependencies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("dependency", fmt.Sprintf("%d", id))
	}
	return nil
}

func (s *Store) DependencyUpdateTaskRefs(ctx context.Context, tx repo.Tx, oldBoard string, oldNum int, newBoard string, newNum int) error {
	t := asTx(tx)
	if _, err := t.ExecContext(ctx,
		`UPDATE dependencies SET task_board = ?, task_num = ? WHERE task_board = ? AND task_num = ?`,
		newBoard, newNum, oldBoard, oldNum); err != nil {
		return err
	}
	_, err := t.ExecContext(ctx,
		`UPDATE dependencies SET depends_on_board = ?, depends_on_num = ? WHERE depends_on_board = ? AND depends_on_num = ?`,
		newBoard, newNum, oldBoard, oldNum)
	return err
}

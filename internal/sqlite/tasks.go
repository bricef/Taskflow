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

type taskRow struct {
	BoardSlug   string        `db:"board_slug"`
	Num         int           `db:"num"`
	Title       string        `db:"title"`
	Description string        `db:"description"`
	State       string        `db:"state"`
	Priority    string        `db:"priority"`
	Tags        StringList    `db:"tags"`
	Assignee    *string       `db:"assignee"`
	DueDate     NullTimestamp `db:"due_date"`
	CreatedBy   string        `db:"created_by"`
	CreatedAt   Timestamp     `db:"created_at"`
	UpdatedAt   Timestamp     `db:"updated_at"`
	Deleted     SQLiteBool    `db:"deleted"`
}

func (s *Store) TaskInsert(ctx context.Context, tx repo.Tx, task model.Task) (model.Task, error) {
	tags := task.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, _ := json.Marshal(tags)

	var dueDate *string
	if task.DueDate != nil {
		s := formatTime(*task.DueDate)
		dueDate = &s
	}

	_, err := asTx(tx).ExecContext(ctx,
		`INSERT INTO tasks (board_slug, num, title, description, state, priority, tags, assignee, due_date, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.BoardSlug, task.Num, task.Title, task.Description, task.State,
		string(task.Priority), string(tagsJSON), task.Assignee, dueDate, task.CreatedBy,
	)
	if err != nil {
		if isForeignKeyError(err) {
			return model.Task{}, &model.ValidationError{Field: "assignee", Message: "actor not found"}
		}
		return model.Task{}, err
	}
	return s.taskGet(ctx, tx, task.BoardSlug, task.Num)
}

func (s *Store) TaskGet(ctx context.Context, boardSlug string, num int) (model.Task, error) {
	return s.taskGet(ctx, nil, boardSlug, num)
}

func (s *Store) taskGet(ctx context.Context, tx repo.Tx, boardSlug string, num int) (model.Task, error) {
	var r taskRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM tasks WHERE board_slug = ? AND num = ?", boardSlug, num)
	if err == sql.ErrNoRows {
		return model.Task{}, notFound("task", fmt.Sprintf("%s/%d", boardSlug, num))
	}
	if err != nil {
		return model.Task{}, err
	}
	return toModel[taskRow, model.Task](r), nil
}

var sortColumns = map[string]string{
	"created_at": "t.created_at",
	"updated_at": "t.updated_at",
	"due_date":   "t.due_date",
	"priority":   `CASE t.priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 WHEN 'none' THEN 5 END`,
}

func (s *Store) TaskList(ctx context.Context, filter model.TaskFilter, sort *model.TaskSort) ([]model.Task, error) {
	q := queryBuilder{}

	if filter.Query != nil && *filter.Query != "" {
		// Sanitize FTS5 query: wrap in double quotes to treat as phrase
		// and avoid syntax errors from special characters.
		safeQuery := `"` + strings.ReplaceAll(*filter.Query, `"`, `""`) + `"`
		q.from = "tasks t JOIN tasks_fts fts ON t.rowid = fts.rowid"
		q.where("fts.tasks_fts MATCH ?", safeQuery)
	} else {
		q.from = "tasks t"
	}

	if filter.BoardSlug != "" {
		q.where("t.board_slug = ?", filter.BoardSlug)
	}
	if !filter.IncludeDeleted {
		q.whereLit("t.deleted = 0")
	}
	if filter.State != nil {
		q.where("t.state = ?", *filter.State)
	}
	if filter.Assignee != nil {
		q.where("t.assignee = ?", *filter.Assignee)
	}
	if filter.Priority != nil {
		q.where("t.priority = ?", string(*filter.Priority))
	}
	if filter.Tag != nil {
		q.where("EXISTS (SELECT 1 FROM json_each(t.tags) WHERE value = ?)", *filter.Tag)
	}

	if sort != nil {
		col := sortColumns[sort.Field]
		if col == "" {
			col = "t.created_at"
		}
		q.orderBy(col, sort.Desc)
	} else {
		q.orderBy("t.created_at", false)
	}

	var rows []taskRow
	query, args := q.selectQuery("t.*")
	err := sqlx.SelectContext(ctx, s.db, &rows, query, args...)
	if err != nil {
		return nil, err
	}
	return toModelSlice[taskRow, model.Task](rows), nil
}

func (s *Store) TaskUpdate(ctx context.Context, tx repo.Tx, params model.UpdateTaskParams) (model.Task, error) {
	var setClauses []string
	var args []any

	if params.Title.Set {
		setClauses = append(setClauses, "title = ?")
		args = append(args, params.Title.Value)
	}
	if params.Description.Set {
		setClauses = append(setClauses, "description = ?")
		args = append(args, params.Description.Value)
	}
	if params.State.Set {
		setClauses = append(setClauses, "state = ?")
		args = append(args, params.State.Value)
	}
	if params.Priority.Set {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, string(params.Priority.Value))
	}
	if params.Tags.Set {
		tagsJSON, _ := json.Marshal(params.Tags.Value)
		setClauses = append(setClauses, "tags = ?")
		args = append(args, string(tagsJSON))
	}
	if params.Assignee.Set {
		setClauses = append(setClauses, "assignee = ?")
		args = append(args, params.Assignee.Value)
	}
	if params.DueDate.Set {
		var dueDate *string
		if params.DueDate.Value != nil {
			formatted := formatTime(*params.DueDate.Value)
			dueDate = &formatted
		}
		setClauses = append(setClauses, "due_date = ?")
		args = append(args, dueDate)
	}

	if len(setClauses) == 0 {
		return s.taskGet(ctx, tx, params.BoardSlug, params.Num)
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, nowUTC())
	args = append(args, params.BoardSlug, params.Num)

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE board_slug = ? AND num = ?", strings.Join(setClauses, ", "))
	result, err := asTx(tx).ExecContext(ctx, query, args...)
	if err != nil {
		return model.Task{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return model.Task{}, notFound("task", fmt.Sprintf("%s/%d", params.BoardSlug, params.Num))
	}
	return s.taskGet(ctx, tx, params.BoardSlug, params.Num)
}

func (s *Store) TaskSetDeleted(ctx context.Context, tx repo.Tx, boardSlug string, num int) error {
	result, err := asTx(tx).ExecContext(ctx,
		`UPDATE tasks SET deleted = 1, updated_at = ? WHERE board_slug = ? AND num = ? AND deleted = 0`,
		nowUTC(), boardSlug, num)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("task", fmt.Sprintf("%s/%d", boardSlug, num))
	}
	return nil
}

func (s *Store) TaskDeleteByBoardAndNums(ctx context.Context, tx repo.Tx, boardSlug string, nums []int) error {
	if len(nums) == 0 {
		return nil
	}
	placeholders := make([]string, len(nums))
	args := []any{boardSlug}
	for i, n := range nums {
		placeholders[i] = "?"
		args = append(args, n)
	}
	query := fmt.Sprintf("DELETE FROM tasks WHERE board_slug = ? AND num IN (%s)", strings.Join(placeholders, ","))
	_, err := asTx(tx).ExecContext(ctx, query, args...)
	return err
}

func (s *Store) TaskListTags(ctx context.Context, boardSlug string) ([]model.TagCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT value AS tag, COUNT(*) AS count
		 FROM tasks, json_each(tasks.tags)
		 WHERE tasks.board_slug = ? AND tasks.deleted = 0
		 GROUP BY value
		 ORDER BY count DESC, value ASC`,
		boardSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.TagCount
	for rows.Next() {
		var tc model.TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tc)
	}
	return tags, rows.Err()
}

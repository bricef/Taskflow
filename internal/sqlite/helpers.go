package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/model"
)

func nowUTC() string {
	return time.Now().UTC().Format(tsFormat)
}

func formatTime(t time.Time) string {
	return t.Format(tsFormat)
}

func isConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint")
}

func isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "FOREIGN KEY constraint failed") || strings.Contains(msg, "foreign key constraint")
}

func notFound(resource, id string) error {
	return &model.NotFoundError{Resource: resource, ID: id}
}

func conflict(resource, field, value string) error {
	return &model.ConflictError{Resource: resource, Field: field, Value: value}
}

func conflictMsg(resource, msg string) error {
	return &model.ConflictError{Resource: resource, Message: msg}
}

// insertRow inserts a row into the given table using db struct tags for column mapping.
// skipCols are columns that should be excluded from the INSERT (e.g., auto-generated ones).
// Returns the last insert ID (0 for tables without AUTOINCREMENT).
func insertRow(ctx context.Context, tx *sqlx.Tx, table string, row any, skipCols ...string) (int64, error) {
	skip := make(map[string]bool, len(skipCols))
	for _, c := range skipCols {
		skip[c] = true
	}

	var cols []string
	var placeholders []string
	v := reflect.ValueOf(row)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" || skip[tag] {
			continue
		}
		cols = append(cols, tag)
		placeholders = append(placeholders, ":"+tag)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	result, err := tx.NamedExecContext(ctx, query, row)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// queryBuilder accumulates the parts of a SELECT query.
type queryBuilder struct {
	from       string
	conditions []string
	args       []any
	order      string
}

func (q *queryBuilder) where(cond string, arg any) {
	q.conditions = append(q.conditions, cond)
	q.args = append(q.args, arg)
}

func (q *queryBuilder) whereLit(cond string) {
	q.conditions = append(q.conditions, cond)
}

func (q *queryBuilder) orderBy(col string, desc bool) {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	q.order = fmt.Sprintf("%s %s", col, dir)
}

func (q *queryBuilder) selectQuery(cols string) (string, []any) {
	query := fmt.Sprintf("SELECT %s FROM %s", cols, q.from)
	if len(q.conditions) > 0 {
		query += " WHERE " + strings.Join(q.conditions, " AND ")
	}
	if q.order != "" {
		query += " ORDER BY " + q.order
	}
	return query, q.args
}

// toModelSlice converts a slice of row structs to model structs using the generic mapper.
func toModelSlice[R any, M any](rows []R) []M {
	if rows == nil {
		return nil
	}
	models := make([]M, len(rows))
	for i, r := range rows {
		models[i] = toModel[R, M](r)
	}
	return models
}

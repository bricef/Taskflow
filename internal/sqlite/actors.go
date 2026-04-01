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

type actorRow struct {
	Name        string     `db:"name"`
	DisplayName string     `db:"display_name"`
	Type        string     `db:"type"`
	Role        string     `db:"role"`
	APIKeyHash  string     `db:"api_key_hash"`
	CreatedAt   Timestamp  `db:"created_at"`
	Active      SQLiteBool `db:"active"`
}

func (s *Store) ActorInsert(ctx context.Context, tx repo.Tx, actor model.Actor) (model.Actor, error) {
	row := fromModel[model.Actor, actorRow](actor)
	_, err := insertRow(ctx, asTx(tx), "actors", row, "created_at", "active")
	if err != nil {
		if isConstraintError(err) {
			return model.Actor{}, conflict("actor", "name", actor.Name)
		}
		return model.Actor{}, err
	}
	return s.actorGet(ctx, tx, actor.Name)
}

func (s *Store) ActorGet(ctx context.Context, name string) (model.Actor, error) {
	return s.actorGet(ctx, nil, name)
}

func (s *Store) actorGet(ctx context.Context, tx repo.Tx, name string) (model.Actor, error) {
	var r actorRow
	err := sqlx.GetContext(ctx, s.txOrDB(tx), &r, "SELECT * FROM actors WHERE name = ?", name)
	if err == sql.ErrNoRows {
		return model.Actor{}, notFound("actor", name)
	}
	if err != nil {
		return model.Actor{}, err
	}
	return toModel[actorRow, model.Actor](r), nil
}

func (s *Store) ActorGetByAPIKeyHash(ctx context.Context, hash string) (model.Actor, error) {
	var r actorRow
	err := sqlx.GetContext(ctx, s.db, &r, "SELECT * FROM actors WHERE api_key_hash = ? AND active = 1", hash)
	if err == sql.ErrNoRows {
		return model.Actor{}, notFound("actor", hash)
	}
	if err != nil {
		return model.Actor{}, err
	}
	return toModel[actorRow, model.Actor](r), nil
}

func (s *Store) ActorList(ctx context.Context) ([]model.Actor, error) {
	var rows []actorRow
	err := sqlx.SelectContext(ctx, s.db, &rows, "SELECT * FROM actors ORDER BY name")
	if err != nil {
		return nil, err
	}
	return toModelSlice[actorRow, model.Actor](rows), nil
}

func (s *Store) ActorUpdate(ctx context.Context, tx repo.Tx, params model.UpdateActorParams) (model.Actor, error) {
	var setClauses []string
	var args []any

	if params.DisplayName.Set {
		setClauses = append(setClauses, "display_name = ?")
		args = append(args, params.DisplayName.Value)
	}
	if params.Role.Set {
		setClauses = append(setClauses, "role = ?")
		args = append(args, string(params.Role.Value))
	}
	if params.Active.Set {
		setClauses = append(setClauses, "active = ?")
		args = append(args, SQLiteBool(params.Active.Value))
	}

	if len(setClauses) == 0 {
		return s.actorGet(ctx, tx, params.Name)
	}

	args = append(args, params.Name)
	query := fmt.Sprintf("UPDATE actors SET %s WHERE name = ?", strings.Join(setClauses, ", "))
	result, err := asTx(tx).ExecContext(ctx, query, args...)
	if err != nil {
		return model.Actor{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return model.Actor{}, notFound("actor", params.Name)
	}
	return s.actorGet(ctx, tx, params.Name)
}

func (s *Store) ActorUpdateKeyHash(ctx context.Context, name, newHash string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE actors SET api_key_hash = ? WHERE name = ?", newHash, name)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return notFound("actor", name)
	}
	return nil
}

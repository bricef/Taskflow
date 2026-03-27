package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/bricef/taskflow/internal/repo"
	"github.com/bricef/taskflow/migrations"

	_ "modernc.org/sqlite"
)

var _ repo.Store = (*Store)(nil)

type Store struct {
	db *sqlx.DB
}

// New opens (or creates) a SQLite database and runs migrations.
// For file-backed databases, pass a path like "./taskflow.db".
// For in-memory databases (testing), pass ":memory:".
func New(path string) (*Store, error) {
	var dsn string
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	} else {
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(wal)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	}

	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if err := migrate(db.DB, migrations.FS); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InTransaction(ctx context.Context, fn func(tx repo.Tx) error) error {
	sqlxTx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer sqlxTx.Rollback()

	if err := fn(sqlxTx); err != nil {
		return err
	}
	return sqlxTx.Commit()
}

// asTx extracts the *sqlx.Tx from a repo.Tx.
func asTx(tx repo.Tx) *sqlx.Tx {
	return tx.(*sqlx.Tx)
}

// txOrDB returns an sqlx queryer — the transaction if non-nil, otherwise the DB.
func (s *Store) txOrDB(tx repo.Tx) sqlx.ExtContext {
	if tx != nil {
		return asTx(tx)
	}
	return s.db
}

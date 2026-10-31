// Package store owns the SQLite database: opening it, migrating it, and the repositories on top.
package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/TomiCode/cnsl/internal/migrations"
)

type Store struct {
	DB *sqlx.DB
}

// Open opens (creating if needed) the SQLite database at path and applies pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sqlx.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := migrateUp(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

// dsn builds a modernc.org/sqlite DSN. Every pooled connection gets the same pragmas; write
// transactions take the lock up front (_txlock=immediate) so concurrent writers wait on
// busy_timeout instead of failing with SQLITE_BUSY halfway through.
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Set("_txlock", "immediate")
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

func migrateUp(db *sqlx.DB) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	defer src.Close()

	driver, err := sqlite.WithInstance(db.DB, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	// Not calling m.Close(): it would close the shared *sql.DB.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

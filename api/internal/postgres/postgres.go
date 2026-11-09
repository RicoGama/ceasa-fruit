// Package postgres handles the database connection and migrations.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// The //go:embed directive copies the .sql files INTO the binary at compile time,
// so the final executable does not need the migrations folder on the server.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect opens a connection pool (connections are reused across requests).
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		// %w wraps the original error so callers can inspect it with errors.Is/As.
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	// pgxpool.New does not actually connect; Ping makes sure the database is reachable.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel() // defer runs when the function returns, no matter how.

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, nil
}

// Migrate applies pending migrations (goose tracks applied ones in the
// goose_db_version table).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// goose works with database/sql; pgx's stdlib package bridges the two.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}

// Package database owns the Postgres connection pool. Phase 0 only opens
// and pings the pool — sqlc-generated queries land in Phase 1.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates a pgx connection pool for the given URL and verifies
// connectivity with a ping, mirroring the `SELECT 1` boot check in the
// source app's src/index.ts.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

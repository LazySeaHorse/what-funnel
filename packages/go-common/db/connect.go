package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig parses the DSN and applies default pool limits and compatibility settings.
func PoolConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	maxConns := int32(3)
	if envMax := os.Getenv("DB_POOL_MAX_CONNS"); envMax != "" {
		if n, err := strconv.Atoi(envMax); err == nil && n > 0 {
			maxConns = int32(n)
		}
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute

	if os.Getenv("DB_PGBOUNCER") == "true" || cfg.ConnConfig.Port == 6432 {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
		cfg.ConnConfig.StatementCacheCapacity = 0
	}

	return cfg, nil
}

// Connect opens a pgxpool connection to the given DSN and verifies connectivity.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := PoolConfig(dsn)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}

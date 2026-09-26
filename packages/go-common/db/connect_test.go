package db

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolConfig_Defaults(t *testing.T) {
	os.Unsetenv("DB_POOL_MAX_CONNS")
	os.Unsetenv("DB_PGBOUNCER")

	cfg, err := PoolConfig("postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable")
	require.NoError(t, err)

	assert.Equal(t, int32(3), cfg.MaxConns)
	assert.Equal(t, int32(1), cfg.MinConns)
	assert.Equal(t, 5*time.Minute, cfg.MaxConnIdleTime)
	assert.Equal(t, 30*time.Minute, cfg.MaxConnLifetime)
	assert.NotEqual(t, pgx.QueryExecModeExec, cfg.ConnConfig.DefaultQueryExecMode)
	assert.NotZero(t, cfg.ConnConfig.StatementCacheCapacity)
}

func TestPoolConfig_MaxConnsEnv(t *testing.T) {
	t.Setenv("DB_POOL_MAX_CONNS", "10")

	cfg, err := PoolConfig("postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable")
	require.NoError(t, err)

	assert.Equal(t, int32(10), cfg.MaxConns)
	assert.Equal(t, int32(1), cfg.MinConns)
	assert.Equal(t, 5*time.Minute, cfg.MaxConnIdleTime)
	assert.Equal(t, 30*time.Minute, cfg.MaxConnLifetime)
}

func TestPoolConfig_PgBouncerEnv(t *testing.T) {
	t.Setenv("DB_PGBOUNCER", "true")

	cfg, err := PoolConfig("postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable")
	require.NoError(t, err)

	assert.Equal(t, pgx.QueryExecModeExec, cfg.ConnConfig.DefaultQueryExecMode)
	assert.Equal(t, 0, cfg.ConnConfig.StatementCacheCapacity)
}

func TestPoolConfig_PgBouncerPort(t *testing.T) {
	os.Unsetenv("DB_PGBOUNCER")

	cfg, err := PoolConfig("postgres://whatfunnel:whatfunnel@localhost:6432/whatfunnel?sslmode=disable")
	require.NoError(t, err)

	assert.Equal(t, pgx.QueryExecModeExec, cfg.ConnConfig.DefaultQueryExecMode)
	assert.Equal(t, 0, cfg.ConnConfig.StatementCacheCapacity)
}

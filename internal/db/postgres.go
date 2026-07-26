// Package db manages the PostgreSQL connection pool used by the
// application. It knows nothing about HTTP or business logic — only how to
// connect to, close, and health-check the database.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/progspac-vnn/CachingStampede/internal/config"
)

// Postgres wraps a PostgreSQL connection pool.
type Postgres struct {
	Pool *pgxpool.Pool
	log  *zap.Logger
}

// Connect establishes a PostgreSQL connection pool using the given
// configuration and verifies connectivity with a ping before returning.
func Connect(ctx context.Context, cfg config.PostgresConfig, log *zap.Logger) (*Postgres, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("db: failed to parse postgres config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: failed to create postgres pool: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: failed to ping postgres: %w", err)
	}

	log.Info("postgres connection pool established",
		zap.String("host", cfg.Host),
		zap.String("database", cfg.Database),
		zap.Int32("max_conns", cfg.MaxConns),
	)

	return &Postgres{Pool: pool, log: log}, nil
}

// Close releases all resources held by the connection pool.
func (p *Postgres) Close() {
	p.Pool.Close()
	p.log.Info("postgres connection pool closed")
}

// Health reports whether the database is reachable.
func (p *Postgres) Health(ctx context.Context) error {
	if err := p.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("db: health check failed: %w", err)
	}
	return nil
}

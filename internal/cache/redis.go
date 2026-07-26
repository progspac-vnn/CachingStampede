// Package cache manages the Redis client used by the application and the
// distributed lock primitive built on it. It knows nothing about HTTP or
// business logic, or about product-specific cache keys — those decisions
// belong to the service layer.
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/progspac-vnn/CachingStampede/internal/config"
)

// Redis wraps a Redis client.
type Redis struct {
	Client *redis.Client
	log    *zap.Logger
}

// Connect establishes a Redis client using the given configuration and
// verifies connectivity with a PING before returning.
func Connect(ctx context.Context, cfg config.RedisConfig, log *zap.Logger) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        cfg.Addr(),
		Password:    cfg.Password,
		DB:          cfg.DB,
		DialTimeout: cfg.DialTimeout,
	})

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(connectCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cache: failed to ping redis: %w", err)
	}

	log.Info("redis client connected",
		zap.String("addr", cfg.Addr()),
		zap.Int("db", cfg.DB),
	)

	return &Redis{Client: client, log: log}, nil
}

// Close releases all resources held by the Redis client.
func (r *Redis) Close() error {
	if err := r.Client.Close(); err != nil {
		return fmt.Errorf("cache: failed to close redis client: %w", err)
	}
	r.log.Info("redis client closed")
	return nil
}

// Health reports whether Redis is reachable.
func (r *Redis) Health(ctx context.Context) error {
	if err := r.Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache: health check failed: %w", err)
	}
	return nil
}

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/progspac-vnn/CachingStampede/internal/model"
	"github.com/progspac-vnn/CachingStampede/internal/repository"
)

const lockKeyPrefix = "lock:"

// fetchCoalesced ensures that, no matter how many concurrent callers reach
// this point for the same key, exactly one of them actually queries
// PostgreSQL. golang.org/x/sync/singleflight coalesces callers within this
// process; a Redis distributed lock coordinates across processes (e.g.
// multiple replicas of this API sharing the same cache and database).
func (s *ProductService) fetchCoalesced(ctx context.Context, key string, id int64, requestID string) (*model.Product, error) {
	v, err, shared := s.flight.Do(key, func() (interface{}, error) {
		return s.fetchWithLock(ctx, key, id, requestID)
	})
	if err != nil {
		return nil, err
	}
	if shared {
		s.log.Info("request coalesced by singleflight",
			zap.String("request_id", requestID),
			zap.Int64("product_id", id),
		)
	}
	return v.(*model.Product), nil
}

// fetchWithLock acquires a distributed lock before querying PostgreSQL, so
// that other API processes racing on the same expired key wait for (or
// fail open past) this fetch instead of each querying the database
// themselves.
func (s *ProductService) fetchWithLock(ctx context.Context, key string, id int64, requestID string) (*model.Product, error) {
	lockKey := lockKeyPrefix + key

	token, acquired, err := s.locker.AcquireLock(ctx, lockKey, s.lockTTL)
	if err != nil {
		s.log.Warn("distributed lock unavailable, proceeding without cross-process coordination",
			zap.String("request_id", requestID),
			zap.Int64("product_id", id),
			zap.Error(err),
		)
		return s.fetchFromDB(ctx, key, id)
	}

	if !acquired {
		s.metrics.LockContended()
		s.log.Info("refill lock held elsewhere, waiting for it to populate the cache",
			zap.String("request_id", requestID),
			zap.Int64("product_id", id),
		)
		if product := s.waitForCache(ctx, key); product != nil {
			return product, nil
		}
		// Gave up waiting — fail open rather than block indefinitely.
		return s.fetchFromDB(ctx, key, id)
	}

	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.locker.ReleaseLock(releaseCtx, lockKey, token); err != nil {
			s.log.Warn("failed to release refill lock", zap.Int64("product_id", id), zap.Error(err))
		}
	}()

	return s.fetchFromDB(ctx, key, id)
}

// waitForCache polls Redis briefly for another process's fetch to
// complete, so a lock loser can return the freshly cached value instead of
// hitting the database itself. It gives up after lockWait has elapsed.
func (s *ProductService) waitForCache(ctx context.Context, key string) *model.Product {
	const pollInterval = 20 * time.Millisecond
	deadline := time.Now().Add(s.lockWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollInterval):
		}

		if product := s.peekCache(ctx, key); product != nil {
			return product
		}
	}
	return nil
}

// fetchFromDB queries the repository, populates the cache on success, and
// records the fetch — the one metric that proves coalescing worked: it
// should stay flat even as concurrent request volume spikes.
func (s *ProductService) fetchFromDB(ctx context.Context, key string, id int64) (*model.Product, error) {
	s.metrics.DBFetch()

	product, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrProductNotFound) {
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("service: failed to get product %d: %w", id, err)
	}

	s.populateCache(ctx, key, id, product)
	return product, nil
}

// Package service contains the application's business logic. Services
// orchestrate repositories and infrastructure clients — HTTP concerns
// belong in handlers, storage access belongs in repositories.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/progspac-vnn/CachingStampede/internal/config"
	"github.com/progspac-vnn/CachingStampede/internal/middleware"
	"github.com/progspac-vnn/CachingStampede/internal/model"
)

// ErrProductNotFound is returned when a requested product does not exist.
var ErrProductNotFound = errors.New("service: product not found")

// productRepository is the subset of ProductRepository this service
// depends on, defined here so it can be mocked in tests.
type productRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Product, error)
}

// cacheClient is the subset of *redis.Client this service depends on,
// defined here so it can be mocked in tests.
type cacheClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// cacheLocker is the subset of *cache.Redis's distributed lock this
// service depends on, defined here so it can be mocked in tests.
type cacheLocker interface {
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (token string, acquired bool, err error)
	ReleaseLock(ctx context.Context, key, token string) error
}

// cacheMetricsRecorder is the subset of *metrics.Metrics this service
// depends on, defined here so it can be mocked in tests.
type cacheMetricsRecorder interface {
	CacheHit()
	CacheMiss()
	CacheStaleServed()
	LockContended()
	DBFetch()
}

// cacheEntry is the wrapper stored in Redis: the product plus when it was
// cached, so freshness can be judged against the soft TTL independently of
// Redis's own (jittered, longer) hard expiry.
type cacheEntry struct {
	Product  model.Product `json:"product"`
	CachedAt time.Time     `json:"cached_at"`
}

// ProductService implements a cache-aside read path for products, hardened
// against cache stampedes:
//
//   - A fresh Redis hit returns immediately.
//   - A stale-but-present hit (past the soft TTL, within the stale window)
//     is also returned immediately, with a background refresh coalesced
//     through the same mechanism as a hard miss (stale-while-revalidate).
//   - A hard miss coalesces concurrent in-process callers via singleflight,
//     and coordinates across processes via a Redis distributed lock, so a
//     burst of concurrent requests against one expired key results in one
//     database query, not one per request.
//   - Redis's own TTL is the soft TTL plus the stale window plus random
//     jitter, so cache entries don't all expire from Redis in lockstep.
type ProductService struct {
	repo    productRepository
	cache   cacheClient
	locker  cacheLocker
	flight  *singleflight.Group
	metrics cacheMetricsRecorder
	log     *zap.Logger

	softTTL  time.Duration
	staleTTL time.Duration
	jitter   float64
	lockTTL  time.Duration
	lockWait time.Duration
}

// NewProductService constructs a ProductService.
func NewProductService(
	repo productRepository,
	cache cacheClient,
	locker cacheLocker,
	cacheCfg config.CacheConfig,
	metrics cacheMetricsRecorder,
	log *zap.Logger,
) *ProductService {
	return &ProductService{
		repo:     repo,
		cache:    cache,
		locker:   locker,
		flight:   &singleflight.Group{},
		metrics:  metrics,
		log:      log,
		softTTL:  cacheCfg.TTL,
		staleTTL: cacheCfg.StaleTTL,
		jitter:   cacheCfg.Jitter,
		lockTTL:  cacheCfg.LockTTL,
		lockWait: cacheCfg.LockWait,
	}
}

// GetProduct returns the product with the given ID, preferring the cache.
// It returns ErrProductNotFound if no such product exists.
func (s *ProductService) GetProduct(ctx context.Context, id int64) (*model.Product, error) {
	key := productCacheKey(id)
	requestID := middleware.RequestIDFromContext(ctx)

	product, stale := s.lookupCache(ctx, key, id, requestID)
	if product != nil {
		if stale {
			s.metrics.CacheStaleServed()
			s.triggerBackgroundRefresh(key, id)
		}
		return product, nil
	}

	return s.fetchCoalesced(ctx, key, id, requestID)
}

// lookupCache attempts to serve the product from Redis. It returns
// (product, stale): product is nil on a hard miss or an unusable cache
// entry; stale is true when the returned product is past its soft TTL,
// signalling the caller should trigger a background refresh.
func (s *ProductService) lookupCache(ctx context.Context, key string, id int64, requestID string) (*model.Product, bool) {
	val, err := s.cache.Get(ctx, key).Result()
	switch {
	case err == nil:
		var entry cacheEntry
		if unmarshalErr := json.Unmarshal([]byte(val), &entry); unmarshalErr != nil {
			s.log.Warn("failed to unmarshal cached product, treating as miss",
				zap.String("request_id", requestID),
				zap.Int64("product_id", id),
				zap.Error(unmarshalErr),
			)
			return nil, false
		}

		age := time.Since(entry.CachedAt)
		stale := age > s.softTTL
		if stale {
			s.log.Info("cache stale hit, serving while revalidating",
				zap.String("request_id", requestID),
				zap.Int64("product_id", id),
				zap.Duration("age", age),
			)
		} else {
			s.log.Info("cache hit",
				zap.String("request_id", requestID),
				zap.Int64("product_id", id),
			)
		}
		s.metrics.CacheHit()
		product := entry.Product
		return &product, stale

	case errors.Is(err, redis.Nil):
		s.log.Info("cache miss",
			zap.String("request_id", requestID),
			zap.Int64("product_id", id),
		)
		s.metrics.CacheMiss()
		return nil, false

	default:
		s.log.Warn("cache lookup failed, falling back to database",
			zap.String("request_id", requestID),
			zap.Int64("product_id", id),
			zap.Error(err),
		)
		return nil, false
	}
}

// peekCache reads a cache entry without any logging or metrics side
// effects, for internal polling use (waiting on someone else's refill)
// where every attempt is not itself a meaningful hit or miss event.
func (s *ProductService) peekCache(ctx context.Context, key string) *model.Product {
	val, err := s.cache.Get(ctx, key).Result()
	if err != nil {
		return nil
	}
	var entry cacheEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil
	}
	product := entry.Product
	return &product
}

// populateCache best-effort writes product to Redis with a jittered hard
// TTL. Cache write failures are logged but never fail the request — the
// database result is still returned to the caller.
func (s *ProductService) populateCache(ctx context.Context, key string, id int64, product *model.Product) {
	entry := cacheEntry{Product: *product, CachedAt: time.Now()}
	payload, err := json.Marshal(entry)
	if err != nil {
		s.log.Warn("failed to marshal product for cache", zap.Int64("product_id", id), zap.Error(err))
		return
	}

	if err := s.cache.Set(ctx, key, payload, s.jitteredHardTTL()).Err(); err != nil {
		s.log.Warn("failed to populate cache", zap.Int64("product_id", id), zap.Error(err))
	}
}

// jitteredHardTTL returns the Redis expiry for a cache entry: the soft TTL
// plus the stale-serving window, with up to ±jitter fraction of random
// variance so entries cached around the same time don't all expire from
// Redis at the exact same instant.
func (s *ProductService) jitteredHardTTL() time.Duration {
	base := s.softTTL + s.staleTTL
	if s.jitter <= 0 {
		return base
	}
	variance := float64(base) * s.jitter
	delta := (rand.Float64()*2 - 1) * variance
	return base + time.Duration(delta)
}

// triggerBackgroundRefresh asynchronously refreshes a stale cache entry.
// Concurrent stale hits on the same key are coalesced by the same
// singleflight group used for hard misses, so only one refresh actually
// executes no matter how many stale reads triggered it.
func (s *ProductService) triggerBackgroundRefresh(key string, id int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := s.fetchCoalesced(ctx, key, id, "background-refresh"); err != nil {
			s.log.Warn("background cache refresh failed", zap.Int64("product_id", id), zap.Error(err))
		}
	}()
}

func productCacheKey(id int64) string {
	return fmt.Sprintf("product:%d", id)
}

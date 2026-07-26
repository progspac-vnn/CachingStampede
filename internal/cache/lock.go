package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// releaseScript atomically deletes a lock key only if its value still
// matches the token that acquired it, so a caller can never release a lock
// it no longer holds (e.g. one that already expired and was re-acquired by
// someone else).
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`)

// AcquireLock attempts to acquire a distributed lock identified by key,
// valid for ttl. It returns a token identifying this acquisition (required
// to release it) and whether the lock was actually acquired — false simply
// means another holder currently has it, which is an expected outcome, not
// an error.
func (r *Redis) AcquireLock(ctx context.Context, key string, ttl time.Duration) (token string, acquired bool, err error) {
	token, err = generateToken()
	if err != nil {
		return "", false, err
	}

	ok, err := r.Client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", false, err
	}

	return token, ok, nil
}

// ReleaseLock releases a lock previously acquired with AcquireLock, but
// only if token still matches — it is a no-op if the lock already expired
// and was acquired by someone else.
func (r *Redis) ReleaseLock(ctx context.Context, key, token string) error {
	return releaseScript.Run(ctx, r.Client, []string{key}, token).Err()
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("cache: failed to generate lock token")
	}
	return hex.EncodeToString(b), nil
}

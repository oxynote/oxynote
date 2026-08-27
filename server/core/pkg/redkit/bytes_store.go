package redkit

import (
	"context"
	"errors"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// BytesStore is a helper struct for managing opaque byte payloads in
// redis. Unlike ValueStore it does not encode what it stores, so
// already-serialised blobs are not inflated by a second encoding.
type BytesStore struct {
	pool        *redis.Pool
	expireAfter time.Duration
}

// NewBytesStore creates a new byte store instance. An expireAfter below
// one second stores values without an expiry.
func NewBytesStore(
	pool *redis.Pool,
	expireAfter time.Duration,
) *BytesStore {
	return &BytesStore{
		pool:        pool,
		expireAfter: expireAfter,
	}
}

// Start blocks until ctx is cancelled. Redis reclaims what it holds
// on its own, so the store has no maintenance of its own to run.
func (bs *BytesStore) Start(ctx context.Context) {
	<-ctx.Done()
}

// Set stores the payload under the given key.
func (bs *BytesStore) Set(ctx context.Context, key string, value []byte) error {
	conn, err := bs.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck // error provides no meaningful info

	args := []any{key, value}

	// redis rejects "EX 0", which is what any expiry below one second
	// truncates to; treat those as no expiry at all.
	if secs := int(bs.expireAfter.Seconds()); secs > 0 {
		args = append(args, "EX", secs)
	}

	_, err = conn.Do("SET", args...)
	if err != nil {
		return err
	}

	return nil
}

// Get retrieves the payload stored under the given key, returning
// errutil.ErrNotFound when the key is absent or expired.
func (bs *BytesStore) Get(ctx context.Context, key string) ([]byte, error) {
	conn, err := bs.pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close() //nolint:errcheck // error provides no meaningful info

	value, err := redis.Bytes(conn.Do("GET", key))

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, redis.ErrNil):
		return nil, errutil.ErrNotFound
	default:
		return nil, err
	}

	return value, nil
}

// Delete removes the payload stored under the given key.
func (bs *BytesStore) Delete(ctx context.Context, key string) error {
	conn, err := bs.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck // error provides no meaningful info

	_, err = conn.Do("DEL", key)
	if err != nil {
		return err
	}

	return nil
}

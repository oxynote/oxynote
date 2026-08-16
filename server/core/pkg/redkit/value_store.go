// Package redkit provides small Redis-backed storage helpers.
package redkit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// ValueStore is a helper struct for managing simple value storage in redis.
type ValueStore[V any] struct {
	pool        *redis.Pool
	expireAfter time.Duration
}

// NewValueStore creates a new value store instance. An expireAfter below one
// second stores values without an expiry.
func NewValueStore[V any](
	pool *redis.Pool,
	expireAfter time.Duration,
) *ValueStore[V] {
	return &ValueStore[V]{
		pool:        pool,
		expireAfter: expireAfter,
	}
}

// Set sets the value for the given key.
func (vs *ValueStore[V]) Set(ctx context.Context, key string, value V) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	conn, err := vs.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck // error provides no meaningful info

	args := []any{key, data}

	// redis rejects "EX 0", which is what any expiry below one second
	// truncates to; treat those as no expiry at all.
	if secs := int(vs.expireAfter.Seconds()); secs > 0 {
		args = append(args, "EX", secs)
	}

	_, err = conn.Do("SET", args...)
	if err != nil {
		return err
	}

	return nil
}

// Get gets the value for the given key.
func (vs *ValueStore[V]) Get(ctx context.Context, key string) (*V, error) {
	conn, err := vs.pool.GetContext(ctx)
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

	var v V

	err = json.Unmarshal(value, &v)
	if err != nil {
		return nil, err
	}

	return &v, nil
}

// Delete deletes the value for the given key.
func (vs *ValueStore[V]) Delete(ctx context.Context, key string) error {
	conn, err := vs.pool.GetContext(ctx)
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

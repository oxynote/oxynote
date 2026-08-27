package memkit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// ValueStore is a helper struct for managing simple value storage in
// process memory.
type ValueStore[V any] struct {
	cache *ttlcache.Cache[string, []byte]
}

// NewValueStore creates a new value store instance. An expireAfter below
// one second stores values without an expiry.
func NewValueStore[V any](expireAfter time.Duration) *ValueStore[V] {
	return &ValueStore[V]{cache: newCache(expireAfter)}
}

// Start runs the store's sweep of expired entries until ctx is
// cancelled. It blocks, so the caller owns the goroutine it runs on.
func (vs *ValueStore[V]) Start(ctx context.Context) {
	sweep(ctx, vs.cache)
}

// Set sets the value for the given key. Values are encoded rather than
// held by reference, so a stored value cannot be mutated through the
// caller's copy the way it could not be over a connection.
func (vs *ValueStore[V]) Set(_ context.Context, key string, value V) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	vs.cache.Set(key, data, ttlcache.DefaultTTL)

	return nil
}

// Get gets the value for the given key.
func (vs *ValueStore[V]) Get(_ context.Context, key string) (*V, error) {
	item := vs.cache.Get(key)
	if item == nil {
		return nil, errutil.ErrNotFound
	}

	var v V

	err := json.Unmarshal(item.Value(), &v)
	if err != nil {
		return nil, err
	}

	return &v, nil
}

// Delete deletes the value for the given key.
func (vs *ValueStore[V]) Delete(_ context.Context, key string) error {
	vs.cache.Delete(key)

	return nil
}

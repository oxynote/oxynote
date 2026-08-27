package memkit

import (
	"context"
	"slices"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// BytesStore is a helper struct for managing opaque byte payloads in
// process memory. Unlike ValueStore it does not encode what it stores,
// so already-serialised blobs are not inflated by a second encoding.
type BytesStore struct {
	cache *ttlcache.Cache[string, []byte]
}

// NewBytesStore creates a new byte store instance. An expireAfter below
// one second stores values without an expiry.
func NewBytesStore(expireAfter time.Duration) *BytesStore {
	return &BytesStore{cache: newCache(expireAfter)}
}

// Start runs the store's sweep of expired entries until ctx is
// cancelled. It blocks, so the caller owns the goroutine it runs on.
func (bs *BytesStore) Start(ctx context.Context) {
	sweep(ctx, bs.cache)
}

// Set stores the payload under the given key. The payload is copied, so
// a stored blob cannot be mutated through the caller's slice the way it
// could not be over a connection.
func (bs *BytesStore) Set(_ context.Context, key string, value []byte) error {
	bs.cache.Set(key, slices.Clone(value), ttlcache.DefaultTTL)

	return nil
}

// Get retrieves the payload stored under the given key, returning
// errutil.ErrNotFound when the key is absent or expired.
func (bs *BytesStore) Get(_ context.Context, key string) ([]byte, error) {
	item := bs.cache.Get(key)
	if item == nil {
		return nil, errutil.ErrNotFound
	}

	return slices.Clone(item.Value()), nil
}

// Delete removes the payload stored under the given key.
func (bs *BytesStore) Delete(_ context.Context, key string) error {
	bs.cache.Delete(key)

	return nil
}

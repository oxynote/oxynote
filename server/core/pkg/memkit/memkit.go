// Package memkit provides in-memory counterparts to the redkit storage
// helpers, for deployments running without a Valkey (Redis) instance.
// The contracts match redkit's exactly, so a consumer holding one of the
// storage interfaces cannot tell which backing it was given.
//
// What redis does for the stores it backs, a memkit store has to do for
// itself: Start runs the sweep reclaiming entries once they expire.
// Reads never return an expired entry either way, so a store nobody
// started still answers correctly — it just keeps what it should have
// dropped.
package memkit

import (
	"context"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// _sweepInterval is how often a store reclaims the entries that have
// expired since its last pass.
var _sweepInterval = time.Minute * 5

// newCache creates the cache backing a store. An expireAfter below one
// second holds entries without an expiry, matching the redis stores,
// where the equivalent "EX 0" is rejected.
func newCache(expireAfter time.Duration) *ttlcache.Cache[string, []byte] {
	ttl := ttlcache.NoTTL

	if expireAfter >= time.Second {
		ttl = expireAfter
	}

	// reads must not extend an entry's life: redis GET does not touch a
	// key's expiry either.
	return ttlcache.New(
		ttlcache.WithTTL[string, []byte](ttl),
		ttlcache.WithDisableTouchOnHit[string, []byte](),
	)
}

// sweep reclaims a cache's expired entries until ctx is cancelled. It
// blocks, so the caller owns the goroutine it runs on.
func sweep(ctx context.Context, cache *ttlcache.Cache[string, []byte]) {
	timeutil.NewPeriodicExec(
		_sweepInterval,
		0,
		func(context.Context) {
			cache.DeleteExpired()
		},
		nil,
		false,
	).Start(ctx)
}

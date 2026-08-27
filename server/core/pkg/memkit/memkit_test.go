package memkit

import (
	"context"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_newCache(t *testing.T) {
	cc := map[string]struct {
		ExpireAfter time.Duration
		TTL         time.Duration
	}{
		"Zero expiry holds entries without an expiry": {
			TTL: ttlcache.NoTTL,
		},
		"Sub-second expiry holds entries without an expiry": {
			ExpireAfter: time.Millisecond,
			TTL:         ttlcache.NoTTL,
		},
		"Expiry of a second or more is applied": {
			ExpireAfter: time.Hour * 168,
			TTL:         time.Hour * 168,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cache := newCache(c.ExpireAfter)
			require.NotNil(t, cache)

			item := cache.Set("test", []byte("123"), ttlcache.DefaultTTL)
			require.NotNil(t, item)
			assert.Equal(t, c.TTL, item.TTL())

			// a read must leave the entry's expiry where it was.
			assert.Equal(t, item.ExpiresAt(), cache.Get("test").ExpiresAt())
		})
	}
}

func Test_sweep(t *testing.T) {
	defer func(interval time.Duration) {
		_sweepInterval = interval
	}(_sweepInterval)

	_sweepInterval = time.Millisecond

	cache := ttlcache.New(
		ttlcache.WithTTL[string, []byte](time.Millisecond * 10),
	)
	cache.Set("test", []byte("123"), ttlcache.DefaultTTL)
	require.Equal(t, 1, cache.Len())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		sweep(ctx, cache)
	}()

	assert.Eventually(t, func() bool {
		return cache.Len() == 0
	}, time.Second, time.Millisecond)

	cancel()

	// the sweep owns no goroutine of its own, so it has to return once
	// the context it was given is cancelled.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sweep did not stop")
	}
}

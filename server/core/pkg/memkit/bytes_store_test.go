package memkit

import (
	"context"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewBytesStore(t *testing.T) {
	bs := NewBytesStore(time.Hour * 168)
	require.NotNil(t, bs)
	require.NotNil(t, bs.cache)

	item := bs.cache.Set("test", []byte("123"), ttlcache.DefaultTTL)
	require.NotNil(t, item)
	assert.Equal(t, time.Hour*168, item.TTL())
}

func Test_BytesStore_Start(t *testing.T) {
	t.Parallel()

	bs := NewBytesStore(time.Hour * 168)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		bs.Start(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("start did not stop")
	}
}

func Test_BytesStore_Set(t *testing.T) {
	cc := map[string]struct {
		Existing []byte
		Value    []byte
		Result   []byte
	}{
		"Existing value is overwritten": {
			Existing: []byte("000"),
			Value:    []byte("123"),
			Result:   []byte("123"),
		},
		"Successfully set element": {
			Value:  []byte("123"),
			Result: []byte("123"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			bs := NewBytesStore(time.Hour * 168)

			if c.Existing != nil {
				bs.cache.Set("test", c.Existing, ttlcache.DefaultTTL)
			}

			err := bs.Set(context.Background(), "test", c.Value)
			assert.NoError(t, err)

			item := bs.cache.Get("test")
			require.NotNil(t, item)
			assert.Equal(t, c.Result, item.Value())

			// the payload is copied in, so the caller's slice is no
			// longer a handle on what the store holds.
			c.Value[0] = 'x'

			assert.Equal(t, c.Result, bs.cache.Get("test").Value())
		})
	}
}

func Test_BytesStore_Get(t *testing.T) {
	cc := map[string]struct {
		Stored  []byte
		Expired bool
		Result  []byte
		Err     error
	}{
		"Key is not found": {
			Err: errutil.ErrNotFound,
		},
		"Key has expired": {
			Stored:  []byte("123"),
			Expired: true,
			Err:     errutil.ErrNotFound,
		},
		"Successfully retrieved value": {
			Stored: []byte("123"),
			Result: []byte("123"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			expireAfter := time.Hour * 168
			if c.Expired {
				expireAfter = time.Millisecond
			}

			// the constructor's sub-second rule would drop the expiry
			// altogether, so an expiring case builds its own cache.
			bs := &BytesStore{
				cache: ttlcache.New(
					ttlcache.WithTTL[string, []byte](expireAfter),
				),
			}

			if c.Stored != nil {
				bs.cache.Set("test", c.Stored, ttlcache.DefaultTTL)
			}

			if c.Expired {
				time.Sleep(time.Millisecond * 20)
			}

			res, err := bs.Get(context.Background(), "test")

			if c.Err != nil {
				assert.Nil(t, res)
				assert.Equal(t, c.Err, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, c.Result, res)

			// the payload is copied out, so mutating it cannot reach
			// what the store holds.
			res[0] = 'x'

			assert.Equal(t, c.Result, bs.cache.Get("test").Value())
		})
	}
}

func Test_BytesStore_Delete(t *testing.T) {
	cc := map[string]struct {
		Stored []byte
	}{
		"Key is not found": {},
		"Successfully deleted element": {
			Stored: []byte("123"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			bs := NewBytesStore(time.Hour * 168)

			if c.Stored != nil {
				bs.cache.Set("test", c.Stored, ttlcache.DefaultTTL)
			}

			err := bs.Delete(context.Background(), "test")
			assert.NoError(t, err)
			assert.Zero(t, bs.cache.Len())
		})
	}
}

package memkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewValueStore(t *testing.T) {
	vs := NewValueStore[string](time.Hour * 168)
	require.NotNil(t, vs)
	require.NotNil(t, vs.cache)

	item := vs.cache.Set("test", []byte("123"), ttlcache.DefaultTTL)
	require.NotNil(t, item)
	assert.Equal(t, time.Hour*168, item.TTL())
}

func Test_ValueStore_Start(t *testing.T) {
	t.Parallel()

	vs := NewValueStore[string](time.Hour * 168)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		vs.Start(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("start did not stop")
	}
}

func Test_ValueStore_Set(t *testing.T) {
	cc := map[string]struct {
		Existing []byte
		Value    any
		Result   []byte
		Err      error
	}{
		"Invalid value": {
			Value: struct {
				Fn func()
			}{},
			Err: assert.AnError,
		},
		"Existing value is overwritten": {
			Existing: []byte(`"000"`),
			Value:    "123",
			Result:   []byte(`"123"`),
		},
		"Successfully set element": {
			Value:  "123",
			Result: []byte(`"123"`),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			vs := NewValueStore[any](time.Hour * 168)

			if c.Existing != nil {
				vs.cache.Set("test", c.Existing, ttlcache.DefaultTTL)
			}

			err := vs.Set(context.Background(), "test", c.Value)

			if c.Err != nil {
				assert.Error(t, err)
				assert.Zero(t, vs.cache.Len())

				return
			}

			assert.NoError(t, err)

			item := vs.cache.Get("test")
			require.NotNil(t, item)
			assert.Equal(t, c.Result, item.Value())
		})
	}
}

func Test_ValueStore_Get(t *testing.T) {
	type testStruct struct {
		Value string `json:"value"`
	}

	cc := map[string]struct {
		Stored  []byte
		Expired bool
		Result  testStruct
		Err     error
	}{
		"Key is not found": {
			Err: errutil.ErrNotFound,
		},
		"Key has expired": {
			Stored:  []byte(`{"value":"123"}`),
			Expired: true,
			Err:     errutil.ErrNotFound,
		},
		"Invalid value": {
			Stored: []byte(`{"value":"123"`),
			Err:    assert.AnError,
		},
		"Successfully retrieved value": {
			Stored: []byte(`{"value":"123"}`),
			Result: testStruct{
				Value: "123",
			},
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
			vs := &ValueStore[testStruct]{
				cache: ttlcache.New(
					ttlcache.WithTTL[string, []byte](expireAfter),
				),
			}

			if c.Stored != nil {
				vs.cache.Set("test", c.Stored, ttlcache.DefaultTTL)
			}

			if c.Expired {
				time.Sleep(time.Millisecond * 20)
			}

			res, err := vs.Get(context.Background(), "test")

			if c.Err != nil {
				assert.Nil(t, res)

				if errors.Is(c.Err, assert.AnError) {
					assert.Error(t, err)
					return
				}

				assert.Equal(t, c.Err, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, c.Result, *res)
		})
	}
}

func Test_ValueStore_Delete(t *testing.T) {
	cc := map[string]struct {
		Stored []byte
	}{
		"Key is not found": {},
		"Successfully deleted element": {
			Stored: []byte(`"123"`),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			vs := NewValueStore[string](time.Hour * 168)

			if c.Stored != nil {
				vs.cache.Set("test", c.Stored, ttlcache.DefaultTTL)
			}

			err := vs.Delete(context.Background(), "test")
			assert.NoError(t, err)
			assert.Zero(t, vs.cache.Len())
		})
	}
}

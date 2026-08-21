package persist

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBlobStore builds a byte store backed by a map, so a paused turn
// genuinely round-trips through the same serialisation a Redis-backed
// run would.
func stubBlobStore() *BlobStoreMock {
	var (
		mu   sync.Mutex
		data = map[string][]byte{}
	)

	return &BlobStoreMock{
		GetFunc: func(_ context.Context, key string) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()

			value, ok := data[key]
			if !ok {
				return nil, errutil.ErrNotFound
			}

			return value, nil
		},
		SetFunc: func(_ context.Context, key string, value []byte) error {
			mu.Lock()
			defer mu.Unlock()

			data[key] = value

			return nil
		},
		DeleteFunc: func(_ context.Context, key string) error {
			mu.Lock()
			defer mu.Unlock()

			delete(data, key)

			return nil
		},
	}
}

func Test_NewCheckpoints(t *testing.T) {
	t.Parallel()

	store := stubBlobStore()
	log := discardLog()

	c := NewCheckpoints(log, store)
	require.NotNil(t, c)

	assert.Same(t, log, c.log)
	assert.Same(t, store, c.store)
}

func Test_Checkpoints_Get(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *BlobStoreMock
		Result []byte
		Found  bool
		Err    error
	}{
		"Stored checkpoint is returned": {
			Store: &BlobStoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return []byte("paused"), nil
				},
			},
			Result: []byte("paused"),
			Found:  true,
		},
		"Absent checkpoint is a fresh run, not a failure": {
			Store: &BlobStoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return nil, errutil.ErrNotFound
				},
			},
		},
		"Error returned by store.Get": {
			Store: &BlobStoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, found, err := NewCheckpoints(discardLog(), c.Store).Get(context.Background(), "k")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.Found, found)

			ff := c.Store.GetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:checkpoint:k", ff[0].Key)
		})
	}
}

func Test_Checkpoints_Set(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store *BlobStoreMock
		Err   error
	}{
		"Checkpoint is stored": {Store: stubBlobStore()},
		"Error returned by store.Set": {
			Store: &BlobStoreMock{
				SetFunc: func(_ context.Context, _ string, _ []byte) error {
					return assert.AnError
				},
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := NewCheckpoints(discardLog(), c.Store).Set(context.Background(), "k", []byte("paused"))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			ff := c.Store.SetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:checkpoint:k", ff[0].Key)
			assert.Equal(t, []byte("paused"), ff[0].Value)
		})
	}
}

func Test_Checkpoints_Delete(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store *BlobStoreMock
		Err   error
	}{
		"Checkpoint is dropped": {Store: stubBlobStore()},
		"Error returned by store.Delete": {
			Store: &BlobStoreMock{
				DeleteFunc: func(_ context.Context, _ string) error {
					return assert.AnError
				},
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := NewCheckpoints(discardLog(), c.Store).Delete(context.Background(), "k")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			ff := c.Store.DeleteCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:checkpoint:k", ff[0].Key)
		})
	}
}

func Test_Checkpoints_Clear(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *BlobStoreMock
		Logged string
	}{
		"Checkpoint is dropped": {Store: stubBlobStore()},
		"Error returned by store.Delete": {
			// a failed clear is logged, never fatal: the turn it belonged
			// to is over either way.
			Store: &BlobStoreMock{
				DeleteFunc: func(_ context.Context, _ string) error {
					return assert.AnError
				},
			},
			Logged: "failed to clear checkpoint",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			NewCheckpoints(slog.New(slog.NewTextHandler(&buf, nil)), c.Store).
				Clear(context.Background(), SessionKey("org", "user"))

			ff := c.Store.DeleteCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:checkpoint:assistant:session:org:user", ff[0].Key)

			if c.Logged == "" {
				assert.Empty(t, buf.String())

				return
			}

			assert.Contains(t, buf.String(), c.Logged)
		})
	}
}

func Test_checkpointKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:checkpoint:k", checkpointKey("k"))
}

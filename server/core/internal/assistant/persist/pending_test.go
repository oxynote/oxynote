package persist

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPendingStore builds a pending-confirmation store backed by
// memory, so a question asked in one session can be read back by the
// next one.
func stubPendingStore() *PendingStoreMock {
	var (
		mu   sync.Mutex
		data = map[string]PendingConfirm{}
	)

	return &PendingStoreMock{
		GetFunc: func(_ context.Context, key string) (*PendingConfirm, error) {
			mu.Lock()
			defer mu.Unlock()

			value, ok := data[key]
			if !ok {
				return nil, errutil.ErrNotFound
			}

			return &value, nil
		},
		SetFunc: func(_ context.Context, key string, value PendingConfirm) error {
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

// stubPending is the confirmation these tests store and read back.
func stubPending() PendingConfirm {
	return PendingConfirm{
		TurnID:       "t1",
		InterruptIDs: []string{"a"},
		Actions: []protocol.ConfirmAction{
			{Tool: "insert_block", DocumentID: "d", Summary: "Insert a callout"},
		},
	}
}

func Test_NewPendings(t *testing.T) {
	t.Parallel()

	store := stubPendingStore()
	log := discardLog()

	p := NewPendings(log, store)
	require.NotNil(t, p)

	assert.Same(t, log, p.log)
	assert.Same(t, store, p.store)
}

func Test_Pendings_Load(t *testing.T) {
	t.Parallel()

	pending := stubPending()

	cc := map[string]struct {
		Store  *PendingStoreMock
		Result *PendingConfirm
	}{
		"Outstanding confirmation is restored": {
			Store: &PendingStoreMock{
				GetFunc: func(_ context.Context, _ string) (*PendingConfirm, error) {
					return &pending, nil
				},
			},
			Result: &pending,
		},
		"Nothing outstanding": {
			Store: &PendingStoreMock{
				GetFunc: func(_ context.Context, _ string) (*PendingConfirm, error) {
					return nil, errutil.ErrNotFound
				},
			},
		},
		"Error returned by store.Get": {
			// an unreadable record leaves the turn parked rather than
			// resuming writes nobody approved.
			Store: &PendingStoreMock{
				GetFunc: func(_ context.Context, _ string) (*PendingConfirm, error) {
					return nil, assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := NewPendings(discardLog(), c.Store).Load(context.Background(), SessionKey("org", "user"))
			assert.Equal(t, c.Result, got)

			ff := c.Store.GetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
		})
	}
}

func Test_Pendings_Save(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *PendingStoreMock
		Logged string
	}{
		"Confirmation is stored": {Store: stubPendingStore()},
		"Error returned by store.Set": {
			// a failed save is logged, never fatal: the turn is already
			// parked and the user still has to be asked.
			Store: &PendingStoreMock{
				SetFunc: func(_ context.Context, _ string, _ PendingConfirm) error {
					return assert.AnError
				},
			},
			Logged: "failed to save pending confirmation",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			NewPendings(slog.New(slog.NewTextHandler(&buf, nil)), c.Store).
				Save(context.Background(), SessionKey("org", "user"), stubPending())

			ff := c.Store.SetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
			assert.Equal(t, stubPending(), ff[0].Value)

			if c.Logged == "" {
				assert.Empty(t, buf.String())

				return
			}

			assert.Contains(t, buf.String(), c.Logged)
		})
	}
}

func Test_Pendings_Clear(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *PendingStoreMock
		Logged string
	}{
		"Confirmation is forgotten": {Store: stubPendingStore()},
		"Error returned by store.Delete": {
			Store: &PendingStoreMock{
				DeleteFunc: func(_ context.Context, _ string) error {
					return assert.AnError
				},
			},
			Logged: "failed to clear pending confirmation",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			NewPendings(slog.New(slog.NewTextHandler(&buf, nil)), c.Store).
				Clear(context.Background(), SessionKey("org", "user"))

			ff := c.Store.DeleteCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)

			if c.Logged == "" {
				assert.Empty(t, buf.String())

				return
			}

			assert.Contains(t, buf.String(), c.Logged)
		})
	}
}

func Test_pendingKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:session:org:user:pending", pendingKey(SessionKey("org", "user")))
}

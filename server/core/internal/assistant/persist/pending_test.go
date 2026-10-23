package persist

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPendings(t *testing.T) {
	t.Parallel()

	store := &PendingStoreMock{}
	log := slog.New(slog.DiscardHandler)

	p := NewPendings(log, store)
	require.NotNil(t, p)

	assert.Same(t, log, p.log)
	assert.Same(t, store, p.store)
}

func Test_Pendings_Load(t *testing.T) {
	t.Parallel()

	pending := PendingConfirm{
		TurnID:       "t1",
		InterruptIDs: []string{"a"},
		Actions: []protocol.ConfirmAction{
			{Tool: "insert_block", DocumentID: "d", Summary: "Insert a callout"},
		},
	}

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

			got := NewPendings(slog.New(slog.DiscardHandler), c.Store).Load(context.Background(), SessionKey("org", "user"))
			assert.Equal(t, c.Result, got)

			ff := c.Store.GetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
		})
	}
}

func Test_Pendings_Save(t *testing.T) {
	t.Parallel()

	pending := PendingConfirm{
		TurnID:       "t1",
		InterruptIDs: []string{"a"},
		Actions: []protocol.ConfirmAction{
			{Tool: "insert_block", DocumentID: "d", Summary: "Insert a callout"},
		},
	}

	cc := map[string]struct {
		Store  *PendingStoreMock
		Logged string
	}{
		"Confirmation is stored": {Store: &PendingStoreMock{}},
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
				Save(context.Background(), SessionKey("org", "user"), pending)

			ff := c.Store.SetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
			assert.Equal(t, pending, ff[0].Value)

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
		"Confirmation is forgotten": {Store: &PendingStoreMock{}},
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

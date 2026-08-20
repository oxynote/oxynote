package assistant

import (
	"context"
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
		data = map[string]pendingConfirm{}
	)

	return &PendingStoreMock{
		GetFunc: func(_ context.Context, key string) (*pendingConfirm, error) {
			mu.Lock()
			defer mu.Unlock()

			value, ok := data[key]
			if !ok {
				return nil, errutil.ErrNotFound
			}

			return &value, nil
		},
		SetFunc: func(_ context.Context, key string, value pendingConfirm) error {
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
func stubPending() pendingConfirm {
	return pendingConfirm{
		TurnID:       "t1",
		InterruptIDs: []string{"a"},
		Actions: []protocol.ConfirmAction{
			{Tool: "insert_block", DocumentID: "d", Summary: "Insert a callout"},
		},
	}
}

func Test_Manager_loadPending(t *testing.T) {
	t.Parallel()

	pending := stubPending()

	cc := map[string]struct {
		Pendings *PendingStoreMock
		Result   *pendingConfirm
	}{
		"Outstanding confirmation is restored": {
			Pendings: &PendingStoreMock{
				GetFunc: func(_ context.Context, _ string) (*pendingConfirm, error) {
					return &pending, nil
				},
			},
			Result: &pending,
		},
		"Nothing outstanding": {
			Pendings: &PendingStoreMock{
				GetFunc: func(_ context.Context, _ string) (*pendingConfirm, error) {
					return nil, errutil.ErrNotFound
				},
			},
		},
		"Error returned by pendings.Get": {
			// an unreadable record leaves the turn parked rather than
			// resuming writes nobody approved.
			Pendings: &PendingStoreMock{
				GetFunc: func(_ context.Context, _ string) (*pendingConfirm, error) {
					return nil, assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := testManager()
			m.pendings = c.Pendings

			assert.Equal(t, c.Result, m.loadPending(context.Background(), "assistant:session:org:user"))

			ff := c.Pendings.GetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
		})
	}
}

func Test_Manager_savePending(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Pendings *PendingStoreMock
	}{
		"Confirmation is stored": {Pendings: stubPendingStore()},
		"Error returned by pendings.Set": {
			Pendings: &PendingStoreMock{
				SetFunc: func(_ context.Context, _ string, _ pendingConfirm) error {
					return assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := testManager()
			m.pendings = c.Pendings

			// a failed save is logged, never fatal: the turn is already
			// parked and the user still has to be asked.
			m.savePending(context.Background(), "assistant:session:org:user", stubPending())

			ff := c.Pendings.SetCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
			assert.Equal(t, stubPending(), ff[0].Value)
		})
	}
}

func Test_Manager_clearPending(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Pendings *PendingStoreMock
	}{
		"Confirmation is forgotten": {Pendings: stubPendingStore()},
		"Error returned by pendings.Delete": {
			Pendings: &PendingStoreMock{
				DeleteFunc: func(_ context.Context, _ string) error {
					return assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := testManager()
			m.pendings = c.Pendings

			m.clearPending(context.Background(), "assistant:session:org:user")

			ff := c.Pendings.DeleteCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "assistant:session:org:user:pending", ff[0].Key)
		})
	}
}

func Test_createPendingKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:session:org:user:pending",
		createPendingKey(createSessionKey("org", "user")))
}

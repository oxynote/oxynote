package persist

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewHistory(t *testing.T) {
	t.Parallel()

	store := &HistoryStoreMock{}
	log := discardLog()

	h := NewHistory(log, store)
	require.NotNil(t, h)

	assert.Same(t, log, h.log)
	assert.Same(t, store, h.store)
}

func Test_History_Load(t *testing.T) {
	t.Parallel()

	msgs := []*schema.Message{schema.UserMessage("hi")}

	cc := map[string]struct {
		Store  *HistoryStoreMock
		Result []*schema.Message
	}{
		"Stored conversation is restored": {
			Store: &HistoryStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return &msgs, nil
				},
			},
			Result: msgs,
		},
		"Empty stored conversation": {
			Store: &HistoryStoreMock{
				//nolint:nilnil // the case under test is a store with no value and no error
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return nil, nil
				},
			},
		},
		"Absent conversation starts empty": {
			Store: &HistoryStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return nil, errutil.ErrNotFound
				},
			},
		},
		"Error returned by store.Get": {
			// a broken history costs the user their context, not their
			// ability to chat.
			Store: &HistoryStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return nil, assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, NewHistory(discardLog(), c.Store).Load(context.Background(), "k"))
		})
	}
}

func Test_History_Save(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *HistoryStoreMock
		Logged string
	}{
		"Conversation is stored": {Store: &HistoryStoreMock{}},
		"Error returned by store.Set": {
			// a failed save is logged, never fatal: losing history must
			// not take the conversation down with it.
			Store: &HistoryStoreMock{
				SetFunc: func(_ context.Context, _ string, _ []*schema.Message) error {
					return assert.AnError
				},
			},
			Logged: "failed to save conversation",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			NewHistory(slog.New(slog.NewTextHandler(&buf, nil)), c.Store).
				Save(context.Background(), "k", nil)

			assert.Len(t, c.Store.SetCalls(), 1)

			if c.Logged == "" {
				assert.Empty(t, buf.String())

				return
			}

			assert.Contains(t, buf.String(), c.Logged)
		})
	}
}

func Test_History_Clear(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *HistoryStoreMock
		Logged string
	}{
		"Conversation is forgotten": {Store: &HistoryStoreMock{}},
		"Error returned by store.Delete": {
			Store: &HistoryStoreMock{
				DeleteFunc: func(_ context.Context, _ string) error {
					return assert.AnError
				},
			},
			Logged: "failed to delete conversation",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			NewHistory(slog.New(slog.NewTextHandler(&buf, nil)), c.Store).
				Clear(context.Background(), "k")

			ff := c.Store.DeleteCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "k", ff[0].Key)

			if c.Logged == "" {
				assert.Empty(t, buf.String())

				return
			}

			assert.Contains(t, buf.String(), c.Logged)
		})
	}
}

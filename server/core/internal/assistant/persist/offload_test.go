package persist

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewOffload(t *testing.T) {
	t.Parallel()

	store := &BlobStoreMock{}

	o := NewOffload(store)
	require.NotNil(t, o)
	assert.Same(t, store, o.store)
}

func Test_offloadKey(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp    string
		Result string
	}{
		"Plain path":            {Inp: "trunc/abc", Result: "assistant:offload:trunc/abc"},
		"Leading slash trimmed": {Inp: "/trunc/abc", Result: "assistant:offload:trunc/abc"},
		"Empty path":            {Inp: "", Result: "assistant:offload:"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, offloadKey(c.Inp))
		})
	}
}

func Test_Offload_Write(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store *BlobStoreMock
		Req   *filesystem.WriteRequest
		Err   error
	}{
		"Stored": {
			Store: &BlobStoreMock{},
			Req:   &filesystem.WriteRequest{FilePath: "trunc/1", Content: "payload"},
		},
		"Error returned by store.Set": {
			Store: &BlobStoreMock{
				SetFunc: func(_ context.Context, _ string, _ []byte) error {
					return assert.AnError
				},
			},
			Req: &filesystem.WriteRequest{FilePath: "trunc/1", Content: "payload"},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := NewOffload(c.Store).Write(context.Background(), c.Req)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.Len(t, c.Store.SetCalls(), 1)
			assert.Equal(t, "assistant:offload:trunc/1", c.Store.SetCalls()[0].Key)
		})
	}
}

func Test_Offload_Read(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *BlobStoreMock
		Result string
		Err    error
	}{
		"Stored output is returned": {
			Store: &BlobStoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return []byte("payload"), nil
				},
			},
			Result: "payload",
		},
		"Expired output tells the model to re-run the tool": {
			Store: &BlobStoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Err: errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`),
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

			res, err := NewOffload(c.Store).Read(context.Background(), "trunc/1")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

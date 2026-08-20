package offload

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_New(t *testing.T) {
	t.Parallel()

	store := &StoreMock{}

	b := New(store)
	require.NotNil(t, b)
	assert.Equal(t, store, b.store)
}

func Test_key(t *testing.T) {
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

			assert.Equal(t, c.Result, key(c.Inp))
		})
	}
}

func Test_Backend_Write(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store *StoreMock
		Req   *filesystem.WriteRequest
		Err   bool
	}{
		"Stored": {
			Store: &StoreMock{},
			Req:   &filesystem.WriteRequest{FilePath: "trunc/1", Content: "payload"},
		},
		"Error returned by store.Set": {
			Store: &StoreMock{
				SetFunc: func(_ context.Context, _ string, _ []byte) error {
					return assert.AnError
				},
			},
			Req: &filesystem.WriteRequest{FilePath: "trunc/1", Content: "payload"},
			Err: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := New(c.Store).Write(context.Background(), c.Req)
			if c.Err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Len(t, c.Store.SetCalls(), 1)
			assert.Equal(t, "assistant:offload:trunc/1", c.Store.SetCalls()[0].Key)
		})
	}
}

func Test_Backend_read(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *StoreMock
		Result string
		Err    bool
	}{
		"Stored output is returned": {
			Store: &StoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return []byte("payload"), nil
				},
			},
			Result: "payload",
		},
		"Expired output tells the model to re-run the tool": {
			Store: &StoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Err: true,
		},
		"Error returned by store.Get": {
			Store: &StoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return nil, assert.AnError
				},
			},
			Err: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := New(c.Store).read(context.Background(), "trunc/1")
			if c.Err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_readTool_Info(t *testing.T) {
	t.Parallel()

	info, err := New(&StoreMock{}).ReadTool().Info(context.Background())
	require.NoError(t, err)

	// the tool is named for tool output, not files: sitting next to
	// read_block, a generic read_file would invite the wrong call.
	assert.Equal(t, ReadToolName, info.Name)
	assert.NotEmpty(t, info.Desc)
	require.NotNil(t, info.ParamsOneOf)
}

func Test_readTool_InvokableRun(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *StoreMock
		Args   string
		Result string
		Err    bool
	}{
		"Malformed arguments": {
			Store: &StoreMock{},
			Args:  `{`,
			Err:   true,
		},
		"Missing path": {
			Store: &StoreMock{},
			Args:  `{}`,
			Err:   true,
		},
		"Stored output is returned": {
			Store: &StoreMock{
				GetFunc: func(_ context.Context, _ string) ([]byte, error) {
					return []byte("payload"), nil
				},
			},
			Args:   `{"file_path":"trunc/1"}`,
			Result: "payload",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := New(c.Store).ReadTool().InvokableRun(context.Background(), c.Args)
			if c.Err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

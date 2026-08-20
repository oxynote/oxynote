package assistant

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gomodule/redigo/redis"
	mock "github.com/oxynote/oxynote/server/core/internal/_mock"
	protocolMock "github.com/oxynote/oxynote/server/core/internal/assistant/protocol/_mock"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// the Google API transport behind the Gemini client starts an
	// opencensus worker from a package init, so it is already running
	// before any test does.
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

// discardLog returns a logger that writes nowhere.
func discardLog() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// stubChatModel builds a chat model that answers with the given
// messages in order. The last one repeats once the script runs out, so
// a test that under-specifies gets a terminating answer rather than a
// hanging agent loop.
//
// Every answer is a fresh message, as a real provider's would be: the
// agent stamps an id onto each response it receives, so handing out the
// same message twice would let two runs write over each other.
func stubChatModel(responses ...*schema.Message) *mock.ChatModel {
	var (
		mu  sync.Mutex
		idx int
	)

	next := func() *schema.Message {
		mu.Lock()
		defer mu.Unlock()

		if len(responses) == 0 {
			return schema.AssistantMessage("done", nil)
		}

		if idx > len(responses)-1 {
			idx = len(responses) - 1
		}

		res := *responses[idx]
		idx++

		return &res
	}

	cm := &mock.ChatModel{
		GenerateFunc: func(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
			return next(), nil
		},
		StreamFunc: func(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray([]*schema.Message{next()}), nil
		},
	}

	// the agent binds its tools before the first call, and the stub
	// default would hand it a nil model to run with.
	cm.WithToolsFunc = func(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
		return cm, nil
	}

	return cm
}

// testManager builds a Manager with stub dependencies. Its stores are
// in-memory rather than don't-care, because a turn that pauses has to
// read back the checkpoint and confirmation it just wrote.
func testManager() *Manager {
	return &Manager{
		log:      discardLog(),
		db:       &toolsMock.DB{},
		search:   &toolsMock.Searcher{},
		applier:  &toolsMock.EditApplier{},
		history:  &HistoryStoreMock{},
		blobs:    stubBlobStore(),
		pendings: stubPendingStore(),
		metrics:  newMetrics(metricutil.NewFactory("test", prometheus.NewRegistry()), "claude"),
		model:    stubChatModel(),
		summary:  stubChatModel(),
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	fc := metricutil.NewFactory("test", prometheus.NewRegistry())
	m := NewManager(discardLog(), nil, &redis.Pool{}, nil, nil, fc, nil, nil, "claude")

	require.NotNil(t, m)
	assert.NotNil(t, m.log)
	assert.NotNil(t, m.history)
	assert.NotNil(t, m.blobs)
	assert.NotNil(t, m.pendings)
	assert.NotNil(t, m.metrics)
	assert.Nil(t, m.tree)

	// an unset summarization model falls back to the chat model, so
	// summarization costs nothing to configure.
	assert.Equal(t, m.model, m.summary)
}

func Test_Manager_SetTreeNotifier(t *testing.T) {
	t.Parallel()

	m := testManager()
	require.Nil(t, m.tree)

	m.SetTreeNotifier(nil)
	assert.Nil(t, m.tree)
}

func Test_Manager_Chat(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Conn *protocolMock.SessionConn
		Err  error
	}{
		"Clean client close ends the chat": {
			Conn: &protocolMock.SessionConn{
				ReadFunc: func(_ context.Context) ([]byte, error) {
					return nil, io.EOF
				},
			},
		},
		"Error returned by conn.Read": {
			Conn: &protocolMock.SessionConn{
				ReadFunc: func(_ context.Context) ([]byte, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := testManager()

			err := m.Chat(context.Background(), "org", "user", c.Conn)
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_createSessionKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:session:org:user", createSessionKey("org", "user"))
}

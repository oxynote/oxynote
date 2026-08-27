package assistant

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gomodule/redigo/redis"
	mock "github.com/oxynote/oxynote/server/core/internal/_mock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	persistMock "github.com/oxynote/oxynote/server/core/internal/assistant/persist/_mock"
	protocolMock "github.com/oxynote/oxynote/server/core/internal/assistant/protocol/_mock"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/memkit"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/redkit"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
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

// stores are the mocked stores a test manager persists through, kept
// reachable so a test can decide what a session reads back or assert on
// what it wrote. The manager only ever sees them through persist, which
// keeps them to itself.
type stores struct {
	history  *persistMock.HistoryStore
	blobs    *persistMock.BlobStore
	pendings *persistMock.PendingStore
}

// stubStores builds the stores a test manager runs over. They are
// in-memory rather than don't-care, because a turn that pauses has to
// read back the checkpoint and confirmation it just wrote.
func stubStores() *stores {
	var (
		mu       sync.Mutex
		blobs    = map[string][]byte{}
		confirms = map[string]persist.PendingConfirm{}
	)

	return &stores{
		history: &persistMock.HistoryStore{},
		blobs: &persistMock.BlobStore{
			GetFunc: func(_ context.Context, key string) ([]byte, error) {
				mu.Lock()
				defer mu.Unlock()

				value, ok := blobs[key]
				if !ok {
					return nil, errutil.ErrNotFound
				}

				return value, nil
			},
			SetFunc: func(_ context.Context, key string, value []byte) error {
				mu.Lock()
				defer mu.Unlock()

				blobs[key] = value

				return nil
			},
			DeleteFunc: func(_ context.Context, key string) error {
				mu.Lock()
				defer mu.Unlock()

				delete(blobs, key)

				return nil
			},
		},
		pendings: &persistMock.PendingStore{
			GetFunc: func(_ context.Context, key string) (*persist.PendingConfirm, error) {
				mu.Lock()
				defer mu.Unlock()

				value, ok := confirms[key]
				if !ok {
					return nil, errutil.ErrNotFound
				}

				return &value, nil
			},
			SetFunc: func(_ context.Context, key string, value persist.PendingConfirm) error {
				mu.Lock()
				defer mu.Unlock()

				confirms[key] = value

				return nil
			},
			DeleteFunc: func(_ context.Context, key string) error {
				mu.Lock()
				defer mu.Unlock()

				delete(confirms, key)

				return nil
			},
		},
	}
}

// testManager builds a Manager with stub dependencies.
func testManager() *Manager {
	m, _ := testManagerStores()

	return m
}

// testManagerStores builds a Manager alongside the stores it persists
// through, for tests that drive or assert on what was stored.
func testManagerStores() (*Manager, *stores) {
	st := stubStores()
	log := discardLog()

	m := &Manager{
		log: log,
		db:  &toolsMock.DB{},
		search: &toolsMock.Searcher{
			ConfiguredFunc: func() bool { return true },
		},
		jobs:        search.NewJobs(true),
		applier:     &toolsMock.EditApplier{},
		history:     persist.NewHistory(log, st.history),
		checkpoints: persist.NewCheckpoints(log, st.blobs),
		pendings:    persist.NewPendings(log, st.pendings),
		offload:     persist.NewOffload(st.blobs),
		metrics:     newMetrics(metricutil.NewFactory("test", prometheus.NewRegistry()), "claude"),
		model:       stubChatModel(),
		summary:     stubChatModel(),
	}

	m.turns.m = make(map[string]struct{})

	return m, st
}

func Test_Manager_Configured(t *testing.T) {
	t.Parallel()

	assert.True(t, testManager().Configured())
	assert.False(t, (&Manager{}).Configured())
}

func Test_NewManager(t *testing.T) {
	cc := map[string]struct {
		Pool   *redis.Pool
		Blobs  any
		Values any
	}{
		"Without a pool the conversation is kept in memory": {
			Blobs:  &memkit.BytesStore{},
			Values: &memkit.ValueStore[[]*schema.Message]{},
		},
		"With a pool the conversation is kept in redis": {
			Pool:   &redis.Pool{},
			Blobs:  &redkit.BytesStore{},
			Values: &redkit.ValueStore[[]*schema.Message]{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			fc := metricutil.NewFactory("test", prometheus.NewRegistry())
			m := NewManager(discardLog(), nil, c.Pool, nil, nil, fc, nil, nil, search.NewJobs(false), nil, "claude")

			require.NotNil(t, m)
			assert.NotNil(t, m.log)
			assert.NotNil(t, m.history)
			assert.NotNil(t, m.checkpoints)
			assert.NotNil(t, m.pendings)
			assert.NotNil(t, m.offload)
			assert.NotNil(t, m.metrics)
			assert.NotNil(t, m.turns.m)
			assert.Nil(t, m.tree)

			// the checkpoint and the offloaded results share one byte
			// store, so three stores back the four persist types and
			// the shared one is swept once.
			require.Len(t, m.stores, 3)
			assert.IsType(t, c.Blobs, m.stores[0])
			assert.IsType(t, c.Values, m.stores[1])

			// an unset summarization model falls back to the chat
			// model, so summarization costs nothing to configure.
			assert.Equal(t, m.model, m.summary)
		})
	}
}

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	var started sync.WaitGroup

	started.Add(3)

	newStore := func() *persistMock.BlobStore {
		return &persistMock.BlobStore{
			StartFunc: func(ctx context.Context) {
				started.Done()

				<-ctx.Done()
			},
		}
	}

	stores := []*persistMock.BlobStore{newStore(), newStore(), newStore()}

	m := &Manager{
		stores: []persist.Starter{stores[0], stores[1], stores[2]},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		m.Start(ctx)
	}()

	started.Wait()

	// every store runs until the manager's context is cancelled, and
	// the manager blocks for as long as they do.
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("manager did not stop")
	}

	for _, s := range stores {
		assert.Len(t, s.StartCalls(), 1)
	}
}

func Test_Manager_SetTreeNotifier(t *testing.T) {
	t.Parallel()

	m := testManager()
	require.Nil(t, m.tree)

	m.SetTreeNotifier(nil)
	assert.Nil(t, m.tree)
}

func Test_Manager_ToolSet(t *testing.T) {
	t.Parallel()

	m := testManager()

	s := m.ToolSet("org1", "user1")
	require.NotNil(t, s)

	// the set carries the full registry — the eighteen document tools,
	// the nine data-source tools and the offloaded-result reader —
	// wired from the manager's shared dependencies and scoped to the
	// requested pair.
	assert.Len(t, s.Tools(), 28)
}

func Test_Manager_claimTurn(t *testing.T) {
	t.Parallel()

	m := testManager()

	require.True(t, m.claimTurn("k1"))

	// held until released, and per key: another conversation's turns
	// are not serialized with this one's.
	assert.False(t, m.claimTurn("k1"))
	assert.True(t, m.claimTurn("k2"))
}

func Test_Manager_releaseTurn(t *testing.T) {
	t.Parallel()

	m := testManager()

	require.True(t, m.claimTurn("k1"))
	m.releaseTurn("k1")

	assert.True(t, m.claimTurn("k1"))
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

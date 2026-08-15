package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	protocolMock "github.com/oxynote/oxynote/server/core/internal/assistant/protocol/_mock"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// userMsg builds a plain user text message.
func userMsg(text string) anthropic.MessageParam {
	return anthropic.NewUserMessage(anthropic.NewTextBlock(text))
}

// assistantMsg builds an assistant message from the given blocks.
func assistantMsg(blocks ...anthropic.ContentBlockParamUnion) anthropic.MessageParam {
	return anthropic.MessageParam{
		Role:    anthropic.MessageParamRoleAssistant,
		Content: blocks,
	}
}

// toolUseBlock builds a tool_use content block.
func toolUseBlock(id, name, args string) anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    id,
			Name:  name,
			Input: json.RawMessage(args),
		},
	}
}

// toolResultMsg builds a user message carrying only a tool_result.
func toolResultMsg(toolUseID string) anthropic.MessageParam {
	return anthropic.MessageParam{
		Role:    anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{makeToolResult(toolUseID, json.RawMessage(`{}`))},
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	db := &toolsMock.DB{}
	search := &toolsMock.Searcher{}
	applier := &toolsMock.EditApplier{}

	m := NewManager(log, db, &redis.Pool{}, "key", metricutil.NewFactory("test", nil), applier, search)
	require.NotNil(t, m)

	assert.NotNil(t, m.log)
	assert.Same(t, db, m.db)
	assert.Same(t, search, m.search)
	assert.NotNil(t, m.store)
	assert.NotNil(t, m.client)
	assert.Same(t, applier, m.applier)
	assert.NotNil(t, m.metrics)
	assert.Nil(t, m.tree)
}

func Test_Manager_SetTreeNotifier(t *testing.T) {
	t.Parallel()

	tree := &toolsMock.TreeNotifier{}
	m := &Manager{}

	m.SetTreeNotifier(tree)
	assert.Same(t, tree, m.tree)
}

func Test_Manager_NewSession(t *testing.T) {
	stored := []anthropic.MessageParam{userMsg("hi"), assistantMsg(anthropic.NewTextBlock("hello"))}

	cc := map[string]struct {
		Store        *SessionStoreMock
		Messages     []anthropic.MessageParam
		HistoryCalls int
		Err          error
	}{
		"Error returned by store.Get": {
			Store: &SessionStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]anthropic.MessageParam, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		"No stored history": {
			Store: &SessionStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]anthropic.MessageParam, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Messages:     []anthropic.MessageParam{},
			HistoryCalls: 0,
		},
		"Stored history restores and replays": {
			Store: &SessionStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]anthropic.MessageParam, error) {
					return &stored, nil
				},
			},
			Messages:     stored,
			HistoryCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			writer := &protocolMock.SessionWriter{}
			m := &Manager{log: slog.New(slog.DiscardHandler), store: c.Store}

			session, err := m.NewSession(context.Background(), "org", "user", writer)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, session)
			t.Cleanup(func() { assert.NoError(t, session.Close()) })

			assert.Same(t, m, session.man)
			assert.Equal(t, "org", session.orgID)
			assert.Equal(t, "user", session.userID)
			assert.Same(t, writer, session.writer)
			assert.NotNil(t, session.supv)
			assert.NotNil(t, session.tools)
			assert.Equal(t, c.Messages, session.messages)

			ff := writer.WriteJSONCalls()
			require.Len(t, ff, c.HistoryCalls)

			if c.HistoryCalls > 0 {
				history, ok := ff[0].Msg.(protocol.HistoryMessage)
				require.True(t, ok)
				assert.Len(t, history.Messages, 2)
			}
		})
	}
}

func Test_Manager_saveMessages(t *testing.T) {
	t.Parallel()

	// error is logged, not returned
	var buf bytes.Buffer

	store := &SessionStoreMock{
		SetFunc: func(_ context.Context, _ string, _ []anthropic.MessageParam) error {
			return assert.AnError
		},
	}
	m := &Manager{log: slog.New(slog.NewTextHandler(&buf, nil)), store: store}

	m.saveMessages(context.Background(), "org", "user", []anthropic.MessageParam{userMsg("hi")})
	assert.Contains(t, buf.String(), "failed to save session messages")

	// success stores under the session key
	buf.Reset()

	store.SetFunc = func(_ context.Context, key string, msgs []anthropic.MessageParam) error {
		assert.Equal(t, "assistant:session:org:user", key)
		assert.Len(t, msgs, 1)

		return nil
	}

	m.saveMessages(context.Background(), "org", "user", []anthropic.MessageParam{userMsg("hi")})
	assert.Empty(t, buf.String())
	assert.Len(t, store.SetCalls(), 2)
}

func Test_Manager_deleteMessages(t *testing.T) {
	t.Parallel()

	// error is logged, not returned
	var buf bytes.Buffer

	store := &SessionStoreMock{
		DeleteFunc: func(_ context.Context, _ string) error {
			return assert.AnError
		},
	}
	m := &Manager{log: slog.New(slog.NewTextHandler(&buf, nil)), store: store}

	m.deleteMessages(context.Background(), "org", "user")
	assert.Contains(t, buf.String(), "failed to delete session messages")

	// success deletes under the session key
	buf.Reset()

	store.DeleteFunc = func(_ context.Context, key string) error {
		assert.Equal(t, "assistant:session:org:user", key)

		return nil
	}

	m.deleteMessages(context.Background(), "org", "user")
	assert.Empty(t, buf.String())
	assert.Len(t, store.DeleteCalls(), 2)
}

func Test_stripToolMessages(t *testing.T) {
	cc := map[string]struct {
		Input  []anthropic.MessageParam
		Result []anthropic.MessageParam
	}{
		"Plain conversation is untouched": {
			Input:  []anthropic.MessageParam{userMsg("hi"), assistantMsg(anthropic.NewTextBlock("hello"))},
			Result: []anthropic.MessageParam{userMsg("hi"), assistantMsg(anthropic.NewTextBlock("hello"))},
		},
		"Assistant tool_use blocks are stripped": {
			Input: []anthropic.MessageParam{
				assistantMsg(anthropic.NewTextBlock("looking"), toolUseBlock("t1", "read_block", `{}`)),
			},
			Result: []anthropic.MessageParam{
				assistantMsg(anthropic.NewTextBlock("looking")),
			},
		},
		"Assistant message with only tool_use is dropped": {
			Input: []anthropic.MessageParam{
				assistantMsg(toolUseBlock("t1", "read_block", `{}`)),
			},
			Result: []anthropic.MessageParam{},
		},
		"User message with only tool_result is dropped": {
			Input:  []anthropic.MessageParam{toolResultMsg("t1")},
			Result: []anthropic.MessageParam{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, stripToolMessages(c.Input))
		})
	}
}

func Test_createSessionKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:session:org:user", createSessionKey("org", "user"))
}

func Test_buildHistory(t *testing.T) {
	cc := map[string]struct {
		Input  []anthropic.MessageParam
		Result []protocol.HistoryEntry
	}{
		"Text conversation converts role and content": {
			Input: []anthropic.MessageParam{
				userMsg("hi"),
				assistantMsg(anthropic.NewTextBlock("hello")),
			},
			Result: []protocol.HistoryEntry{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			},
		},
		"Assistant narration with tool_use is skipped": {
			Input: []anthropic.MessageParam{
				assistantMsg(anthropic.NewTextBlock("let me look"), toolUseBlock("t1", "read_block", `{}`)),
				assistantMsg(anthropic.NewTextBlock("done")),
			},
			Result: []protocol.HistoryEntry{
				{Role: "assistant", Content: "done"},
			},
		},
		"Messages without text are skipped": {
			Input:  []anthropic.MessageParam{toolResultMsg("t1")},
			Result: []protocol.HistoryEntry{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, buildHistory(c.Input))
		})
	}
}

func Test_hasToolUse(t *testing.T) {
	t.Parallel()

	assert.True(t, hasToolUse(assistantMsg(toolUseBlock("t1", "read_block", `{}`))))
	assert.False(t, hasToolUse(assistantMsg(anthropic.NewTextBlock("hi"))))
}

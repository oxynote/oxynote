package assistant

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
)

func Test_Manager_loadMessages(t *testing.T) {
	t.Parallel()

	msgs := []*schema.Message{schema.UserMessage("hi")}

	cc := map[string]struct {
		History *HistoryStoreMock
		Result  []*schema.Message
	}{
		"Stored conversation is restored": {
			History: &HistoryStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return &msgs, nil
				},
			},
			Result: msgs,
		},
		"Empty stored conversation": {
			History: &HistoryStoreMock{
				//nolint:nilnil // the case under test is a store with no value and no error
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return nil, nil
				},
			},
		},
		"Absent conversation starts empty": {
			History: &HistoryStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return nil, errutil.ErrNotFound
				},
			},
		},
		"Error returned by history.Get": {
			// a broken history costs the user their context, not their
			// ability to chat.
			History: &HistoryStoreMock{
				GetFunc: func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return nil, assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := testManager()
			m.history = c.History

			assert.Equal(t, c.Result, m.loadMessages(context.Background(), "k"))
		})
	}
}

func Test_Manager_saveMessages(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		History *HistoryStoreMock
	}{
		"Conversation is stored": {History: &HistoryStoreMock{}},
		"Error returned by history.Set": {
			History: &HistoryStoreMock{
				SetFunc: func(_ context.Context, _ string, _ []*schema.Message) error {
					return assert.AnError
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := testManager()
			m.history = c.History

			// a failed save is logged, never fatal: losing history must
			// not take the conversation down with it.
			m.saveMessages(context.Background(), "k", nil)

			assert.Len(t, c.History.SetCalls(), 1)
		})
	}
}

func Test_buildHistory(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp    []*schema.Message
		Result []protocol.HistoryEntry
	}{
		"Empty conversation": {
			Result: []protocol.HistoryEntry{},
		},
		"User and assistant turns are shown": {
			Inp: []*schema.Message{
				schema.UserMessage("what does this say"),
				schema.AssistantMessage("it describes the rate limiter", nil),
			},
			Result: []protocol.HistoryEntry{
				{Role: "user", Content: "what does this say"},
				{Role: "assistant", Content: "it describes the rate limiter"},
			},
		},
		"Narration accompanying a tool call is skipped": {
			Inp: []*schema.Message{
				schema.UserMessage("summarise it"),
				schema.AssistantMessage("let me read it", []schema.ToolCall{{ID: "1"}}),
				schema.AssistantMessage("it covers three topics", nil),
			},
			Result: []protocol.HistoryEntry{
				{Role: "user", Content: "summarise it"},
				{Role: "assistant", Content: "it covers three topics"},
			},
		},
		"Tool and system messages are machinery, not conversation": {
			Inp: []*schema.Message{
				schema.SystemMessage("you are rubber duck"),
				schema.ToolMessage("{}", "1"),
				schema.UserMessage("hi"),
			},
			Result: []protocol.HistoryEntry{
				{Role: "user", Content: "hi"},
			},
		},
		"Empty and nil messages are dropped": {
			Inp: []*schema.Message{
				nil,
				schema.UserMessage(""),
				schema.UserMessage("hi"),
			},
			Result: []protocol.HistoryEntry{
				{Role: "user", Content: "hi"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, buildHistory(c.Inp))
		})
	}
}

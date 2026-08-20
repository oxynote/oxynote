package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
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

func Test_NewObserver(t *testing.T) {
	t.Parallel()

	labels := &LabellerMock{}
	writer := &WriterMock{}

	o := NewObserver(labels, writer, nil)
	require.NotNil(t, o)

	assert.NotNil(t, o.BaseChatModelAgentMiddleware)
	assert.Equal(t, labels, o.labels)
	assert.Equal(t, writer, o.writer)
	assert.Nil(t, o.history)
}

func Test_Observer_AfterAgent(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		State     *adk.ChatModelAgentState
		OmitFunc  bool
		Result    []*schema.Message
		WasCalled bool
	}{
		"Conversation is handed back": {
			State: &adk.ChatModelAgentState{
				Messages: []*schema.Message{schema.UserMessage("hi")},
			},
			Result:    []*schema.Message{schema.UserMessage("hi")},
			WasCalled: true,
		},
		"Missing state is ignored": {},
		"Missing callback is ignored": {
			State:    &adk.ChatModelAgentState{},
			OmitFunc: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				got    []*schema.Message
				called bool
			)

			history := func(msgs []*schema.Message) {
				got = msgs
				called = true
			}

			if c.OmitFunc {
				history = nil
			}

			o := NewObserver(&LabellerMock{}, &WriterMock{}, history)

			ctx, err := o.AfterAgent(context.Background(), c.State)
			require.NoError(t, err)
			assert.NotNil(t, ctx)

			assert.Equal(t, c.WasCalled, called)
			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_Observer_WrapInvokableToolCall(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Label     string
		Announced bool
	}{
		"Labelled tool is announced before it runs": {
			Label:     "Reading Runbook",
			Announced: true,
		},
		"Unlabelled tool runs silently": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var announcedBeforeRun bool

			writer := &WriterMock{}
			labels := &LabellerMock{
				LabelFunc: func(_ context.Context, _ tools.Name, _ json.RawMessage) string {
					return c.Label
				},
			}

			o := NewObserver(labels, writer, nil)

			endpoint := func(_ context.Context, _ string, _ ...tool.Option) (string, error) {
				// the point of the wrapper is that the user hears about
				// the tool before it does its work, not after.
				announcedBeforeRun = len(writer.WriteJSONCalls()) == 1

				return "{}", nil
			}

			wrapped, err := o.WrapInvokableToolCall(
				context.Background(),
				endpoint,
				&adk.ToolContext{Name: string(tools.NameReadBlock), CallID: "1"},
			)
			require.NoError(t, err)

			res, err := wrapped(context.Background(), `{"document_id":"d"}`)
			require.NoError(t, err)
			assert.Equal(t, "{}", res)

			require.Len(t, labels.LabelCalls(), 1)
			assert.Equal(t, tools.NameReadBlock, labels.LabelCalls()[0].Name)

			if !c.Announced {
				assert.Empty(t, writer.WriteJSONCalls())

				return
			}

			require.Len(t, writer.WriteJSONCalls(), 1)
			assert.True(t, announcedBeforeRun)

			msg, ok := writer.WriteJSONCalls()[0].Msg.(protocol.ToolStatusMessage)
			require.True(t, ok)
			assert.Equal(t, string(tools.NameReadBlock), msg.Tool)
			assert.Equal(t, c.Label, msg.Label)
		})
	}
}

func Test_Observer_WrapInvokableToolCall_propagatesError(t *testing.T) {
	t.Parallel()

	o := NewObserver(&LabellerMock{}, &WriterMock{}, nil)

	endpoint := func(_ context.Context, _ string, _ ...tool.Option) (string, error) {
		return "", assert.AnError
	}

	wrapped, err := o.WrapInvokableToolCall(
		context.Background(),
		endpoint,
		&adk.ToolContext{Name: "x"},
	)
	require.NoError(t, err)

	_, err = wrapped(context.Background(), `{}`)
	assert.Equal(t, assert.AnError, err)
}

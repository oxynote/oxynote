package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewObserver(t *testing.T) {
	t.Parallel()

	labels := &LabellerMock{}
	writer := &WriterMock{}

	o := NewObserver(labels, writer, nil, nil)
	require.NotNil(t, o)

	assert.NotNil(t, o.BaseChatModelAgentMiddleware)
	assert.Equal(t, labels, o.labels)
	assert.Equal(t, writer, o.writer)
	assert.Nil(t, o.history)
	assert.Nil(t, o.observe)
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

			o := NewObserver(&LabellerMock{}, &WriterMock{}, history, nil)

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
		Label       string
		Announced   bool
		Interrupt   bool
		OmitObserve bool
		Err         error
		Observed    string
	}{
		"Labelled tool is announced before it runs": {
			Label:     "Reading Runbook",
			Announced: true,
			Observed:  string(tools.NameReadBlock) + ":success",
		},
		"Unlabelled tool runs silently": {
			Observed: string(tools.NameReadBlock) + ":success",
		},
		"Error returned by the wrapped endpoint": {
			Err:      errors.New("endpoint failed"),
			Observed: string(tools.NameReadBlock) + ":error",
		},
		"Interrupted tool is not counted": {
			Interrupt: true,
			Err:       assert.AnError,
		},
		"Nil observe callback is tolerated": {
			OmitObserve: true,
		},
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

			var observed []string

			observe := func(name tools.Name, status string, seconds float64) {
				observed = append(observed, string(name)+":"+status)

				assert.GreaterOrEqual(t, seconds, 0.0)
			}

			if c.OmitObserve {
				observe = nil
			}

			o := NewObserver(labels, writer, nil, observe)

			endpoint := func(_ context.Context, _ string, _ ...tool.Option) (string, error) {
				// the point of the wrapper is that the user hears about
				// the tool before it does its work, not after.
				announcedBeforeRun = len(writer.WriteJSONCalls()) == 1

				if c.Interrupt {
					return "", compose.NewInterruptAndRerunErr("awaiting confirmation")
				}

				if c.Err != nil {
					return "", c.Err
				}

				return "{}", nil
			}

			wrapped, err := o.WrapInvokableToolCall(
				context.Background(),
				endpoint,
				&adk.ToolContext{Name: string(tools.NameReadBlock), CallID: "1"},
			)
			require.NoError(t, err)

			res, err := wrapped(context.Background(), `{"document_id":"d"}`)
			testutil.AssertEqualError(t, c.Err, err)

			// interrupts pause the tool rather than finish it, so they
			// must not be counted as an outcome.
			if c.Observed == "" {
				assert.Empty(t, observed)
			} else {
				assert.Equal(t, []string{c.Observed}, observed)
			}

			if err != nil {
				return
			}

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

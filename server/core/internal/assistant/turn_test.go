package assistant

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prepTurn builds a session and a turn within it, wired to a recorder
// capturing everything the turn writes back.
func prepTurn(t *testing.T, m *Manager) (*turn, *recorder) {
	t.Helper()

	s, rec := prepSession(t, m)

	return s.newTurn(), rec
}

func Test_session_newTurn(t *testing.T) {
	t.Parallel()

	s, _ := prepSession(t, testManager())

	tn := s.newTurn()
	require.NotNil(t, tn)
	assert.Equal(t, s, tn.sess)

	// the id is minted with the turn, so a confirmation raised later
	// carries the id the turn has had all along.
	_, err := xid.FromString(tn.id)
	assert.NoError(t, err)
}

func Test_turn_fail(t *testing.T) {
	t.Parallel()

	tn, rec := prepTurn(t, testManager())

	tn.fail(context.Background(), "assistant run failed", assert.AnError)

	msg, ok := find[protocol.ErrorMessage](rec)
	require.True(t, ok)
	assert.Equal(t, "failed to execute AI request", msg.Message)

	// the turn is closed out too, or the client waits for a reply that
	// never arrives.
	assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))
}

func Test_turn_run(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Events  []*adk.AgentEvent
		Pending bool
		Expect  func(t *testing.T, rec *recorder)
	}{
		"Exhausted run finishes the turn": {
			Expect: func(t *testing.T, rec *recorder) {
				assert.Equal(t, []string{string(protocol.ServerTypeDone)}, rec.types())
			},
		},
		"Empty events are skipped": {
			Events: []*adk.AgentEvent{nil, {}},
			Expect: func(t *testing.T, rec *recorder) {
				assert.Equal(t, []string{string(protocol.ServerTypeDone)}, rec.types())
			},
		},
		"Failed run is reported": {
			Events: []*adk.AgentEvent{{Err: assert.AnError}},
			Expect: func(t *testing.T, rec *recorder) {
				msg, ok := find[protocol.ErrorMessage](rec)
				require.True(t, ok)
				assert.Equal(t, "failed to execute AI request", msg.Message)
			},
		},
		"Model output is relayed": {
			Events: []*adk.AgentEvent{{
				Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{
						Role:    schema.Assistant,
						Message: schema.AssistantMessage("here you go", nil),
					},
				},
			}},
			Expect: func(t *testing.T, rec *recorder) {
				assert.Equal(t, "here you go", rec.deltaText())
				assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))
			},
		},
		"Interrupted run parks instead of finishing": {
			Events: []*adk.AgentEvent{{
				Action: &adk.AgentAction{
					Interrupted: &adk.InterruptInfo{
						InterruptContexts: []*adk.InterruptCtx{{
							ID: "i1",
							Info: tools.ActionSummary{
								Tool:    tools.NameCreateDocument,
								Summary: "Create Runbook",
							},
						}},
					},
				},
			}},
			Pending: true,
			Expect: func(t *testing.T, rec *recorder) {
				// the caller asks the user once the session is free, so
				// consume itself sends nothing terminal.
				assert.Empty(t, rec.types())
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tn, rec := prepTurn(t, testManager())

			iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

			for _, evt := range c.Events {
				gen.Send(evt)
			}

			gen.Close()

			pending := tn.run(context.Background(), iter)

			assert.Equal(t, c.Pending, pending != nil)
			c.Expect(t, rec)
		})
	}
}

func Test_turn_finish(t *testing.T) {
	t.Parallel()

	m, st := testManagerStores()
	tn, rec := prepTurn(t, m)

	tn.finish(context.Background())

	assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))
	assert.Len(t, st.history.SetCalls(), 1)

	// a completed turn is not resumable, so its checkpoint would
	// otherwise hold a whole conversation until the key expired.
	assert.Len(t, st.blobs.DeleteCalls(), 1)
}

func Test_turn_relayOutput(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Out    *adk.AgentOutput
		Result string
	}{
		"Nothing to relay": {},
		"Output without a message": {
			Out: &adk.AgentOutput{},
		},
		"Tool results carry no prose for the user": {
			Out: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Role:    schema.Tool,
					Message: schema.ToolMessage("{}", "1"),
				},
			},
		},
		"Complete message is forwarded": {
			Out: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Role:    schema.Assistant,
					Message: schema.AssistantMessage("done reading", nil),
				},
			},
			Result: "done reading",
		},
		"Streamed message is forwarded": {
			Out: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Role:        schema.Assistant,
					IsStreaming: true,
					MessageStream: schema.StreamReaderFromArray([]*schema.Message{
						schema.AssistantMessage("done ", nil),
						schema.AssistantMessage("reading", nil),
					}),
				},
			},
			Result: "done reading",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tn, rec := prepTurn(t, testManager())

			tn.relayOutput(context.Background(), c.Out)

			assert.Equal(t, c.Result, rec.deltaText())
		})
	}
}

func Test_turn_relayMessage(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Msg    *schema.Message
		Result string
		Kind   protocol.TextEndKind
	}{
		"Nothing to relay": {},
		"Empty content is not shown": {
			Msg: schema.AssistantMessage("", nil),
		},
		"Answer is committed": {
			Msg:    schema.AssistantMessage("the limit is 100", nil),
			Result: "the limit is 100",
			Kind:   protocol.TextEndKindMessage,
		},
		"Narration before a tool call is transient": {
			Msg: schema.AssistantMessage("let me look", []schema.ToolCall{
				{ID: "1", Function: schema.FunctionCall{Name: "read_block"}},
			}),
			Result: "let me look",
			Kind:   protocol.TextEndKindStatus,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tn, rec := prepTurn(t, testManager())

			tn.relayMessage(context.Background(), c.Msg)

			assert.Equal(t, c.Result, rec.deltaText())

			end, ok := find[protocol.TextEndMessage](rec)
			assert.Equal(t, c.Result != "", ok)

			if ok {
				assert.Equal(t, c.Kind, end.Kind)
			}
		})
	}
}

func Test_turn_relayStream(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Stream *schema.StreamReader[*schema.Message]
		Result string
		Ended  bool
		Kind   protocol.TextEndKind
	}{
		"Nothing to relay": {},
		"Empty chunks are skipped": {
			Stream: schema.StreamReaderFromArray([]*schema.Message{
				nil,
				schema.AssistantMessage("", nil),
			}),
		},
		"Answer is streamed then committed": {
			Stream: schema.StreamReaderFromArray([]*schema.Message{
				schema.AssistantMessage("the limit ", nil),
				schema.AssistantMessage("is 100", nil),
			}),
			Result: "the limit is 100",
			Ended:  true,
			Kind:   protocol.TextEndKindMessage,
		},
		"Narration before a tool call is transient": {
			Stream: schema.StreamReaderFromArray([]*schema.Message{
				schema.AssistantMessage("let me look", []schema.ToolCall{
					{ID: "1", Function: schema.FunctionCall{Name: "read_block"}},
				}),
			}),
			Result: "let me look",
			Ended:  true,
			Kind:   protocol.TextEndKindStatus,
		},
		"Broken stream keeps what arrived": {
			Stream: func() *schema.StreamReader[*schema.Message] {
				sr, sw := schema.Pipe[*schema.Message](2)
				sw.Send(schema.AssistantMessage("partial", nil), nil)
				sw.Send(nil, errors.New("connection reset"))
				sw.Close()

				return sr
			}(),
			Result: "partial",
			Ended:  true,
			Kind:   protocol.TextEndKindMessage,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tn, rec := prepTurn(t, testManager())

			tn.relayStream(context.Background(), c.Stream)

			assert.Equal(t, c.Result, rec.deltaText())

			end, ok := find[protocol.TextEndMessage](rec)
			require.Equal(t, c.Ended, ok)

			if ok {
				assert.Equal(t, c.Kind, end.Kind)
			}
		})
	}
}

func Test_turn_recordUsage(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Msg *schema.Message
	}{
		"Response without metadata": {
			Msg: schema.AssistantMessage("hi", nil),
		},
		"Metadata without usage": {
			Msg: &schema.Message{ResponseMeta: &schema.ResponseMeta{}},
		},
		"Usage is recorded": {
			Msg: &schema.Message{
				ResponseMeta: &schema.ResponseMeta{
					Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tn, _ := prepTurn(t, testManager())

			// usage reporting is fire-and-forget; the metric itself is
			// covered where it is defined.
			tn.recordUsage(c.Msg)
		})
	}
}

func Test_turn_confirmation(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	cc := map[string]struct {
		Info    *adk.InterruptInfo
		IDs     []string
		Actions []protocol.ConfirmAction
		Done    bool
	}{
		"Writes are described for the user": {
			Info: &adk.InterruptInfo{
				InterruptContexts: []*adk.InterruptCtx{{
					ID: "i1",
					Info: tools.ActionSummary{
						Tool:         tools.NameUpdateBlockText,
						DocumentID:   docID,
						DocumentName: "Runbook",
						Summary:      "Reword the intro",
					},
				}},
			},
			IDs: []string{"i1"},
			Actions: []protocol.ConfirmAction{{
				Tool:         string(tools.NameUpdateBlockText),
				DocumentID:   docID.String(),
				DocumentName: "Runbook",
				Summary:      "Reword the intro",
			}},
		},
		"A write naming no document carries no id": {
			Info: &adk.InterruptInfo{
				InterruptContexts: []*adk.InterruptCtx{{
					ID:   "i1",
					Info: tools.ActionSummary{Tool: tools.NameCreateDocument, Summary: "Create document \"Runbook\""},
				}},
			},
			IDs: []string{"i1"},
			Actions: []protocol.ConfirmAction{{
				Tool:    string(tools.NameCreateDocument),
				Summary: "Create document \"Runbook\"",
			}},
		},
		"Foreign interrupt is still resumable": {
			// an interrupt this package did not raise has no user-facing
			// description, but leaving it out would hang the turn.
			Info: &adk.InterruptInfo{
				InterruptContexts: []*adk.InterruptCtx{
					nil,
					{ID: "i2", Info: "something else"},
				},
			},
			IDs:     []string{"i2"},
			Actions: []protocol.ConfirmAction{},
		},
		"Nothing resumable ends the turn": {
			Info: &adk.InterruptInfo{},
			Done: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tn, rec := prepTurn(t, testManager())

			pending := tn.confirmation(context.Background(), c.Info)

			if c.Done {
				assert.Nil(t, pending)
				assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))

				return
			}

			require.NotNil(t, pending)
			assert.Equal(t, c.IDs, pending.InterruptIDs)
			assert.Equal(t, c.Actions, pending.Actions)

			_, err := xid.FromString(pending.TurnID)
			assert.NoError(t, err)
		})
	}
}

func Test_textEndKind(t *testing.T) {
	t.Parallel()

	// text preceding a tool call is narration the client shows and
	// discards; text that ends a turn is the answer and is committed.
	assert.Equal(t, protocol.TextEndKindStatus, textEndKind(true))
	assert.Equal(t, protocol.TextEndKindMessage, textEndKind(false))
}

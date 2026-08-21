package assistant

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/rs/xid"
)

// turn is one user message being answered: the agent run it starts, the
// output relayed back to the client, and the outcome — finished, failed,
// or parked on a confirmation the user has to answer.
//
// It exists for the length of that one exchange. The session it belongs
// to outlives it and holds everything that spans several: the
// conversation, the active document, the connection itself.
type turn struct {
	// sess is the conversation this turn belongs to.
	sess *session

	// id correlates a confirmation request with the client's answer. It
	// is minted when the turn starts rather than when a confirmation is
	// built, so a turn is identifiable for its whole life.
	id string
}

// newTurn creates a fresh instance of turn.
func (s *session) newTurn() *turn {
	return &turn{sess: s, id: xid.New().String()}
}

// fail reports a turn that could not run and closes it out.
func (t *turn) fail(ctx context.Context, msg string, err error) {
	t.sess.man.log.Error(msg, slog.String("error", err.Error()))
	t.sess.writer.WriteJSON(ctx, protocol.NewErrorMessage("failed to execute AI request"))
	t.sess.writer.WriteJSON(ctx, protocol.NewDoneMessage())
}

// run relays one agent run to the client, translating agent events
// into protocol messages until the run finishes, fails, or pauses for a
// confirmation.
//
// A run that pauses returns the confirmation it is waiting on and sends
// no done: the turn is still in progress from the user's point of view.
// The caller asks the user once the session is free, so an immediate
// answer is not mistaken for a second concurrent turn.
func (t *turn) run(
	ctx context.Context,
	iter *adk.AsyncIterator[*adk.AgentEvent],
) *persist.PendingConfirm {
	for {
		event, ok := iter.Next()
		if !ok {
			t.finish(ctx)

			return nil
		}

		if event == nil {
			continue
		}

		if event.Err != nil {
			t.fail(ctx, "assistant run failed", event.Err)

			return nil
		}

		if event.Action != nil && event.Action.Interrupted != nil {
			return t.confirmation(ctx, event.Action.Interrupted)
		}

		t.relayOutput(ctx, event.Output)
	}
}

// finish persists the conversation and tells the client the turn is
// complete.
func (t *turn) finish(ctx context.Context) {
	t.sess.mu.Lock()
	msgs := t.sess.messages
	t.sess.mu.Unlock()

	t.sess.man.history.Save(ctx, t.sess.key, msgs)
	t.sess.man.checkpoints.Clear(ctx, t.sess.key)
	t.sess.writer.WriteJSON(ctx, protocol.NewDoneMessage())
}

// relayOutput forwards one event's model output to the client.
func (t *turn) relayOutput(ctx context.Context, out *adk.AgentOutput) {
	if out == nil || out.MessageOutput == nil {
		return
	}

	mv := out.MessageOutput

	// tool results carry no prose for the user; the status pill was
	// already sent before the tool ran.
	if mv.Role != schema.Assistant {
		return
	}

	if !mv.IsStreaming {
		t.relayMessage(ctx, mv.Message)

		return
	}

	t.relayStream(ctx, mv.MessageStream)
}

// relayMessage forwards a complete assistant message.
func (t *turn) relayMessage(ctx context.Context, msg *schema.Message) {
	if msg == nil {
		return
	}

	t.recordUsage(msg)

	if msg.Content == "" {
		return
	}

	t.sess.writer.WriteJSON(ctx, protocol.NewTextDeltaMessage(msg.Content))
	t.sess.writer.WriteJSON(ctx, protocol.NewTextEndMessage(textEndKind(len(msg.ToolCalls) > 0)))
}

// relayStream forwards an assistant message as it is generated, so the
// user sees the reply appear rather than waiting for it.
func (t *turn) relayStream(ctx context.Context, stream *schema.StreamReader[*schema.Message]) {
	if stream == nil {
		return
	}

	defer stream.Close()

	var (
		chunks   []*schema.Message
		streamed bool
	)

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.sess.man.log.Warn("assistant stream ended early",
					slog.String("error", err.Error()),
				)
			}

			break
		}

		if chunk == nil {
			continue
		}

		chunks = append(chunks, chunk)

		if chunk.Content != "" {
			t.sess.writer.WriteJSON(ctx, protocol.NewTextDeltaMessage(chunk.Content))

			streamed = true
		}
	}

	full, err := schema.ConcatMessages(chunks)
	if err != nil {
		t.sess.man.log.Warn("assistant stream could not be assembled",
			slog.String("error", err.Error()),
		)
	}

	if full != nil {
		t.recordUsage(full)
	}

	// a run of text that precedes a tool call is the model narrating
	// what it is about to do, not its answer. The client shows those as
	// a transient status line and only commits final replies.
	if streamed {
		t.sess.writer.WriteJSON(ctx, protocol.NewTextEndMessage(
			textEndKind(full != nil && len(full.ToolCalls) > 0),
		))
	}
}

// recordUsage reports a model response's token usage.
func (t *turn) recordUsage(msg *schema.Message) {
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}

	t.sess.man.metrics.recordTokenUsage(msg.ResponseMeta.Usage)
}

// confirmation describes every write the model wants to make, for
// the user to approve or refuse. Returning nil means the run interrupted
// with nothing that can be resumed, which is treated as a finished turn
// rather than leaving the client waiting forever.
func (t *turn) confirmation(
	ctx context.Context,
	info *adk.InterruptInfo,
) *persist.PendingConfirm {
	actions := make([]protocol.ConfirmAction, 0, len(info.InterruptContexts))
	ids := make([]string, 0, len(info.InterruptContexts))

	for _, ic := range info.InterruptContexts {
		if ic == nil {
			continue
		}

		summary, ok := ic.Info.(tools.ConfirmActionSummary)
		if !ok {
			// an interrupt this package did not raise has no user-facing
			// description, but it still has to be resumed or the turn
			// would hang forever.
			ids = append(ids, ic.ID)

			continue
		}

		ids = append(ids, ic.ID)
		actions = append(actions, protocol.ConfirmAction{
			Tool:         summary.Tool,
			DocumentID:   summary.DocumentID,
			DocumentName: summary.DocumentName,
			Summary:      summary.Summary,
		})
	}

	if len(ids) == 0 {
		t.sess.man.log.Warn("assistant interrupted without any resumable point")
		t.finish(ctx)

		return nil
	}

	t.sess.man.log.Info("assistant confirm requested",
		slog.String("turn_id", t.id),
		slog.Int("action_count", len(actions)),
	)

	return &persist.PendingConfirm{
		TurnID:       t.id,
		InterruptIDs: ids,
		Actions:      actions,
	}
}

// textEndKind reports how the client should treat the text run that
// just ended.
func textEndKind(precedesToolCall bool) protocol.TextEndKind {
	if precedesToolCall {
		return protocol.TextEndKindStatus
	}

	return protocol.TextEndKindMessage
}

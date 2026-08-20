package tools

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// SessionKeyAutoApprove marks a turn in which the user answered a
// confirmation with "approve all". It lives in the agent session so it
// survives the checkpoint written when the turn is interrupted.
const SessionKeyAutoApprove = "oxynote_assistant_auto_approve"

// _registerOnce guards the one-time registration of the types that
// cross an interrupt. Registering twice panics.
var _registerOnce sync.Once

// registerConfirmTypes teaches the checkpoint serialiser about the
// values a paused write carries. A turn parked on a confirmation is
// written to Redis, and an unregistered type fails that write, which
// would surface as a failed turn rather than a prompt.
func registerConfirmTypes() {
	_registerOnce.Do(func() {
		schema.RegisterName[confirmState]("oxynote_assistant_confirm_state")
		schema.RegisterName[ConfirmActionSummary]("oxynote_assistant_confirm_action")
	})
}

// ConfirmActionSummary is the human-readable description of a pending
// write op surfaced to the confirm UI. Built from the same JSON args
// the tool was about to receive: describing a write never executes it.
type ConfirmActionSummary struct {
	// Tool is the name of the tool being proposed.
	Tool string

	// DocumentID is the document the op targets, when known.
	DocumentID string

	// DocumentName is the document's display name, when the summary
	// helper could look it up.
	DocumentName string

	// Summary is a single-line description (e.g. "Insert callout
	// after 'Rate limit' section").
	Summary string
}

// Decision is the user's answer to one confirmation request. It travels
// from the client back into the interrupted tool as resume data.
type Decision struct {
	// Approved indicates whether the proposed writes may proceed.
	Approved bool `json:"approved"`
}

// confirmState is the state a write tool preserves across an interrupt.
// The tool re-derives everything it needs from its arguments on rerun,
// so nothing has to be carried here; the type exists to give the
// interrupt a stable, registered shape.
type confirmState struct{}

// autoApproved reports whether the current turn is running under an
// "approve all" answer.
func autoApproved(ctx context.Context) bool {
	v, ok := adk.GetSessionValue(ctx, SessionKeyAutoApprove)
	if !ok {
		return false
	}

	approved, ok := v.(bool)

	return ok && approved
}

// confirming gates a write tool behind user confirmation.
//
// The gate interrupts *before* the wrapped tool does any work. Resuming
// reruns the tool from the top, so a tool that acted first and asked
// second would apply its edit twice.
//
// It is applied by the registry rather than by each tool, so a write
// cannot slip through by forgetting to ask.
type confirming struct {
	// InvokableTool is the gated tool. Embedding it forwards Info, so
	// the model sees the tool's own description unchanged.
	tool.InvokableTool

	// summary describes the pending write for the user.
	summary Confirmer

	// destructive tools ask every time, even under an approve-all.
	destructive bool
}

// InvokableRun asks for confirmation, then runs the wrapped tool only
// once the user has approved it.
func (c *confirming) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	args := json.RawMessage(argumentsInJSON)

	wasInterrupted, _, _ := compose.GetInterruptState[confirmState](ctx)
	if !wasInterrupted {
		// an "approve all" answer earlier in this turn covers every
		// later write, but never a destructive one: approving a batch
		// of text edits is not consent to delete.
		if autoApproved(ctx) && !c.destructive {
			return c.InvokableTool.InvokableRun(ctx, argumentsInJSON, opts...)
		}

		return "", c.interrupt(ctx, args)
	}

	isResumeTarget, hasDecision, decision := compose.GetResumeContext[Decision](ctx)
	if !isResumeTarget {
		// another interrupt point is being resumed. Re-interrupt so this
		// confirmation stays pending instead of being silently dropped.
		return "", c.interrupt(ctx, args)
	}

	if !hasDecision || !decision.Approved {
		return declinedResult()
	}

	return c.InvokableTool.InvokableRun(ctx, argumentsInJSON, opts...)
}

// interrupt suspends the run and hands the client a human-readable
// description of the write awaiting approval.
func (c *confirming) interrupt(ctx context.Context, args json.RawMessage) error {
	return compose.StatefulInterrupt(ctx, c.summary.Confirm(ctx, args), confirmState{})
}

// declinedResult is the tool result reported to the model when the user
// refuses a write. It is a normal result rather than an error so the
// model can acknowledge the refusal and carry on.
func declinedResult() (string, error) {
	res, err := json.Marshal(map[string]any{
		"approved": false,
		"reason":   "user declined",
	})
	if err != nil {
		// NOCOV: the payload is a literal of JSON-encodable types.
		return "", err
	}

	return string(res), nil
}

// Confirmer is implemented by every tool that mutates a document.
//
// Implementing it has two consequences, deliberately welded together:
// the tool is gated behind user confirmation, and its result is never
// cleared from the conversation by the context middlewares. The model
// has to keep knowing what it changed, while a stale read can always be
// taken again.
type Confirmer interface {
	// Confirm should describe the pending write for the user, without
	// performing it.
	Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary
}

// Destructive marks a tool that removes content. These are confirmed
// every time, even inside a turn the user auto-approved: their tool
// descriptions promise as much, and an approve-all meant for text edits
// is not consent to delete a document.
type Destructive interface {
	// Destructive should distinguish a removal from an ordinary write.
	Destructive()
}

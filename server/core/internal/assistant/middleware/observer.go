// Package middleware holds what Oxynote plugs into the agent run: the
// observer that keeps the connected client informed, and the context
// middlewares that keep a long conversation affordable.
package middleware

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

// Observer hooks into the agent run to do two things the framework has
// no opinion about: report to the client which tool is running, and hand
// back the conversation as the middlewares left it, so the next turn
// resumes from the compacted history rather than the raw one.
type Observer struct {
	*adk.BaseChatModelAgentMiddleware

	// labels resolves a tool call into a human-readable status line.
	labels Labeller

	// writer relays status updates to the connected client.
	writer Writer

	// history receives the conversation at the end of a completed run.
	history func(msgs []*schema.Message)
}

// NewObserver creates a fresh instance of Observer. The history callback
// is invoked once per completed run; it may be nil when the caller does
// not retain conversations.
func NewObserver(
	labels Labeller,
	writer Writer,
	history func(msgs []*schema.Message),
) *Observer {
	return &Observer{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		labels:                       labels,
		writer:                       writer,
		history:                      history,
	}
}

// AfterAgent captures the conversation as the run finished with it,
// including whatever the context middlewares cleared or summarised.
func (o *Observer) AfterAgent(
	ctx context.Context,
	state *adk.ChatModelAgentState,
) (context.Context, error) {
	if state != nil && o.history != nil {
		o.history(state.Messages)
	}

	return ctx, nil
}

// WrapInvokableToolCall announces each tool call to the client before it
// runs. The tool-result event arrives only once the work is done, which
// is too late to show the user what is happening.
func (o *Observer) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	name := tools.Name(tCtx.Name)

	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		if label := o.labels.Label(ctx, name, json.RawMessage(argumentsInJSON)); label != "" {
			o.writer.WriteJSON(ctx, protocol.NewToolStatusMessage(string(name), label))
		}

		return endpoint(ctx, argumentsInJSON, opts...)
	}, nil
}

// Labeller describes a pending tool call in words a person can read.
//
//go:generate ../../../scripts/codegen/mock -t internal Labeller labeller
type Labeller interface {
	// Label should return a short line describing what the named tool
	// is about to do, or an empty string for tools too noisy or too
	// generic to announce.
	Label(ctx context.Context, name tools.Name, args json.RawMessage) string
}

// Writer relays a message to the connected client.
//
//go:generate ../../../scripts/codegen/mock -t internal Writer writer
type Writer interface {
	// WriteJSON should send the message to the client.
	WriteJSON(ctx context.Context, msg any)
}

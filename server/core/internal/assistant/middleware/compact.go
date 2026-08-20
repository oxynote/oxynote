package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/offload"
)

// NewCompaction builds the two middlewares that keep a long conversation
// affordable without losing it.
//
// Reduction handles tool-result bloat: an oversized result is offloaded
// and previewed rather than destroyed, and old read results are cleared
// once the conversation outgrows its token budget. Summarization handles
// length: past a threshold the conversation is summarised rather than
// having its oldest messages silently dropped.
//
// Write results are never cleared. The model has to remember what it
// changed in order to talk about it in a later turn, and re-reading a
// document cannot recover the fact that it was the one who edited it.
func NewCompaction(
	ctx context.Context,
	summaryModel model.ToolCallingChatModel,
	backend *offload.Backend,
	writeNames []string,
) ([]adk.ChatModelAgentMiddleware, error) {
	// a typed nil would satisfy reduction's interface field and defeat
	// its own nil check, surfacing as a panic on the first oversized
	// result rather than here.
	if backend == nil {
		return nil, errors.New("offload backend is required")
	}

	red, err := reduction.New(ctx, &reduction.Config{
		Backend:              backend,
		ReadFileToolName:     offload.ReadToolName,
		ClearExcludeTools:    writeNames,
		ClearMessageRewriter: rewriteClearedRead,
	})
	if err != nil {
		return nil, fmt.Errorf("building context reduction: %w", err)
	}

	sum, err := summarization.New(ctx, &summarization.Config{Model: summaryModel})
	if err != nil {
		return nil, fmt.Errorf("building context summarization: %w", err)
	}

	return []adk.ChatModelAgentMiddleware{red, sum}, nil
}

// rewriteClearedRead replaces a cleared read with a short note saying the
// content is stale, so the model knows it once read the document and that
// what it remembers may no longer be current.
func rewriteClearedRead(
	_ context.Context,
	toolCallMsg *schema.Message,
	_ []*schema.Message,
) ([]*schema.Message, error) {
	names := make([]string, 0, len(toolCallMsg.ToolCalls))
	for _, tc := range toolCallMsg.ToolCalls {
		names = append(names, tc.Function.Name)
	}

	return []*schema.Message{
		schema.UserMessage(fmt.Sprintf(
			"<system-reminder>Earlier output from %s was cleared to save space. "+
				"Call the tool again if you need current content.</system-reminder>",
			strings.Join(names, ", "),
		)),
	}, nil
}

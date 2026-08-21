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
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

// _maxToolResultChars is the largest single tool result kept in the
// conversation whole. Past it the result is offloaded and replaced with
// a head/tail preview plus a handle the model can read back.
//
// It sits where a document has to be genuinely large before the model
// stops seeing it in full — around five hundred blocks of summary — so
// truncation stays the exception rather than something ordinary reads
// hit. Lower, and routine reads would cost an extra turn to page back
// in; higher, and one result could take a quarter of the context
// window. This is eino's own default, restated as a decision rather
// than inherited silently.
const _maxToolResultChars = 50_000

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
	backend *persist.Offload,
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
		ReadFileToolName:     string(tools.NameReadToolOutput),
		MaxLengthForTrunc:    _maxToolResultChars,
		ClearExcludeTools:    writeNames,
		ClearMessageRewriter: newClearRewriter(writeNames),
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

// newClearRewriter builds the rewriter that replaces a cleared read
// round with a short note saying the content is stale, so the model
// knows it once read the document and that what it remembers may no
// longer be current.
//
// eino invokes the rewriter for every round before it consults
// ClearExcludeTools, so a round containing any write call is handed
// back exactly as it was — rewriting it here would destroy the write
// result despite the exclusion list and invite the model to re-execute
// the write. The later per-call pass then clears whatever reads share
// the round while keeping the writes.
func newClearRewriter(writeNames []string) func(
	ctx context.Context,
	toolCallMsg *schema.Message,
	toolResponseMsgs []*schema.Message,
) ([]*schema.Message, error) {
	writes := make(map[string]struct{}, len(writeNames))
	for _, name := range writeNames {
		writes[name] = struct{}{}
	}

	return func(
		_ context.Context,
		toolCallMsg *schema.Message,
		toolResponseMsgs []*schema.Message,
	) ([]*schema.Message, error) {
		names := make([]string, 0, len(toolCallMsg.ToolCalls))

		for _, tc := range toolCallMsg.ToolCalls {
			if _, ok := writes[tc.Function.Name]; ok {
				return append([]*schema.Message{toolCallMsg}, toolResponseMsgs...), nil
			}

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
}

package middleware

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

// _maxToolResultChars is the largest tool result kept whole. A larger
// one is offloaded and replaced with a preview and a handle. It is
// eino's default, about five hundred blocks of document, so ordinary
// reads stay whole.
const _maxToolResultChars = 50_000

// NewCompaction builds the two middlewares that keep a long conversation
// affordable. Reduction offloads oversized results and clears old reads.
// Summarization summarises the conversation past a threshold. Write and
// load_tools results are never cleared: the model must remember what it
// changed, and a loaded group stays callable through its references.
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

	kept := append(slices.Clone(writeNames), _loadToolsName)

	red, err := reduction.New(ctx, &reduction.Config{
		Backend:              backend,
		ReadFileToolName:     string(tools.NameReadToolOutput),
		MaxLengthForTrunc:    _maxToolResultChars,
		ClearExcludeTools:    kept,
		ClearMessageRewriter: newClearRewriter(kept),
	})
	if err != nil {
		return nil, fmt.Errorf("building context reduction: %w", err)
	}

	sum, err := summarization.New(ctx, &summarization.Config{Model: summaryModel})
	if err != nil {
		return nil, fmt.Errorf("building context summarization: %w", err)
	}

	return []adk.ChatModelAgentMiddleware{
		red,
		sum,
	}, nil
}

// newClearRewriter replaces a cleared read round with a note that the
// content is stale. eino calls it before it checks ClearExcludeTools, so
// a round holding a write is returned unchanged. The later per-call pass
// clears its reads and keeps the writes.
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

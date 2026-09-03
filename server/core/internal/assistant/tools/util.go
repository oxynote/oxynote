package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
)

// result is the single place tool result envelopes are serialised;
// centralising it keeps the JSON shape consistent.
func result(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshalling tool result: %w", err)
	}

	return string(data), nil
}

// joinOpErrors renders per-operation failures as one message. The
// index is left out: a tool sends a single operation, so naming its
// position says nothing the reader can act on.
func joinOpErrors(errs []edit.OpError) string {
	msgs := make([]string, 0, len(errs))

	for _, e := range errs {
		msgs = append(msgs, e.Message)
	}

	return strings.Join(msgs, "; ")
}

// errRequired reports an argument the tool cannot act without.
func errRequired(key string) error {
	return fmt.Errorf("%s is required", key)
}

// blockKindLabel turns a canonical block type into a friendlier label
// for the confirm UI. Unknown types fall through verbatim.
func blockKindLabel(kind block.Type) string {
	switch kind {
	case block.BlockParagraph:
		return "a paragraph"
	case block.BlockHeading:
		return "a heading"
	case block.BlockBlockquote:
		return "a blockquote"
	case block.BlockBulletList:
		return "a bullet list"
	case block.BlockOrderedList:
		return "an ordered list"
	case block.BlockTaskList:
		return "a task list"
	case block.BlockCallout:
		return "a callout"
	case block.BlockCode:
		return "a code block"
	case block.BlockTitledCode:
		return "a titled code block"
	case block.BlockMermaid:
		return "a mermaid diagram"
	case block.BlockHorizontalRule:
		return "a divider"
	case block.BlockImage:
		return "an image"
	case block.BlockFigma:
		return "a figma embed"
	case block.BlockMetric:
		return "a metric"
	case block.BlockMetricGrid:
		return "a metric grid"
	case block.BlockSplitDoc:
		return "a split documentation block"
	case block.BlockParamList:
		return "a parameter list"
	}

	if kind == "" {
		return "a block"
	}

	return "a " + string(kind) + " block"
}

// textPreview returns at most maxLen runes of s, collapsed to a single
// line. Used for surfacing short snippets of an upcoming text edit in
// the confirm UI.
func textPreview(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	return strutil.Ellipsize(s, maxLen)
}

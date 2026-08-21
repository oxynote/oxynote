package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
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

// summarize builds a confirmation for a write that targets an existing
// document: the document is resolved once, and the tool supplies only
// the phrasing for its own change.
func summarize(inp DescribeInput, name Name, phrase func(subject string) string) ConfirmActionSummary {
	out := ConfirmActionSummary{Tool: string(name)}

	if docID := inp.DocumentID(); docID != "" {
		out.DocumentID = docID
		out.DocumentName = inp.DocumentName(docID)
	}

	out.Summary = phrase(subjectFor(out.DocumentName))

	return out
}

// blockType reads the canonical type of the block a write tool was
// handed, for the confirm summary.
func blockType(inp DescribeInput) string {
	var in struct {
		Block struct {
			Type string `json:"type"`
		} `json:"block"`
	}

	inp.Probe(&in)

	return in.Block.Type
}

// subjectFor returns the display subject for label and summary strings:
// the document's name when known, a generic fallback otherwise.
func subjectFor(docName string) string {
	if docName == "" {
		return "document"
	}

	return docName
}

// blockKindLabel turns a canonical type string into a friendlier label
// for the confirm UI. Unknown types fall through verbatim.
func blockKindLabel(kind string) string {
	switch block.Type(kind) {
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

	return "a " + kind + " block"
}

// textPreview returns at most maxLen runes of s, collapsed to a single
// line. Used for surfacing short snippets of an upcoming text edit in
// the confirm UI.
func textPreview(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	return strutil.Ellipsize(s, maxLen)
}

// Package block defines the AI-facing block model that maps to the
// ProseMirror document schema used by the editor. The AI authors and
// reads documents in this form; the package's Expand function converts
// a canonical Block into a document.Block tree (with uids assigned)
// that hocuspocus can apply to a Y.Doc, and the Compact function does
// the reverse for cheap AI reads.
//
// The canonical surface intentionally hides nested wrapper nodes that
// the TipTap schema requires (list item wrappers, the left/right
// sides of splitDocumentation, the four-deep nesting of parameter
// list items). Compound types expose their structural children as
// typed top-level fields on Block (Items, Left, Right, Params, …)
// rather than smuggling them through Attrs — Attrs is reserved for
// configuration like a heading's level or a callout's icon.
package block

import "github.com/oxynote/oxynote/server/core/internal/document"

// Type is the canonical name of a block as the AI sees it.
// Canonical names use snake_case to match the rest of the AI tool
// surface; they are distinct from the camelCase node names used by
// the underlying ProseMirror schema.
type Type string

// Canonical block types. The list mirrors what the editor renders;
// any TipTap node type not listed here is either a macro internal
// (e.g. splitDocumentationLeftSide) or an inline node not exposed
// at the block level (e.g. text nodes).
const (
	// BlockParagraph is a standard paragraph. Carries inline text.
	BlockParagraph Type = "paragraph"

	// BlockHeading is a heading block. Carries inline text. Required
	// attr "level" must be 1, 2 or 3.
	BlockHeading Type = "heading"

	// BlockBlockquote is a quoted paragraph. Carries inline text.
	BlockBlockquote Type = "blockquote"

	// BlockBulletList is an unordered list. Items hold a Block each
	// (typically a paragraph).
	BlockBulletList Type = "bullet_list"

	// BlockOrderedList is an ordered list. Items hold a Block each
	// (typically a paragraph).
	BlockOrderedList Type = "ordered_list"

	// BlockTaskList is a task/checklist. TaskItems hold a Block plus
	// a Checked flag per row.
	BlockTaskList Type = "task_list"

	// BlockCallout is a callout block. Either Text (shorthand for a
	// single-paragraph callout) or Items (paragraphs and/or lists)
	// carry the content. Optional attr "icon" (defaults to
	// "lucide:text").
	BlockCallout Type = "callout"

	// BlockCode is a fenced code block. Text holds raw code (no
	// markdown parsing). Optional attr "language".
	BlockCode Type = "code"

	// BlockTitledCode is a titled code block used inside the right
	// side of split_doc. Text holds raw code; attrs "title" (plain
	// text, required) and "language" (optional) name the block.
	BlockTitledCode Type = "titled_code"

	// BlockMermaid is a mermaid diagram block. Text holds raw
	// diagram source (no markdown).
	BlockMermaid Type = "mermaid"

	// BlockHorizontalRule is a horizontal divider.
	BlockHorizontalRule Type = "horizontal_rule"

	// BlockImage is an image block. Required attr "src"; optional
	// "alt", "title", "width".
	BlockImage Type = "image"

	// BlockFigma is an embedded Figma frame. Required attr "src";
	// optional "width", "height".
	BlockFigma Type = "figma"

	// BlockMetric is a metric visualization. Attrs are opaque
	// configuration; AI rarely authors new metric blocks but reads
	// preserve the attrs verbatim.
	BlockMetric Type = "metric"

	// BlockMetricGrid wraps one or more metric blocks at the top
	// level of a document. Items hold metric blocks.
	BlockMetricGrid Type = "metric_grid"

	// BlockSplitDoc is the split-documentation macro. Left carries
	// the heading-led prose (heading + paragraphs/lists/callouts +
	// optional param_lists); Right carries the illustrative blocks
	// (titled_code, metric). Optional attr "inversed" flips the
	// visual order.
	BlockSplitDoc Type = "split_doc"

	// BlockParamList is the parameter-list macro. Header holds the
	// section header; Params holds the rows.
	BlockParamList Type = "split_doc_param_list"
)

// Block is the canonical representation of a single block as the AI
// reads and writes it. Not every field is meaningful for every block
// type:
//
//   - Text — paragraph, heading, blockquote, callout (shorthand),
//     code, titled_code (the code body), mermaid.
//   - Attrs — per-type configuration: heading.level, image.src/alt,
//     callout.icon, code.language, split_doc.inversed,
//     titled_code.title/language, etc.
//   - Items — symmetric compound containers: bullet_list,
//     ordered_list, callout (when not using Text shorthand),
//     metric_grid.
//   - TaskItems — task_list.
//   - Left, Right — split_doc.
//   - Header, Params — param_list.
//
// Validate enforces these constraints before Expand runs.
type Block struct {
	// Type is the canonical block kind.
	Type Type `json:"type"`

	// UID is the block's stable identifier. Optional on input: when
	// empty, Expand generates a fresh nanoid; when set, Expand
	// preserves it so the AI can reference the same block across
	// turns. Compact always populates UID from the underlying
	// document.Block.
	UID string `json:"uid,omitempty"`

	// Text is the inline content of text-bearing blocks, expressed
	// in the canonical minimal-markdown subset (see markdown.go).
	// For code, titled_code, and mermaid, Text is treated as raw
	// (no markdown parsing).
	Text string `json:"text,omitempty"`

	// Attrs holds per-type configuration attributes. Legal keys and
	// value types are defined by each Type's Schema entry; Validate
	// rejects unknown or wrong-typed attrs before Expand runs.
	Attrs document.Attributes `json:"attrs,omitempty"`

	// Items holds what a container block holds: a list's entries, a
	// blockquote's or callout's contents, a metric_grid's metrics.
	Items []Block `json:"items,omitempty"`

	// Children holds the tail of a single list or task-list item. The
	// editor's item wrapper is a paragraph followed by any number of
	// further blocks — everything indented under the entry — and Children
	// is that second half.
	//
	// It describes a different relationship than Items: Items is what a
	// container holds, Children is what hangs under one of its entries.
	// Both can appear on one block, when an entry's own content is a list.
	// Every position other than a list item rejects Children.
	Children []Block `json:"children,omitempty"`

	// TaskItems holds the rows of a task_list. Each row pairs a
	// content Block with a Checked flag.
	TaskItems []TaskItem `json:"task_items,omitempty"`

	// Left is the left-side content of a split_doc. The first block
	// must be a heading; subsequent blocks may be paragraphs, lists,
	// callouts, or param_lists.
	Left []Block `json:"left,omitempty"`

	// Right is the right-side content of a split_doc. Each block
	// must be a titled_code or a metric.
	Right []Block `json:"right,omitempty"`

	// Header is the section header of a param_list. Plain text (no
	// markdown).
	Header string `json:"header,omitempty"`

	// Params is the ordered list of parameter rows in a param_list.
	Params []ParamItem `json:"params,omitempty"`
}

// TaskItem is one row of a task_list, carrying an explicit checked
// state alongside a content Block (typically a paragraph).
type TaskItem struct {
	// UID is the row's stable identifier. Same semantics as
	// Block.UID.
	UID string `json:"uid,omitempty"`

	// Checked is whether the task is marked done.
	Checked bool `json:"checked,omitempty"`

	// Block is the row's content block.
	Block Block `json:"block"`
}

// ParamItem is one row in a parameter list.
type ParamItem struct {
	// UID is the row's stable identifier. Same semantics as
	// Block.UID.
	UID string `json:"uid,omitempty"`

	// Name is the parameter name (e.g. "size_limit"). Plain text.
	Name string `json:"name"`

	// Type is the parameter type (e.g. "number", "string?", "User").
	// Plain text — free-form per the language being described.
	Type string `json:"type,omitempty"`

	// Description is the parameter description. Minimal-markdown.
	Description string `json:"description,omitempty"`
}

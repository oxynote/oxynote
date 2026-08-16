package document

// BlockNodeType is the type tag carried on every node in a
// ProseMirror document tree (Block.Type). The named type forces
// callers to compare against the BlockNode* constants below rather
// than typing raw strings.
type BlockNodeType string

// ProseMirror node-type names registered by the editor's TipTap
// schema. Match the names registered in
// web/app/components/editor/blocks/* and
// server/auth-realtime/src/schema/index.ts; renaming an editor extension means
// renaming the matching constant here.
const (
	// BlockNodeDoc is the root node wrapping a TipTap document.
	BlockNodeDoc BlockNodeType = "doc"

	// BlockNodeText is the inline text leaf node carrying actual
	// characters and marks.
	BlockNodeText BlockNodeType = "text"

	// BlockNodeParagraph is a standard paragraph block.
	BlockNodeParagraph BlockNodeType = "paragraph"

	// BlockNodeHeading is a heading block carrying a level
	// attribute (1-3).
	BlockNodeHeading BlockNodeType = "heading"

	// BlockNodeBlockquote wraps inline content in a quoted
	// paragraph.
	BlockNodeBlockquote BlockNodeType = "blockquote"

	// BlockNodeHorizontalRule is a divider atom.
	BlockNodeHorizontalRule BlockNodeType = "horizontalRule"

	// BlockNodeBulletList contains listItem children.
	BlockNodeBulletList BlockNodeType = "bulletList"

	// BlockNodeOrderedList contains listItem children.
	BlockNodeOrderedList BlockNodeType = "orderedList"

	// BlockNodeListItem wraps a paragraph or nested list inside
	// a bullet or ordered list.
	BlockNodeListItem BlockNodeType = "listItem"

	// BlockNodeTaskList contains taskItem children.
	BlockNodeTaskList BlockNodeType = "taskList"

	// BlockNodeTaskItem carries a checked attribute and wraps a
	// paragraph inside a task list.
	BlockNodeTaskItem BlockNodeType = "taskItem"

	// BlockNodeCalloutBlock contains paragraphs and lists
	// rendered with an icon and accent background.
	BlockNodeCalloutBlock BlockNodeType = "calloutBlock"

	// BlockNodeCodeBlock is a fenced code block with language
	// highlighting.
	BlockNodeCodeBlock BlockNodeType = "codeBlock"

	// BlockNodeCodeBlockTitle is the title row of a
	// titledCodeBlock.
	BlockNodeCodeBlockTitle BlockNodeType = "codeBlockTitle"

	// BlockNodeTitledCodeBlock wraps a codeBlockTitle and a
	// codeBlock inside the right side of a splitDocumentation
	// block.
	BlockNodeTitledCodeBlock BlockNodeType = "titledCodeBlock"

	// BlockNodeMermaidBlock is a fenced mermaid diagram source
	// block.
	BlockNodeMermaidBlock BlockNodeType = "mermaidBlock"

	// BlockNodeImageBlock is an image atom carrying
	// src/alt/title/width.
	BlockNodeImageBlock BlockNodeType = "imageBlock"

	// BlockNodeFigmaBlock embeds a Figma frame via
	// src/width/height.
	BlockNodeFigmaBlock BlockNodeType = "figmaBlock"

	// BlockNodeMetricBlock is a single metric visualization with
	// an opaque configuration payload.
	BlockNodeMetricBlock BlockNodeType = "metricBlock"

	// BlockNodeMetricGrid wraps one or more metricBlock children
	// at the top level of a document.
	BlockNodeMetricGrid BlockNodeType = "metricGrid"

	// BlockNodeSplitDoc is the splitDocumentation macro: a
	// heading-led left side and an illustrative right side.
	BlockNodeSplitDoc BlockNodeType = "splitDocumentation"

	// BlockNodeSplitDocLeft is the inner wrapper for the left
	// side of splitDocumentation. Macro-internal — not editable
	// as a standalone block.
	BlockNodeSplitDocLeft BlockNodeType = "splitDocumentationLeftSide"

	// BlockNodeSplitDocRight is the inner wrapper for the right
	// side of splitDocumentation. Macro-internal.
	BlockNodeSplitDocRight BlockNodeType = "splitDocumentationRightSide"

	// BlockNodeParamList is the parameter-list macro container.
	BlockNodeParamList BlockNodeType = "splitDocumentationParameterList"

	// BlockNodeParamListHeader is the parameter-list section
	// header. Macro-internal.
	BlockNodeParamListHeader BlockNodeType = "splitDocumentationParameterListHeader"

	// BlockNodeParamListItem is one row inside a parameter list.
	// Macro-internal.
	BlockNodeParamListItem BlockNodeType = "splitDocumentationParameterListItem"

	// BlockNodeParamListItemHeader wraps the title + type cells
	// of a parameter row. Macro-internal.
	BlockNodeParamListItemHeader BlockNodeType = "splitDocumentationParameterListItemHeader"

	// BlockNodeParamListItemTitle is the parameter-name cell of
	// a parameter row. Macro-internal.
	BlockNodeParamListItemTitle BlockNodeType = "splitDocumentationParameterListItemHeaderTitle"

	// BlockNodeParamListItemType is the parameter-type cell of
	// a parameter row. Macro-internal.
	BlockNodeParamListItemType BlockNodeType = "splitDocumentationParameterListItemHeaderType"
)

package block

import "github.com/oxynote/oxynote/server/core/internal/document"

// _canonicalTypes maps every ProseMirror node type the AI can read to its
// canonical name. Wrapper nodes the AI never authors — listItem,
// splitDocumentationLeftSide and the parameter-list nesting — are absent:
// they are folded into their macro's typed fields by Compact and have no
// canonical type of their own.
var _canonicalTypes = map[document.BlockNodeType]Type{
	document.BlockNodeParagraph:       BlockParagraph,
	document.BlockNodeHeading:         BlockHeading,
	document.BlockNodeBlockquote:      BlockBlockquote,
	document.BlockNodeBulletList:      BlockBulletList,
	document.BlockNodeOrderedList:     BlockOrderedList,
	document.BlockNodeTaskList:        BlockTaskList,
	document.BlockNodeCalloutBlock:    BlockCallout,
	document.BlockNodeCodeBlock:       BlockCode,
	document.BlockNodeTitledCodeBlock: BlockTitledCode,
	document.BlockNodeMermaidBlock:    BlockMermaid,
	document.BlockNodeHorizontalRule:  BlockHorizontalRule,
	document.BlockNodeImageBlock:      BlockImage,
	document.BlockNodeFigmaBlock:      BlockFigma,
	document.BlockNodeMetricBlock:     BlockMetric,
	document.BlockNodeMetricGrid:      BlockMetricGrid,
	document.BlockNodeSplitDoc:        BlockSplitDoc,
	document.BlockNodeParamList:       BlockParamList,
}

// _typeOrder lists every canonical type once, in the order the block
// model documents them. Callers publishing the set — a tool schema's
// enum — take it from here so the order is the same on every build,
// which is what keeps a provider's prompt cache warm across restarts.
var _typeOrder = []Type{
	BlockParagraph,
	BlockHeading,
	BlockBlockquote,
	BlockBulletList,
	BlockOrderedList,
	BlockTaskList,
	BlockCallout,
	BlockCode,
	BlockTitledCode,
	BlockMermaid,
	BlockHorizontalRule,
	BlockImage,
	BlockFigma,
	BlockMetric,
	BlockMetricGrid,
	BlockSplitDoc,
	BlockParamList,
}

// Types returns every canonical block type, in documentation order.
func Types() []string {
	out := make([]string, 0, len(_typeOrder))

	for _, t := range _typeOrder {
		out = append(out, string(t))
	}

	return out
}

// RootTypes returns the canonical block types that may sit directly
// under the document root, in documentation order. The rest reach the
// document inside a macro and are never placed on their own.
func RootTypes() []string {
	out := make([]string, 0, len(_allowedAtRoot))

	for _, t := range _typeOrder {
		if _allowedAtRoot[t] {
			out = append(out, string(t))
		}
	}

	return out
}

// CanonicalType returns the canonical name of a ProseMirror node type. The
// second return value reports whether the node type has one at all.
func CanonicalType(pm document.BlockNodeType) (Type, bool) {
	t, ok := _canonicalTypes[pm]

	return t, ok
}

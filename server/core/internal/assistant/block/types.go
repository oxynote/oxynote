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

// CanonicalType returns the canonical name of a ProseMirror node type. The
// second return value reports whether the node type has one at all.
func CanonicalType(pm document.BlockNodeType) (Type, bool) {
	t, ok := _canonicalTypes[pm]

	return t, ok
}

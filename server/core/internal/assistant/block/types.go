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
	document.BlockNodeFileBlock:       BlockFile,
	document.BlockNodeFigmaBlock:      BlockFigma,
	document.BlockNodeMetricBlock:     BlockMetric,
	document.BlockNodeMetricGrid:      BlockMetricGrid,
	document.BlockNodeSplitDoc:        BlockSplitDoc,
	document.BlockNodeParamList:       BlockParamList,
}

// _partNames names the ProseMirror nodes without a canonical type the
// way a message to the model refers to them: by the block they belong
// to.
var _partNames = map[document.BlockNodeType]string{
	document.BlockNodeDoc:                 "the document root",
	document.BlockNodeListItem:            "a list entry",
	document.BlockNodeTaskItem:            "a task list entry",
	document.BlockNodeCodeBlockTitle:      "a titled_code's title",
	document.BlockNodeSplitDocLeft:        "a split_doc's left side",
	document.BlockNodeSplitDocRight:       "a split_doc's right side",
	document.BlockNodeParamListHeader:     "a split_doc_param_list's header",
	document.BlockNodeParamListItem:       "a split_doc_param_list row",
	document.BlockNodeParamListItemHeader: "a split_doc_param_list row",
	document.BlockNodeParamListItemTitle:  "a split_doc_param_list row's name",
	document.BlockNodeParamListItemType:   "a split_doc_param_list row's type",
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
	BlockFile,
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

// DescribeNode returns how a message to the model names a ProseMirror
// node type: by its canonical type, or by the block it is part of when
// it has no type of its own. An unknown node type is returned as it is.
func DescribeNode(pm document.BlockNodeType) string {
	if t, ok := _canonicalTypes[pm]; ok {
		return string(t)
	}

	if name, ok := _partNames[pm]; ok {
		return name
	}

	return string(pm)
}

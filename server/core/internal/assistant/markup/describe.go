package markup

import (
	"cmp"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _nodeNames names each ProseMirror node type the way a message to the
// model refers to it: by the element it reads and writes.
var _nodeNames = map[document.BlockNodeType]string{
	document.BlockNodeDoc:                 "the document root",
	document.BlockNodeParagraph:           "<p>",
	document.BlockNodeHeading:             "a heading",
	document.BlockNodeBlockquote:          "<blockquote>",
	document.BlockNodeBulletList:          "<ul>",
	document.BlockNodeOrderedList:         "<ol>",
	document.BlockNodeListItem:            "<li>",
	document.BlockNodeTaskList:            "<tasks>",
	document.BlockNodeTaskItem:            "<task>",
	document.BlockNodeCalloutBlock:        "<callout>",
	document.BlockNodeCodeBlock:           "<pre>",
	document.BlockNodeTitledCodeBlock:     "a titled <pre>",
	document.BlockNodeCodeBlockTitle:      "a titled <pre>'s title",
	document.BlockNodeMermaidBlock:        "<mermaid>",
	document.BlockNodeHorizontalRule:      "<hr>",
	document.BlockNodeImageBlock:          "<img>",
	document.BlockNodeFileBlock:           "<file>",
	document.BlockNodeFigmaBlock:          "<figma>",
	document.BlockNodeMetricGrid:          "<metrics>",
	document.BlockNodeMetricBlock:         "<metric>",
	document.BlockNodeSplitDoc:            "<split_doc>",
	document.BlockNodeSplitDocLeft:        "<left>",
	document.BlockNodeSplitDocRight:       "<right>",
	document.BlockNodeParamList:           "<params>",
	document.BlockNodeParamListHeader:     "a <params> header",
	document.BlockNodeParamListItem:       "<param>",
	document.BlockNodeParamListItemHeader: "<param>",
	document.BlockNodeParamListItemTitle:  "a <param> name",
	document.BlockNodeParamListItemType:   "a <param> type",
}

// _hiddenTypes are the node types with no element of their own; they
// read as part of the element holding them.
var _hiddenTypes = map[document.BlockNodeType]bool{
	document.BlockNodeCodeBlockTitle:      true,
	document.BlockNodeSplitDocLeft:        true,
	document.BlockNodeSplitDocRight:       true,
	document.BlockNodeParamListHeader:     true,
	document.BlockNodeParamListItemHeader: true,
	document.BlockNodeParamListItemTitle:  true,
	document.BlockNodeParamListItemType:   true,
}

// DescribeNode returns how a message to the model names a ProseMirror
// node type. An unknown node type is returned as it is.
func DescribeNode(pm document.BlockNodeType) string {
	return cmp.Or(_nodeNames[pm], string(pm))
}

// Readable finds the block uid names among blocks, or the nearest block
// holding it when uid names a part with no element of its own, such as
// a titled pre's title row, or a param, which only its params holds.
func Readable(blocks []document.Block, uid string) (document.Block, bool) {
	for _, b := range blocks {
		if id, ok := b.UID(); ok && id == uid {
			return b, true
		}

		inner, ok := Readable(b.Content, uid)
		if !ok {
			continue
		}

		if standsAlone(b, inner) {
			return inner, true
		}

		// b may itself be a part, so the level above checks it again.
		return b, true
	}

	return document.Block{}, false
}

// standsAlone reports whether child, inside parent, is an element that
// can be read and replaced on its own. A param cannot: it is only ever
// written inside its params.
func standsAlone(parent, child document.Block) bool {
	if _hiddenTypes[child.Type] {
		return false
	}

	switch parent.Type {
	case document.BlockNodeTitledCodeBlock, document.BlockNodeParamList, document.BlockNodeParamListItem:
		return false
	default:
		return true
	}
}

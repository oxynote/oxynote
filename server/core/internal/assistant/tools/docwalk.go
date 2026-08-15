package tools

import (
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Attribute keys shared between the summary walker and the tool
// schemas.
const (
	_attrLevel    = "level"
	_attrIcon     = "icon"
	_attrLanguage = "language"
	_attrChecked  = "checked"
	_attrSrc      = "src"
	_attrInversed = "inversed"
)

// docSummaryEntry is one row in the assistant-facing document
// summary. Every directly editable block surfaces with its uid so
// the assistant can target edits or deletes precisely.
type docSummaryEntry struct {
	// UID is the block's uid attribute.
	UID string `json:"uid"`

	// Kind is the canonical block kind ("paragraph", "heading",
	// "callout", "bullet_list", "list_item", "split_doc", …). For
	// nested children we emit the inner kind (e.g. "list_item")
	// alongside ParentUID so the AI can reason about hierarchy.
	Kind string `json:"kind"`

	// Text is the flattened text of the block's subtree, useful for
	// the AI to identify the block by its content.
	Text string `json:"text,omitempty"`

	// Depth is the nesting level in this summary (0 = top-level
	// document children).
	Depth int `json:"depth"`

	// ParentUID is the uid of the enclosing block when the entry
	// is nested. Empty at depth 0.
	ParentUID string `json:"parent_uid,omitempty"`

	// Attrs surfaces the small per-kind attrs the AI might want
	// (heading.level, callout.icon, code.language, list task
	// item.checked). Opaque attrs (metric configs) are omitted.
	Attrs map[string]any `json:"attrs,omitempty"`
}

// walkDocForAssistant builds the flat list of summary entries the
// assistant uses. Each entry preserves document order; nested
// blocks follow their parent with depth/parent_uid set so the AI
// can reconstruct the hierarchy if needed.
//
// Macro internals (the LeftSide/RightSide wrappers of
// splitDocumentation, the four-deep nesting inside a parameter
// list) are not exposed: the AI sees the macro as a single entry
// and drills into it via read_block when it needs the detail.
func walkDocForAssistant(blocks []document.Block) []docSummaryEntry {
	w := &docWalker{}
	w.walkLevel(blocks, 0, "")

	return w.out
}

type docWalker struct {
	out []docSummaryEntry
}

// walkLevel emits one entry per block at the current level, then
// recurses into children for compound kinds whose children are
// individually editable (lists, callout, blockquote, metric_grid).
// Macro kinds (split_doc, param_list) emit a single entry without
// descending — their internals are not editable as standalone
// blocks.
func (w *docWalker) walkLevel(blocks []document.Block, depth int, parentUID string) {
	for _, b := range blocks {
		uid, _ := b.UID()
		if uid == "" {
			// A block without a uid can't be referenced from any
			// edit tool, so it's not useful to surface.
			continue
		}

		switch b.Type {
		// list items surface flat because their visible content is
		// the first child paragraph; titled code blocks surface as a
		// single entry because they only exist inside split_doc.right
		// and are editable as a unit — the AI replaces them whole
		// rather than poking at their codeBlockTitle / codeBlock
		// children. Deeper recursion isn't useful for either.
		case document.BlockNodeParagraph,
			document.BlockNodeHeading,
			document.BlockNodeBlockquote,
			document.BlockNodeCodeBlock,
			document.BlockNodeMermaidBlock,
			document.BlockNodeHorizontalRule,
			document.BlockNodeImageBlock,
			document.BlockNodeFigmaBlock,
			document.BlockNodeMetricBlock,
			document.BlockNodeListItem,
			document.BlockNodeTaskItem,
			document.BlockNodeTitledCodeBlock:
			w.emit(b, uid, depth, parentUID)

		case document.BlockNodeBulletList,
			document.BlockNodeOrderedList,
			document.BlockNodeTaskList,
			document.BlockNodeCalloutBlock,
			document.BlockNodeMetricGrid:
			w.emit(b, uid, depth, parentUID)
			w.walkLevel(b.Content, depth+1, uid)

		case document.BlockNodeSplitDoc:
			w.emitSplitDoc(b, uid, depth, parentUID)

		case document.BlockNodeParamList:
			w.emitParamList(b, uid, depth, parentUID)
		default:
		}
	}
}

// emit creates the standard summary entry for a block and appends
// it to the walker's output.
func (w *docWalker) emit(b document.Block, uid string, depth int, parentUID string) {
	entry := docSummaryEntry{
		UID:       uid,
		Kind:      canonicalKindForPM(b.Type),
		Text:      b.Flatten(),
		Depth:     depth,
		ParentUID: parentUID,
		Attrs:     summaryAttrs(b),
	}

	w.out = append(w.out, entry)
}

// emitSplitDoc emits one entry for a splitDocumentation node with
// a text preview drawn from both sides. Children are not descended
// — the AI uses read_block to see the macro's structure when it
// wants to edit individual pieces.
func (w *docWalker) emitSplitDoc(b document.Block, uid string, depth int, parentUID string) {
	w.out = append(w.out, docSummaryEntry{
		UID:       uid,
		Kind:      string(block.BlockSplitDoc),
		Text:      b.Flatten(),
		Depth:     depth,
		ParentUID: parentUID,
		Attrs:     summaryAttrs(b),
	})
}

// emitParamList emits one entry for a splitDocumentationParameterList
// node with text drawn from its header and item names. Children are
// not descended — see emitSplitDoc.
func (w *docWalker) emitParamList(b document.Block, uid string, depth int, parentUID string) {
	w.out = append(w.out, docSummaryEntry{
		UID:       uid,
		Kind:      string(block.BlockParamList),
		Text:      b.Flatten(),
		Depth:     depth,
		ParentUID: parentUID,
	})
}

// canonicalKindForPM maps a ProseMirror node type to the canonical
// kind name the assistant uses elsewhere. Keeping the AI on one
// vocabulary (snake_case canonical names) avoids surprises when it
// reads a summary, then writes via tools that use the canonical
// names.
func canonicalKindForPM(pm document.BlockNodeType) string {
	switch pm {
	case document.BlockNodeParagraph:
		return string(block.BlockParagraph)
	case document.BlockNodeHeading:
		return string(block.BlockHeading)
	case document.BlockNodeBlockquote:
		return string(block.BlockBlockquote)
	case document.BlockNodeBulletList:
		return string(block.BlockBulletList)
	case document.BlockNodeOrderedList:
		return string(block.BlockOrderedList)
	case document.BlockNodeTaskList:
		return string(block.BlockTaskList)
	case document.BlockNodeListItem:
		return "list_item"
	case document.BlockNodeTaskItem:
		return "task_item"
	case document.BlockNodeCalloutBlock:
		return string(block.BlockCallout)
	case document.BlockNodeCodeBlock:
		return string(block.BlockCode)
	case document.BlockNodeTitledCodeBlock:
		return string(block.BlockTitledCode)
	case document.BlockNodeMermaidBlock:
		return string(block.BlockMermaid)
	case document.BlockNodeHorizontalRule:
		return string(block.BlockHorizontalRule)
	case document.BlockNodeImageBlock:
		return string(block.BlockImage)
	case document.BlockNodeFigmaBlock:
		return string(block.BlockFigma)
	case document.BlockNodeMetricBlock:
		return string(block.BlockMetric)
	case document.BlockNodeMetricGrid:
		return string(block.BlockMetricGrid)
	case document.BlockNodeSplitDoc:
		return string(block.BlockSplitDoc)
	case document.BlockNodeParamList:
		return string(block.BlockParamList)
	default:
		return string(pm)
	}
}

// summaryAttrs filters the block's raw PM attrs down to the small
// per-kind subset the AI cares about for reads. uid is dropped
// (already on the entry); opaque metric configurations are dropped
// because they would balloon the payload and the AI never edits
// them by hand anyway.
func summaryAttrs(b document.Block) map[string]any { //nolint:cyclop // this function is complex, however, it's well-structured
	switch b.Type {
	case document.BlockNodeHeading:
		if v, ok := b.Attrs[_attrLevel]; ok {
			return map[string]any{_attrLevel: v}
		}
	case document.BlockNodeCalloutBlock:
		if v, ok := b.Attrs[_attrIcon]; ok {
			return map[string]any{_attrIcon: v}
		}
	case document.BlockNodeCodeBlock:
		if v, ok := b.Attrs[_attrLanguage]; ok && v != "" {
			return map[string]any{_attrLanguage: v}
		}
	case document.BlockNodeTaskItem:
		if v, ok := b.Attrs[_attrChecked]; ok {
			return map[string]any{_attrChecked: v}
		}
	case document.BlockNodeImageBlock:
		out := map[string]any{}

		for _, k := range []string{_attrSrc, "alt", "title", "width"} {
			if v, ok := b.Attrs[k]; ok && v != nil && v != "" {
				out[k] = v
			}
		}

		if len(out) > 0 {
			return out
		}
	case document.BlockNodeFigmaBlock:
		out := map[string]any{}

		for _, k := range []string{_attrSrc, "width", "height"} {
			if v, ok := b.Attrs[k]; ok && v != nil && v != "" {
				out[k] = v
			}
		}

		if len(out) > 0 {
			return out
		}
	case document.BlockNodeSplitDoc:
		if a, ok := b.Attrs.Get(_attrInversed); ok && a.Bool() {
			return map[string]any{_attrInversed: true}
		}
	default:
		return nil
	}

	return nil
}

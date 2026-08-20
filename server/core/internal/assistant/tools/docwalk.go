package tools

import (
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _walkerKinds names the list wrappers the canonical model has no type
// for. The summary reports them anyway, since it describes the hierarchy
// the AI targets its edits at.
var _walkerKinds = map[document.BlockNodeType]string{
	document.BlockNodeListItem: "list_item",
	document.BlockNodeTaskItem: "task_item",
}

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

// docWalker accumulates the summary entries walkDocForAssistant
// produces as it recurses through the document tree.
type docWalker struct {
	// out collects the emitted entries in document order.
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
		// the first child paragraph; deeper recursion isn't useful.
		// The macro kinds (split_doc, param_list) also surface as a
		// single entry without descending — their internals are not
		// editable as standalone blocks, so the AI drills into them
		// via read_block. Titled code blocks never reach this walk:
		// they exist only inside split_doc.right.
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
			document.BlockNodeSplitDoc,
			document.BlockNodeParamList:
			w.emit(b, uid, depth, parentUID)

		case document.BlockNodeBulletList,
			document.BlockNodeOrderedList,
			document.BlockNodeTaskList,
			document.BlockNodeCalloutBlock,
			document.BlockNodeMetricGrid:
			w.emit(b, uid, depth, parentUID)
			w.walkLevel(b.Content, depth+1, uid)
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

// canonicalKindForPM maps a ProseMirror node type to the canonical kind
// name the assistant uses elsewhere. Keeping the AI on one vocabulary
// (snake_case canonical names) avoids surprises when it reads a summary,
// then writes via tools that use the canonical names.
//
// listItem and taskItem have no canonical type — the canonical model folds
// them into their list's typed fields — but the summary names them anyway,
// since it reports the hierarchy the AI targets its edits at.
func canonicalKindForPM(pm document.BlockNodeType) string {
	if kind, ok := _walkerKinds[pm]; ok {
		return kind
	}

	t, ok := block.CanonicalType(pm)
	if !ok {
		return string(pm)
	}

	return string(t)
}

// summaryAttrs filters the block's raw PM attrs down to the small
// per-kind subset the AI cares about for reads. uid is dropped
// (already on the entry); opaque metric configurations are dropped
// because they would balloon the payload and the AI never edits
// them by hand anyway.
func summaryAttrs(b document.Block) map[string]any { //nolint:cyclop // this function is complex, however, it's well-structured
	switch b.Type {
	case document.BlockNodeHeading:
		if v, ok := b.Attrs[document.AttrLevel]; ok {
			return map[string]any{document.AttrLevel: v}
		}
	case document.BlockNodeCalloutBlock:
		if v, ok := b.Attrs[document.AttrIcon]; ok {
			return map[string]any{document.AttrIcon: v}
		}
	case document.BlockNodeCodeBlock:
		if v, ok := b.Attrs[document.AttrLanguage]; ok && v != "" {
			return map[string]any{document.AttrLanguage: v}
		}
	case document.BlockNodeTaskItem:
		if v, ok := b.Attrs[document.AttrChecked]; ok {
			return map[string]any{document.AttrChecked: v}
		}
	case document.BlockNodeImageBlock:
		out := map[string]any{}

		for _, k := range []string{document.AttrSrc, document.AttrAlt, document.AttrTitle, document.AttrWidth} {
			if v, ok := b.Attrs[k]; ok && v != nil && v != "" {
				out[k] = v
			}
		}

		if len(out) > 0 {
			return out
		}
	case document.BlockNodeFigmaBlock:
		out := map[string]any{}

		for _, k := range []string{document.AttrSrc, document.AttrWidth, document.AttrHeight} {
			if v, ok := b.Attrs[k]; ok && v != nil && v != "" {
				out[k] = v
			}
		}

		if len(out) > 0 {
			return out
		}
	case document.BlockNodeSplitDoc:
		if a, ok := b.Attrs.Get(document.AttrInversed); ok && a.Bool() {
			return map[string]any{document.AttrInversed: true}
		}
	default:
		return nil
	}

	return nil
}

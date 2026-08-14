package aiblock

import (
	"errors"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// ErrUnsupportedPMType is returned by Compact when a document.Block
// uses a TipTap node type that isn't part of the canonical taxonomy.
// Callers should treat the block as opaque (skip, or surface a
// dedicated "unknown" placeholder to the AI).
var ErrUnsupportedPMType = errors.New("canonical: unsupported ProseMirror type")

// Compact converts a document.Block (ProseMirror JSON) into the
// canonical Block representation surfaced to the AI. Wrapper nodes
// the AI never authors (listItem, splitDocumentationLeftSide,
// splitDocumentationParameterListItem, …) are folded into the
// enclosing macro's typed fields.
//
// The returned Block preserves the source block's uid attribute so
// the AI can refer to it in subsequent edit operations.
func Compact(b document.Block) (Block, error) {
	uid, _ := b.UID()

	switch b.Type {
	case document.BlockNodeParagraph:
		return compactParagraph(b, uid), nil
	case document.BlockNodeHeading:
		return compactHeading(b, uid), nil
	case document.BlockNodeBlockquote:
		return compactBlockquote(b, uid)
	case document.BlockNodeBulletList:
		return compactList(b, uid, BlockBulletList)
	case document.BlockNodeOrderedList:
		return compactList(b, uid, BlockOrderedList)
	case document.BlockNodeTaskList:
		return compactTaskList(b, uid)
	case document.BlockNodeCalloutBlock:
		return compactCallout(b, uid)
	case document.BlockNodeCodeBlock:
		return compactCode(b, uid), nil
	case document.BlockNodeTitledCodeBlock:
		return compactTitledCode(b, uid), nil
	case document.BlockNodeMermaidBlock:
		return compactMermaid(b, uid), nil
	case document.BlockNodeHorizontalRule:
		return Block{Type: BlockHorizontalRule, UID: uid}, nil
	case document.BlockNodeImageBlock:
		return compactImage(b, uid), nil
	case document.BlockNodeFigmaBlock:
		return compactFigma(b, uid), nil
	case document.BlockNodeMetricBlock:
		return compactMetric(b, uid), nil
	case document.BlockNodeMetricGrid:
		return compactMetricGrid(b, uid)
	case document.BlockNodeSplitDoc:
		return compactSplitDoc(b, uid)
	case document.BlockNodeParamList:
		return compactParamList(b, uid)
	default:
		return Block{}, fmt.Errorf("%w: %q", ErrUnsupportedPMType, b.Type)
	}
}

// CompactMany compacts a slice of document.Blocks, short-circuiting
// on the first error.
func CompactMany(blocks []document.Block) ([]Block, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	out := make([]Block, 0, len(blocks))

	for i, b := range blocks {
		c, err := Compact(b)
		if err != nil {
			return nil, fmt.Errorf("compact block %d (%s): %w", i, b.Type, err)
		}

		out = append(out, c)
	}

	return out, nil
}

func compactParagraph(b document.Block, uid string) Block {
	return Block{
		Type: BlockParagraph,
		UID:  uid,
		Text: EmitInlineMarkdown(b.Content),
	}
}

func compactHeading(b document.Block, uid string) Block {
	level := readIntAttr(b.Attrs, _attrLevel, 1)

	return Block{
		Type:  BlockHeading,
		UID:   uid,
		Text:  EmitInlineMarkdown(b.Content),
		Attrs: map[string]any{_attrLevel: level},
	}
}

func compactBlockquote(b document.Block, uid string) (Block, error) {
	// The canonical model treats blockquote as a single-paragraph
	// wrapper. When the source has exactly one paragraph child,
	// promote its text. Otherwise fall back to Items so callers can
	// still see the content.
	if len(b.Content) == 1 && b.Content[0].Type == document.BlockNodeParagraph {
		return Block{
			Type: BlockBlockquote,
			UID:  uid,
			Text: EmitInlineMarkdown(b.Content[0].Content),
		}, nil
	}

	items, err := CompactMany(b.Content)
	if err != nil {
		return Block{}, fmt.Errorf("blockquote: %w", err)
	}

	return Block{
		Type:  BlockBlockquote,
		UID:   uid,
		Items: items,
	}, nil
}

// compactList compacts a bulletList or orderedList by unwrapping
// each listItem and compacting its single content child.
func compactList(b document.Block, uid string, kind BlockType) (Block, error) {
	items := make([]Block, 0, len(b.Content))

	for i, li := range b.Content {
		if li.Type != document.BlockNodeListItem {
			return Block{}, fmt.Errorf("%s child %d: expected listItem, got %s", kind, i, li.Type)
		}

		if len(li.Content) == 0 {
			items = append(items, Block{Type: BlockParagraph})

			continue
		}

		inner, err := Compact(li.Content[0])
		if err != nil {
			return Block{}, fmt.Errorf("%s item %d: %w", kind, i, err)
		}

		items = append(items, inner)
	}

	return Block{
		Type:  kind,
		UID:   uid,
		Items: items,
	}, nil
}

// compactTaskList compacts a taskList by unwrapping each taskItem
// into a canonical TaskItem.
func compactTaskList(b document.Block, uid string) (Block, error) {
	items := make([]TaskItem, 0, len(b.Content))

	for i, ti := range b.Content {
		if ti.Type != document.BlockNodeTaskItem {
			return Block{}, fmt.Errorf("task_list child %d: expected taskItem, got %s", i, ti.Type)
		}

		itemUID, _ := ti.UID()
		checked := readBoolAttr(ti.Attrs, _attrChecked, false)

		var inner Block

		if len(ti.Content) > 0 {
			c, err := Compact(ti.Content[0])
			if err != nil {
				return Block{}, fmt.Errorf("task_list item %d: %w", i, err)
			}

			inner = c
		} else {
			inner = Block{Type: BlockParagraph}
		}

		items = append(items, TaskItem{
			UID:     itemUID,
			Checked: checked,
			Block:   inner,
		})
	}

	return Block{
		Type:      BlockTaskList,
		UID:       uid,
		TaskItems: items,
	}, nil
}

// compactCallout compacts a calloutBlock. When the callout holds a
// single paragraph child, the canonical block uses the Text
// shorthand; otherwise Items carries the child blocks verbatim.
func compactCallout(b document.Block, uid string) (Block, error) {
	icon := readStringAttr(b.Attrs, _attrIcon, "")
	attrs := map[string]any{}

	if icon != "" {
		attrs[_attrIcon] = icon
	}

	if len(b.Content) == 1 && b.Content[0].Type == document.BlockNodeParagraph {
		return Block{
			Type:  BlockCallout,
			UID:   uid,
			Text:  EmitInlineMarkdown(b.Content[0].Content),
			Attrs: attrsOrNil(attrs),
		}, nil
	}

	items, err := CompactMany(b.Content)
	if err != nil {
		return Block{}, fmt.Errorf("callout items: %w", err)
	}

	return Block{
		Type:  BlockCallout,
		UID:   uid,
		Items: items,
		Attrs: attrsOrNil(attrs),
	}, nil
}

func compactCode(b document.Block, uid string) Block {
	attrs := map[string]any{}

	if lang := readStringAttr(b.Attrs, _attrLanguage, ""); lang != "" {
		attrs[_attrLanguage] = lang
	}

	return Block{
		Type:  BlockCode,
		UID:   uid,
		Text:  flattenText(b.Content),
		Attrs: attrsOrNil(attrs),
	}
}

// compactTitledCode reads the title from the first child
// (codeBlockTitle) and the body from the second child (codeBlock).
// Defensive on malformed input: missing children produce empty
// title/body rather than an error.
func compactTitledCode(b document.Block, uid string) Block {
	attrs := map[string]any{}

	var (
		title string
		body  string
		lang  string
	)

	for _, child := range b.Content {
		switch child.Type {
		case document.BlockNodeCodeBlockTitle:
			title = flattenText(child.Content)
		case document.BlockNodeCodeBlock:
			body = flattenText(child.Content)
			lang = readStringAttr(child.Attrs, _attrLanguage, "")
		default:
		}
	}

	if title != "" {
		attrs[_attrTitle] = title
	}

	if lang != "" {
		attrs[_attrLanguage] = lang
	}

	return Block{
		Type:  BlockTitledCode,
		UID:   uid,
		Text:  body,
		Attrs: attrsOrNil(attrs),
	}
}

func compactMermaid(b document.Block, uid string) Block {
	return Block{
		Type: BlockMermaid,
		UID:  uid,
		Text: flattenText(b.Content),
	}
}

func compactImage(b document.Block, uid string) Block {
	attrs := map[string]any{}

	if src := readStringAttr(b.Attrs, _attrSrc, ""); src != "" {
		attrs[_attrSrc] = src
	}

	if alt := readStringAttr(b.Attrs, _attrAlt, ""); alt != "" {
		attrs[_attrAlt] = alt
	}

	if title := readStringAttr(b.Attrs, _attrTitle, ""); title != "" {
		attrs[_attrTitle] = title
	}

	if width := readIntAttr(b.Attrs, _attrWidth, 0); width > 0 {
		attrs[_attrWidth] = width
	}

	return Block{
		Type:  BlockImage,
		UID:   uid,
		Attrs: attrsOrNil(attrs),
	}
}

func compactFigma(b document.Block, uid string) Block {
	attrs := map[string]any{}

	if src := readStringAttr(b.Attrs, _attrSrc, ""); src != "" {
		attrs[_attrSrc] = src
	}

	if width := readIntAttr(b.Attrs, _attrWidth, 0); width > 0 {
		attrs[_attrWidth] = width
	}

	if height := readIntAttr(b.Attrs, _attrHeight, 0); height > 0 {
		attrs[_attrHeight] = height
	}

	return Block{
		Type:  BlockFigma,
		UID:   uid,
		Attrs: attrsOrNil(attrs),
	}
}

// compactMetric preserves every non-uid attribute on the source
// metric block — the configuration shape is opaque to the canonical
// layer and the AI sees it through-pass.
func compactMetric(b document.Block, uid string) Block {
	attrs := make(map[string]any, len(b.Attrs))

	for k, v := range b.Attrs {
		if k == _attrUID {
			continue
		}

		attrs[k] = v
	}

	return Block{
		Type:  BlockMetric,
		UID:   uid,
		Attrs: attrsOrNil(attrs),
	}
}

func compactMetricGrid(b document.Block, uid string) (Block, error) {
	items, err := CompactMany(b.Content)
	if err != nil {
		return Block{}, fmt.Errorf("metric_grid items: %w", err)
	}

	return Block{
		Type:  BlockMetricGrid,
		UID:   uid,
		Items: items,
	}, nil
}

// compactSplitDoc folds the splitDocumentationLeftSide and
// splitDocumentationRightSide wrapper nodes into the canonical
// block's Left/Right fields.
func compactSplitDoc(b document.Block, uid string) (Block, error) {
	out := Block{Type: BlockSplitDoc, UID: uid}

	if inversed := readBoolAttr(b.Attrs, _attrInversed, false); inversed {
		out.Attrs = map[string]any{_attrInversed: true}
	}

	for _, side := range b.Content {
		switch side.Type {
		case document.BlockNodeSplitDocLeft:
			items, err := CompactMany(side.Content)
			if err != nil {
				return Block{}, fmt.Errorf("split_doc left: %w", err)
			}

			out.Left = items
		case document.BlockNodeSplitDocRight:
			items, err := CompactMany(side.Content)
			if err != nil {
				return Block{}, fmt.Errorf("split_doc right: %w", err)
			}

			out.Right = items
		default:
		}
	}

	return out, nil
}

// compactParamList unwraps the parameter-list tree, surfacing the
// header text and each item's name / type / description.
func compactParamList(b document.Block, uid string) (Block, error) {
	out := Block{Type: BlockParamList, UID: uid}

	for _, child := range b.Content {
		switch child.Type {
		case document.BlockNodeParamListHeader:
			out.Header = flattenText(child.Content)
		case document.BlockNodeParamListItem:
			out.Params = append(out.Params, compactParamListItem(child))
		default:
		}
	}

	return out, nil
}

// compactParamListItem turns one splitDocumentationParameterListItem
// node into a canonical ParamItem.
func compactParamListItem(b document.Block) ParamItem {
	uid, _ := b.UID()
	item := ParamItem{UID: uid}

	for _, child := range b.Content {
		switch child.Type {
		case document.BlockNodeParamListItemHeader:
			for _, sub := range child.Content {
				switch sub.Type {
				case document.BlockNodeParamListItemTitle:
					item.Name = flattenText(sub.Content)
				case document.BlockNodeParamListItemType:
					item.Type = flattenText(sub.Content)
				default:
				}
			}
		case document.BlockNodeParagraph:
			item.Description = EmitInlineMarkdown(child.Content)
		default:
		}
	}

	return item
}

// flattenText concatenates the text of inline text nodes ignoring
// marks. Used for code/mermaid/header bodies that carry raw
// non-markdown text.
func flattenText(content []document.Block) string {
	var out []byte

	for _, n := range content {
		if n.Type == document.BlockNodeText {
			out = append(out, n.Text...)
		}
	}

	return string(out)
}

// attrsOrNil returns m when it has at least one entry, nil
// otherwise. Compact output uses nil maps rather than empty ones so
// JSON-marshalled canonical blocks omit the field cleanly.
func attrsOrNil(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}

	return m
}

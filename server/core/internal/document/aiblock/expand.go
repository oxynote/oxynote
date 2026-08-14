package aiblock

import (
	"errors"
	"fmt"
	"maps"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// ErrUnknownBlockType is returned by Expand when asked to expand a
// canonical block whose Type is not in the registry. Validate should
// have caught this; the error exists as a safety net for direct
// callers.
var ErrUnknownBlockType = errors.New("canonical: unknown block type")

// Expand converts a canonical Block into a document.Block ready to
// be applied to a Y.Doc. Every node in the resulting tree carries a
// "uid" attribute: when the input supplies one (Block.UID,
// TaskItem.UID, ParamItem.UID, or an attr on a passed-through
// metric block), Expand preserves it; otherwise a fresh nanoid is
// generated. Macro types (split_doc, param_list) construct the
// nested TipTap wrapper nodes that the editor's schema requires.
//
// Expand assumes the input has already been validated by Validate.
// Calling Expand on an invalid block may produce a document.Block
// that hocuspocus rejects when applied.
func Expand(b Block) (document.Block, error) {
	switch b.Type {
	case BlockParagraph:
		return expandParagraph(b), nil
	case BlockHeading:
		return expandHeading(b)
	case BlockBlockquote:
		return expandBlockquote(b), nil
	case BlockBulletList:
		return expandBulletOrOrderedList(b, document.BlockNodeBulletList)
	case BlockOrderedList:
		return expandBulletOrOrderedList(b, document.BlockNodeOrderedList)
	case BlockTaskList:
		return expandTaskList(b)
	case BlockCallout:
		return expandCallout(b)
	case BlockCode:
		return expandCode(b), nil
	case BlockTitledCode:
		return expandTitledCode(b), nil
	case BlockMermaid:
		return expandMermaid(b), nil
	case BlockHorizontalRule:
		return expandHorizontalRule(b), nil
	case BlockImage:
		return expandImage(b), nil
	case BlockFigma:
		return expandFigma(b), nil
	case BlockMetric:
		return expandMetric(b), nil
	case BlockMetricGrid:
		return expandMetricGrid(b)
	case BlockSplitDoc:
		return expandSplitDoc(b)
	case BlockParamList:
		return expandParamList(b)
	}

	return document.Block{}, fmt.Errorf("%w: %q", ErrUnknownBlockType, b.Type)
}

// ExpandMany expands a slice of canonical Blocks into a slice of
// document.Blocks, short-circuiting on the first error.
func ExpandMany(blocks []Block) ([]document.Block, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	out := make([]document.Block, 0, len(blocks))

	for i, b := range blocks {
		expanded, err := Expand(b)
		if err != nil {
			return nil, fmt.Errorf("expand block %d (%s): %w", i, b.Type, err)
		}

		out = append(out, expanded)
	}

	return out, nil
}

// resolveUID returns the supplied uid when non-empty, otherwise a
// freshly generated nanoid. Centralised so the policy is consistent
// across every block kind.
func resolveUID(supplied string) string {
	if supplied != "" {
		return supplied
	}

	return document.GenerateNodeUID()
}

// uidAttrs returns the standard {uid: …} attribute map.
func uidAttrs(uid string) map[string]any {
	return map[string]any{_attrUID: uid}
}

// expandParagraph builds a paragraph node from a canonical block,
// parsing Text as inline minimal-markdown.
func expandParagraph(b Block) document.Block {
	return document.Block{
		Type:    document.BlockNodeParagraph,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: ParseInlineMarkdown(b.Text),
	}
}

// expandHeading builds a heading node, reading the required level
// attribute. Level defaults to 1 when missing; Validate should have
// already enforced 1-3.
func expandHeading(b Block) (document.Block, error) {
	level := readIntAttr(b.Attrs, _attrLevel, 1)

	attrs := uidAttrs(resolveUID(b.UID))
	attrs[_attrLevel] = level

	return document.Block{
		Type:    document.BlockNodeHeading,
		Attrs:   attrs,
		Content: ParseInlineMarkdown(b.Text),
	}, nil
}

// expandBlockquote builds a blockquote node wrapping a single
// paragraph carrying the inline text.
func expandBlockquote(b Block) document.Block {
	inner := document.Block{
		Type:    document.BlockNodeParagraph,
		Attrs:   uidAttrs(document.GenerateNodeUID()),
		Content: ParseInlineMarkdown(b.Text),
	}

	return document.Block{
		Type:    document.BlockNodeBlockquote,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: []document.Block{inner},
	}
}

// expandBulletOrOrderedList builds a bulletList or orderedList node.
// Each canonical Item is wrapped in a listItem; if the item's
// canonical type is paragraph, the paragraph node becomes the
// listItem's content; otherwise the item is expanded as-is and used
// directly.
func expandBulletOrOrderedList(b Block, pmType document.BlockNodeType) (document.Block, error) {
	children := make([]document.Block, 0, len(b.Items))

	for i, item := range b.Items {
		expanded, err := Expand(item)
		if err != nil {
			return document.Block{}, fmt.Errorf("list item %d: %w", i, err)
		}

		children = append(children, document.Block{
			Type:    document.BlockNodeListItem,
			Attrs:   uidAttrs(document.GenerateNodeUID()),
			Content: []document.Block{expanded},
		})
	}

	return document.Block{
		Type:    pmType,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: children,
	}, nil
}

// expandTaskList builds a taskList node. Each TaskItem becomes a
// taskItem node carrying a checked attr and wrapping the row's
// content block.
func expandTaskList(b Block) (document.Block, error) {
	children := make([]document.Block, 0, len(b.TaskItems))

	for i, item := range b.TaskItems {
		expanded, err := Expand(item.Block)
		if err != nil {
			return document.Block{}, fmt.Errorf("task item %d: %w", i, err)
		}

		attrs := uidAttrs(resolveUID(item.UID))
		attrs[_attrChecked] = item.Checked

		children = append(children, document.Block{
			Type:    document.BlockNodeTaskItem,
			Attrs:   attrs,
			Content: []document.Block{expanded},
		})
	}

	return document.Block{
		Type:    document.BlockNodeTaskList,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: children,
	}, nil
}

// expandCallout builds a calloutBlock. Either Text (single-paragraph
// shorthand) or Items (any combination of paragraphs and lists)
// supplies the content. Icon defaults to "lucide:text" matching the
// editor's default.
func expandCallout(b Block) (document.Block, error) {
	attrs := uidAttrs(resolveUID(b.UID))
	attrs[_attrIcon] = readStringAttr(b.Attrs, _attrIcon, "lucide:text")

	var children []document.Block

	if len(b.Items) > 0 {
		expanded, err := ExpandMany(b.Items)
		if err != nil {
			return document.Block{}, fmt.Errorf("callout items: %w", err)
		}

		children = expanded
	} else {
		children = []document.Block{{
			Type:    document.BlockNodeParagraph,
			Attrs:   uidAttrs(document.GenerateNodeUID()),
			Content: ParseInlineMarkdown(b.Text),
		}}
	}

	return document.Block{
		Type:    document.BlockNodeCalloutBlock,
		Attrs:   attrs,
		Content: children,
	}, nil
}

// expandCode builds a codeBlock. Text is treated as raw code and
// emitted as a single text node (no markdown parsing).
func expandCode(b Block) document.Block {
	attrs := uidAttrs(resolveUID(b.UID))
	if lang := readStringAttr(b.Attrs, _attrLanguage, ""); lang != "" {
		attrs[_attrLanguage] = lang
	}

	return document.Block{
		Type:    document.BlockNodeCodeBlock,
		Attrs:   attrs,
		Content: rawTextContent(b.Text),
	}
}

// expandTitledCode builds a titledCodeBlock with its required
// codeBlockTitle + codeBlock children. Title is plain text; the code
// body is raw.
func expandTitledCode(b Block) document.Block {
	title := readStringAttr(b.Attrs, _attrTitle, "")
	lang := readStringAttr(b.Attrs, _attrLanguage, "")

	codeAttrs := uidAttrs(document.GenerateNodeUID())
	if lang != "" {
		codeAttrs[_attrLanguage] = lang
	}

	titleNode := document.Block{
		Type:    document.BlockNodeCodeBlockTitle,
		Attrs:   uidAttrs(document.GenerateNodeUID()),
		Content: rawTextContent(title),
	}

	codeNode := document.Block{
		Type:    document.BlockNodeCodeBlock,
		Attrs:   codeAttrs,
		Content: rawTextContent(b.Text),
	}

	return document.Block{
		Type:    document.BlockNodeTitledCodeBlock,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: []document.Block{titleNode, codeNode},
	}
}

// expandMermaid builds a mermaidBlock. Text is treated as raw
// diagram source.
func expandMermaid(b Block) document.Block {
	return document.Block{
		Type:    document.BlockNodeMermaidBlock,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: rawTextContent(b.Text),
	}
}

// expandHorizontalRule builds a horizontalRule atom node.
func expandHorizontalRule(b Block) document.Block {
	return document.Block{
		Type:  document.BlockNodeHorizontalRule,
		Attrs: uidAttrs(resolveUID(b.UID)),
	}
}

// expandImage builds an imageBlock atom node with src/alt/title/
// width attributes.
func expandImage(b Block) document.Block {
	attrs := uidAttrs(resolveUID(b.UID))

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

	return document.Block{
		Type:  document.BlockNodeImageBlock,
		Attrs: attrs,
	}
}

// expandFigma builds a figmaBlock atom node.
func expandFigma(b Block) document.Block {
	attrs := uidAttrs(resolveUID(b.UID))

	if src := readStringAttr(b.Attrs, _attrSrc, ""); src != "" {
		attrs[_attrSrc] = src
	}

	if width := readIntAttr(b.Attrs, _attrWidth, 0); width > 0 {
		attrs[_attrWidth] = width
	}

	if height := readIntAttr(b.Attrs, _attrHeight, 0); height > 0 {
		attrs[_attrHeight] = height
	}

	return document.Block{
		Type:  document.BlockNodeFigmaBlock,
		Attrs: attrs,
	}
}

// expandMetric builds a metricBlock. All caller-supplied attrs are
// passed through unchanged (the metric configuration shape is
// opaque to the canonical layer); the uid is taken from Block.UID
// or freshly generated.
func expandMetric(b Block) document.Block {
	attrs := copyAttrs(b.Attrs)
	attrs[_attrUID] = resolveUID(b.UID)

	return document.Block{
		Type:  document.BlockNodeMetricBlock,
		Attrs: attrs,
	}
}

// expandMetricGrid builds a metricGrid wrapping each item (expected
// to be a metric).
func expandMetricGrid(b Block) (document.Block, error) {
	children, err := ExpandMany(b.Items)
	if err != nil {
		return document.Block{}, fmt.Errorf("metric_grid items: %w", err)
	}

	return document.Block{
		Type:    document.BlockNodeMetricGrid,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: children,
	}, nil
}

// expandSplitDoc builds the full splitDocumentation tree, wrapping
// Left and Right in their splitDocumentationLeftSide /
// splitDocumentationRightSide nodes.
func expandSplitDoc(b Block) (document.Block, error) {
	left, err := ExpandMany(b.Left)
	if err != nil {
		return document.Block{}, fmt.Errorf("split_doc left: %w", err)
	}

	right, err := ExpandMany(b.Right)
	if err != nil {
		return document.Block{}, fmt.Errorf("split_doc right: %w", err)
	}

	leftNode := document.Block{
		Type:    document.BlockNodeSplitDocLeft,
		Attrs:   uidAttrs(document.GenerateNodeUID()),
		Content: left,
	}

	rightNode := document.Block{
		Type:    document.BlockNodeSplitDocRight,
		Attrs:   uidAttrs(document.GenerateNodeUID()),
		Content: right,
	}

	attrs := uidAttrs(resolveUID(b.UID))
	if inversed := readBoolAttr(b.Attrs, _attrInversed, false); inversed {
		attrs[_attrInversed] = true
	}

	return document.Block{
		Type:    document.BlockNodeSplitDoc,
		Attrs:   attrs,
		Content: []document.Block{leftNode, rightNode},
	}, nil
}

// expandParamList builds the full splitDocumentationParameterList
// tree: a header node followed by one item per Params entry. Each
// item nests an itemHeader (title + type) and a paragraph carrying
// the description.
func expandParamList(b Block) (document.Block, error) {
	children := make([]document.Block, 0, 1+len(b.Params))

	children = append(children, document.Block{
		Type:    document.BlockNodeParamListHeader,
		Attrs:   uidAttrs(document.GenerateNodeUID()),
		Content: rawTextContent(b.Header),
	})

	for _, p := range b.Params {
		titleNode := document.Block{
			Type:    document.BlockNodeParamListItemTitle,
			Attrs:   uidAttrs(document.GenerateNodeUID()),
			Content: rawTextContent(p.Name),
		}

		typeNode := document.Block{
			Type:    document.BlockNodeParamListItemType,
			Attrs:   uidAttrs(document.GenerateNodeUID()),
			Content: rawTextContent(p.Type),
		}

		headerNode := document.Block{
			Type:    document.BlockNodeParamListItemHeader,
			Attrs:   uidAttrs(document.GenerateNodeUID()),
			Content: []document.Block{titleNode, typeNode},
		}

		descNode := document.Block{
			Type:    document.BlockNodeParagraph,
			Attrs:   uidAttrs(document.GenerateNodeUID()),
			Content: ParseInlineMarkdown(p.Description),
		}

		children = append(children, document.Block{
			Type:    document.BlockNodeParamListItem,
			Attrs:   uidAttrs(resolveUID(p.UID)),
			Content: []document.Block{headerNode, descNode},
		})
	}

	return document.Block{
		Type:    document.BlockNodeParamList,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: children,
	}, nil
}

// rawTextContent returns the inline content slice for a node that
// stores raw text (code, mermaid, parameter title/type, code block
// title, paragraph-list header). Empty text returns nil so the
// resulting node has no content rather than an empty text node.
func rawTextContent(text string) []document.Block {
	if text == "" {
		return nil
	}

	return []document.Block{{Type: document.BlockNodeText, Text: text}}
}

// readStringAttr returns attrs[key] when it is a string, otherwise
// def. Used by expanders to read typed canonical attrs without
// repeatedly type-asserting.
func readStringAttr(attrs map[string]any, key, def string) string {
	if attrs == nil {
		return def
	}

	if v, ok := attrs[key].(string); ok {
		return v
	}

	return def
}

// readIntAttr returns attrs[key] coerced to int. Supports the
// numeric types that json.Unmarshal can land in (float64, int,
// int64). Falls back to def on missing/wrong type.
func readIntAttr(attrs map[string]any, key string, def int) int {
	if attrs == nil {
		return def
	}

	switch v := attrs[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}

	return def
}

// readBoolAttr returns attrs[key] as a bool, falling back to def.
func readBoolAttr(attrs map[string]any, key string, def bool) bool {
	if attrs == nil {
		return def
	}

	if v, ok := attrs[key].(bool); ok {
		return v
	}

	return def
}

// copyAttrs returns a shallow copy of attrs, or an empty map when
// attrs is nil. Used to pass through opaque metric configuration
// without mutating the caller's map.
func copyAttrs(attrs map[string]any) map[string]any {
	out := make(map[string]any, len(attrs)+1)

	maps.Copy(out, attrs)

	return out
}

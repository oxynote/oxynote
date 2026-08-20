package block

import (
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
)

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
		return expandHeading(b), nil
	case BlockBlockquote:
		return expandBlockquote(b)
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

	return document.Block{}, fmt.Errorf("unknown block type %q", b.Type)
}

// expandMany expands a slice of canonical Blocks into a slice of
// document.Blocks, short-circuiting on the first error.
func expandMany(blocks []Block) ([]document.Block, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	out := make([]document.Block, 0, len(blocks))

	for i, b := range blocks {
		expanded, err := Expand(b)
		if err != nil {
			return nil, fmt.Errorf("expanding block %d (%s): %w", i, b.Type, err)
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

	return strutil.NanoID()
}

// uidAttrs returns the standard {uid: …} attribute map.
func uidAttrs(uid string) document.Attributes {
	return document.Attributes{document.AttrUID: uid}
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
func expandHeading(b Block) document.Block {
	level := 1
	if a, ok := b.Attrs.Get(document.AttrLevel); ok {
		level = a.Int()
	}

	attrs := uidAttrs(resolveUID(b.UID))
	attrs[document.AttrLevel] = level

	return document.Block{
		Type:    document.BlockNodeHeading,
		Attrs:   attrs,
		Content: ParseInlineMarkdown(b.Text),
	}
}

// expandBlockquote builds a blockquote node. Either Text (single-paragraph
// shorthand) or Items (any combination of paragraphs and lists) supplies the
// content, matching what Validate accepts and what Compact produces.
func expandBlockquote(b Block) (document.Block, error) {
	children, err := textOrItemsContent(b)
	if err != nil {
		return document.Block{}, fmt.Errorf("blockquote items: %w", err)
	}

	return document.Block{
		Type:    document.BlockNodeBlockquote,
		Attrs:   uidAttrs(resolveUID(b.UID)),
		Content: children,
	}, nil
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

		nested, err := expandMany(item.Children)
		if err != nil {
			return document.Block{}, fmt.Errorf("list item %d children: %w", i, err)
		}

		content := append([]document.Block{expanded}, nested...)

		children = append(children, document.Block{
			Type:    document.BlockNodeListItem,
			Attrs:   uidAttrs(strutil.NanoID()),
			Content: content,
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

		nested, err := expandMany(item.Block.Children)
		if err != nil {
			return document.Block{}, fmt.Errorf("task item %d children: %w", i, err)
		}

		content := append([]document.Block{expanded}, nested...)

		attrs := uidAttrs(resolveUID(item.UID))
		attrs[document.AttrChecked] = item.Checked

		children = append(children, document.Block{
			Type:    document.BlockNodeTaskItem,
			Attrs:   attrs,
			Content: content,
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

	attrs[document.AttrIcon] = "lucide:text"
	if a, ok := b.Attrs.Get(document.AttrIcon); ok {
		attrs[document.AttrIcon] = a.String()
	}

	children, err := textOrItemsContent(b)
	if err != nil {
		return document.Block{}, fmt.Errorf("callout items: %w", err)
	}

	return document.Block{
		Type:    document.BlockNodeCalloutBlock,
		Attrs:   attrs,
		Content: children,
	}, nil
}

// textOrItemsContent builds the child slice of a block that accepts
// either the single-paragraph Text shorthand or explicit Items:
// present Items are expanded as-is, otherwise Text becomes one
// paragraph node.
func textOrItemsContent(b Block) ([]document.Block, error) {
	if len(b.Items) > 0 {
		return expandMany(b.Items)
	}

	return []document.Block{{
		Type:    document.BlockNodeParagraph,
		Attrs:   uidAttrs(strutil.NanoID()),
		Content: ParseInlineMarkdown(b.Text),
	}}, nil
}

// expandCode builds a codeBlock. Text is treated as raw code and
// emitted as a single text node (no markdown parsing).
func expandCode(b Block) document.Block {
	attrs := uidAttrs(resolveUID(b.UID))
	if a, ok := b.Attrs.Get(document.AttrLanguage); ok && a.String() != "" {
		attrs[document.AttrLanguage] = a.String()
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
	var title, lang string

	if a, ok := b.Attrs.Get(document.AttrTitle); ok {
		title = a.String()
	}

	if a, ok := b.Attrs.Get(document.AttrLanguage); ok {
		lang = a.String()
	}

	codeAttrs := uidAttrs(strutil.NanoID())
	if lang != "" {
		codeAttrs[document.AttrLanguage] = lang
	}

	titleNode := document.Block{
		Type:    document.BlockNodeCodeBlockTitle,
		Attrs:   uidAttrs(strutil.NanoID()),
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

	copyStringAttrs(attrs, b.Attrs, document.AttrSrc, document.AttrAlt, document.AttrTitle)
	copyPositiveIntAttrs(attrs, b.Attrs, document.AttrWidth)

	return document.Block{
		Type:  document.BlockNodeImageBlock,
		Attrs: attrs,
	}
}

// expandFigma builds a figmaBlock atom node.
func expandFigma(b Block) document.Block {
	attrs := uidAttrs(resolveUID(b.UID))

	copyStringAttrs(attrs, b.Attrs, document.AttrSrc)
	copyPositiveIntAttrs(attrs, b.Attrs, document.AttrWidth, document.AttrHeight)

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
	attrs := b.Attrs.Copy()
	attrs[document.AttrUID] = resolveUID(b.UID)

	return document.Block{
		Type:  document.BlockNodeMetricBlock,
		Attrs: attrs,
	}
}

// expandMetricGrid builds a metricGrid wrapping each item (expected
// to be a metric).
func expandMetricGrid(b Block) (document.Block, error) {
	children, err := expandMany(b.Items)
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
	left, err := expandMany(b.Left)
	if err != nil {
		return document.Block{}, fmt.Errorf("split_doc left: %w", err)
	}

	right, err := expandMany(b.Right)
	if err != nil {
		return document.Block{}, fmt.Errorf("split_doc right: %w", err)
	}

	leftNode := document.Block{
		Type:    document.BlockNodeSplitDocLeft,
		Attrs:   uidAttrs(strutil.NanoID()),
		Content: left,
	}

	rightNode := document.Block{
		Type:    document.BlockNodeSplitDocRight,
		Attrs:   uidAttrs(strutil.NanoID()),
		Content: right,
	}

	attrs := uidAttrs(resolveUID(b.UID))
	if a, ok := b.Attrs.Get(document.AttrInversed); ok && a.Bool() {
		attrs[document.AttrInversed] = true
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
		Attrs:   uidAttrs(strutil.NanoID()),
		Content: rawTextContent(b.Header),
	})

	for _, p := range b.Params {
		titleNode := document.Block{
			Type:    document.BlockNodeParamListItemTitle,
			Attrs:   uidAttrs(strutil.NanoID()),
			Content: rawTextContent(p.Name),
		}

		typeNode := document.Block{
			Type:    document.BlockNodeParamListItemType,
			Attrs:   uidAttrs(strutil.NanoID()),
			Content: rawTextContent(p.Type),
		}

		headerNode := document.Block{
			Type:    document.BlockNodeParamListItemHeader,
			Attrs:   uidAttrs(strutil.NanoID()),
			Content: []document.Block{titleNode, typeNode},
		}

		descNode := document.Block{
			Type:    document.BlockNodeParagraph,
			Attrs:   uidAttrs(strutil.NanoID()),
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

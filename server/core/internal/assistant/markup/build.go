package markup

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
)

// _defaultCalloutIcon is the icon a callout written without one gets,
// as in the editor.
const _defaultCalloutIcon = "lucide:text"

// _maxBlocks caps the blocks one write may hold at the top level, each
// of which is one operation for the realtime service.
const _maxBlocks = 500

// _containers are the elements that only hold blocks, each with the
// node it writes and the one element it holds, if it holds only one.
var _containers = map[string]struct {
	// Type is the node the element writes.
	Type document.BlockNodeType

	// Child is the one element it holds; empty when it holds any block.
	Child string
}{
	_elemBlockquote: {document.BlockNodeBlockquote, ""},
	_elemCallout:    {document.BlockNodeCalloutBlock, ""},
	_elemBullets:    {document.BlockNodeBulletList, _elemEntry},
	_elemOrdered:    {document.BlockNodeOrderedList, _elemEntry},
	_elemTasks:      {document.BlockNodeTaskList, _elemTask},
	_elemMetrics:    {document.BlockNodeMetricGrid, _elemMetric},
}

// _atoms are the elements that hold nothing but attributes: the node
// each writes, its string attributes and its positive integer ones.
var _atoms = map[string]struct {
	// Type is the node the element writes.
	Type document.BlockNodeType

	// Strings are its string attributes.
	Strings []string

	// Ints are its positive integer attributes.
	Ints []string
}{
	_elemImage: {document.BlockNodeImageBlock, []string{document.AttrSrc, document.AttrAlt, document.AttrTitle}, []string{document.AttrWidth}},
	_elemFigma: {document.BlockNodeFigmaBlock, []string{document.AttrSrc}, []string{document.AttrWidth, document.AttrHeight}},
}

// Build turns markup into the blocks it describes, to be written into
// stored. An element whose id stored holds replaces that block: it keeps
// the block's uid and block comment, and is the stored block itself when
// its visible content is unchanged, so what the markup does not show
// survives. Every other element gets a new uid.
func Build(markup string, stored document.RootBlock) ([]document.Block, error) {
	nodes, err := parse(markup)
	if err != nil {
		return nil, err
	}

	b := builder{stored: stored, used: map[string]bool{}}

	blocks, err := b.blocks(nodes, "the markup")
	if err != nil {
		return nil, err
	}

	if len(blocks) == 0 {
		return nil, errors.New("the markup holds no block; to remove one, use delete_block")
	}

	if len(blocks) > _maxBlocks {
		return nil, fmt.Errorf("%d blocks are over the limit of %d per call; send them in several calls", len(blocks), _maxBlocks)
	}

	// an element may name a block that another element in the markup
	// also keeps as a hidden part, such as a list entry's paragraph.
	if uid, found := findDuplicateUID(blocks, map[string]bool{}); found {
		return nil, fmt.Errorf("id %q is kept twice: by its own element and as part of another one; leave it out of one of them", uid)
	}

	return blocks, nil
}

// builder turns parsed elements into blocks.
type builder struct {
	// stored is the document being written into.
	stored document.RootBlock

	// used holds the ids the markup's elements have named, since one id
	// names one block.
	used map[string]bool
}

// blocks builds an element per block. where names the container for
// the message about text between the elements.
func (b *builder) blocks(nodes []node, where string) ([]document.Block, error) {
	elems, err := elements(nodes, where, "an element such as <p>")
	if err != nil {
		return nil, err
	}

	var out []document.Block

	for _, n := range elems {
		stored, err := b.replaced(n)
		if err != nil {
			return nil, err
		}

		blk, err := b.build(n, stored)
		if err != nil {
			return nil, err
		}

		out = append(out, keepIfUnchanged(stored, blk))
	}

	return out, nil
}

// replaced returns the stored block n's id names, or the zero block for
// an element without an id.
func (b *builder) replaced(n node) (document.Block, error) {
	id, ok := n.attrs[_attrID]
	if !ok {
		return document.Block{}, nil
	}

	if b.used[id] {
		return document.Block{}, fmt.Errorf("line %d: id %q is used twice; leave it out of the copy", n.line, id)
	}

	stored, found := b.stored.FindByUID(id)
	if !found {
		return document.Block{}, fmt.Errorf("line %d: id %q is not a block this write can keep; leave id out for a new block", n.line, id)
	}

	b.used[id] = true

	return stored, nil
}

// build turns one element into its block, nested ones included. stored
// is the block it replaces, or the zero block.
func (b *builder) build(n node, stored document.Block) (document.Block, error) {
	if c, ok := _containers[n.name]; ok {
		return b.buildContainer(n, c.Type, c.Child, stored)
	}

	if a, ok := _atoms[n.name]; ok {
		return buildAtom(n, a.Type, a.Strings, a.Ints, stored)
	}

	switch n.name {
	case _elemParagraph, "h1", "h2", "h3":
		return buildText(n, stored)
	case _elemEntry, _elemTask:
		return b.buildEntry(n, stored)
	case _elemCode:
		return buildCode(n, stored), nil
	case _elemMermaid:
		return newBlock(document.BlockNodeMermaidBlock, stored, textNodes([]run{{text: n.text}}, stored.Content)), nil
	case _elemRule:
		return newBlock(document.BlockNodeHorizontalRule, stored, nil), nil
	case _elemMetric:
		return buildMetric(n, stored)
	case _elemSplitDoc:
		return b.buildSplitDoc(n, stored)
	case _elemParams:
		return b.buildParams(n, stored)
	case _elemFile, _elemUnsupported:
		// neither can be written: an uploaded file or a node this model
		// does not know is only ever kept as it is, by its own element.
		_, known := _nodeNames[stored.Type]
		if stored.Type != document.BlockNodeFileBlock && (known || stored.Type == "") {
			return document.Block{}, fmt.Errorf("line %d: <%s> can only keep a block get_document showed as <%s>, by its id", n.line, n.name, n.name)
		}

		return stored, nil
	case _elemLeft, _elemRight:
		return document.Block{}, fmt.Errorf("line %d: <%s> belongs inside <%s>", n.line, n.name, _elemSplitDoc)
	case _elemParam:
		return document.Block{}, fmt.Errorf("line %d: <%s> belongs inside <%s>", n.line, n.name, _elemParams)
	default:
		return document.Block{}, fmt.Errorf("line %d: <%s> is inline; put it inside a <p>", n.line, n.name)
	}
}

// buildText builds a paragraph or a heading. A heading takes plain text,
// its level from the element name.
func buildText(n node, stored document.Block) (document.Block, error) {
	paragraph := n.name == _elemParagraph

	for _, c := range n.children {
		if _, isTag := _tagMarks[c.name]; isTag && !paragraph {
			return document.Block{}, fmt.Errorf("line %d: a heading takes plain text, so <%s> is not allowed in it", c.line, c.name)
		}
	}

	content, err := inline(n.children, stored.Content)
	if err != nil {
		return document.Block{}, err
	}

	if paragraph {
		return newBlock(document.BlockNodeParagraph, stored, content), nil
	}

	blk := newBlock(document.BlockNodeHeading, stored, content)
	blk.Attrs[document.AttrLevel] = int(n.name[1] - '0')

	return blk, nil
}

// buildContainer builds an element holding blocks. child, when set, is
// the one element name the container holds.
func (b *builder) buildContainer(n node, typ document.BlockNodeType, child string, stored document.Block) (document.Block, error) {
	if child != "" {
		for _, c := range n.children {
			if c.name != "" && c.name != child {
				return document.Block{}, fmt.Errorf("line %d: <%s> holds only <%s>, not <%s>", c.line, n.name, child, c.name)
			}
		}
	}

	content, err := b.blocks(n.children, "<"+n.name+">")
	if err != nil {
		return document.Block{}, err
	}

	blk := newBlock(typ, stored, content)

	switch typ {
	case document.BlockNodeCalloutBlock:
		blk.Attrs[document.AttrIcon] = cmp.Or(n.attrs[document.AttrIcon], _defaultCalloutIcon)
	case document.BlockNodeOrderedList:
		if start, err := strconv.Atoi(n.attrs[document.AttrStart]); err == nil && start > 1 {
			blk.Attrs[document.AttrStart] = start
		}
	default:
	}

	return blk, nil
}

// buildEntry builds a list or task entry: the text before its first
// element is the entry's paragraph, and the elements after it are nested
// under it.
func (b *builder) buildEntry(n node, stored document.Block) (document.Block, error) {
	split := len(n.children)
	blank := true

	for i, c := range n.children {
		if _, inlineTag := _tagMarks[c.name]; c.name != "" && !inlineTag {
			split = i

			break
		}

		blank = blank && c.name == "" && strings.TrimSpace(c.text) == ""
	}

	textNodes, nestedNodes := n.children[:split], n.children[split:]

	// models used to HTML often wrap an entry's text in a <p>. A <p>
	// with an id is a block the markup keeps, so it stays nested.
	if blank && split < len(n.children) {
		first := n.children[split]

		if _, hasID := first.attrs[_attrID]; first.name == _elemParagraph && !hasID {
			textNodes, nestedNodes = first.children, n.children[split+1:]
		}
	}

	typ := document.BlockNodeListItem
	if n.name == _elemTask {
		typ = document.BlockNodeTaskItem
	}

	storedParagraph := stored.FirstChild(document.BlockNodeParagraph)

	text, err := inline(textNodes, storedParagraph.Content)
	if err != nil {
		return document.Block{}, err
	}

	nested, err := b.blocks(nestedNodes, "<"+n.name+"> after its text")
	if err != nil {
		return document.Block{}, err
	}

	blk := newBlock(typ, stored, append([]document.Block{newBlock(document.BlockNodeParagraph, storedParagraph, text)}, nested...))

	if typ == document.BlockNodeTaskItem {
		blk.Attrs[document.AttrChecked] = n.attrs[document.AttrChecked] == "true"
	}

	return blk, nil
}

// buildCode builds a pre: a titled code block when it carries a title,
// a code block otherwise.
func buildCode(n node, stored document.Block) document.Block {
	title, titled := n.attrs[document.AttrTitle]

	storedCode := stored
	if titled {
		storedCode = stored.FirstChild(document.BlockNodeCodeBlock)
	}

	codeBlock := newBlock(document.BlockNodeCodeBlock, storedCode, textNodes([]run{{text: n.text}}, storedCode.Content))
	if lang := n.attrs[document.AttrLanguage]; lang != "" {
		codeBlock.Attrs[document.AttrLanguage] = lang
	}

	if !titled {
		return codeBlock
	}

	return newBlock(document.BlockNodeTitledCodeBlock, stored, []document.Block{
		titleBlock(document.BlockNodeCodeBlockTitle, title, stored),
		codeBlock,
	})
}

// buildSplitDoc builds a split documentation block from its left and
// right sides.
func (b *builder) buildSplitDoc(n node, stored document.Block) (document.Block, error) {
	sides, err := elements(n.children, "<split_doc>", "<left> or <right>")
	if err != nil {
		return document.Block{}, err
	}

	if len(sides) != 2 || sides[0].name != _elemLeft || sides[1].name != _elemRight {
		return document.Block{}, fmt.Errorf("line %d: <split_doc> holds a <left> and then a <right>", n.line)
	}

	left, err := b.blocks(sides[0].children, "<left>")
	if err != nil {
		return document.Block{}, err
	}

	right, err := b.blocks(sides[1].children, "<right>")
	if err != nil {
		return document.Block{}, err
	}

	blk := newBlock(document.BlockNodeSplitDoc, stored, []document.Block{
		newBlock(document.BlockNodeSplitDocLeft, stored.FirstChild(document.BlockNodeSplitDocLeft), left),
		newBlock(document.BlockNodeSplitDocRight, stored.FirstChild(document.BlockNodeSplitDocRight), right),
	})

	blk.Attrs[document.AttrInversed] = n.attrs[document.AttrInversed] == "true"

	return blk, nil
}

// buildParams builds a parameter list from its header and its param
// rows.
func (b *builder) buildParams(n node, stored document.Block) (document.Block, error) {
	content := []document.Block{titleBlock(document.BlockNodeParamListHeader, n.attrs[_attrHeader], stored)}

	rows, err := elements(n.children, "<params>", "a <param>")
	if err != nil {
		return document.Block{}, err
	}

	for _, c := range rows {
		if c.name != _elemParam {
			return document.Block{}, fmt.Errorf("line %d: <params> holds only <param>, not <%s>", c.line, c.name)
		}

		storedRow, err := b.replaced(c)
		if err != nil {
			return document.Block{}, err
		}

		row, err := buildParam(c, storedRow)
		if err != nil {
			return document.Block{}, err
		}

		content = append(content, keepIfUnchanged(storedRow, row))
	}

	return newBlock(document.BlockNodeParamList, stored, content), nil
}

// buildParam builds one parameter row: a header holding its name and
// type cells, then its description.
func buildParam(n node, stored document.Block) (document.Block, error) {
	storedHeader := stored.FirstChild(document.BlockNodeParamListItemHeader)
	storedDescription := stored.FirstChild(document.BlockNodeParagraph)

	description, err := inline(n.children, storedDescription.Content)
	if err != nil {
		return document.Block{}, err
	}

	return newBlock(document.BlockNodeParamListItem, stored, []document.Block{
		newBlock(document.BlockNodeParamListItemHeader, storedHeader, []document.Block{
			titleBlock(document.BlockNodeParamListItemTitle, n.attrs[document.AttrName], storedHeader),
			titleBlock(document.BlockNodeParamListItemType, n.attrs[_attrType], storedHeader),
		}),
		newBlock(document.BlockNodeParagraph, storedDescription, description),
	}), nil
}

// buildMetric builds a metric block from its JSON attributes.
func buildMetric(n node, stored document.Block) (document.Block, error) {
	attrs, err := decodeMetricAttrs(n.text)
	if err != nil {
		return document.Block{}, fmt.Errorf("line %d: %w", n.line, err)
	}

	for k := range _hiddenMetricAttrs {
		delete(attrs, k)
	}

	setSimulation(attrs, stored)

	blk := newBlock(document.BlockNodeMetricBlock, stored, nil)
	maps.Copy(blk.Attrs, attrs)

	return blk, nil
}

// buildAtom builds an element that holds nothing but attributes.
func buildAtom(n node, typ document.BlockNodeType, strs, ints []string, stored document.Block) (document.Block, error) {
	blk := newBlock(typ, stored, nil)

	for _, k := range strs {
		if v := n.attrs[k]; v != "" {
			blk.Attrs[k] = v
		}
	}

	for _, k := range ints {
		v, ok := n.attrs[k]
		if !ok {
			continue
		}

		i, err := strconv.Atoi(v)
		if err != nil || i <= 0 {
			return document.Block{}, fmt.Errorf("line %d: <%s> %s has to be a positive whole number", n.line, n.name, k)
		}

		blk.Attrs[k] = i
	}

	return blk, nil
}

// elements returns the elements among nodes. Text between them may only
// be white space; where and hint name the container and what the text
// has to sit in, for the message.
func elements(nodes []node, where, hint string) ([]node, error) {
	var out []node

	for _, n := range nodes {
		if n.name != "" {
			out = append(out, n)

			continue
		}

		if strings.TrimSpace(n.text) != "" {
			return nil, fmt.Errorf("line %d: text in %s has to sit in %s", n.line, where, hint)
		}
	}

	return out, nil
}

// newBlock returns a block of type typ holding content. It keeps the uid
// and block comment of stored, the block it replaces, and gets a new uid
// when stored is the zero block. The markup shows neither the uid of a
// hidden part nor any block comment, so this is what keeps them.
func newBlock(typ document.BlockNodeType, stored document.Block, content []document.Block) document.Block {
	uid, _ := stored.UID()
	if uid == "" {
		uid = strutil.NanoID()
	}

	attrs := document.Attributes{document.AttrUID: uid}

	if comment, ok := stored.Attrs.Value(document.AttrCommentID); ok {
		attrs[document.AttrCommentID] = comment
	}

	return document.Block{Type: typ, Attrs: attrs, Content: content}
}

// titleBlock builds the part of type typ that holds one line of plain
// text, such as a titled pre's title. It keeps the uid, block comment
// and comment marks of the part of that type parent holds.
func titleBlock(typ document.BlockNodeType, text string, parent document.Block) document.Block {
	stored := parent.FirstChild(typ)

	return newBlock(typ, stored, textNodes([]run{{text: text}}, stored.Content))
}

// keepIfUnchanged returns stored when built renders the same, and built
// otherwise. The markup shows everything a writer can change, so the same
// markup means the stored block is still right, hidden parts and all.
func keepIfUnchanged(stored, built document.Block) document.Block {
	if stored.Type == built.Type && Render([]document.Block{stored}) == Render([]document.Block{built}) {
		return stored
	}

	return built
}

// findDuplicateUID returns a uid that blocks and their nested blocks
// hold more than once. seen holds the uids met so far.
func findDuplicateUID(blocks []document.Block, seen map[string]bool) (string, bool) {
	for _, blk := range blocks {
		if uid, ok := blk.UID(); ok {
			if seen[uid] {
				return uid, true
			}

			seen[uid] = true
		}

		if uid, found := findDuplicateUID(blk.Content, seen); found {
			return uid, true
		}
	}

	return "", false
}

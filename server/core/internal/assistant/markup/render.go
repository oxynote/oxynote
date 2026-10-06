package markup

import (
	jsonv2 "encoding/json/v2"
	"strconv"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _indent is one level of nesting in rendered markup.
const _indent = "  "

// _plainContainers names the elements of the nodes that hold blocks and
// carry no attribute but their id.
var _plainContainers = map[document.BlockNodeType]string{
	document.BlockNodeBlockquote: _elemBlockquote,
	document.BlockNodeBulletList: _elemBullets,
	document.BlockNodeTaskList:   _elemTasks,
	document.BlockNodeMetricGrid: _elemMetrics,
}

// _textEscaper escapes the characters that would start markup in text.
var _textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;")

// _attrEscaper escapes the characters that would end or start markup in
// an attribute value.
var _attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;")

// Render returns blocks as markup: one element per block, nested
// elements indented one level deeper than their parent.
func Render(blocks []document.Block) string {
	var r renderer

	for _, b := range blocks {
		r.block(b, 0)
	}

	return r.sb.String()
}

// renderer accumulates rendered markup.
type renderer struct {
	// sb holds the markup written so far.
	sb strings.Builder
}

// block renders one block and everything nested in it.
func (r *renderer) block(b document.Block, depth int) {
	if name, ok := _plainContainers[b.Type]; ok {
		r.container(name, startTag(name, b), b.Content, depth)

		return
	}

	switch b.Type {
	case document.BlockNodeParagraph:
		r.line(depth, startTag(_elemParagraph, b)+renderInline(b.Content)+endTag(_elemParagraph))
	case document.BlockNodeHeading:
		name := "h" + strconv.Itoa(max(1, b.Attrs.Get(document.AttrLevel).Int()))
		r.line(depth, startTag(name, b)+_textEscaper.Replace(b.PlainText())+endTag(name))
	case document.BlockNodeCalloutBlock:
		r.container(_elemCallout, startTag(_elemCallout, b, blockAttr(b, document.AttrIcon)), b.Content, depth)
	case document.BlockNodeOrderedList:
		start := attr{key: document.AttrStart}
		if n := b.Attrs.Get(document.AttrStart).Int(); n > 1 {
			start.value = n
		}

		r.container(_elemOrdered, startTag(_elemOrdered, b, start), b.Content, depth)
	case document.BlockNodeListItem:
		r.entry(_elemEntry, startTag(_elemEntry, b), b, depth)
	case document.BlockNodeTaskItem:
		r.entry(_elemTask, startTag(_elemTask, b, blockAttr(b, document.AttrChecked)), b, depth)
	case document.BlockNodeCodeBlock:
		r.raw(depth, startTag(_elemCode, b, blockAttr(b, document.AttrLanguage)), _elemCode, b.PlainText())
	case document.BlockNodeTitledCodeBlock:
		code := b.FirstChild(document.BlockNodeCodeBlock)

		// an empty title still renders, since a pre without one is plain
		// code, which is another block.
		title := attr{key: document.AttrTitle, value: b.FirstChild(document.BlockNodeCodeBlockTitle).PlainText(), keepEmpty: true}
		r.raw(depth, startTag(_elemCode, b, title, blockAttr(code, document.AttrLanguage)), _elemCode, code.PlainText())
	case document.BlockNodeMermaidBlock:
		r.raw(depth, startTag(_elemMermaid, b), _elemMermaid, b.PlainText())
	case document.BlockNodeHorizontalRule:
		r.line(depth, emptyTag(_elemRule, b))
	case document.BlockNodeImageBlock:
		r.line(depth, emptyTag(_elemImage, b,
			blockAttr(b, document.AttrSrc),
			blockAttr(b, document.AttrAlt),
			blockAttr(b, document.AttrTitle),
			blockAttr(b, document.AttrWidth),
		))
	case document.BlockNodeFigmaBlock:
		r.line(depth, emptyTag(_elemFigma, b,
			blockAttr(b, document.AttrSrc),
			blockAttr(b, document.AttrWidth),
			blockAttr(b, document.AttrHeight),
		))
	case document.BlockNodeFileBlock:
		r.line(depth, emptyTag(_elemFile, b,
			blockAttr(b, document.AttrSrc),
			blockAttr(b, document.AttrName),
			blockAttr(b, document.AttrContentType),
			blockAttr(b, document.AttrSize),
		))
	case document.BlockNodeMetricBlock:
		r.raw(depth, startTag(_elemMetric, b), _elemMetric, metricJSON(b))
	case document.BlockNodeSplitDoc:
		r.container(_elemSplitDoc, startTag(_elemSplitDoc, b, blockAttr(b, document.AttrInversed)), b.Content, depth)
	case document.BlockNodeSplitDocLeft:
		r.container(_elemLeft, "<"+_elemLeft+">", b.Content, depth)
	case document.BlockNodeSplitDocRight:
		r.container(_elemRight, "<"+_elemRight+">", b.Content, depth)
	case document.BlockNodeParamList:
		header := attr{key: _attrHeader, value: b.FirstChild(document.BlockNodeParamListHeader).PlainText()}
		r.container(_elemParams, startTag(_elemParams, b, header), b.Content, depth)
	case document.BlockNodeParamListHeader:
		// its text is the params' header attribute.
	case document.BlockNodeParamListItem:
		r.param(b, depth)
	default:
		r.line(depth, emptyTag(_elemUnsupported, b, attr{key: _attrType, value: string(b.Type)}))
	}
}

// line writes one line of markup at the given depth.
func (r *renderer) line(depth int, s string) {
	r.sb.WriteString(strings.Repeat(_indent, depth))
	r.sb.WriteString(s)
	r.sb.WriteByte('\n')
}

// container writes an element whose content is blocks, each on its
// own lines one level deeper.
func (r *renderer) container(name, openTag string, children []document.Block, depth int) {
	r.line(depth, openTag)

	for _, c := range children {
		r.block(c, depth+1)
	}

	r.line(depth, endTag(name))
}

// entry writes a list or task entry: the text of its leading paragraph,
// then whatever is nested under it.
func (r *renderer) entry(name, openTag string, b document.Block, depth int) {
	children := b.Content

	var text string

	if len(children) > 0 && children[0].Type == document.BlockNodeParagraph {
		text = renderInline(children[0].Content)
		children = children[1:]
	}

	if len(children) == 0 {
		r.line(depth, openTag+text+endTag(name))

		return
	}

	r.container(name, openTag+text, children, depth)
}

// raw writes an element whose content is raw text, kept as it is but
// for its own closing tag, which would end it early.
func (r *renderer) raw(depth int, openTag, name, text string) {
	// parsing drops one line break after the opening tag and one before
	// the closing tag, so text that starts or ends with one gets another.
	if strings.HasPrefix(text, "\n") {
		text = "\n" + text
	}

	if strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	r.line(depth, openTag+escapeRaw(text, name)+endTag(name))
}

// param writes one parameter row with its name, type and description.
func (r *renderer) param(row document.Block, depth int) {
	header := row.FirstChild(document.BlockNodeParamListItemHeader)
	name := attr{key: document.AttrName, value: header.FirstChild(document.BlockNodeParamListItemTitle).PlainText()}
	typ := attr{key: _attrType, value: header.FirstChild(document.BlockNodeParamListItemType).PlainText()}
	description := row.FirstChild(document.BlockNodeParagraph).Content

	r.line(depth, startTag(_elemParam, row, name, typ)+renderInline(description)+endTag(_elemParam))
}

// metricJSON returns a metric's attributes as JSON, leaving out the
// hidden ones and those without a value.
func metricJSON(b document.Block) string {
	attrs := make(map[string]any, len(b.Attrs))

	for k, v := range b.Attrs {
		if v != nil && !_hiddenMetricAttrs[k] {
			attrs[k] = v
		}
	}

	// keys are sorted so that the same attributes always render the
	// same. json/v2 leaves the < and & of queries unescaped.
	out, err := jsonv2.Marshal(attrs, jsonv2.Deterministic(true))
	if err != nil {
		// NOCOV: attributes decoded from JSON always encode back.
		return "{}"
	}

	return string(out)
}

// attr is one attribute of a rendered element.
type attr struct {
	// key is the attribute name.
	key string

	// value is the attribute value: a string, a bool or a number.
	value any

	// keepEmpty indicates that the attribute renders even when empty.
	keepEmpty bool
}

// text returns the value as written: a string as it is, a bool only
// when true, a number only when positive. Any other value is "".
func (a attr) text() string {
	switch v := a.value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
	case int:
		if v > 0 {
			return strconv.Itoa(v)
		}
	case float64:
		if n := int(v); n > 0 {
			return strconv.Itoa(n)
		}
	}

	return ""
}

// blockAttr returns b's attribute key as a rendered attribute.
func blockAttr(b document.Block, key string) attr {
	return attr{key: key, value: b.Attrs[key]}
}

// startTag returns the start tag of the element for b, its id first.
func startTag(name string, b document.Block, attrs ...attr) string {
	return "<" + name + renderAttrs(b, attrs) + ">"
}

// emptyTag returns the tag of an element for b that holds nothing.
func emptyTag(name string, b document.Block, attrs ...attr) string {
	return "<" + name + renderAttrs(b, attrs) + "/>"
}

// endTag returns the end tag of the named element.
func endTag(name string) string {
	return "</" + name + ">"
}

// renderAttrs renders b's id and the given attributes. One without a
// value is left out unless it is kept.
func renderAttrs(b document.Block, attrs []attr) string {
	var sb strings.Builder

	if uid, _ := b.UID(); uid != "" {
		sb.WriteString(` id="` + _attrEscaper.Replace(uid) + `"`)
	}

	for _, a := range attrs {
		if text := a.text(); text != "" || a.keepEmpty {
			sb.WriteString(" " + a.key + `="` + _attrEscaper.Replace(text) + `"`)
		}
	}

	return sb.String()
}

// escapeRaw escapes the one sequence raw text cannot hold as is: its
// element's closing tag, in any case. A closing tag the text already
// holds escaped gets one more amp; so that reading it back undoes
// exactly one level.
func escapeRaw(s, name string) string {
	s = _rawElements[name].ReplaceAllString(s, "&amp;${1}lt;/${2}")

	return _rawCloseTags[name].ReplaceAllString(s, "&lt;/${1}")
}

package markup

import (
	"fmt"
	"strings"
)

// _knownTags are the element names the parser reads as tags. Anything
// else that looks like a tag is literal text, so a stray < in prose
// never fails a write.
var _knownTags = map[string]bool{
	_elemParagraph: true, "h1": true, "h2": true, "h3": true,
	_elemBlockquote: true, _elemBullets: true, _elemOrdered: true, _elemEntry: true,
	_elemTasks: true, _elemTask: true, _elemCallout: true, _elemCode: true,
	_elemMermaid: true, _elemRule: true, _elemImage: true, _elemFigma: true,
	_elemFile: true, _elemMetrics: true, _elemMetric: true, _elemSplitDoc: true,
	_elemLeft: true, _elemRight: true, _elemParams: true, _elemParam: true,
	_elemUnsupported: true,
	_tagBold:         true, _tagItalic: true, _tagUnderline: true, _tagStrike: true,
	_tagCode: true, _tagLink: true,
}

// _tagAliases maps the HTML spellings models often write to the tag
// they mean.
var _tagAliases = map[string]string{
	"strong": _tagBold,
	"em":     _tagItalic,
	"del":    _tagStrike,
}

// _entityDecoder decodes the character references text and attribute
// values may hold. Any other & stays literal.
var _entityDecoder = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'")

// _nameBytes are the bytes element and attribute names are made of.
const _nameBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

// _maxMarkupLength caps the markup one write may send, in bytes.
const _maxMarkupLength = 1 << 20

// _maxDepth caps how deep elements nest, tags included. Building walks
// the tree recursively, so an unbounded depth could exhaust the stack.
const _maxDepth = 64

// node is one piece of parsed markup: an element, or a run of text when
// name is empty.
type node struct {
	// name is the element name; empty for text.
	name string

	// attrs holds the element's attributes, decoded.
	attrs map[string]string

	// children holds an element's content.
	children []node

	// text is a text run, or a raw element's content.
	text string

	// line is the line the node starts on, for messages.
	line int
}

// tag is an opening tag.
type tag struct {
	// name is the element name.
	name string

	// attrs holds the tag's attributes, decoded.
	attrs map[string]string

	// end is the position after the tag.
	end int

	// selfClosing indicates that the tag closes itself, as in <hr/>.
	selfClosing bool
}

// parse reads markup into its top-level nodes.
func parse(s string) ([]node, error) {
	if len(s) > _maxMarkupLength {
		return nil, fmt.Errorf("the markup is %d bytes, over the limit of %d; send it in several calls", len(s), _maxMarkupLength)
	}

	p := parser{s: s}

	return p.nodes("", 0)
}

// parser walks markup once, front to back.
type parser struct {
	// s is the markup.
	s string

	// i is the position of the next unread byte.
	i int

	// counted is the position line has counted newlines up to, and
	// lines the newlines before it. p.i only moves forward, so counting
	// on from there keeps the parse linear.
	counted, lines int

	// depth is the number of elements open at p.i.
	depth int
}

// nodes reads nodes up to the closing tag of the element named
// closeName, opened on line opened, or to the end when closeName is
// empty.
func (p *parser) nodes(closeName string, opened int) ([]node, error) {
	var out []node

	for p.i < len(p.s) {
		if name, end, ok := p.closeTag(); ok {
			if name != closeName {
				expected := "nothing"
				if closeName != "" {
					expected = endTag(closeName)
				}

				return nil, fmt.Errorf("line %d: </%s> closes nothing open; expected %s", p.line(), name, expected)
			}

			p.i = end

			return out, nil
		}

		if t, ok := p.openTag(); ok {
			el, err := p.element(t)
			if err != nil {
				return nil, err
			}

			out = append(out, el)

			continue
		}

		out = append(out, p.text())
	}

	if closeName != "" {
		return nil, fmt.Errorf("line %d: <%s> is never closed", opened, closeName)
	}

	return out, nil
}

// element reads the element whose opening tag t starts at p.i.
func (p *parser) element(t tag) (node, error) {
	el := node{name: t.name, attrs: t.attrs, line: p.line()}
	p.i = t.end

	if t.selfClosing {
		return el, nil
	}

	if _, raw := _rawElements[t.name]; raw {
		start := p.i

		for {
			k := strings.Index(p.s[p.i:], "</")
			if k < 0 {
				return node{}, fmt.Errorf("line %d: <%s> is never closed", el.line, t.name)
			}

			p.i += k

			if name, end, ok := p.closeTag(); ok && name == t.name {
				// a line break right after the opening tag or right
				// before the closing one is layout, as after <pre> in
				// HTML.
				el.text = strings.TrimSuffix(strings.TrimPrefix(unescapeRaw(p.s[start:p.i], t.name), "\n"), "\n")
				p.i = end

				return el, nil
			}

			p.i += len("</")
		}
	}

	if p.depth == _maxDepth {
		return node{}, fmt.Errorf("line %d: elements nest more than %d deep", el.line, _maxDepth)
	}

	p.depth++

	children, err := p.nodes(t.name, el.line)
	if err != nil {
		return node{}, err
	}

	p.depth--
	el.children = children

	return el, nil
}

// text reads text up to the next known tag, decoding entities. A < that
// starts no known tag is part of the text.
func (p *parser) text() node {
	start, line := p.i, p.line()

	// the first byte is text: nodes calls this only where no known tag
	// starts.
	p.i++

	for {
		k := strings.IndexByte(p.s[p.i:], '<')
		if k < 0 {
			p.i = len(p.s)

			break
		}

		p.i += k

		if _, _, ok := p.closeTag(); ok {
			break
		}

		if _, ok := p.openTag(); ok {
			break
		}

		p.i++
	}

	return node{text: _entityDecoder.Replace(p.s[start:p.i]), line: line}
}

// openTag reads the opening tag of a known element at p.i without moving
// past it. ok is false when none starts there.
func (p *parser) openTag() (tag, bool) {
	if p.s[p.i] != '<' {
		return tag{}, false
	}

	name, i := p.tagName(p.i + len("<"))
	if !_knownTags[name] {
		return tag{}, false
	}

	t := tag{name: name, attrs: map[string]string{}}

	for {
		i = p.skipSpace(i)

		switch {
		case i >= len(p.s):
			return tag{}, false
		case p.s[i] == '>':
			t.end = i + len(">")

			return t, true
		case strings.HasPrefix(p.s[i:], "/>"):
			t.end = i + len("/>")
			t.selfClosing = true

			return t, true
		}

		key, value, next, ok := p.attribute(i)
		if !ok {
			return tag{}, false
		}

		t.attrs[key] = value
		i = next
	}
}

// attribute reads the attribute at i: its key, its decoded value and the
// position after it. A bare attribute, such as checked, is true.
func (p *parser) attribute(i int) (string, string, int, bool) {
	start := i
	i = p.nameEnd(i)

	if i == start {
		return "", "", 0, false
	}

	key := p.s[start:i]

	if i >= len(p.s) || p.s[i] != '=' {
		return key, "true", i, true
	}

	i++

	if i >= len(p.s) || (p.s[i] != '"' && p.s[i] != '\'') {
		return "", "", 0, false
	}

	k := strings.IndexByte(p.s[i+1:], p.s[i])
	if k < 0 {
		return "", "", 0, false
	}

	// the value, and the quotes either side of it.
	return key, _entityDecoder.Replace(p.s[i+1 : i+1+k]), i + k + len(`""`), true
}

// closeTag reads the closing tag of a known element at p.i without
// moving past it: its name and the position after it.
func (p *parser) closeTag() (string, int, bool) {
	if !strings.HasPrefix(p.s[p.i:], "</") {
		return "", 0, false
	}

	name, i := p.tagName(p.i + len("</"))
	i = p.skipSpace(i)

	if !_knownTags[name] || i >= len(p.s) || p.s[i] != '>' {
		return "", 0, false
	}

	return name, i + len(">"), true
}

// tagName reads the element name at i, lowercased and with any alias
// resolved, and returns it with the position after it.
func (p *parser) tagName(i int) (string, int) {
	end := p.nameEnd(i)

	name := strings.ToLower(p.s[i:end])
	if alias, ok := _tagAliases[name]; ok {
		name = alias
	}

	return name, end
}

// nameEnd returns the position after the element or attribute name at
// i.
func (p *parser) nameEnd(i int) int {
	return len(p.s) - len(strings.TrimLeft(p.s[i:], _nameBytes))
}

// skipSpace returns the position of the first byte from i on that is
// not white space.
func (p *parser) skipSpace(i int) int {
	return len(p.s) - len(strings.TrimLeft(p.s[i:], " \t\n\r"))
}

// line returns the line of p.i, counting from 1.
func (p *parser) line() int {
	p.lines += strings.Count(p.s[p.counted:p.i], "\n")
	p.counted = p.i

	return p.lines + 1
}

// unescapeRaw undoes escapeRaw: an escaped closing tag reads as the tag,
// and one with more amp; loses one.
func unescapeRaw(s, name string) string {
	return _rawElements[name].ReplaceAllStringFunc(s, func(m string) string {
		if tagName, ok := strings.CutPrefix(m, "&lt;/"); ok {
			return "</" + tagName
		}

		return strings.Replace(m, "amp;", "", 1)
	})
}

package block

import (
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Inline mark types used in canonical-markdown output.
const (
	_markBold      = "bold"
	_markItalic    = "italic"
	_markUnderline = "underline"
	_markStrike    = "strike"
	_markCode      = "code"
	_markLink      = "link"
)

// _escapeGrowHeadroom is the extra builder capacity reserved for the
// backslashes escaping inserts.
const _escapeGrowHeadroom = 4

// _attrHref is the attribute key carrying a link mark's target URL.
const _attrHref = "href"

// ParseInlineMarkdown converts a minimal-markdown string into a slice
// of ProseMirror text nodes with marks. The supported syntax is:
//
//	**bold**       → mark bold
//	*italic*       → mark italic
//	_underline_    → mark underline
//	~~strike~~     → mark strike
//	`code`         → mark code (literal; no inner parsing)
//	[label](url)   → mark link with href=url; label is recursively parsed
//	\X             → literal X (backslash escape)
//
// Unmatched delimiters are emitted as literal text. Newlines inside
// the input are preserved verbatim; the canonical block model treats
// multi-paragraph content as separate paragraph blocks, so a single
// block's text should not normally contain newlines.
//
// An empty input returns a nil slice (a valid representation of an
// empty paragraph).
func ParseInlineMarkdown(input string) []document.Block {
	if input == "" {
		return nil
	}

	atoms := parseSpan(input, nil)

	return atomsToTextNodes(atoms)
}

// emitInlineMarkdown is the inverse of ParseInlineMarkdown: it
// renders a slice of ProseMirror inline text nodes back into the
// canonical-markdown subset. Round-tripping parse(emit(x)) is
// idempotent at the level of (text, mark-set) atoms — text nodes
// with the same mark-set are merged on emit.
func emitInlineMarkdown(nodes []document.Block) string {
	var (
		buf    strings.Builder
		active []activeMark
	)

	for _, n := range nodes {
		if n.Type != document.BlockNodeText {
			continue
		}

		desired := extractMarks(n.Marks)

		// Close marks that are no longer desired, in reverse open
		// order, until the active stack is a prefix of desired.
		for len(active) > 0 {
			top := active[len(active)-1]
			if isMarkPrefix(active, desired) && containsMark(desired, top) {
				break
			}

			buf.WriteString(closeDelim(top))

			active = active[:len(active)-1]
		}

		// Open the marks present in desired but not yet active. We
		// open them in the order they appear in desired so the
		// nesting matches the parse output.
		for _, m := range desired {
			if !containsMark(active, m) {
				buf.WriteString(openDelim(m))
				active = append(active, m)
			}
		}

		buf.WriteString(escapeMarkdown(n.Text, active))
	}

	// Close any marks still open at the end.
	for _, v := range slices.Backward(active) {
		buf.WriteString(closeDelim(v))
	}

	return buf.String()
}

// activeMark is one mark in the open-stack used during emit.
type activeMark struct {
	// kind is the mark type ("bold", "italic", "link", …).
	kind string

	// href is set when kind == _markLink.
	href string
}

// inlineAtom is a maximal run of text sharing the same ordered set
// of marks. The parser produces a stream of atoms which are then
// merged into ProseMirror text nodes.
type inlineAtom struct {
	// text is the literal run.
	text string

	// marks is the ordered set of marks active over text. The order
	// matches the open order so emit can re-derive the same
	// nesting.
	marks []activeMark
}

// parseSpan parses the entire input string with the given outer
// mark stack already active. It is called recursively for the
// contents of bold/italic/underline/strike spans and link labels.
func parseSpan(s string, outer []activeMark) []inlineAtom { //nolint:gocognit // this function is complex, however, it's well-structured
	var out []inlineAtom

	cur := strings.Builder{}
	flush := func() {
		if cur.Len() == 0 {
			return
		}

		out = append(out, inlineAtom{
			text:  cur.String(),
			marks: append([]activeMark(nil), outer...),
		})
		cur.Reset()
	}

	i := 0
	for i < len(s) {
		c := s[i]

		// Backslash escape: consume the next byte as literal.
		if c == '\\' && i+1 < len(s) {
			cur.WriteByte(s[i+1])
			i += 2

			continue
		}

		// Code span: literal until matching backtick. No parsing
		// inside.
		if c == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				flush()

				out = append(out, inlineAtom{
					text:  s[i+1 : i+1+end],
					marks: append(append([]activeMark(nil), outer...), activeMark{kind: _markCode}),
				})
				i = i + 1 + end + 1

				continue
			}
		}

		// Link: [label](url). The label is recursively parsed; the
		// url is taken literally (no inline parsing inside).
		if c == '[' {
			if label, href, end, ok := scanLink(s, i); ok {
				flush()

				linkMark := activeMark{kind: _markLink, href: href}
				inner := parseSpan(label, append(append([]activeMark(nil), outer...), linkMark))
				out = append(out, inner...)
				i = end

				continue
			}
		}

		// Two-character mark delimiters: ** and ~~. Check before
		// the single-character ones so "**" is not consumed as two
		// italics. If no close is found, consume both bytes as
		// literal so we don't fall through to the single-char
		// branch and treat ** as ***italic*italic*.
		if i+1 < len(s) { //nolint:nestif // the branching is sequential and readable
			pair := s[i : i+2]

			if pair == "**" || pair == "~~" {
				mark := _markBold
				if pair == "~~" {
					mark = _markStrike
				}

				if end := scanDelimClose(s, i+len(pair), pair); end >= 0 {
					flush()

					inner := parseSpan(s[i+len(pair):end], append(append([]activeMark(nil), outer...), activeMark{kind: mark}))
					out = append(out, inner...)
					i = end + len(pair)

					continue
				}

				cur.WriteString(pair)

				i += 2

				continue
			}
		}

		// Single-character mark delimiters: * and _.
		if c == '*' || c == '_' {
			mark := _markItalic
			if c == '_' {
				mark = _markUnderline
			}

			if end := scanDelimClose(s, i+1, string(c)); end >= 0 {
				flush()

				inner := parseSpan(s[i+1:end], append(append([]activeMark(nil), outer...), activeMark{kind: mark}))
				out = append(out, inner...)
				i = end + 1

				continue
			}
		}

		cur.WriteByte(c)

		i++
	}

	flush()

	return out
}

// scanLink tries to parse a markdown link starting at s[i] == '['.
// It returns the label (raw, still containing markdown), the href,
// the byte offset just after the closing ')', and ok=true on a
// successful match. On any malformed shape it returns ok=false and
// the caller treats the '[' as literal.
func scanLink(s string, i int) (label, href string, end int, ok bool) {
	// Find the matching ']'. Allow escaped brackets inside the
	// label and skip over backtick code spans so a `]` inside code
	// doesn't end the label.
	j := i + 1

	for j < len(s) {
		switch s[j] {
		case '\\':
			j += 2

			continue
		case '`':
			if k := strings.IndexByte(s[j+1:], '`'); k >= 0 {
				j = j + 1 + k + 1

				continue
			}

			return "", "", 0, false
		case ']':
			if j+1 < len(s) && s[j+1] == '(' {
				end := strings.IndexByte(s[j+2:], ')')
				if end < 0 {
					return "", "", 0, false
				}

				return s[i+1 : j], s[j+2 : j+2+end], j + 2 + end + 1, true
			}

			return "", "", 0, false
		}

		j++
	}

	return "", "", 0, false
}

// scanDelimClose returns the byte offset of the closing delimiter
// matching delim starting from byte position start, or -1 if no
// closing delimiter is found. It skips over escapes, code spans,
// and link labels so a delimiter inside those does not end the span.
func scanDelimClose(s string, start int, delim string) int {
	i := start

	for i < len(s) {
		c := s[i]

		if c == '\\' && i+1 < len(s) {
			i += 2

			continue
		}

		if c == '`' {
			if k := strings.IndexByte(s[i+1:], '`'); k >= 0 {
				i = i + 1 + k + 1

				continue
			}

			i++

			continue
		}

		if c == '[' {
			if _, _, end, ok := scanLink(s, i); ok {
				i = end

				continue
			}
		}

		if strings.HasPrefix(s[i:], delim) {
			return i
		}

		i++
	}

	return -1
}

// atomsToTextNodes merges adjacent atoms with identical mark sets
// and emits ProseMirror text nodes.
func atomsToTextNodes(atoms []inlineAtom) []document.Block {
	if len(atoms) == 0 {
		return nil
	}

	out := make([]document.Block, 0, len(atoms))

	var (
		buf      strings.Builder
		curMarks []activeMark
		haveOpen bool
	)

	flush := func() {
		if !haveOpen || buf.Len() == 0 {
			buf.Reset()

			return
		}

		out = append(out, document.Block{
			Type:  document.BlockNodeText,
			Text:  buf.String(),
			Marks: marksToDocument(curMarks),
		})
		buf.Reset()
	}

	for _, a := range atoms {
		if a.text == "" {
			continue
		}

		if !haveOpen || !marksEqual(curMarks, a.marks) {
			flush()

			curMarks = a.marks
			haveOpen = true
		}

		buf.WriteString(a.text)
	}

	flush()

	return out
}

// marksEqual reports whether two mark stacks describe the same set
// of active marks. Order matters because nested marks emit in open
// order.
func marksEqual(a, b []activeMark) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].kind != b[i].kind || a[i].href != b[i].href {
			return false
		}
	}

	return true
}

// marksToDocument converts the parser's internal mark stack into
// document.Mark values suitable for a ProseMirror text node.
func marksToDocument(marks []activeMark) []document.Mark {
	if len(marks) == 0 {
		return nil
	}

	out := make([]document.Mark, 0, len(marks))

	for _, m := range marks {
		dm := document.Mark{Type: m.kind}
		if m.kind == _markLink {
			dm.Attrs = document.Attributes{_attrHref: m.href}
		}

		out = append(out, dm)
	}

	return out
}

// extractMarks converts a document.Mark slice into the parser's
// internal mark stack. Marks of unknown types are dropped.
func extractMarks(marks []document.Mark) []activeMark {
	if len(marks) == 0 {
		return nil
	}

	out := make([]activeMark, 0, len(marks))

	for _, m := range marks {
		switch m.Type {
		case _markBold, _markItalic, _markUnderline, _markStrike, _markCode:
			out = append(out, activeMark{kind: m.Type})
		case _markLink:
			var href string

			if a, ok := m.Attrs.Get(_attrHref); ok {
				href = a.String()
			}

			out = append(out, activeMark{kind: _markLink, href: href})
		}
	}

	return out
}

// containsMark reports whether the stack contains a mark with the
// same kind (and, for link, the same href) as m.
func containsMark(stack []activeMark, m activeMark) bool {
	for _, s := range stack {
		if s.kind == m.kind && s.href == m.href {
			return true
		}
	}

	return false
}

// isMarkPrefix reports whether prefix is an order-preserving prefix
// of full.
func isMarkPrefix(prefix, full []activeMark) bool {
	if len(prefix) > len(full) {
		return false
	}

	for i, p := range prefix {
		if p.kind != full[i].kind || p.href != full[i].href {
			return false
		}
	}

	return true
}

// openDelim returns the markdown opening delimiter for a mark.
func openDelim(m activeMark) string {
	switch m.kind {
	case _markBold:
		return "**"
	case _markItalic:
		return "*"
	case _markUnderline:
		return "_"
	case _markStrike:
		return "~~"
	case _markCode:
		return "`"
	case _markLink:
		return "["
	}

	return ""
}

// closeDelim returns the markdown closing delimiter for a mark.
// Most marks use the same delimiter for open and close; link uses
// "](url)".
func closeDelim(m activeMark) string {
	switch m.kind {
	case _markBold:
		return "**"
	case _markItalic:
		return "*"
	case _markUnderline:
		return "_"
	case _markStrike:
		return "~~"
	case _markCode:
		return "`"
	case _markLink:
		return "](" + m.href + ")"
	}

	return ""
}

// escapeMarkdown returns text with markdown metacharacters escaped
// for emission. Inside a code span, no escaping is performed because
// code content is literal.
func escapeMarkdown(text string, active []activeMark) string {
	for _, m := range active {
		if m.kind == _markCode {
			return text
		}
	}

	if !strings.ContainsAny(text, "\\*_~`[") {
		return text
	}

	var b strings.Builder

	b.Grow(len(text) + _escapeGrowHeadroom)

	for i := range len(text) {
		c := text[i]
		switch c {
		case '\\', '*', '_', '~', '`', '[':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

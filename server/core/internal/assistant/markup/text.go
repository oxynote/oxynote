package markup

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _lineBreak matches a line break with the indentation around it, which
// inline text reads as one space, as HTML does.
var _lineBreak = regexp.MustCompile(`[ \t]*\r?\n[ \t\r\n]*`)

// _leadingBreak and _trailingBreak match a line break at the start or
// end of a block's text, which is the markup's layout, not text.
var (
	_leadingBreak  = regexp.MustCompile(`^[ \t]*\r?\n[ \t\r\n]*`)
	_trailingBreak = regexp.MustCompile(`\r?\n[ \t\r\n]*$`)
)

// _linkScheme matches the scheme a link starts with.
var _linkScheme = regexp.MustCompile(`^([a-z][a-z0-9+.-]*):`)

// _unsafeLinkSchemes are the schemes a written link may not use: the
// editor opens a link as it is, so these would run code or open a local
// file on a click. Any other scheme is a link someone may have stored.
var _unsafeLinkSchemes = map[string]bool{"javascript": true, "vbscript": true, "data": true, "file": true}

// run is a stretch of text sharing its visible marks.
type run struct {
	// text is the stretch's text.
	text string

	// marks are its visible marks, in nesting order.
	marks []document.Mark
}

// char is one rune of text with its marks.
type char struct {
	// r is the rune.
	r rune

	// marks are the rune's marks.
	marks []document.Mark
}

// inline builds the text nodes of a block from text and inline tags.
// stored is the content of the block being replaced: its hidden marks,
// such as comments, carry over to the text this edit leaves unchanged.
func inline(nodes []node, stored []document.Block) ([]document.Block, error) {
	runs, err := textRuns(nodes, nil)
	if err != nil {
		return nil, err
	}

	return textNodes(collapseWhiteSpace(runs), stored), nil
}

// textRuns returns the text under nodes as runs, each with the marks of
// the tags around it on top of marks.
func textRuns(nodes []node, marks []document.Mark) ([]run, error) {
	var runs []run

	for _, n := range nodes {
		if n.name == "" {
			runs = append(runs, run{text: n.text, marks: marks})

			continue
		}

		markType, inlineTag := _tagMarks[n.name]
		if !inlineTag {
			return nil, fmt.Errorf("line %d: <%s> is a block; it cannot sit inside text", n.line, n.name)
		}

		m := document.Mark{Type: markType}

		if n.name == _tagLink {
			if scheme := linkScheme(n.attrs[_attrHref]); _unsafeLinkSchemes[scheme] {
				return nil, fmt.Errorf("line %d: a link cannot use %s:; link to a web address instead", n.line, scheme)
			}

			m.Attrs = document.Attributes{_attrHref: n.attrs[_attrHref]}
		}

		// a tag inside the same tag adds nothing: a node holds a mark
		// type once.
		inner := slices.DeleteFunc(slices.Clone(marks), func(o document.Mark) bool { return o.Type == markType })
		inner = append(inner, m)

		// the same formatting has to come out as the same mark list,
		// whatever order the tags were written in.
		slices.SortStableFunc(inner, func(a, b document.Mark) int { return tagOrder(a.Type) - tagOrder(b.Type) })

		innerRuns, err := textRuns(n.children, inner)
		if err != nil {
			return nil, err
		}

		runs = append(runs, innerRuns...)
	}

	return runs, nil
}

// linkScheme returns the lowercase scheme href starts with, or "" for a
// path. Browsers ignore white space and control characters in a scheme,
// so they are dropped before reading it.
func linkScheme(href string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r <= ' ' {
			return -1
		}

		return r
	}, strings.ToLower(href))

	if m := _linkScheme.FindStringSubmatch(cleaned); m != nil {
		return m[1]
	}

	return ""
}

// collapseWhiteSpace drops a line break at either end of the text and
// reads one inside it, with the indentation around it, as one space.
// Spaces before a closing line break are text and stay, like any other
// spaces on the text's own line.
func collapseWhiteSpace(runs []run) []run {
	if len(runs) == 0 {
		return runs
	}

	runs[0].text = _leadingBreak.ReplaceAllString(runs[0].text, "")
	runs[len(runs)-1].text = _trailingBreak.ReplaceAllString(runs[len(runs)-1].text, "")

	for i := range runs {
		runs[i].text = _lineBreak.ReplaceAllString(runs[i].text, " ")
	}

	return runs
}

// textNodes turns runs into text nodes. The hidden marks of stored are
// laid over the text this edit kept: the start and the end the old text
// and the new have in common.
func textNodes(runs []run, stored []document.Block) []document.Block {
	var next, old []char

	for _, r := range runs {
		for _, c := range r.text {
			next = append(next, char{c, r.marks})
		}
	}

	for _, n := range stored {
		if n.Type != document.BlockNodeText {
			continue
		}

		var hidden []document.Mark

		for _, m := range n.Marks {
			if tagOrder(m.Type) < 0 {
				hidden = append(hidden, m)
			}
		}

		for _, c := range n.Text {
			old = append(old, char{c, hidden})
		}
	}

	// a rune with no hidden marks shares its run's mark list, so only
	// the few that carry hidden marks get a list of their own.
	start := 0
	for start < len(next) && start < len(old) && next[start].r == old[start].r {
		if len(old[start].marks) > 0 {
			next[start].marks = append(slices.Clone(next[start].marks), old[start].marks...)
		}

		start++
	}

	for end := 1; end <= len(next)-start && end <= len(old)-start && next[len(next)-end].r == old[len(old)-end].r; end++ {
		if hidden := old[len(old)-end].marks; len(hidden) > 0 {
			at := len(next) - end
			next[at].marks = append(slices.Clone(next[at].marks), hidden...)
		}
	}

	var (
		out  []document.Block
		text []rune
	)

	for i, c := range next {
		text = append(text, c.r)

		if i+1 < len(next) && slices.EqualFunc(c.marks, next[i+1].marks, document.Mark.Equal) {
			continue
		}

		out = append(out, document.Block{Type: document.BlockNodeText, Text: string(text), Marks: c.marks})
		text = text[:0]
	}

	return out
}

// tagOrder returns the position of markType's tag in _tagNesting, or -1
// for a mark the model does not see.
func tagOrder(markType string) int {
	return slices.IndexFunc(_tagNesting, func(tag string) bool { return _tagMarks[tag] == markType })
}

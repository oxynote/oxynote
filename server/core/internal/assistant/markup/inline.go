package markup

import (
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// tagMark is one visible mark as a tag.
type tagMark struct {
	// tag is the element name.
	tag string

	// open is the tag that opens the mark, a link's href included.
	open string
}

// renderInline renders inline content as text with tags. Neighbouring
// text nodes share the tags they have in common, which nest in the
// _tagNesting order.
func renderInline(content []document.Block) string {
	var (
		sb        strings.Builder
		openMarks []tagMark
	)

	for _, n := range content {
		if n.Type != document.BlockNodeText {
			continue
		}

		want := visibleMarks(n.Marks)

		kept := 0
		for kept < len(openMarks) && kept < len(want) && openMarks[kept] == want[kept] {
			kept++
		}

		for i := len(openMarks) - 1; i >= kept; i-- {
			sb.WriteString(endTag(openMarks[i].tag))
		}

		openMarks = openMarks[:kept]

		for _, m := range want[kept:] {
			sb.WriteString(m.open)
			openMarks = append(openMarks, m)
		}

		sb.WriteString(_textEscaper.Replace(n.Text))
	}

	for _, o := range slices.Backward(openMarks) {
		sb.WriteString(endTag(o.tag))
	}

	return sb.String()
}

// visibleMarks returns the marks the model sees, in nesting order.
func visibleMarks(marks []document.Mark) []tagMark {
	var out []tagMark

	for _, tag := range _tagNesting {
		for _, m := range marks {
			if m.Type != _tagMarks[tag] {
				continue
			}

			tm := tagMark{tag: tag, open: "<" + tag + ">"}

			if tag == _tagLink {
				tm.open = `<a href="` + _attrEscaper.Replace(m.Attrs.Get(_attrHref).String()) + `">`
			}

			out = append(out, tm)
		}
	}

	return out
}

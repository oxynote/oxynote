package markup

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

func Test_renderInline(t *testing.T) {
	t.Parallel()

	text := func(s string, marks ...string) document.Block {
		b := document.Block{Type: document.BlockNodeText, Text: s}

		for _, m := range marks {
			b.Marks = append(b.Marks, document.Mark{Type: m})
		}

		return b
	}

	cc := map[string]struct {
		Content  []document.Block
		Expected string
	}{
		"Nothing at all": {},
		"Plain text is escaped": {
			Content:  []document.Block{text("a < b & c")},
			Expected: "a &lt; b &amp; c",
		},
		"Neighbours share the tags they have in common": {
			Content:  []document.Block{text("a", "bold"), text("b", "bold", "italic"), text("c")},
			Expected: "<b>a<i>b</i></b>c",
		},
		"Tags nest in a fixed order whatever the mark order": {
			Content:  []document.Block{text("a", "italic", "bold")},
			Expected: "<b><i>a</i></b>",
		},
		"Hidden marks render nothing": {
			Content:  []document.Block{text("a", "comment")},
			Expected: "a",
		},
		"Tags still open at the end are closed": {
			Content:  []document.Block{text("a", "bold", "code")},
			Expected: "<b><code>a</code></b>",
		},
		"Nodes that are not text are skipped": {
			Content:  []document.Block{{Type: document.BlockNodeParagraph}, text("a")},
			Expected: "a",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, renderInline(c.Content))
		})
	}
}

func Test_visibleMarks(t *testing.T) {
	t.Parallel()

	got := visibleMarks([]document.Mark{
		{Type: "comment"},
		{Type: "italic"},
		{Type: "link", Attrs: document.Attributes{_attrHref: `https://x.test/?a=1&b="2"`}},
	})

	assert.Equal(t, []tagMark{
		{tag: _tagLink, open: `<a href="https://x.test/?a=1&amp;b=&quot;2&quot;">`},
		{tag: _tagItalic, open: "<i>"},
	}, got)
	assert.Nil(t, visibleMarks(nil))
}

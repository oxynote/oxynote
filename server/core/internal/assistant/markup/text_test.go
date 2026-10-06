package markup

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_inline(t *testing.T) {
	t.Parallel()

	nodes, err := parse("<p>\n  Hi <b>there</b>\n</p>")
	require.NoError(t, err)

	got, err := inline(nodes[0].children, nil)
	require.NoError(t, err)
	assert.Equal(t, []document.Block{
		{Type: document.BlockNodeText, Text: "Hi "},
		{Type: document.BlockNodeText, Text: "there", Marks: []document.Mark{{Type: "bold"}}},
	}, got)
}

func Test_textRuns(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Markup string
		Result []run
		Err    error
	}{
		"Plain text": {
			Markup: "<p>a</p>",
			Result: []run{{text: "a"}},
		},
		"Tags in any order give the same marks": {
			Markup: `<p><i><a href="x">a</a></i></p>`,
			Result: []run{{text: "a", marks: []document.Mark{{Type: "link", Attrs: document.Attributes{_attrHref: "x"}}, {Type: "italic"}}}},
		},
		"Block inside text": {
			Markup: "<p><hr/></p>",
			Err:    assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			nodes, err := parse(c.Markup)
			require.NoError(t, err)

			got, err := textRuns(nodes[0].children, nil)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_linkScheme(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https", linkScheme("HTTPS://x.test"))
	assert.Equal(t, "javascript", linkScheme(" java\nscript:alert(1)"))
	assert.Equal(t, "tel", linkScheme("tel:+1"))
	assert.Empty(t, linkScheme("/docs/a:b"))
	assert.Empty(t, linkScheme("#top"))
}

func Test_collapseWhiteSpace(t *testing.T) {
	t.Parallel()

	got := collapseWhiteSpace([]run{{text: "\n  a\n    b "}, {text: " c \n  "}})
	assert.Equal(t, []run{{text: "a b "}, {text: " c "}}, got)
	assert.Empty(t, collapseWhiteSpace(nil))
}

func Test_textNodes(t *testing.T) {
	t.Parallel()

	comment := document.Mark{Type: "comment", Attrs: document.Attributes{"commentId": "c1"}}
	bold := document.Mark{Type: "bold"}

	cc := map[string]struct {
		Runs     []run
		Stored   []document.Block
		Expected []document.Block
	}{
		"Nothing stored": {
			Runs:     []run{{text: "ab"}, {text: "c", marks: []document.Mark{bold}}},
			Expected: []document.Block{{Type: document.BlockNodeText, Text: "ab"}, {Type: document.BlockNodeText, Text: "c", Marks: []document.Mark{bold}}},
		},
		"Comment kept on the unchanged start": {
			Runs:   []run{{text: "hello world"}},
			Stored: []document.Block{{Type: document.BlockNodeText, Text: "hello", Marks: []document.Mark{comment}}, {Type: document.BlockNodeText, Text: " there"}},
			Expected: []document.Block{
				{Type: document.BlockNodeText, Text: "hello", Marks: []document.Mark{comment}},
				{Type: document.BlockNodeText, Text: " world"},
			},
		},
		"Comment kept on the unchanged end": {
			Runs:   []run{{text: "big cat"}},
			Stored: []document.Block{{Type: document.BlockNodeText, Text: "a "}, {Type: document.BlockNodeText, Text: "cat", Marks: []document.Mark{comment, bold}}},
			Expected: []document.Block{
				{Type: document.BlockNodeText, Text: "big "},
				{Type: document.BlockNodeText, Text: "cat", Marks: []document.Mark{comment}},
			},
		},
		"Stored nodes that are not text are skipped": {
			Runs:     []run{{text: "a"}},
			Stored:   []document.Block{{Type: document.BlockNodeParagraph}},
			Expected: []document.Block{{Type: document.BlockNodeText, Text: "a"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, textNodes(c.Runs, c.Stored))
		})
	}
}

func Test_tagOrder(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, tagOrder("link"))
	assert.Less(t, tagOrder("bold"), tagOrder("code"))
	assert.Equal(t, -1, tagOrder("comment"))
}

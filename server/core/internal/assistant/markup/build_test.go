package markup

import (
	"errors"
	"maps"
	"regexp"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _idAttr matches an id attribute in rendered markup.
var _idAttr = regexp.MustCompile(` id="[^"]*"`)

// _keptOnly matches the elements that can only be kept by id.
var _keptOnly = regexp.MustCompile(`(?m)^\s*<(file|unsupported) [^\n]*/>\n`)

func Test_Build(t *testing.T) {
	t.Parallel()

	root := loadFixture(t, "all_blocks")

	// edit renders the stored block id names, changed by replacing old
	// with replacement.
	edit := func(id, old, replacement string) string {
		stored, ok := root.FindByUID(id)
		require.True(t, ok)

		return strings.Replace(Render([]document.Block{stored}), old, replacement, 1)
	}

	stored := func(id string) document.Block {
		b, ok := root.FindByUID(id)
		require.True(t, ok)

		return b
	}

	uids := func(blocks ...document.Block) []string {
		out := make([]string, 0, len(blocks))

		for _, b := range blocks {
			uid, _ := b.UID()
			out = append(out, uid)
		}

		return out
	}

	type tcase struct {
		Markup string
		Stored document.RootBlock

		// Result is the built blocks rendered without ids; empty skips
		// the check.
		Result string

		Check func(t *testing.T, got []document.Block)
		Err   error
	}

	cc := map[string]tcase{
		"Comment survives on text the edit left": {
			Markup: edit("p1", "Plain ", "Simple "),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()

				var commented []string

				for _, n := range got[0].Content {
					for _, m := range n.Marks {
						if m.Type == "comment" {
							commented = append(commented, n.Text)
						}
					}
				}

				assert.Equal(t, []string{"p1"}, uids(got[0]))
				assert.Equal(t, "Simple ", got[0].Content[0].Text)
				assert.Equal(t, []string{"a comment"}, commented)
			},
		},
		"Block comment survives an edit": {
			Markup: edit("p1", "Plain ", "Simple "),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, "nc-p1", got[0].Attrs[document.AttrCommentID])
			},
		},
		"Titled code keeps its parts' uids and comments": {
			Markup: edit("tc1", "POST /login", "POST /signin"),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []string{"tct", "tcc"}, uids(got[0].Content...))
				assert.Equal(t, "POST /signin", got[0].Content[0].Content[0].Text)
				assert.Equal(t, "nc-tcc", got[0].Content[1].Attrs[document.AttrCommentID])
			},
		},
		"Entry keeps its paragraph uid": {
			Markup: edit("li1", "One", "Uno"),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []string{"li1p"}, uids(got[0].Content[0]))
				assert.Equal(t, "Uno", got[0].Content[0].Content[0].Text)
			},
		},
		"Split doc keeps its sides and unchanged children": {
			Markup: edit("sd1", "Issues a token.", "Issues a session."),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []string{"sl", "sr"}, uids(got[0].Content...))
				assert.Equal(t, stored("sr"), got[0].Content[1])
			},
		},
		"Parameter row keeps its parts' uids": {
			Markup: edit("pl1", `type="string"`, `type="email"`),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()

				row := got[0].Content[1]
				assert.Equal(t, []string{"pi1", "pih", "pity"}, uids(row, row.Content[0], row.Content[0].Content[1]))
				assert.Equal(t, "email", row.Content[0].Content[1].Content[0].Text)
			},
		},
		"Unchanged param row stays as stored": {
			Markup: edit("pl1", `header="`, `header="New `),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, stored("pi1"), got[0].Content[1])
			},
		},
		"Metric keeps its simulation flag with the same preset": {
			Markup: edit("mb1", `"width":"standard"`, `"width":"wide"`),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, true, got[0].Attrs[document.AttrSimulationActive])
				assert.Equal(t, "wide", got[0].Attrs[document.AttrWidth])
			},
		},
		"Metric drops a simulation flag the markup sent": {
			Markup: `<metric>{"simulationActive":true,"uid":"x","nodeCommentId":"c"}</metric>`,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.NotContains(t, got[0].Attrs, document.AttrSimulationActive)
				assert.NotEqual(t, "x", got[0].Attrs[document.AttrUID])
				assert.NotContains(t, got[0].Attrs, document.AttrCommentID)
			},
		},
		"Entry with a kept p keeps it nested": {
			Markup: `<li id="li1">One<p id="p1">Plain</p></li>`,
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []string{"li1p", "p1"}, uids(got[0].Content...))
			},
		},
		"Line breaks in text read as spaces": {
			Markup: "<p>\n  Wrapped\n    text with <strong>strong</strong>\n  words.\n</p>",
			Result: "<p>Wrapped text with <b>strong</b> words.</p>\n",
		},
		"Spaces on the text's own line stay": {
			Markup: "<p> a b </p>",
			Result: "<p> a b </p>\n",
		},
		"Same tag nested is one mark": {
			Markup: "<p><b>a<b>b</b></b></p>",
			Result: "<p><b>ab</b></p>\n",
		},
		"Entry text wrapped in a p is the entry's text": {
			Markup: "<ul><li>\n  <p>x</p>\n</li></ul>",
			Result: "<ul>\n  <li>x</li>\n</ul>\n",
		},
		"Line breaks around raw text are layout": {
			Markup: "<pre>\nx\n</pre>",
			Result: "<pre>x</pre>\n",
		},
		"Malformed markup":        {Markup: "<p>x", Err: errors.New("line 1: <p> is never closed")},
		"Unknown id":              {Markup: `<p id="nope">x</p>`, Stored: root, Err: errors.New(`line 1: id "nope" is not a block this write can keep; leave id out for a new block`)},
		"Id used twice":           {Markup: `<p id="p2">x</p><p id="p2">y</p>`, Stored: root, Err: errors.New(`line 1: id "p2" is used twice; leave it out of the copy`)},
		"Text outside an element": {Markup: "hello", Err: errors.New("line 1: text in the markup has to sit in an element such as <p>")},
		"Text in a container":     {Markup: "<callout>hi</callout>", Err: errors.New("line 1: text in <callout> has to sit in an element such as <p>")},
		"Tag in a heading":        {Markup: "<h2>a <b>b</b></h2>", Err: errors.New("line 1: a heading takes plain text, so <b> is not allowed in it")},
		"Block inside text":       {Markup: "<p>a <hr/></p>", Err: errors.New("line 1: <hr> is a block; it cannot sit inside text")},
		"Link to a script":        {Markup: `<p><a href=" Java	Script:alert(1)">x</a></p>`, Err: errors.New("line 1: a link cannot use javascript:; link to a web address instead")},
		// links a person stored may use any other scheme, or not even
		// parse, and a write that keeps them has to go through.
		"Links of other schemes stay": {
			Markup: `<p><a href="tel:+15551234">call</a> <a href="https://x.test/50%off">sale</a></p>`,
			Result: `<p><a href="tel:+15551234">call</a> <a href="https://x.test/50%off">sale</a></p>` + "\n",
		},
		"Entry text ending in a space keeps it before nested blocks": {
			Markup: "<ul><li>Item \n  <ul><li>x</li></ul>\n</li></ul>",
			Result: "<ul>\n  <li>Item \n    <ul>\n      <li>x</li>\n    </ul>\n  </li>\n</ul>\n",
		},
		"List of the wrong entries":  {Markup: "<ul><task>x</task></ul>", Err: errors.New("line 1: <ul> holds only <li>, not <task>")},
		"Split doc without sides":    {Markup: "<split_doc><right/></split_doc>", Err: errors.New("line 1: <split_doc> holds a <left> and then a <right>")},
		"Text in a split doc":        {Markup: "<split_doc>x</split_doc>", Err: errors.New("line 1: text in <split_doc> has to sit in <left> or <right>")},
		"Params of the wrong rows":   {Markup: `<params header="h"><p>x</p></params>`, Err: errors.New("line 1: <params> holds only <param>, not <p>")},
		"Text in params":             {Markup: `<params header="h">x</params>`, Err: errors.New("line 1: text in <params> has to sit in a <param>")},
		"Param with an unknown id":   {Markup: `<params header="h"><param id="nope" name="a"/></params>`, Stored: root, Err: errors.New(`line 1: id "nope" is not a block this write can keep; leave id out for a new block`)},
		"Width that is not a number": {Markup: `<img src="x" width="wide"/>`, Err: errors.New("line 1: <img> width has to be a positive whole number")},
		"Metric that is not JSON":    {Markup: "<metric>{</metric>", Err: assert.AnError},
		"No block":                   {Markup: "  \n ", Err: errors.New("the markup holds no block; to remove one, use delete_block")},
		"More blocks than one call takes": {
			Markup: strings.Repeat("<hr/>", _maxBlocks+1),
			Err:    errors.New("501 blocks are over the limit of 500 per call; send them in several calls"),
		},
		"Metric with a bad value": {Markup: `<metric>{"width":"huge"}</metric>`, Err: assert.AnError},
		"New file":                {Markup: `<file src="x"/>`, Err: errors.New("line 1: <file> can only keep a block get_document showed as <file>, by its id")},
		"Unsupported keeping a known block": {
			Markup: `<unsupported id="p1"/>`,
			Stored: root,
			Err:    errors.New("line 1: <unsupported> can only keep a block get_document showed as <unsupported>, by its id"),
		},
		"Id kept as an element and as a part": {
			Markup: `<li id="li1">One</li><p id="li1p">x</p>`,
			Stored: root,
			Err:    errors.New(`id "li1p" is kept twice: by its own element and as part of another one; leave it out of one of them`),
		},
		"Side outside a split doc": {Markup: "<left></left>", Err: errors.New("line 1: <left> belongs inside <split_doc>")},
		"Param outside params":     {Markup: `<param name="a"/>`, Err: errors.New("line 1: <param> belongs inside <params>")},
		"Inline tag at the top":    {Markup: "<b>x</b>", Err: errors.New("line 1: <b> is inline; put it inside a <p>")},
		"Entry with text after a list": {
			Markup: "<ul><li>a<ul><li>b</li></ul>c</li></ul>",
			Err:    errors.New("line 1: text in <li> after its text has to sit in an element such as <p>"),
		},
	}

	// markup read and written back unchanged is the stored document
	// itself, hidden marks and parts included. Without ids it builds
	// new blocks that read back the same.
	for _, name := range _fixtures {
		fixture := loadFixture(t, name)
		rendered := Render(fixture.Content)
		withoutIDs := _keptOnly.ReplaceAllString(_idAttr.ReplaceAllString(rendered, ""), "")

		maps.Copy(cc, map[string]tcase{
			name + " kept as it is": {
				Markup: rendered,
				Stored: fixture,
				Check: func(t *testing.T, got []document.Block) {
					t.Helper()
					assert.Equal(t, fixture.Content, got)
				},
			},
			name + " written as new": {
				Markup: withoutIDs,
				Result: withoutIDs,
			},
		})
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := Build(c.Markup, c.Stored)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Result != "" {
				assert.Equal(t, c.Result, _idAttr.ReplaceAllString(Render(got), ""))
			}

			if c.Check != nil {
				c.Check(t, got)
			}
		})
	}
}

func Test_elements(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Markup string
		Result string
		Err    error
	}{
		"Elements in order, blank text dropped": {
			Markup: "<p>a</p>\n  <hr/>\n",
			Result: `p@1["a"] hr@2`,
		},
		"Nothing at all": {},
		"Text that is not blank": {
			Markup: "<p>a</p> b",
			Err:    errors.New("line 1: text in <x> has to sit in a <y>"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			nodes, err := parse(c.Markup)
			require.NoError(t, err)

			got, err := elements(nodes, "<x>", "a <y>")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, dump(got))
		})
	}
}

func Test_newBlock(t *testing.T) {
	t.Parallel()

	stored := document.Block{
		Type:  document.BlockNodeParagraph,
		Attrs: document.Attributes{document.AttrUID: "p1", document.AttrCommentID: "c1", "align": "left"},
	}

	got := newBlock(document.BlockNodeHeading, stored, nil)
	assert.Equal(t, document.Block{
		Type:  document.BlockNodeHeading,
		Attrs: document.Attributes{document.AttrUID: "p1", document.AttrCommentID: "c1"},
	}, got)

	got = newBlock(document.BlockNodeParagraph, document.Block{}, nil)
	uid, _ := got.UID()
	assert.NotEmpty(t, uid)
	assert.Len(t, got.Attrs, 1)
}

func Test_titleBlock(t *testing.T) {
	t.Parallel()

	comment := document.Mark{Type: "comment", Attrs: document.Attributes{"commentId": "c1"}}
	parent := document.Block{Content: []document.Block{{
		Type:    document.BlockNodeCodeBlockTitle,
		Attrs:   document.Attributes{document.AttrUID: "t1"},
		Content: []document.Block{{Type: document.BlockNodeText, Text: "GET /a", Marks: []document.Mark{comment}}},
	}}}

	// the part keeps its uid and its comment on the text left as it was.
	got := titleBlock(document.BlockNodeCodeBlockTitle, "GET /a/b", parent)
	assert.Equal(t, document.Block{
		Type:  document.BlockNodeCodeBlockTitle,
		Attrs: document.Attributes{document.AttrUID: "t1"},
		Content: []document.Block{
			{Type: document.BlockNodeText, Text: "GET /a", Marks: []document.Mark{comment}},
			{Type: document.BlockNodeText, Text: "/b"},
		},
	}, got)

	assert.Nil(t, titleBlock(document.BlockNodeCodeBlockTitle, "", document.Block{}).Content)
}

func Test_keepIfUnchanged(t *testing.T) {
	t.Parallel()

	stored := document.Block{
		Type:    document.BlockNodeParagraph,
		Attrs:   document.Attributes{document.AttrUID: "p", "textAlign": "center"},
		Content: []document.Block{{Type: document.BlockNodeText, Text: "a"}},
	}
	same := document.Block{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "p"}, Content: stored.Content}
	changed := document.Block{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "p"}}

	assert.Equal(t, stored, keepIfUnchanged(stored, same))
	assert.Equal(t, changed, keepIfUnchanged(stored, changed))
	assert.Equal(t, same, keepIfUnchanged(document.Block{}, same))
}

func Test_findDuplicateUID(t *testing.T) {
	t.Parallel()

	block := func(uid string, content ...document.Block) document.Block {
		return document.Block{Attrs: document.Attributes{document.AttrUID: uid}, Content: content}
	}

	_, found := findDuplicateUID([]document.Block{block("a", block("b")), block("c")}, map[string]bool{})
	assert.False(t, found)

	uid, found := findDuplicateUID([]document.Block{block("a", block("b")), block("c", block("b"))}, map[string]bool{})
	assert.True(t, found)
	assert.Equal(t, "b", uid)
}

package markup

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/icon"
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

	uids := func(blocks ...document.Block) []string {
		out := make([]string, 0, len(blocks))

		for _, b := range blocks {
			uid, _ := b.UID()
			out = append(out, uid)
		}

		return out
	}

	// cells lists a block's column, row, columns and rows in its grid.
	cells := func(b document.Block) []any {
		return []any{
			b.Attrs[document.AttrWidePosX],
			b.Attrs[document.AttrWidePosY],
			b.Attrs[document.AttrWidth],
			b.Attrs[document.AttrHeight],
		}
	}

	grid, ok := root.FindByUID("g1")
	require.True(t, ok)

	legacyGrid := grid
	legacyGrid.Type = document.BlockNodeMetricGrid
	legacyGrid.Attrs = document.Attributes{document.AttrUID: "g1", document.AttrCommentID: "nc-g1"}

	// irregular is the fixture's metric with a width of 10 columns. No
	// named width spans 10 columns.
	irregular, ok := root.FindByUID("mb1")
	require.True(t, ok)

	irregular.Attrs = maps.Clone(irregular.Attrs)
	irregular.Attrs[document.AttrWidth] = 10.0

	// placedGrid's blocks read pc, pa, pb. child renders one of them.
	placedGrid := placedTestGrid(t)
	child := func(uid string) string {
		b, ok := placedGrid.FindByUID(uid)
		require.True(t, ok)

		return Render([]document.Block{b})
	}

	unknown := document.Block{Type: "mysteryBlock", Attrs: document.Attributes{document.AttrUID: "x1"}}
	mixedGrid := grid
	mixedGrid.Content = append(slices.Clone(grid.Content), unknown)

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
		"Titled code keeps the uids and comments of its parts": {
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

				right, ok := root.FindByUID("sr")
				require.True(t, ok)
				assert.Equal(t, []string{"sl", "sr"}, uids(got[0].Content...))
				assert.Equal(t, right, got[0].Content[1])
			},
		},
		"Parameter row keeps the uids of its parts": {
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

				row, ok := root.FindByUID("pi1")
				require.True(t, ok)
				assert.Equal(t, row, got[0].Content[1])
			},
		},
		"Metric keeps its simulation flag with the same preset": {
			Markup: edit("mb1", `"width":"standard"`, `"width":"wide"`),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, true, got[0].Attrs[document.AttrSimulationActive])
				assert.Equal(t, 24, got[0].Attrs[document.AttrWidth])
			},
		},
		"Metric re-sent keeps its cells in the grid": {
			Markup: edit("mb1", `"refreshInterval":"1m"`, `"refreshInterval":"5m"`),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []any{0.0, 0.0, 12.0, 1.0}, cells(got[0]))
			},
		},
		"Metric given another named width and height takes their columns and rows": {
			Markup: strings.Replace(edit("mb1", `"width":"standard"`, `"width":"wide"`), `"height":"standard"`, `"height":"tall"`, 1),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []any{0.0, 0.0, 24, 2}, cells(got[0]))
			},
		},
		"Metric keeps a width without a name the markup left as it read it": {
			Markup: strings.Replace(Render([]document.Block{irregular}), `"refreshInterval":"1m"`, `"refreshInterval":"5m"`, 1),
			Stored: document.RootBlock{Content: []document.Block{irregular}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []any{0.0, 0.0, 10.0, 1.0}, cells(got[0]))
			},
		},
		"Log keeps a width name it was saved with when the markup leaves it": {
			Markup: edit("lb1", `"title":"Errors"`, `"title":"Failures"`),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, "standard", got[0].Attrs[document.AttrWidth])
			},
		},
		"Log saved with a width name given another takes its columns": {
			Markup: edit("lb1", `"width":"standard"`, `"width":"compact"`),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, 8, got[0].Attrs[document.AttrWidth])
			},
		},
		"New metric has no cells": {
			Markup: `<metrics>{"width":"compact"}</metrics>`,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []any{nil, nil, 8, nil}, cells(got[0]))
			},
		},
		"Metric re-sent without its window keeps it": {
			Markup: edit("mb1", `"timeRange":"last_1_hour",`, ""),
			Stored: root,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, "last_1_hour", got[0].Attrs[document.AttrTimeRange])
				assert.Equal(t, "1m", got[0].Attrs[document.AttrRefreshInterval])
			},
		},
		"New metric follows the built-in variables": {
			Markup: `<metrics>{}</metrics>`,
			Result: "<metrics>{\"refreshInterval\":\"$metric_refresh\",\"timeRange\":\"$metric_range\"}</metrics>\n",
		},
		"Metric drops a simulation flag the markup sent": {
			Markup: `<metrics>{"simulationActive":true,"uid":"x","nodeCommentId":"c"}</metrics>`,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.NotContains(t, got[0].Attrs, document.AttrSimulationActive)
				assert.NotEqual(t, "x", got[0].Attrs[document.AttrUID])
				assert.NotContains(t, got[0].Attrs, document.AttrCommentID)
			},
		},
		"New metrics build a block grid": {
			Markup: `<grid><metrics>{}</metrics></grid>`,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, document.BlockNodeBlockGrid, got[0].Type)
			},
		},
		"Metric under its former name builds a metric": {
			Markup: `<grid><metric>{"title":"Up"}</metric></grid>`,
			Result: "<grid>\n  <metrics row=\"0\" column=\"0\" stack=\"0\">{\"refreshInterval\":\"$metric_refresh\",\"timeRange\":\"$metric_range\",\"title\":\"Up\"}</metrics>\n</grid>\n",
		},
		"New logs build a log block": {
			Markup: `<grid><logs>{"title":"Errors","order":"asc"}</logs></grid>`,
			Result: "<grid>\n  <logs row=\"0\" column=\"0\" stack=\"0\">{\"order\":\"asc\",\"refreshInterval\":\"$metric_refresh\",\"timeRange\":\"$metric_range\",\"title\":\"Errors\"}</logs>\n</grid>\n",
		},
		"Logs drop the uid and comment the markup sent": {
			Markup: `<logs>{"uid":"x","nodeCommentId":"c"}</logs>`,
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, document.BlockNodeLogBlock, got[0].Type)
				assert.NotEqual(t, "x", got[0].Attrs[document.AttrUID])
				assert.NotContains(t, got[0].Attrs, document.AttrCommentID)
			},
		},
		"Legacy grid kept by its id becomes a block grid": {
			Markup: Render([]document.Block{legacyGrid}),
			Stored: document.RootBlock{Content: []document.Block{legacyGrid}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, document.BlockNodeBlockGrid, got[0].Type)
				assert.Equal(t, legacyGrid.Attrs, got[0].Attrs)
				assert.Equal(t, legacyGrid.Content, got[0].Content)
			},
		},
		"Grid keeps a block it cannot write by its id": {
			Markup: Render([]document.Block{mixedGrid}),
			Stored: document.RootBlock{Content: []document.Block{mixedGrid}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, mixedGrid.Content, got[0].Content)
			},
		},
		"Grid with a block removed keeps the cells of the rest": {
			Markup: `<grid id="pg">` + child("pc") + child("pb") + `</grid>`,
			Stored: document.RootBlock{Content: []document.Block{placedGrid}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, []string{"pc", "pb"}, uids(got[0].Content...))
				assert.Equal(t, []any{0.0, 0.0, 12.0, 1.0}, cells(got[0].Content[0]))
				assert.Equal(t, []any{0.0, 1.0, 24.0, 1.0}, cells(got[0].Content[1]))
			},
		},
		"Grid ignores a row, column and stacked place the markup changed": {
			Markup: strings.Replace(Render([]document.Block{placedGrid}), `id="pc" row="0" column="0" stack="0"`, `id="pc" row="5" column="3" stack="2"`, 1),
			Stored: document.RootBlock{Content: []document.Block{placedGrid}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, placedGrid, got[0])
			},
		},
		"Grid with its blocks reordered stays as stored": {
			Markup: `<grid id="pg">` + child("pb") + child("pc") + child("pa") + `</grid>`,
			Stored: document.RootBlock{Content: []document.Block{placedGrid}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				assert.Equal(t, placedGrid, got[0])
			},
		},
		"Grid with a new block keeps the places of the others": {
			Markup: strings.Replace(Render([]document.Block{placedGrid}), "</grid>", "<metrics>{}</metrics></grid>", 1),
			Stored: document.RootBlock{Content: []document.Block{placedGrid}},
			Check: func(t *testing.T, got []document.Block) {
				t.Helper()
				require.Len(t, got[0].Content, 4)
				assert.Equal(t, []string{"pc", "pa", "pb"}, uids(got[0].Content[:3]...))
				assert.Equal(t, []any{0.0, 0.0, 12.0, 1.0}, cells(got[0].Content[0]))
				assert.NotContains(t, got[0].Content[3].Attrs, document.AttrWidePosX)
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
		"Spaces on the line of the text stay": {
			Markup: "<p> a b </p>",
			Result: "<p> a b </p>\n",
		},
		"Same tag nested is one mark": {
			Markup: "<p><b>a<b>b</b></b></p>",
			Result: "<p><b>ab</b></p>\n",
		},
		"Entry text wrapped in a p is the text of the entry": {
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
		"Callout icon the picker lacks": {
			Markup: `<callout icon="lucide:rocket"><p>x</p></callout>`,
			Err:    fmt.Errorf("line 1: %w", icon.Validate("lucide:rocket")),
		},
		"Callout with the default icon": {
			Markup: `<callout icon="lucide:text"><p>x</p></callout>`,
			Result: "<callout icon=\"lucide:text\">\n  <p>x</p>\n</callout>\n",
		},
		"Kept callout keeps an icon the picker lacks": {
			Markup: `<callout id="c1" icon="lucide:info"><p>x</p></callout>`,
			Stored: document.RootBlock{Content: []document.Block{{
				Type:    document.BlockNodeCalloutBlock,
				Attrs:   document.Attributes{document.AttrUID: "c1", document.AttrIcon: "lucide:info"},
				Content: []document.Block{{Type: document.BlockNodeParagraph}},
			}}},
			Result: "<callout icon=\"lucide:info\">\n  <p>x</p>\n</callout>\n",
		},
		"Tag in a heading":  {Markup: "<h2>a <b>b</b></h2>", Err: errors.New("line 1: a heading takes plain text, so <b> is not allowed in it")},
		"Block inside text": {Markup: "<p>a <hr/></p>", Err: errors.New("line 1: <hr> is a block; it cannot sit inside text")},
		"Link to a script":  {Markup: `<p><a href=" Java	Script:alert(1)">x</a></p>`, Err: errors.New("line 1: a link cannot use javascript:; link to a web address instead")},
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
		"List of the wrong entries": {Markup: "<ul><task>x</task></ul>", Err: errors.New("line 1: <ul> holds only <li>, not <task>")},
		"Split doc without sides":   {Markup: "<split_doc><right/></split_doc>", Err: errors.New("line 1: <split_doc> holds a <left> and then a <right>")},
		"Text in a split doc":       {Markup: "<split_doc>x</split_doc>", Err: errors.New("line 1: text in <split_doc> has to sit in <left> or <right>")},
		"Split doc with an h2 title": {
			Markup: "<split_doc><left><h2>T</h2><p>x</p></left><right><mermaid>graph LR; A-->B;</mermaid></right></split_doc>",
			Err:    errors.New("line 1: <left> opens with an <h1>, the title of the split doc"),
		},
		"Split doc without a title": {
			Markup: "<split_doc><left><p>x</p></left><right><mermaid>graph LR; A-->B;</mermaid></right></split_doc>",
			Err:    errors.New("line 1: <left> opens with an <h1>, the title of the split doc"),
		},
		"Params of the wrong rows":   {Markup: `<params header="h"><p>x</p></params>`, Err: errors.New("line 1: <params> holds only <param>, not <p>")},
		"Text in params":             {Markup: `<params header="h">x</params>`, Err: errors.New("line 1: text in <params> has to sit in a <param>")},
		"Param with an unknown id":   {Markup: `<params header="h"><param id="nope" name="a"/></params>`, Stored: root, Err: errors.New(`line 1: id "nope" is not a block this write can keep; leave id out for a new block`)},
		"Width that is not a number": {Markup: `<img src="x" width="wide"/>`, Err: errors.New("line 1: <img> width has to be a positive whole number")},
		"Metric that is not JSON":    {Markup: "<metrics>{</metrics>", Err: assert.AnError},
		"No block":                   {Markup: "  \n ", Err: errors.New("the markup holds no block; to remove one, use delete_block")},
		"More blocks than one call takes": {
			Markup: strings.Repeat("<hr/>", _maxBlocks+1),
			Err:    errors.New("501 blocks are over the limit of 500 per call; send them in several calls"),
		},
		"Metric with a bad value":  {Markup: `<metrics>{"width":"huge"}</metrics>`, Err: assert.AnError},
		"Logs with a bad value":    {Markup: `<logs>{"order":"sideways"}</logs>`, Err: assert.AnError},
		"Grid of the wrong blocks": {Markup: "<grid><p>x</p></grid>", Err: errors.New("line 1: <grid> holds only <metrics> or <logs>, not <p>")},
		"New file":                 {Markup: `<file src="x"/>`, Err: errors.New("line 1: <file> can only keep a block get_document showed as <file>, by its id")},
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
		"Param outside params":     {Markup: `<param name="a"/>`, Err: errors.New("line 1: <param> belongs inside <params>; to add one, send the whole <params> to replace_blocks")},
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
		"Elements in order and blank text dropped": {
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

	cc := map[string]struct {
		Type   document.BlockNodeType
		Stored document.Block
		Result document.Block

		// NewUID, when set, expects a fresh uid as the only attribute in
		// place of Result.
		NewUID bool
	}{
		"Stored uid and block comment kept": {
			Type: document.BlockNodeHeading,
			Stored: document.Block{
				Type:  document.BlockNodeParagraph,
				Attrs: document.Attributes{document.AttrUID: "p1", document.AttrCommentID: "c1", "align": "left"},
			},
			Result: document.Block{
				Type:  document.BlockNodeHeading,
				Attrs: document.Attributes{document.AttrUID: "p1", document.AttrCommentID: "c1"},
			},
		},
		"Nothing stored": {
			Type:   document.BlockNodeParagraph,
			NewUID: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := newBlock(c.Type, c.Stored, nil)

			if c.NewUID {
				uid, _ := got.UID()
				assert.NotEmpty(t, uid)
				assert.Len(t, got.Attrs, 1)

				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
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

	cc := map[string]struct {
		Stored document.Block
		Built  document.Block
		Result document.Block
	}{
		"Same rendering keeps the stored block": {Stored: stored, Built: same, Result: stored},
		"Changed rendering takes the built one": {Stored: stored, Built: changed, Result: changed},
		"Nothing stored":                        {Built: same, Result: same},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, keepIfUnchanged(c.Stored, c.Built))
		})
	}
}

func Test_findDuplicateUID(t *testing.T) {
	t.Parallel()

	block := func(uid string, content ...document.Block) document.Block {
		return document.Block{Attrs: document.Attributes{document.AttrUID: uid}, Content: content}
	}

	cc := map[string]struct {
		Blocks []document.Block
		UID    string
		Found  bool
	}{
		"Every uid held once": {
			Blocks: []document.Block{block("a", block("b")), block("c")},
		},
		"Nested uid held twice": {
			Blocks: []document.Block{block("a", block("b")), block("c", block("b"))},
			UID:    "b",
			Found:  true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			uid, found := findDuplicateUID(c.Blocks, map[string]bool{})
			assert.Equal(t, c.Found, found)
			assert.Equal(t, c.UID, uid)
		})
	}
}

func Test_setGridCells(t *testing.T) {
	t.Parallel()

	stored := gridBlock("a", document.Attributes{
		document.AttrWidePosX: 12.0,
		document.AttrWidePosY: 1.0,
		document.AttrStackPos: 2.0,
		document.AttrWidth:    10.0,
		document.AttrHeight:   2.0,
	})
	places := func(attrs document.Attributes) document.Attributes {
		attrs[document.AttrWidePosX] = 12.0
		attrs[document.AttrWidePosY] = 1.0
		attrs[document.AttrStackPos] = 2.0

		return attrs
	}

	cc := map[string]struct {
		Attrs  document.Attributes
		Stored document.Block
		Result document.Attributes
	}{
		"New block takes the columns and rows of its names": {
			Attrs:  document.Attributes{document.AttrWidth: "compact", document.AttrHeight: "tall"},
			Result: document.Attributes{document.AttrWidth: 8, document.AttrHeight: 2},
		},
		"New block keeps a name the grid does not know": {
			Attrs:  document.Attributes{document.AttrWidth: "huge"},
			Result: document.Attributes{document.AttrWidth: "huge"},
		},
		"Stored block keeps its places and the size it was read with": {
			Attrs:  document.Attributes{document.AttrWidth: "standard", document.AttrHeight: "tall"},
			Stored: stored,
			Result: places(document.Attributes{document.AttrWidth: 10.0, document.AttrHeight: 2.0}),
		},
		"Stored block given another name takes its columns": {
			Attrs:  document.Attributes{document.AttrWidth: "wide", document.AttrHeight: "tall"},
			Stored: stored,
			Result: places(document.Attributes{document.AttrWidth: 24, document.AttrHeight: 2.0}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			setGridCells(c.Attrs, c.Stored)
			assert.Equal(t, c.Result, c.Attrs)
		})
	}
}

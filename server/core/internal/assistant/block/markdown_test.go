package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseInlineMarkdown(t *testing.T) {
	cc := map[string]struct {
		Input    string
		Expected []document.Block
	}{
		"Plain text": {
			Input:    "hello world",
			Expected: []document.Block{{Type: "text", Text: "hello world"}},
		},
		"Empty string": {
			Input:    "",
			Expected: nil,
		},
		"Bold span": {
			Input: "say **hi** there",
			Expected: []document.Block{
				{Type: "text", Text: "say "},
				{Type: "text", Text: "hi", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: " there"},
			},
		},
		"Italic span": {
			Input: "*ital* x",
			Expected: []document.Block{
				{Type: "text", Text: "ital", Marks: []document.Mark{{Type: "italic"}}},
				{Type: "text", Text: " x"},
			},
		},
		"Underline span": {
			Input: "_under_ x",
			Expected: []document.Block{
				{Type: "text", Text: "under", Marks: []document.Mark{{Type: "underline"}}},
				{Type: "text", Text: " x"},
			},
		},
		"Strike span": {
			Input: "~~gone~~",
			Expected: []document.Block{
				{Type: "text", Text: "gone", Marks: []document.Mark{{Type: "strike"}}},
			},
		},
		"Code span is literal": {
			Input: "use `os.Exit(1)` then",
			Expected: []document.Block{
				{Type: "text", Text: "use "},
				{Type: "text", Text: "os.Exit(1)", Marks: []document.Mark{{Type: "code"}}},
				{Type: "text", Text: " then"},
			},
		},
		"Code span does not parse inner markdown": {
			Input: "`**not bold**`",
			Expected: []document.Block{
				{Type: "text", Text: "**not bold**", Marks: []document.Mark{{Type: "code"}}},
			},
		},
		"Bold and underline nested": {
			Input: "**bold _and_ under**",
			Expected: []document.Block{
				{Type: "text", Text: "bold ", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: "and", Marks: []document.Mark{{Type: "bold"}, {Type: "underline"}}},
				{Type: "text", Text: " under", Marks: []document.Mark{{Type: "bold"}}},
			},
		},
		"Link with markdown label": {
			Input: "[**oxy**](https://x)",
			Expected: []document.Block{
				{Type: "text", Text: "oxy", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://x"}},
					{Type: "bold"},
				}},
			},
		},
		"Plain link": {
			Input: "see [docs](https://docs)",
			Expected: []document.Block{
				{Type: "text", Text: "see "},
				{Type: "text", Text: "docs", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://docs"}},
				}},
			},
		},
		"Escaped asterisks are literal": {
			Input: `\*not italic\*`,
			Expected: []document.Block{
				{Type: "text", Text: "*not italic*"},
			},
		},
		"Unmatched bold becomes literal": {
			Input: "**unfinished",
			Expected: []document.Block{
				{Type: "text", Text: "**unfinished"},
			},
		},
		"Unmatched bracket becomes literal": {
			Input: "[label without",
			Expected: []document.Block{
				{Type: "text", Text: "[label without"},
			},
		},
		"Empty bold span": {
			Input: "x ** ** y",
			Expected: []document.Block{
				{Type: "text", Text: "x "},
				{Type: "text", Text: " ", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: " y"},
			},
		},
		"Adjacent same-mark runs merge": {
			Input: "**a****b**",
			Expected: []document.Block{
				{Type: "text", Text: "ab", Marks: []document.Mark{{Type: "bold"}}},
			},
		},
		"Bold run extended by a bold italic tail": {
			Input: "**a*b***",
			Expected: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: "b", Marks: []document.Mark{{Type: "bold"}, {Type: "italic"}}},
			},
		},
		"Bold wrapping italic via a triple run": {
			Input: "***bold italic***",
			Expected: []document.Block{
				{Type: "text", Text: "bold italic", Marks: []document.Mark{{Type: "bold"}, {Type: "italic"}}},
			},
		},
		"Backtick inside link label": {
			Input: "[`x`](https://y)",
			Expected: []document.Block{
				{Type: "text", Text: "x", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://y"}},
					{Type: "code"},
				}},
			},
		},
		"Escaped bracket inside link label": {
			Input: `[a\]b](https://u)`,
			Expected: []document.Block{
				{Type: "text", Text: "a]b", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://u"}},
				}},
			},
		},
		"Unclosed backtick in link label is literal": {
			Input: "[`x](u)",
			Expected: []document.Block{
				{Type: "text", Text: "[`x](u)"},
			},
		},
		"Bracket without url part is literal": {
			Input: "[x]y",
			Expected: []document.Block{
				{Type: "text", Text: "[x]y"},
			},
		},
		"Escaped paren stays inside the href": {
			Input: `[Go](https://x/Go_\(lang\))`,
			Expected: []document.Block{
				{Type: "text", Text: "Go", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://x/Go_(lang)"}},
				}},
			},
		},
		"Link with missing closing paren is literal": {
			Input: "[x](u",
			Expected: []document.Block{
				{Type: "text", Text: "[x](u"},
			},
		},
		"Escape inside bold span": {
			Input: `**a\*b**`,
			Expected: []document.Block{
				{Type: "text", Text: "a*b", Marks: []document.Mark{{Type: "bold"}}},
			},
		},
		"Code span inside bold span": {
			Input: "**`*`**",
			Expected: []document.Block{
				{Type: "text", Text: "*", Marks: []document.Mark{{Type: "bold"}, {Type: "code"}}},
			},
		},
		"Unclosed backtick inside bold span is literal": {
			Input: "**a`b**",
			Expected: []document.Block{
				{Type: "text", Text: "a`b", Marks: []document.Mark{{Type: "bold"}}},
			},
		},
		"Link inside italic span": {
			Input: "*a [l](https://u) b*",
			Expected: []document.Block{
				{Type: "text", Text: "a ", Marks: []document.Mark{{Type: "italic"}}},
				{Type: "text", Text: "l", Marks: []document.Mark{
					{Type: "italic"},
					{Type: "link", Attrs: map[string]any{"href": "https://u"}},
				}},
				{Type: "text", Text: " b", Marks: []document.Mark{{Type: "italic"}}},
			},
		},
		"Empty link label and url yields nothing": {
			Input:    "[]()",
			Expected: nil,
		},
		"Empty code span is dropped": {
			Input: "a``b",
			Expected: []document.Block{
				{Type: "text", Text: "ab"},
			},
		},
		"Escape at end of input is literal": {
			Input: `a\`,
			Expected: []document.Block{
				{Type: "text", Text: `a\`},
			},
		},
		"Adjacent different marks stay separate": {
			Input: "*a*_b_",
			Expected: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{{Type: "italic"}}},
				{Type: "text", Text: "b", Marks: []document.Mark{{Type: "underline"}}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := ParseInlineMarkdown(c.Input)
			assert.Equal(t, c.Expected, got)
		})
	}
}

func Test_emitInlineMarkdown(t *testing.T) {
	cc := map[string]struct {
		Input    []document.Block
		Expected string

		// Markdown asserts a round trip instead of an emitted string:
		// parsing it, emitting the result and parsing again has to yield
		// the same atoms.
		Markdown string
	}{
		"Round trip: plain text":            {Markdown: "plain text"},
		"Round trip: bold":                  {Markdown: "**bold**"},
		"Round trip: bold wrapping italic":  {Markdown: "***both***"},
		"Round trip: italic":                {Markdown: "*italic*"},
		"Round trip: underline":             {Markdown: "_underline_"},
		"Round trip: strike":                {Markdown: "~~strike~~"},
		"Round trip: code":                  {Markdown: "`code`"},
		"Round trip: link":                  {Markdown: "[label](https://x)"},
		"Round trip: mixed marks":           {Markdown: "mix **bold** and *italic* and `code`"},
		"Round trip: escaped literal":       {Markdown: `escaped \* literal`},
		"Round trip: link inside bold":      {Markdown: "**[link inside bold](https://y)**"},
		"Round trip: strike with code mark": {Markdown: "~~strike with `code` inside~~"},
		"Round trip: parenthesised href":    {Markdown: `[Go](https://en.wikipedia.org/wiki/Go_\(programming_language\))`},
		"Round trip: bracket in label":      {Markdown: `[label with \] bracket](https://x)`},
		"Plain text": {
			Input:    []document.Block{{Type: "text", Text: "hello"}},
			Expected: "hello",
		},
		"Bold only": {
			Input: []document.Block{
				{Type: "text", Text: "hi", Marks: []document.Mark{{Type: "bold"}}},
			},
			Expected: "**hi**",
		},
		"Plain then bold": {
			Input: []document.Block{
				{Type: "text", Text: "say "},
				{Type: "text", Text: "hi", Marks: []document.Mark{{Type: "bold"}}},
			},
			Expected: "say **hi**",
		},
		"Special characters escape in plain text": {
			Input: []document.Block{
				{Type: "text", Text: "use * carefully"},
			},
			Expected: `use \* carefully`,
		},
		"Code text with a backtick falls back to escaped plain text": {
			Input: []document.Block{
				{Type: "text", Text: "a`b", Marks: []document.Mark{{Type: "code"}}},
			},
			Expected: "a\\`b",
		},
		"Code mark does not escape inside": {
			Input: []document.Block{
				{Type: "text", Text: "**", Marks: []document.Mark{{Type: "code"}}},
			},
			Expected: "`**`",
		},
		"Link emit": {
			Input: []document.Block{
				{Type: "text", Text: "x", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://y"}},
				}},
			},
			Expected: "[x](https://y)",
		},
		"Link emit escapes a paren in the href": {
			Input: []document.Block{
				{Type: "text", Text: "x", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://y/a_(b)"}},
				}},
			},
			Expected: `[x](https://y/a_(b\))`,
		},
		"Non-text nodes are skipped": {
			Input: []document.Block{
				{Type: document.BlockNodeParagraph},
				{Type: "text", Text: "hi"},
			},
			Expected: "hi",
		},
		"Incompatible mark order reorders to extend the open run": {
			Input: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: "b", Marks: []document.Mark{{Type: "italic"}, {Type: "bold"}}},
			},
			Expected: "**a*b***",
		},
		"Nested mark closes when the next run drops it": {
			Input: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{{Type: "bold"}, {Type: "italic"}}},
				{Type: "text", Text: "b", Marks: []document.Mark{{Type: "bold"}}},
			},
			Expected: "***a*b**",
		},
		"Disjoint mark pair closes the unshared active mark": {
			Input: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: "b", Marks: []document.Mark{{Type: "italic"}, {Type: "underline"}}},
			},
			Expected: "**a***_b_*",
		},
		"Disjoint marks close and reopen": {
			Input: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{{Type: "bold"}}},
				{Type: "text", Text: "b", Marks: []document.Mark{{Type: "italic"}}},
			},
			Expected: "**a***b*",
		},
		"Link href change closes and reopens the link": {
			Input: []document.Block{
				{Type: "text", Text: "a", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://one"}},
				}},
				{Type: "text", Text: "b", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://two"}},
				}},
			},
			Expected: "[a](https://one)[b](https://two)",
		},
		"Unknown mark types are dropped": {
			Input: []document.Block{
				{Type: "text", Text: "hi", Marks: []document.Mark{{Type: "sparkle"}}},
			},
			Expected: "hi",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Markdown != "" {
				parsed := ParseInlineMarkdown(c.Markdown)
				emitted := emitInlineMarkdown(parsed)

				require.Equal(t, parsed, ParseInlineMarkdown(emitted),
					"parse(emit(parse(x))) must equal parse(x); emit produced %q", emitted,
				)

				return
			}

			got := emitInlineMarkdown(c.Input)
			assert.Equal(t, c.Expected, got)
		})
	}
}

func Test_openDelim(t *testing.T) {
	cc := map[string]struct {
		Mark   activeMark
		Result string
	}{
		"Bold":      {Mark: activeMark{kind: "bold"}, Result: "**"},
		"Italic":    {Mark: activeMark{kind: "italic"}, Result: "*"},
		"Underline": {Mark: activeMark{kind: "underline"}, Result: "_"},
		"Strike":    {Mark: activeMark{kind: "strike"}, Result: "~~"},
		"Code":      {Mark: activeMark{kind: "code"}, Result: "`"},
		"Link":      {Mark: activeMark{kind: "link", href: "https://x"}, Result: "["},
		"Unknown":   {Mark: activeMark{kind: "sparkle"}, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, openDelim(c.Mark))
		})
	}
}

func Test_closeDelim(t *testing.T) {
	cc := map[string]struct {
		Mark   activeMark
		Result string
	}{
		"Bold":      {Mark: activeMark{kind: "bold"}, Result: "**"},
		"Italic":    {Mark: activeMark{kind: "italic"}, Result: "*"},
		"Underline": {Mark: activeMark{kind: "underline"}, Result: "_"},
		"Strike":    {Mark: activeMark{kind: "strike"}, Result: "~~"},
		"Code":      {Mark: activeMark{kind: "code"}, Result: "`"},
		"Link":      {Mark: activeMark{kind: "link", href: "https://x"}, Result: "](https://x)"},
		"Link with a parenthesis in the href": {
			Mark:   activeMark{kind: "link", href: "https://x/a_(b)"},
			Result: `](https://x/a_(b\))`,
		},
		"Unknown": {Mark: activeMark{kind: "sparkle"}, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, closeDelim(c.Mark))
		})
	}
}

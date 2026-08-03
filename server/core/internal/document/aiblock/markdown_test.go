package aiblock

import (
	"testing"

	"github.com/oxynote/heimdall/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseInlineMarkdown(t *testing.T) {
	tests := map[string]struct {
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
		"Backtick inside link label": {
			Input: "[`x`](https://y)",
			Expected: []document.Block{
				{Type: "text", Text: "x", Marks: []document.Mark{
					{Type: "link", Attrs: map[string]any{"href": "https://y"}},
					{Type: "code"},
				}},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ParseInlineMarkdown(tc.Input)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func Test_EmitInlineMarkdown(t *testing.T) {
	tests := map[string]struct {
		Input    []document.Block
		Expected string
	}{
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := EmitInlineMarkdown(tc.Input)
			assert.Equal(t, tc.Expected, got)
		})
	}
}

func Test_InlineMarkdownRoundTrip(t *testing.T) {
	tests := map[string]struct {
		Input string
	}{
		"Plain text":            {Input: "plain text"},
		"Bold":                  {Input: "**bold**"},
		"Italic":                {Input: "*italic*"},
		"Underline":             {Input: "_underline_"},
		"Strike":                {Input: "~~strike~~"},
		"Code":                  {Input: "`code`"},
		"Link":                  {Input: "[label](https://x)"},
		"Mixed marks":           {Input: "mix **bold** and *italic* and `code`"},
		"Escaped literal":       {Input: `escaped \* literal`},
		"Link inside bold":      {Input: "**[link inside bold](https://y)**"},
		"Strike with code mark": {Input: "~~strike with `code` inside~~"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parsed := ParseInlineMarkdown(tc.Input)
			emitted := EmitInlineMarkdown(parsed)
			reparsed := ParseInlineMarkdown(emitted)

			require.Equal(t, parsed, reparsed,
				"parse(emit(parse(x))) must equal parse(x); emit produced %q", emitted,
			)
		})
	}
}

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_result(t *testing.T) {
	t.Parallel()

	// error
	_, err := result(make(chan int))
	assert.Error(t, err)

	// success
	res, err := result(map[string]any{"ok": true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, res)
}

func Test_subjectFor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "document", subjectFor(""))
	assert.Equal(t, "'Cat Facts'", subjectFor("'Cat Facts'"))
}

func Test_blockKindLabel(t *testing.T) {
	cc := map[string]struct {
		Kind   string
		Result string
	}{
		"Paragraph":       {Kind: "paragraph", Result: "a paragraph"},
		"Heading":         {Kind: "heading", Result: "a heading"},
		"Blockquote":      {Kind: "blockquote", Result: "a blockquote"},
		"Bullet list":     {Kind: "bullet_list", Result: "a bullet list"},
		"Ordered list":    {Kind: "ordered_list", Result: "an ordered list"},
		"Task list":       {Kind: "task_list", Result: "a task list"},
		"Callout":         {Kind: "callout", Result: "a callout"},
		"Code":            {Kind: "code", Result: "a code block"},
		"Titled code":     {Kind: "titled_code", Result: "a titled code block"},
		"Mermaid":         {Kind: "mermaid", Result: "a mermaid diagram"},
		"Horizontal rule": {Kind: "horizontal_rule", Result: "a divider"},
		"Image":           {Kind: "image", Result: "an image"},
		"Figma":           {Kind: "figma", Result: "a figma embed"},
		"Metric":          {Kind: "metric", Result: "a metric"},
		"Metric grid":     {Kind: "metric_grid", Result: "a metric grid"},
		"Split doc":       {Kind: "split_doc", Result: "a split documentation block"},
		"Param list":      {Kind: "split_doc_param_list", Result: "a parameter list"},
		"Empty":           {Kind: "", Result: "a block"},
		"Unknown":         {Kind: "wibble", Result: "a wibble block"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, blockKindLabel(c.Kind))
		})
	}
}

func Test_textPreview(t *testing.T) {
	cc := map[string]struct {
		Input  string
		MaxLen int
		Result string
	}{
		"Newlines collapse":  {Input: "a\nb\nc", MaxLen: 10, Result: "a b c"},
		"Whitespace trimmed": {Input: "  x  ", MaxLen: 10, Result: "x"},
		"Long text elided":   {Input: "abcdef", MaxLen: 3, Result: "abc…"},
		"Empty input":        {Input: "", MaxLen: 3, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, textPreview(c.Input, c.MaxLen))
		})
	}
}

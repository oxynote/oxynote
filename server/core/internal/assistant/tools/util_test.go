package tools

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
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

func Test_errRequired(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, errRequired("document_id"), "document_id is required")
}

func Test_blockKindLabel(t *testing.T) {
	cc := map[string]struct {
		Kind   block.Type
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

func Test_joinOpErrors(t *testing.T) {
	t.Parallel()

	got := joinOpErrors([]edit.OpError{
		{Index: 0, Message: "block_uid not found: a"},
		{Index: 1, Message: "something else"},
	})

	// each message is rewritten for the model and the index is left out,
	// since a tool ships one operation.
	assert.Equal(t, "no block with uid a in this document; call get_document for the current uids; something else", got)
}

func Test_describeOpError(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Msg    string
		Result string
	}{
		"Reference uid not found": {
			Msg:    "reference_uid not found: r1",
			Result: "no block with uid r1 in this document; call get_document for the current uids",
		},
		"Block uid not found": {
			Msg:    "block_uid not found: b1",
			Result: "no block with uid b1 in this document; call get_document for the current uids",
		},
		"Reference inside the moved block": {
			Msg:    "reference_uid is inside the moved block: r1",
			Result: "reference block r1 sits inside the block being moved; choose a reference outside it",
		},
		"Operation kind named as its tool": {
			Msg:    "update_text does not apply to calloutBlock: use replace_block to rewrite it whole, or update_text on the block holding the text.",
			Result: "update_block_text does not apply to calloutBlock: use replace_block to rewrite it whole, or update_block_text on the block holding the text.",
		},
		"Unknown message passes through": {
			Msg:    "cannot move a block relative to itself: a",
			Result: "cannot move a block relative to itself: a",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, describeOpError(c.Msg))
		})
	}
}

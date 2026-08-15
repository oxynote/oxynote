package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Validate(t *testing.T) {
	tests := map[string]struct {
		Input        Block
		ExpectErr    bool
		ExpectedPath string
	}{
		"Missing type": {
			Input:     Block{Text: "x"},
			ExpectErr: true,
		},
		"Unknown type": {
			Input:     Block{Type: "wibble"},
			ExpectErr: true,
		},
		"Paragraph with task_items is rejected": {
			Input:        Block{Type: BlockParagraph, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr:    true,
			ExpectedPath: "",
		},
		"Heading without level": {
			Input:        Block{Type: BlockHeading, Text: "x"},
			ExpectErr:    true,
			ExpectedPath: "attrs.level",
		},
		"Heading with invalid level": {
			Input:        Block{Type: BlockHeading, Text: "x", Attrs: map[string]any{"level": 4}},
			ExpectErr:    true,
			ExpectedPath: "attrs.level",
		},
		"Heading with valid level passes": {
			Input:     Block{Type: BlockHeading, Text: "x", Attrs: map[string]any{"level": 2}},
			ExpectErr: false,
		},
		"Bullet list requires items": {
			Input:     Block{Type: BlockBulletList},
			ExpectErr: true,
		},
		"Bullet list with non-paragraph item is rejected": {
			Input: Block{
				Type:  BlockBulletList,
				Items: []Block{{Type: BlockCode, Text: "x"}},
			},
			ExpectErr:    true,
			ExpectedPath: "items[0]",
		},
		"Task list requires task_items": {
			Input:     Block{Type: BlockTaskList},
			ExpectErr: true,
		},
		"Task list with empty content block is allowed only as paragraph": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{
					{Block: Block{Type: BlockCode, Text: "x"}},
				},
			},
			ExpectErr:    true,
			ExpectedPath: "task_items[0]/block",
		},
		"Callout requires text or items": {
			Input:     Block{Type: BlockCallout},
			ExpectErr: true,
		},
		"Callout with both text and items is rejected": {
			Input: Block{
				Type:  BlockCallout,
				Text:  "hi",
				Items: []Block{{Type: BlockParagraph, Text: "x"}},
			},
			ExpectErr: true,
		},
		"Titled code requires title": {
			Input:        Block{Type: BlockTitledCode, Text: "x"},
			ExpectErr:    true,
			ExpectedPath: "attrs.title",
		},
		"Image requires src": {
			Input:        Block{Type: BlockImage},
			ExpectErr:    true,
			ExpectedPath: "attrs.src",
		},
		"Figma requires src": {
			Input:        Block{Type: BlockFigma},
			ExpectErr:    true,
			ExpectedPath: "attrs.src",
		},
		"Metric grid requires metric items only": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{{Type: BlockParagraph, Text: "x"}},
			},
			ExpectErr:    true,
			ExpectedPath: "items[0]",
		},
		"Split doc requires left starting with heading": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockParagraph, Text: "x"}},
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			ExpectErr:    true,
			ExpectedPath: "left[0]",
		},
		"Split doc requires right of titled_code or metric": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
				Right: []Block{{Type: BlockParagraph, Text: "x"}},
			},
			ExpectErr:    true,
			ExpectedPath: "right[0]",
		},
		"Split doc heading must be level 1": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "API", Attrs: map[string]any{"level": 2}},
				},
				Right: []Block{
					{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "ex"}},
				},
			},
			ExpectErr:    true,
			ExpectedPath: "left[0].attrs.level",
		},
		"Split doc with valid structure passes": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "API", Attrs: map[string]any{"level": 1}},
					{Type: BlockParagraph, Text: "explain"},
				},
				Right: []Block{
					{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "ex"}},
				},
			},
			ExpectErr: false,
		},
		"Param list requires header and params": {
			Input:     Block{Type: BlockParamList},
			ExpectErr: true,
		},
		"Param list with empty name is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "  ", Type: "string"}},
			},
			ExpectErr:    true,
			ExpectedPath: "params[0]/name",
		},
		"Param list with valid rows passes": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "id", Type: "string", Description: "the id"}},
			},
			ExpectErr: false,
		},
		"Paragraph with items is rejected": {
			Input:     Block{Type: BlockParagraph, Items: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Paragraph with left is rejected": {
			Input:     Block{Type: BlockParagraph, Left: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Paragraph with params is rejected": {
			Input:     Block{Type: BlockParagraph, Header: "H", Params: []ParamItem{{Name: "x"}}},
			ExpectErr: true,
		},
		"Heading with task_items is rejected": {
			Input:     Block{Type: BlockHeading, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Blockquote with both text and items is rejected": {
			Input:     Block{Type: BlockBlockquote, Text: "q", Items: []Block{{Type: BlockParagraph, Text: "x"}}},
			ExpectErr: true,
		},
		"Blockquote without content is rejected": {
			Input:     Block{Type: BlockBlockquote},
			ExpectErr: true,
		},
		"Blockquote with task_items is rejected": {
			Input:     Block{Type: BlockBlockquote, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Blockquote with items passes": {
			Input:     Block{Type: BlockBlockquote, Items: []Block{{Type: BlockParagraph, Text: "x"}}},
			ExpectErr: false,
		},
		"Blockquote with disallowed item is rejected": {
			Input:        Block{Type: BlockBlockquote, Items: []Block{{Type: BlockCode, Text: "x"}}},
			ExpectErr:    true,
			ExpectedPath: "items[0]",
		},
		"Blockquote with invalid nested item is rejected": {
			Input:        Block{Type: BlockBlockquote, Items: []Block{{Type: BlockBulletList}}},
			ExpectErr:    true,
			ExpectedPath: "items[0]",
		},
		"Bullet list with text is rejected": {
			Input:     Block{Type: BlockBulletList, Text: "x", Items: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Bullet list with task_items is rejected": {
			Input:     Block{Type: BlockBulletList, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Ordered list without items is rejected": {
			Input:     Block{Type: BlockOrderedList},
			ExpectErr: true,
		},
		"Bullet list with paragraph items passes": {
			Input:     Block{Type: BlockBulletList, Items: []Block{{Type: BlockParagraph, Text: "x"}}},
			ExpectErr: false,
		},
		"Task list with text is rejected": {
			Input: Block{
				Type:      BlockTaskList,
				Text:      "x",
				TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}},
			},
			ExpectErr: true,
		},
		"Task list with left is rejected": {
			Input: Block{
				Type:      BlockTaskList,
				Left:      []Block{{Type: BlockParagraph}},
				TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}},
			},
			ExpectErr: true,
		},
		"Task list with invalid row block is rejected": {
			Input: Block{
				Type:      BlockTaskList,
				TaskItems: []TaskItem{{Block: Block{Type: BlockHeading, Text: "x"}}},
			},
			ExpectErr: true,
		},
		"Task list with valid rows passes": {
			Input: Block{
				Type:      BlockTaskList,
				TaskItems: []TaskItem{{Checked: true, Block: Block{Type: BlockParagraph, Text: "x"}}},
			},
			ExpectErr: false,
		},
		"Callout with params is rejected": {
			Input:     Block{Type: BlockCallout, Text: "x", Params: []ParamItem{{Name: "p"}}},
			ExpectErr: true,
		},
		"Callout with task_items is rejected": {
			Input:     Block{Type: BlockCallout, Text: "x", TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Metric grid with right is rejected": {
			Input:     Block{Type: BlockMetricGrid, Right: []Block{{Type: BlockMetric}}, Items: []Block{{Type: BlockMetric}}},
			ExpectErr: true,
		},
		"Callout with text passes": {
			Input:     Block{Type: BlockCallout, Text: "watch out"},
			ExpectErr: false,
		},
		"Callout with items passes": {
			Input:     Block{Type: BlockCallout, Items: []Block{{Type: BlockParagraph, Text: "x"}}},
			ExpectErr: false,
		},
		"Callout with disallowed item is rejected": {
			Input:        Block{Type: BlockCallout, Items: []Block{{Type: BlockMermaid, Text: "x"}}},
			ExpectErr:    true,
			ExpectedPath: "items[0]",
		},
		"Code with task_items is rejected": {
			Input:     Block{Type: BlockCode, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Code with items is rejected": {
			Input:     Block{Type: BlockCode, Items: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Code passes": {
			Input:     Block{Type: BlockCode, Text: "x := 1"},
			ExpectErr: false,
		},
		"Titled code with task_items is rejected": {
			Input:     Block{Type: BlockTitledCode, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Titled code with items is rejected": {
			Input:     Block{Type: BlockTitledCode, Items: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Titled code with whitespace title is rejected": {
			Input:        Block{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "   "}},
			ExpectErr:    true,
			ExpectedPath: "attrs.title",
		},
		"Titled code with title passes": {
			Input:     Block{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "ex.go"}},
			ExpectErr: false,
		},
		"Mermaid with task_items is rejected": {
			Input:     Block{Type: BlockMermaid, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Mermaid with items is rejected": {
			Input:     Block{Type: BlockMermaid, Items: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Mermaid passes": {
			Input:     Block{Type: BlockMermaid, Text: "graph TD;"},
			ExpectErr: false,
		},
		"Horizontal rule with task_items is rejected": {
			Input:     Block{Type: BlockHorizontalRule, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Horizontal rule with text is rejected": {
			Input:     Block{Type: BlockHorizontalRule, Text: "x"},
			ExpectErr: true,
		},
		"Horizontal rule passes": {
			Input:     Block{Type: BlockHorizontalRule},
			ExpectErr: false,
		},
		"Image with task_items is rejected": {
			Input:     Block{Type: BlockImage, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Image with text is rejected": {
			Input:     Block{Type: BlockImage, Text: "x", Attrs: map[string]any{"src": "http://x"}},
			ExpectErr: true,
		},
		"Image with src passes": {
			Input:     Block{Type: BlockImage, Attrs: map[string]any{"src": "http://x"}},
			ExpectErr: false,
		},
		"Figma with task_items is rejected": {
			Input:     Block{Type: BlockFigma, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Figma with text is rejected": {
			Input:     Block{Type: BlockFigma, Text: "x", Attrs: map[string]any{"src": "http://f"}},
			ExpectErr: true,
		},
		"Figma with src passes": {
			Input:     Block{Type: BlockFigma, Attrs: map[string]any{"src": "http://f"}},
			ExpectErr: false,
		},
		"Metric with task_items is rejected": {
			Input:     Block{Type: BlockMetric, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			ExpectErr: true,
		},
		"Metric with text is rejected": {
			Input:     Block{Type: BlockMetric, Text: "x"},
			ExpectErr: true,
		},
		"Metric passes": {
			Input:     Block{Type: BlockMetric, Attrs: map[string]any{"query": "up"}},
			ExpectErr: false,
		},
		"Metric grid with header is rejected": {
			Input:     Block{Type: BlockMetricGrid, Header: "H", Items: []Block{{Type: BlockMetric}}},
			ExpectErr: true,
		},
		"Metric grid with text is rejected": {
			Input:     Block{Type: BlockMetricGrid, Text: "x", Items: []Block{{Type: BlockMetric}}},
			ExpectErr: true,
		},
		"Metric grid without items is rejected": {
			Input:     Block{Type: BlockMetricGrid},
			ExpectErr: true,
		},
		"Metric grid with invalid metric is rejected": {
			Input:     Block{Type: BlockMetricGrid, Items: []Block{{Type: BlockMetric, Text: "x"}}},
			ExpectErr: true,
		},
		"Metric grid with metrics passes": {
			Input:     Block{Type: BlockMetricGrid, Items: []Block{{Type: BlockMetric}}},
			ExpectErr: false,
		},
		"Split doc with items is rejected": {
			Input:     Block{Type: BlockSplitDoc, Items: []Block{{Type: BlockParagraph}}},
			ExpectErr: true,
		},
		"Split doc with text is rejected": {
			Input:     Block{Type: BlockSplitDoc, Text: "x"},
			ExpectErr: true,
		},
		"Split doc without left is rejected": {
			Input: Block{
				Type:  BlockSplitDoc,
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			ExpectErr:    true,
			ExpectedPath: "left",
		},
		"Split doc without right is rejected": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
			},
			ExpectErr:    true,
			ExpectedPath: "right",
		},
		"Split doc with invalid left block is rejected": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
					{Type: BlockBulletList},
				},
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			ExpectErr:    true,
			ExpectedPath: "left[1]",
		},
		"Split doc body after param_list is rejected": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
					{Type: BlockParamList, Header: "H", Params: []ParamItem{{Name: "x"}}},
					{Type: BlockParagraph, Text: "late"},
				},
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			ExpectErr:    true,
			ExpectedPath: "left[2]",
		},
		"Split doc with disallowed left body is rejected": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
					{Type: BlockCode, Text: "x"},
				},
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			ExpectErr:    true,
			ExpectedPath: "left[1]",
		},
		"Split doc with invalid right block is rejected": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
				Right: []Block{{Type: BlockTitledCode, Text: "y"}},
			},
			ExpectErr: true,
		},
		"Split doc with trailing param_list passes": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
					{Type: BlockParagraph, Text: "intro"},
					{Type: BlockParamList, Header: "H", Params: []ParamItem{{Name: "x"}}},
				},
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			ExpectErr: false,
		},
		"Param list with items is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Header: "H",
				Items:  []Block{{Type: BlockParagraph}},
				Params: []ParamItem{{Name: "x"}},
			},
			ExpectErr: true,
		},
		"Param list with text is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Text:   "x",
				Header: "H",
				Params: []ParamItem{{Name: "x"}},
			},
			ExpectErr: true,
		},
		"Param list without params is rejected": {
			Input:        Block{Type: BlockParamList, Header: "H"},
			ExpectErr:    true,
			ExpectedPath: "params",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := Validate(tc.Input)
			if !tc.ExpectErr {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)

			var ve *validationError

			require.ErrorAs(t, err, &ve, "expected validationError, got %T", err)

			if tc.ExpectedPath != "" {
				assert.Equal(t, tc.ExpectedPath, ve.Path, "validationError path mismatch (full: %s)", ve.Error())
			}
		})
	}
}

func Test_ValidateAsRoot(t *testing.T) {
	tests := map[string]struct {
		Input     Block
		ExpectErr bool
	}{
		"Paragraph at root passes": {
			Input:     Block{Type: BlockParagraph, Text: "hi"},
			ExpectErr: false,
		},
		"Heading at root passes": {
			Input:     Block{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
			ExpectErr: false,
		},
		"Metric grid at root passes": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{{Type: BlockMetric}},
			},
			ExpectErr: false,
		},
		"Split doc at root passes": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{
					{Type: BlockHeading, Text: "API", Attrs: map[string]any{"level": 1}},
				},
				Right: []Block{
					{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "ex"}},
				},
			},
			ExpectErr: false,
		},
		"Titled code at root is rejected": {
			Input: Block{
				Type:  BlockTitledCode,
				Text:  "x",
				Attrs: map[string]any{"title": "ex"},
			},
			ExpectErr: true,
		},
		"Metric at root is rejected": {
			Input:     Block{Type: BlockMetric},
			ExpectErr: true,
		},
		"Param list at root is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "x"}},
			},
			ExpectErr: true,
		},
		"Invalid block fails before the root check": {
			Input:     Block{Type: BlockHeading, Text: "x"},
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAsRoot(tc.Input)
			if tc.ExpectErr {
				require.Error(t, err)

				var ve *validationError

				assert.ErrorAs(t, err, &ve, "expected validationError, got %T", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_validationError_Error(t *testing.T) {
	t.Parallel()

	// with path
	err := &validationError{Path: "items[0]", Message: "bad block"}
	assert.Equal(t, "items[0]: bad block", err.Error())

	// without path
	err = &validationError{Message: "bad block"}
	assert.Equal(t, "bad block", err.Error())
}

func Test_containerForType(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		Result string
	}{
		"Titled code": {Type: BlockTitledCode, Result: "split_doc.right"},
		"Metric":      {Type: BlockMetric, Result: "metric_grid or split_doc.right"},
		"Param list":  {Type: BlockParamList, Result: "split_doc.left"},
		"Fallback":    {Type: BlockParagraph, Result: "its parent macro"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, containerForType(c.Type))
		})
	}
}

package aiblock

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
			require.True(t, IsValidationError(err), "expected ValidationError, got %T", err)

			if tc.ExpectedPath != "" {
				var ve *ValidationError
				require.ErrorAs(t, err, &ve)
				assert.Equal(t, tc.ExpectedPath, ve.Path, "ValidationError path mismatch (full: %s)", ve.Error())
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAsRoot(tc.Input)
			if tc.ExpectErr {
				require.Error(t, err)
				assert.True(t, IsValidationError(err), "expected ValidationError, got %T", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

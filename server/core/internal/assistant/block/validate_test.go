package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Validate(t *testing.T) {
	cc := map[string]struct {
		Input        Block
		Err          error
		ExpectedPath string
	}{
		"Missing type": {
			Input: Block{Text: "x"},
			Err:   assert.AnError,
		},
		"Unknown type": {
			Input: Block{Type: "wibble"},
			Err:   assert.AnError,
		},
		"Paragraph with task_items is rejected": {
			Input:        Block{Type: BlockParagraph, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:          assert.AnError,
			ExpectedPath: "",
		},
		"Heading without level": {
			Input:        Block{Type: BlockHeading, Text: "x"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.level",
		},
		"Heading with invalid level": {
			Input:        Block{Type: BlockHeading, Text: "x", Attrs: map[string]any{"level": 4}},
			Err:          assert.AnError,
			ExpectedPath: "attrs.level",
		},
		"Heading with valid level passes": {
			Input: Block{Type: BlockHeading, Text: "x", Attrs: map[string]any{"level": 2}},
		},
		"Bullet list requires items": {
			Input: Block{Type: BlockBulletList},
			Err:   assert.AnError,
		},
		"Bullet list with non-paragraph item is rejected": {
			Input: Block{
				Type:  BlockBulletList,
				Items: []Block{{Type: BlockCode, Text: "x"}},
			},
			Err:          assert.AnError,
			ExpectedPath: "items[0]",
		},
		"Bullet list reports the index of the offending item": {
			Input: Block{
				Type: BlockBulletList,
				Items: []Block{
					{Type: BlockParagraph, Text: "a"},
					{Type: BlockParagraph, Text: "b"},
					{Type: BlockCode, Text: "x"},
				},
			},
			Err:          assert.AnError,
			ExpectedPath: "items[2]",
		},
		"Task list requires task_items": {
			Input: Block{Type: BlockTaskList},
			Err:   assert.AnError,
		},
		"Task list with empty content block is allowed only as paragraph": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{
					{Block: Block{Type: BlockCode, Text: "x"}},
				},
			},
			Err:          assert.AnError,
			ExpectedPath: "task_items[0]/block",
		},
		"Callout requires text or items": {
			Input: Block{Type: BlockCallout},
			Err:   assert.AnError,
		},
		"Callout with both text and items is rejected": {
			Input: Block{
				Type:  BlockCallout,
				Text:  "hi",
				Items: []Block{{Type: BlockParagraph, Text: "x"}},
			},
			Err: assert.AnError,
		},
		"Titled code requires title": {
			Input:        Block{Type: BlockTitledCode, Text: "x"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.title",
		},
		"Image requires src": {
			Input:        Block{Type: BlockImage},
			Err:          assert.AnError,
			ExpectedPath: "attrs.src",
		},
		"Figma requires src": {
			Input:        Block{Type: BlockFigma},
			Err:          assert.AnError,
			ExpectedPath: "attrs.src",
		},
		"Metric grid requires metric items only": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{{Type: BlockParagraph, Text: "x"}},
			},
			Err:          assert.AnError,
			ExpectedPath: "items[0]",
		},
		"Split doc requires left starting with heading": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockParagraph, Text: "x"}},
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			Err:          assert.AnError,
			ExpectedPath: "left[0]",
		},
		"Split doc requires right of titled_code or metric": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
				Right: []Block{{Type: BlockParagraph, Text: "x"}},
			},
			Err:          assert.AnError,
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
			Err:          assert.AnError,
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
		},
		"Param list requires header and params": {
			Input: Block{Type: BlockParamList},
			Err:   assert.AnError,
		},
		"Param list with empty name is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "  ", Type: "string"}},
			},
			Err:          assert.AnError,
			ExpectedPath: "params[0]/name",
		},
		"Param list with valid rows passes": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "id", Type: "string", Description: "the id"}},
			},
		},
		"Paragraph with items is rejected": {
			Input: Block{Type: BlockParagraph, Items: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Paragraph with left is rejected": {
			Input: Block{Type: BlockParagraph, Left: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Paragraph with params is rejected": {
			Input: Block{Type: BlockParagraph, Header: "H", Params: []ParamItem{{Name: "x"}}},
			Err:   assert.AnError,
		},
		"Heading with task_items is rejected": {
			Input: Block{Type: BlockHeading, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Blockquote with both text and items is rejected": {
			Input: Block{Type: BlockBlockquote, Text: "q", Items: []Block{{Type: BlockParagraph, Text: "x"}}},
			Err:   assert.AnError,
		},
		"Blockquote without content is rejected": {
			Input: Block{Type: BlockBlockquote},
			Err:   assert.AnError,
		},
		"Blockquote with task_items is rejected": {
			Input: Block{Type: BlockBlockquote, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Blockquote with items passes": {
			Input: Block{Type: BlockBlockquote, Items: []Block{{Type: BlockParagraph, Text: "x"}}},
		},
		"Blockquote with disallowed item is rejected": {
			Input:        Block{Type: BlockBlockquote, Items: []Block{{Type: BlockCode, Text: "x"}}},
			Err:          assert.AnError,
			ExpectedPath: "items[0]",
		},
		"Blockquote with invalid nested item is rejected": {
			Input:        Block{Type: BlockBlockquote, Items: []Block{{Type: BlockBulletList}}},
			Err:          assert.AnError,
			ExpectedPath: "items[0]",
		},
		"Bullet list with invalid nested item is rejected": {
			Input: Block{
				Type:  BlockBulletList,
				Items: []Block{{Type: BlockBulletList}},
			},
			Err:          assert.AnError,
			ExpectedPath: "items[0]",
		},
		"List item children are accepted": {
			Input: Block{
				Type: BlockBulletList,
				Items: []Block{{
					Type: BlockParagraph,
					Text: "one",
					Children: []Block{{
						Type:  BlockBulletList,
						Items: []Block{{Type: BlockParagraph, Text: "nested"}},
					}},
				}},
			},
		},
		"List item children of a disallowed type are rejected": {
			Input: Block{
				Type: BlockBulletList,
				Items: []Block{{
					Type:     BlockParagraph,
					Text:     "one",
					Children: []Block{{Type: BlockCode, Text: "x"}},
				}},
			},
			Err:          assert.AnError,
			ExpectedPath: "items[0]/children[0]",
		},
		"Task item children of a disallowed type are rejected": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{{
					Block: Block{
						Type:     BlockParagraph,
						Text:     "one",
						Children: []Block{{Type: BlockCode, Text: "x"}},
					},
				}},
			},
			Err:          assert.AnError,
			ExpectedPath: "task_items[0]/block/children[0]",
		},
		"Task item children are accepted": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{{
					Block: Block{
						Type: BlockParagraph,
						Text: "one",
						Children: []Block{{
							Type:  BlockBulletList,
							Items: []Block{{Type: BlockParagraph, Text: "nested"}},
						}},
					},
				}},
			},
		},
		// only a list or task-list item's content block carries children;
		// anywhere else they would be silently dropped by Expand.
		"Children outside a list item are rejected": {
			Input: Block{Type: BlockParagraph, Text: "x", Children: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Bullet list with text is rejected": {
			Input: Block{Type: BlockBulletList, Text: "x", Items: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Bullet list with task_items is rejected": {
			Input: Block{Type: BlockBulletList, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Ordered list without items is rejected": {
			Input: Block{Type: BlockOrderedList},
			Err:   assert.AnError,
		},
		"Bullet list with paragraph items passes": {
			Input: Block{Type: BlockBulletList, Items: []Block{{Type: BlockParagraph, Text: "x"}}},
		},
		"Task list with text is rejected": {
			Input: Block{
				Type:      BlockTaskList,
				Text:      "x",
				TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}},
			},
			Err: assert.AnError,
		},
		"Task list with left is rejected": {
			Input: Block{
				Type:      BlockTaskList,
				Left:      []Block{{Type: BlockParagraph}},
				TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}},
			},
			Err: assert.AnError,
		},
		"Task list with invalid row block is rejected": {
			Input: Block{
				Type:      BlockTaskList,
				TaskItems: []TaskItem{{Block: Block{Type: BlockHeading, Text: "x"}}},
			},
			Err: assert.AnError,
		},
		"Task list with valid rows passes": {
			Input: Block{
				Type:      BlockTaskList,
				TaskItems: []TaskItem{{Checked: true, Block: Block{Type: BlockParagraph, Text: "x"}}},
			},
		},
		"Callout with params is rejected": {
			Input: Block{Type: BlockCallout, Text: "x", Params: []ParamItem{{Name: "p"}}},
			Err:   assert.AnError,
		},
		"Callout with task_items is rejected": {
			Input: Block{Type: BlockCallout, Text: "x", TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Metric grid with right is rejected": {
			Input: Block{Type: BlockMetricGrid, Right: []Block{{Type: BlockMetric}}, Items: []Block{{Type: BlockMetric}}},
			Err:   assert.AnError,
		},
		"Callout with text passes": {
			Input: Block{Type: BlockCallout, Text: "watch out"},
		},
		"Callout with items passes": {
			Input: Block{Type: BlockCallout, Items: []Block{{Type: BlockParagraph, Text: "x"}}},
		},
		"Callout with disallowed item is rejected": {
			Input:        Block{Type: BlockCallout, Items: []Block{{Type: BlockMermaid, Text: "x"}}},
			Err:          assert.AnError,
			ExpectedPath: "items[0]",
		},
		"Code with task_items is rejected": {
			Input: Block{Type: BlockCode, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Code with items is rejected": {
			Input: Block{Type: BlockCode, Items: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Code passes": {
			Input: Block{Type: BlockCode, Text: "x := 1"},
		},
		"Titled code with task_items is rejected": {
			Input: Block{Type: BlockTitledCode, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Titled code with items is rejected": {
			Input: Block{Type: BlockTitledCode, Items: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Titled code with whitespace title is rejected": {
			Input:        Block{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "   "}},
			Err:          assert.AnError,
			ExpectedPath: "attrs.title",
		},
		"Titled code with title passes": {
			Input: Block{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"title": "ex.go"}},
		},
		"Mermaid with task_items is rejected": {
			Input: Block{Type: BlockMermaid, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Mermaid with items is rejected": {
			Input: Block{Type: BlockMermaid, Items: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Mermaid passes": {
			Input: Block{Type: BlockMermaid, Text: "graph TD;"},
		},
		"Horizontal rule with task_items is rejected": {
			Input: Block{Type: BlockHorizontalRule, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Horizontal rule with text is rejected": {
			Input: Block{Type: BlockHorizontalRule, Text: "x"},
			Err:   assert.AnError,
		},
		"Horizontal rule passes": {
			Input: Block{Type: BlockHorizontalRule},
		},
		"Image with task_items is rejected": {
			Input: Block{Type: BlockImage, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Image with text is rejected": {
			Input: Block{Type: BlockImage, Text: "x", Attrs: map[string]any{"src": "http://x"}},
			Err:   assert.AnError,
		},
		"Image with src passes": {
			Input: Block{Type: BlockImage, Attrs: map[string]any{"src": "http://x"}},
		},
		"Figma with task_items is rejected": {
			Input: Block{Type: BlockFigma, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Figma with text is rejected": {
			Input: Block{Type: BlockFigma, Text: "x", Attrs: map[string]any{"src": "http://f"}},
			Err:   assert.AnError,
		},
		"Figma with src passes": {
			Input: Block{Type: BlockFigma, Attrs: map[string]any{"src": "http://f"}},
		},
		"Metric with task_items is rejected": {
			Input: Block{Type: BlockMetric, TaskItems: []TaskItem{{Block: Block{Type: BlockParagraph}}}},
			Err:   assert.AnError,
		},
		"Metric with text is rejected": {
			Input: Block{Type: BlockMetric, Text: "x"},
			Err:   assert.AnError,
		},
		"Metric passes": {
			Input: Block{Type: BlockMetric, Attrs: map[string]any{"query": "up"}},
		},
		"Metric grid with header is rejected": {
			Input: Block{Type: BlockMetricGrid, Header: "H", Items: []Block{{Type: BlockMetric}}},
			Err:   assert.AnError,
		},
		"Metric grid with text is rejected": {
			Input: Block{Type: BlockMetricGrid, Text: "x", Items: []Block{{Type: BlockMetric}}},
			Err:   assert.AnError,
		},
		"Metric grid without items is rejected": {
			Input: Block{Type: BlockMetricGrid},
			Err:   assert.AnError,
		},
		"Metric grid with invalid metric is rejected": {
			Input: Block{Type: BlockMetricGrid, Items: []Block{{Type: BlockMetric, Text: "x"}}},
			Err:   assert.AnError,
		},
		"Metric grid with metrics passes": {
			Input: Block{Type: BlockMetricGrid, Items: []Block{{Type: BlockMetric}}},
		},
		"Split doc with items is rejected": {
			Input: Block{Type: BlockSplitDoc, Items: []Block{{Type: BlockParagraph}}},
			Err:   assert.AnError,
		},
		"Split doc with text is rejected": {
			Input: Block{Type: BlockSplitDoc, Text: "x"},
			Err:   assert.AnError,
		},
		"Split doc without left is rejected": {
			Input: Block{
				Type:  BlockSplitDoc,
				Right: []Block{{Type: BlockTitledCode, Text: "y", Attrs: map[string]any{"title": "ex"}}},
			},
			Err:          assert.AnError,
			ExpectedPath: "left",
		},
		"Split doc without right is rejected": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
			},
			Err:          assert.AnError,
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
			Err:          assert.AnError,
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
			Err:          assert.AnError,
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
			Err:          assert.AnError,
			ExpectedPath: "left[1]",
		},
		"Split doc with invalid right block is rejected": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
				Right: []Block{{Type: BlockTitledCode, Text: "y"}},
			},
			Err: assert.AnError,
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
		},
		"Param list with items is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Header: "H",
				Items:  []Block{{Type: BlockParagraph}},
				Params: []ParamItem{{Name: "x"}},
			},
			Err: assert.AnError,
		},
		"Param list with text is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Text:   "x",
				Header: "H",
				Params: []ParamItem{{Name: "x"}},
			},
			Err: assert.AnError,
		},
		"Param list without params is rejected": {
			Input:        Block{Type: BlockParamList, Header: "H"},
			Err:          assert.AnError,
			ExpectedPath: "params",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := Validate(c.Input)

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err == nil {
				return
			}

			var ve *validationError

			require.ErrorAs(t, err, &ve, "expected validationError, got %T", err)

			if c.ExpectedPath != "" {
				assert.Equal(t, c.ExpectedPath, ve.Path, "validationError path mismatch (full: %s)", ve.Error())
			}
		})
	}
}

func Test_ValidateAsRoot(t *testing.T) {
	cc := map[string]struct {
		Input Block
		Err   error
	}{
		"Paragraph at root passes": {
			Input: Block{Type: BlockParagraph, Text: "hi"},
		},
		"Heading at root passes": {
			Input: Block{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
		},
		"Metric grid at root passes": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{{Type: BlockMetric}},
			},
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
		},
		"Titled code at root is rejected": {
			Input: Block{
				Type:  BlockTitledCode,
				Text:  "x",
				Attrs: map[string]any{"title": "ex"},
			},
			Err: assert.AnError,
		},
		"Metric at root is rejected": {
			Input: Block{Type: BlockMetric},
			Err:   assert.AnError,
		},
		"Param list at root is rejected": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "x"}},
			},
			Err: assert.AnError,
		},
		"Invalid block fails before the root check": {
			Input: Block{Type: BlockHeading, Text: "x"},
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := ValidateAsRoot(c.Input)

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err == nil {
				return
			}

			var ve *validationError

			assert.ErrorAs(t, err, &ve, "expected validationError, got %T", err)
		})
	}
}

func Test_ValidateInContainer(t *testing.T) {
	titledCode := Block{
		Type:  BlockTitledCode,
		Text:  "x",
		Attrs: map[string]any{"title": "ex"},
	}
	paragraph := Block{Type: BlockParagraph, Text: "hi"}

	cc := map[string]struct {
		Container document.BlockNodeType
		Input     Block
		Err       error
	}{
		"Root container delegates to the root check": {
			Container: document.BlockNodeDoc,
			Input:     titledCode,
			Err:       assert.AnError,
		},
		"Root container passes a root block": {
			Container: document.BlockNodeDoc,
			Input:     paragraph,
		},
		"Invalid block fails before the container check": {
			Container: document.BlockNodeCalloutBlock,
			Input:     Block{Type: BlockHeading, Text: "x"},
			Err:       assert.AnError,
		},
		"Callout passes a paragraph": {
			Container: document.BlockNodeCalloutBlock,
			Input:     paragraph,
		},
		"Callout rejects a titled code": {
			Container: document.BlockNodeCalloutBlock,
			Input:     titledCode,
			Err:       assert.AnError,
		},
		"Callout rejects a nested callout": {
			Container: document.BlockNodeCalloutBlock,
			Input:     Block{Type: BlockCallout, Text: "note"},
			Err:       assert.AnError,
		},
		"List item passes a paragraph": {
			Container: document.BlockNodeListItem,
			Input:     paragraph,
		},
		"Task item rejects a heading": {
			Container: document.BlockNodeTaskItem,
			Input:     Block{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
			Err:       assert.AnError,
		},
		"Blockquote passes a bullet list": {
			Container: document.BlockNodeBlockquote,
			Input: Block{
				Type:  BlockBulletList,
				Items: []Block{{Type: BlockParagraph, Text: "a"}},
			},
		},
		"Split doc right passes a titled code": {
			Container: document.BlockNodeSplitDocRight,
			Input:     titledCode,
		},
		"Split doc right rejects a paragraph": {
			Container: document.BlockNodeSplitDocRight,
			Input:     paragraph,
			Err:       assert.AnError,
		},
		"Split doc left passes a param list": {
			Container: document.BlockNodeSplitDocLeft,
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "x"}},
			},
		},
		"Split doc left rejects a heading": {
			Container: document.BlockNodeSplitDocLeft,
			Input:     Block{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}},
			Err:       assert.AnError,
		},
		"Metric grid passes a metric": {
			Container: document.BlockNodeMetricGrid,
			Input:     Block{Type: BlockMetric},
		},
		"Metric grid rejects a paragraph": {
			Container: document.BlockNodeMetricGrid,
			Input:     paragraph,
			Err:       assert.AnError,
		},
		"Wrapper-only container rejects everything": {
			Container: document.BlockNodeBulletList,
			Input:     paragraph,
			Err:       assert.AnError,
		},
		"Macro internal rejects everything": {
			Container: document.BlockNodeTitledCodeBlock,
			Input:     paragraph,
			Err:       assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := ValidateInContainer(c.Container, c.Input)

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err == nil {
				return
			}

			var ve *validationError

			assert.ErrorAs(t, err, &ve, "expected validationError, got %T", err)
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

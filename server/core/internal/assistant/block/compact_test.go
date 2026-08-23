package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Compact(t *testing.T) {
	cc := map[string]struct {
		Input    document.Block
		Expected Block
		Err      error
	}{
		"Paragraph emits markdown": {
			Input: document.Block{
				Type:  document.BlockNodeParagraph,
				Attrs: map[string]any{"uid": "p1"},
				Content: []document.Block{
					{Type: "text", Text: "hi "},
					{Type: "text", Text: "bold", Marks: []document.Mark{{Type: "bold"}}},
				},
			},
			Expected: Block{Type: BlockParagraph, UID: "p1", Text: "hi **bold**"},
		},
		"Callout with single paragraph uses Text shorthand": {
			Input: document.Block{
				Type:  document.BlockNodeCalloutBlock,
				Attrs: map[string]any{"uid": "c1", "icon": "lucide:warning"},
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "watch out"}}},
				},
			},
			Expected: Block{
				Type:  BlockCallout,
				UID:   "c1",
				Text:  "watch out",
				Attrs: map[string]any{"icon": "lucide:warning"},
			},
		},
		"Split doc unwraps sides into Left and Right": {
			Input: document.Block{
				Type:  document.BlockNodeSplitDoc,
				Attrs: map[string]any{"uid": "s1", "inversed": true},
				Content: []document.Block{
					{
						Type: document.BlockNodeSplitDocLeft,
						Content: []document.Block{
							{Type: document.BlockNodeHeading, Attrs: map[string]any{"uid": "h1", "level": 2}, Content: []document.Block{{Type: "text", Text: "API"}}},
						},
					},
					{
						Type:    document.BlockNodeSplitDocRight,
						Content: []document.Block{},
					},
				},
			},
			Expected: Block{
				Type:  BlockSplitDoc,
				UID:   "s1",
				Attrs: map[string]any{"inversed": true},
				Left: []Block{
					{Type: BlockHeading, UID: "h1", Text: "API", Attrs: map[string]any{"level": 2}},
				},
			},
		},
		"Ordered list unwraps list items": {
			Input: document.Block{
				Type:  document.BlockNodeOrderedList,
				Attrs: map[string]any{"uid": "ol1"},
				Content: []document.Block{
					{
						Type: document.BlockNodeListItem,
						Content: []document.Block{
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "one"}}},
						},
					},
				},
			},
			Expected: Block{
				Type:  BlockOrderedList,
				UID:   "ol1",
				Items: []Block{{Type: BlockParagraph, Text: "one"}},
			},
		},
		"Empty list item becomes an empty paragraph": {
			Input: document.Block{
				Type:    document.BlockNodeBulletList,
				Content: []document.Block{{Type: document.BlockNodeListItem}},
			},
			Expected: Block{
				Type:  BlockBulletList,
				Items: []Block{{Type: BlockParagraph}},
			},
		},
		// The item wrapper holds a paragraph plus whatever is nested under
		// it; dropping the rest deletes it from the document on write-back.
		"List item keeps its nested list": {
			Input: document.Block{
				Type:  document.BlockNodeBulletList,
				Attrs: map[string]any{"uid": "l1"},
				Content: []document.Block{
					{
						Type: document.BlockNodeListItem,
						Content: []document.Block{
							{
								Type:    document.BlockNodeParagraph,
								Attrs:   map[string]any{"uid": "p1"},
								Content: []document.Block{{Type: "text", Text: "one"}},
							},
							{
								Type:  document.BlockNodeBulletList,
								Attrs: map[string]any{"uid": "l2"},
								Content: []document.Block{
									{
										Type: document.BlockNodeListItem,
										Content: []document.Block{
											{
												Type:    document.BlockNodeParagraph,
												Attrs:   map[string]any{"uid": "p2"},
												Content: []document.Block{{Type: "text", Text: "nested"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Expected: Block{
				Type: BlockBulletList,
				UID:  "l1",
				Items: []Block{{
					Type: BlockParagraph,
					UID:  "p1",
					Text: "one",
					Children: []Block{{
						Type:  BlockBulletList,
						UID:   "l2",
						Items: []Block{{Type: BlockParagraph, UID: "p2", Text: "nested"}},
					}},
				}},
			},
		},
		"Task item keeps its nested list": {
			Input: document.Block{
				Type:  document.BlockNodeTaskList,
				Attrs: map[string]any{"uid": "t1"},
				Content: []document.Block{
					{
						Type:  document.BlockNodeTaskItem,
						Attrs: map[string]any{"uid": "ti1", "checked": true},
						Content: []document.Block{
							{
								Type:    document.BlockNodeParagraph,
								Attrs:   map[string]any{"uid": "p1"},
								Content: []document.Block{{Type: "text", Text: "one"}},
							},
							{
								Type:  document.BlockNodeBulletList,
								Attrs: map[string]any{"uid": "l2"},
								Content: []document.Block{
									{
										Type: document.BlockNodeListItem,
										Content: []document.Block{
											{
												Type:    document.BlockNodeParagraph,
												Attrs:   map[string]any{"uid": "p2"},
												Content: []document.Block{{Type: "text", Text: "nested"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Expected: Block{
				Type: BlockTaskList,
				UID:  "t1",
				TaskItems: []TaskItem{{
					UID:     "ti1",
					Checked: true,
					Block: Block{
						Type: BlockParagraph,
						UID:  "p1",
						Text: "one",
						Children: []Block{{
							Type:  BlockBulletList,
							UID:   "l2",
							Items: []Block{{Type: BlockParagraph, UID: "p2", Text: "nested"}},
						}},
					},
				}},
			},
		},
		"List with non-listItem child fails": {
			Input: document.Block{
				Type:    document.BlockNodeBulletList,
				Content: []document.Block{{Type: document.BlockNodeParagraph}},
			},
			Err: assert.AnError,
		},
		"List item with unsupported content fails": {
			Input: document.Block{
				Type: document.BlockNodeBulletList,
				Content: []document.Block{
					{Type: document.BlockNodeListItem, Content: []document.Block{{Type: "weirdNode"}}},
				},
			},
			Err: assert.AnError,
		},
		"List item with unsupported nested child fails": {
			Input: document.Block{
				Type: document.BlockNodeBulletList,
				Content: []document.Block{
					{
						Type: document.BlockNodeListItem,
						Content: []document.Block{
							{Type: document.BlockNodeParagraph},
							{Type: "weirdNode"},
						},
					},
				},
			},
			Err: assert.AnError,
		},
		"Blockquote with multiple paragraphs falls back to items": {
			Input: document.Block{
				Type:  document.BlockNodeBlockquote,
				Attrs: map[string]any{"uid": "q1"},
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "a"}}},
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "b"}}},
				},
			},
			Expected: Block{
				Type: BlockBlockquote,
				UID:  "q1",
				Items: []Block{
					{Type: BlockParagraph, Text: "a"},
					{Type: BlockParagraph, Text: "b"},
				},
			},
		},
		"Blockquote with unsupported child fails": {
			Input: document.Block{
				Type:    document.BlockNodeBlockquote,
				Content: []document.Block{{Type: "weirdNode"}},
			},
			Err: assert.AnError,
		},
		"Task list with non-taskItem child fails": {
			Input: document.Block{
				Type:    document.BlockNodeTaskList,
				Content: []document.Block{{Type: document.BlockNodeParagraph}},
			},
			Err: assert.AnError,
		},
		"Task item without content becomes an empty paragraph": {
			Input: document.Block{
				Type: document.BlockNodeTaskList,
				Content: []document.Block{
					{Type: document.BlockNodeTaskItem, Attrs: map[string]any{"uid": "t1", "checked": true}},
				},
			},
			Expected: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{
					{UID: "t1", Checked: true, Block: Block{Type: BlockParagraph}},
				},
			},
		},
		"Task item with unsupported content fails": {
			Input: document.Block{
				Type: document.BlockNodeTaskList,
				Content: []document.Block{
					{Type: document.BlockNodeTaskItem, Content: []document.Block{{Type: "weirdNode"}}},
				},
			},
			Err: assert.AnError,
		},
		"Task item with unsupported nested child fails": {
			Input: document.Block{
				Type: document.BlockNodeTaskList,
				Content: []document.Block{
					{
						Type: document.BlockNodeTaskItem,
						Content: []document.Block{
							{Type: document.BlockNodeParagraph},
							{Type: "weirdNode"},
						},
					},
				},
			},
			Err: assert.AnError,
		},
		"Callout with multiple children falls back to items": {
			Input: document.Block{
				Type: document.BlockNodeCalloutBlock,
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "a"}}},
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "b"}}},
				},
			},
			Expected: Block{
				Type: BlockCallout,
				Items: []Block{
					{Type: BlockParagraph, Text: "a"},
					{Type: BlockParagraph, Text: "b"},
				},
			},
		},
		"Callout with unsupported child fails": {
			Input: document.Block{
				Type:    document.BlockNodeCalloutBlock,
				Content: []document.Block{{Type: "weirdNode"}},
			},
			Err: assert.AnError,
		},
		"Code without language omits attrs": {
			Input: document.Block{
				Type:    document.BlockNodeCodeBlock,
				Content: []document.Block{{Type: "text", Text: "x := 1"}},
			},
			Expected: Block{Type: BlockCode, Text: "x := 1"},
		},
		"Titled code ignores unknown children": {
			Input: document.Block{
				Type: document.BlockNodeTitledCodeBlock,
				Content: []document.Block{
					{Type: document.BlockNodeCodeBlockTitle, Content: []document.Block{{Type: "text", Text: "ex.go"}}},
					{Type: document.BlockNodeCodeBlock, Attrs: map[string]any{"language": "go"}, Content: []document.Block{{Type: "text", Text: "x"}}},
					{Type: "weirdNode"},
				},
			},
			Expected: Block{
				Type:  BlockTitledCode,
				Text:  "x",
				Attrs: map[string]any{"title": "ex.go", "language": "go"},
			},
		},
		"Image carries title and width attrs": {
			Input: document.Block{
				Type:  document.BlockNodeImageBlock,
				Attrs: map[string]any{"uid": "i1", "src": "http://x", "title": "shot", "width": 200},
			},
			Expected: Block{
				Type:  BlockImage,
				UID:   "i1",
				Attrs: map[string]any{"src": "http://x", "title": "shot", "width": 200},
			},
		},
		"Metric passes attrs through without uid": {
			Input: document.Block{
				Type:  document.BlockNodeMetricBlock,
				Attrs: map[string]any{"uid": "m1", "query": "up", "unit": "%"},
			},
			Expected: Block{
				Type:  BlockMetric,
				UID:   "m1",
				Attrs: map[string]any{"query": "up", "unit": "%"},
			},
		},
		// a metric written before this layer knew the configuration: a
		// legacy config blob, explicit nulls for unset fields, and an
		// attribute it still does not name. Reading it must leave all
		// of them alone, or typing the schema would break documents
		// already in the system.
		"Metric keeps a legacy config blob, nulls and unknown attrs": {
			Input: document.Block{
				Type: document.BlockNodeMetricBlock,
				Attrs: map[string]any{
					"uid":               "m1",
					"config":            map[string]any{"type": "line_chart"},
					"visualizationType": nil,
					"wibble":            42,
				},
			},
			Expected: Block{
				Type: BlockMetric,
				UID:  "m1",
				Attrs: map[string]any{
					"config":            map[string]any{"type": "line_chart"},
					"visualizationType": nil,
					"wibble":            42,
				},
			},
		},
		"Metric grid wraps metric items": {
			Input: document.Block{
				Type: document.BlockNodeMetricGrid,
				Content: []document.Block{
					{Type: document.BlockNodeMetricBlock, Attrs: map[string]any{"uid": "m1", "query": "up"}},
				},
			},
			Expected: Block{
				Type: BlockMetricGrid,
				Items: []Block{
					{Type: BlockMetric, UID: "m1", Attrs: map[string]any{"query": "up"}},
				},
			},
		},
		"Metric grid with unsupported child fails": {
			Input: document.Block{
				Type:    document.BlockNodeMetricGrid,
				Content: []document.Block{{Type: "weirdNode"}},
			},
			Err: assert.AnError,
		},
		"Split doc left failure propagates": {
			Input: document.Block{
				Type: document.BlockNodeSplitDoc,
				Content: []document.Block{
					{Type: document.BlockNodeSplitDocLeft, Content: []document.Block{{Type: "weirdNode"}}},
				},
			},
			Err: assert.AnError,
		},
		"Split doc right failure propagates": {
			Input: document.Block{
				Type: document.BlockNodeSplitDoc,
				Content: []document.Block{
					{Type: document.BlockNodeSplitDocRight, Content: []document.Block{{Type: "weirdNode"}}},
				},
			},
			Err: assert.AnError,
		},
		"Split doc ignores unknown side nodes": {
			Input: document.Block{
				Type:    document.BlockNodeSplitDoc,
				Content: []document.Block{{Type: "weirdSide"}},
			},
			Expected: Block{Type: BlockSplitDoc},
		},
		"Unsupported node type fails": {
			Input: document.Block{Type: "weirdNode"},
			Err:   assert.AnError,
		},
		"Param list ignores unknown children at every level": {
			Input: document.Block{
				Type: document.BlockNodeParamList,
				Content: []document.Block{
					{Type: document.BlockNodeParamListHeader, Content: []document.Block{{Type: "text", Text: "Body"}}},
					{
						Type: document.BlockNodeParamListItem,
						Content: []document.Block{
							{
								Type: document.BlockNodeParamListItemHeader,
								Content: []document.Block{
									{Type: document.BlockNodeParamListItemTitle, Content: []document.Block{{Type: "text", Text: "id"}}},
									{Type: "weirdSub"},
								},
							},
							{Type: "weirdChild"},
						},
					},
					{Type: "weirdRow"},
				},
			},
			Expected: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{{Name: "id"}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := Compact(c.Input)

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err != nil {
				return
			}

			assert.Equal(t, c.Expected, got)
		})
	}
}

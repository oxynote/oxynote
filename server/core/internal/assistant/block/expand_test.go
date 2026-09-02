package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripUIDsPM recursively removes uid attributes throughout a
// document.Block tree so structural equality assertions don't have
// to know which uids will be generated. It returns a transformed
// copy and leaves the input untouched.
func stripUIDsPM(b document.Block) document.Block {
	out := document.Block{
		Type:  b.Type,
		Text:  b.Text,
		Marks: b.Marks,
	}

	if b.Attrs != nil {
		na := make(map[string]any, len(b.Attrs))
		for k, v := range b.Attrs {
			if k == document.AttrUID {
				continue
			}

			na[k] = v
		}

		if len(na) > 0 {
			out.Attrs = na
		}
	}

	if len(b.Content) > 0 {
		out.Content = make([]document.Block, len(b.Content))
		for i, c := range b.Content {
			out.Content[i] = stripUIDsPM(c)
		}
	}

	return out
}

// stripUIDsCanonical strips UID fields throughout a canonical Block
// tree (Block.UID, TaskItem.UID, ParamItem.UID) so round-trip
// assertions can ignore generated identifiers. It rewrites the
// nested Items/Children/TaskItems/Left/Right/Params slices in
// place, so the input tree is stripped as a side effect.
func stripUIDsCanonical(b Block) Block {
	b.UID = ""

	for i, c := range b.Items {
		b.Items[i] = stripUIDsCanonical(c)
	}

	for i, c := range b.Children {
		b.Children[i] = stripUIDsCanonical(c)
	}

	for i, t := range b.TaskItems {
		b.TaskItems[i] = TaskItem{
			Checked: t.Checked,
			Block:   stripUIDsCanonical(t.Block),
		}
	}

	for i, l := range b.Left {
		b.Left[i] = stripUIDsCanonical(l)
	}

	for i, r := range b.Right {
		b.Right[i] = stripUIDsCanonical(r)
	}

	for i, p := range b.Params {
		p.UID = ""
		b.Params[i] = p
	}

	return b
}

func Test_Expand(t *testing.T) {
	cc := map[string]struct {
		Input    Block
		Expected document.Block
		Err      error

		// RoundTrip asserts that compacting the expanded block returns the
		// input instead of comparing against Expected.
		RoundTrip bool

		// Check replaces the expected-block comparison for a case that
		// asserts on something the comparison strips, such as the uid.
		Check func(t *testing.T, got document.Block)
	}{
		"Round trip: paragraph":  {Input: Block{Type: BlockParagraph, Text: "hi **there**"}, RoundTrip: true},
		"Round trip: heading":    {Input: Block{Type: BlockHeading, Text: "Title", Attrs: map[string]any{"level": 1}}, RoundTrip: true},
		"Round trip: blockquote": {Input: Block{Type: BlockBlockquote, Text: "quote"}, RoundTrip: true},
		"Round trip: blockquote with items": {RoundTrip: true, Input: Block{
			Type: BlockBlockquote,
			Items: []Block{
				{Type: BlockParagraph, Text: "one"},
				{Type: BlockParagraph, Text: "two"},
			},
		}},
		"Round trip: bullet list with a nested list": {RoundTrip: true, Input: Block{
			Type: BlockBulletList,
			Items: []Block{
				{
					Type: BlockParagraph,
					Text: "one",
					Children: []Block{{
						Type:  BlockBulletList,
						Items: []Block{{Type: BlockParagraph, Text: "nested"}},
					}},
				},
				{Type: BlockParagraph, Text: "two"},
			},
		}},
		"Round trip: task list with a nested list": {RoundTrip: true, Input: Block{
			Type: BlockTaskList,
			TaskItems: []TaskItem{{
				Checked: true,
				Block: Block{
					Type: BlockParagraph,
					Text: "done",
					Children: []Block{{
						Type:  BlockBulletList,
						Items: []Block{{Type: BlockParagraph, Text: "nested"}},
					}},
				},
			}},
		}},
		"Round trip: bullet list": {RoundTrip: true, Input: Block{
			Type:  BlockBulletList,
			Items: []Block{{Type: BlockParagraph, Text: "one"}, {Type: BlockParagraph, Text: "two"}},
		}},
		"Round trip: task list": {RoundTrip: true, Input: Block{
			Type: BlockTaskList,
			TaskItems: []TaskItem{
				{Checked: true, Block: Block{Type: BlockParagraph, Text: "done"}},
			},
		}},
		"Round trip: callout text shorthand": {RoundTrip: true, Input: Block{
			Type:  BlockCallout,
			Text:  "warn",
			Attrs: map[string]any{"icon": "lucide:warning"},
		}},
		"Round trip: code": {RoundTrip: true, Input: Block{
			Type:  BlockCode,
			Text:  "x := 1",
			Attrs: map[string]any{"language": "go"},
		}},
		"Round trip: titled code": {RoundTrip: true, Input: Block{
			Type:  BlockTitledCode,
			Text:  "x := 1",
			Attrs: map[string]any{"title": "ex.go", "language": "go"},
		}},
		"Round trip: mermaid":         {Input: Block{Type: BlockMermaid, Text: "graph TD; A-->B;"}, RoundTrip: true},
		"Round trip: horizontal rule": {Input: Block{Type: BlockHorizontalRule}, RoundTrip: true},
		"Round trip: image": {RoundTrip: true, Input: Block{
			Type:  BlockImage,
			Attrs: map[string]any{"src": "http://x", "alt": "x", "width": 100},
		}},
		"Round trip: figma": {RoundTrip: true, Input: Block{
			Type:  BlockFigma,
			Attrs: map[string]any{"src": "http://figma", "width": 320, "height": 200},
		}},
		"Round trip: split doc": {RoundTrip: true, Input: Block{
			Type:  BlockSplitDoc,
			Attrs: map[string]any{"inversed": true},
			Left: []Block{
				{Type: BlockHeading, Text: "API", Attrs: map[string]any{"level": 1}},
				{Type: BlockParagraph, Text: "explain"},
			},
			Right: []Block{
				{Type: BlockTitledCode, Text: "ok", Attrs: map[string]any{"title": "example"}},
			},
		}},
		"Round trip: param list": {RoundTrip: true, Input: Block{
			Type:   BlockParamList,
			Header: "Body",
			Params: []ParamItem{
				{Name: "id", Type: "string", Description: "the **user** id"},
				{Name: "limit", Type: "number?", Description: "page size"},
			},
		}},
		"An explicit uid is preserved": {
			Input: Block{Type: BlockParagraph, UID: "preserve-me", Text: "x"},
			Check: func(t *testing.T, got document.Block) {
				uid, ok := got.UID()
				require.True(t, ok)
				assert.Equal(t, "preserve-me", uid)
			},
		},
		"A missing uid is generated": {
			Input: Block{Type: BlockParagraph, Text: "x"},
			Check: func(t *testing.T, got document.Block) {
				uid, ok := got.UID()
				require.True(t, ok)
				assert.NotEmpty(t, uid)
			},
		},
		"Paragraph with bold mark": {
			Input: Block{Type: BlockParagraph, Text: "say **hi**"},
			Expected: document.Block{
				Type: document.BlockNodeParagraph,
				Content: []document.Block{
					{Type: "text", Text: "say "},
					{Type: "text", Text: "hi", Marks: []document.Mark{{Type: "bold"}}},
				},
			},
		},
		"Heading carries level attr": {
			Input: Block{Type: BlockHeading, Text: "Title", Attrs: map[string]any{"level": 2}},
			Expected: document.Block{
				Type:  document.BlockNodeHeading,
				Attrs: map[string]any{"level": 2},
				Content: []document.Block{
					{Type: "text", Text: "Title"},
				},
			},
		},
		"Blockquote wraps paragraph": {
			Input: Block{Type: BlockBlockquote, Text: "quote"},
			Expected: document.Block{
				Type: document.BlockNodeBlockquote,
				Content: []document.Block{
					{
						Type:    document.BlockNodeParagraph,
						Content: []document.Block{{Type: "text", Text: "quote"}},
					},
				},
			},
		},
		"Blockquote wraps items": {
			Input: Block{
				Type: BlockBlockquote,
				Items: []Block{
					{Type: BlockParagraph, Text: "one"},
					{Type: BlockParagraph, Text: "two"},
				},
			},
			Expected: document.Block{
				Type: document.BlockNodeBlockquote,
				Content: []document.Block{
					{
						Type:    document.BlockNodeParagraph,
						Content: []document.Block{{Type: "text", Text: "one"}},
					},
					{
						Type:    document.BlockNodeParagraph,
						Content: []document.Block{{Type: "text", Text: "two"}},
					},
				},
			},
		},
		"Bullet list wraps each item in listItem > paragraph": {
			Input: Block{
				Type: BlockBulletList,
				Items: []Block{
					{Type: BlockParagraph, Text: "first"},
					{Type: BlockParagraph, Text: "second"},
				},
			},
			Expected: document.Block{
				Type: document.BlockNodeBulletList,
				Content: []document.Block{
					{
						Type: document.BlockNodeListItem,
						Content: []document.Block{
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "first"}}},
						},
					},
					{
						Type: document.BlockNodeListItem,
						Content: []document.Block{
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "second"}}},
						},
					},
				},
			},
		},
		"Task list carries checked flag on each task item": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{
					{Checked: true, Block: Block{Type: BlockParagraph, Text: "done"}},
					{Checked: false, Block: Block{Type: BlockParagraph, Text: "todo"}},
				},
			},
			Expected: document.Block{
				Type: document.BlockNodeTaskList,
				Content: []document.Block{
					{
						Type:  document.BlockNodeTaskItem,
						Attrs: map[string]any{"checked": true},
						Content: []document.Block{
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "done"}}},
						},
					},
					{
						Type:  document.BlockNodeTaskItem,
						Attrs: map[string]any{"checked": false},
						Content: []document.Block{
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "todo"}}},
						},
					},
				},
			},
		},
		"Callout text shorthand wraps a paragraph": {
			Input: Block{Type: BlockCallout, Text: "warn", Attrs: map[string]any{"icon": "lucide:warning"}},
			Expected: document.Block{
				Type:  document.BlockNodeCalloutBlock,
				Attrs: map[string]any{"icon": "lucide:warning"},
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "warn"}}},
				},
			},
		},
		"Code is emitted raw without markdown parsing": {
			Input: Block{Type: BlockCode, Text: "**not** bold", Attrs: map[string]any{"language": "go"}},
			Expected: document.Block{
				Type:    document.BlockNodeCodeBlock,
				Attrs:   map[string]any{"language": "go"},
				Content: []document.Block{{Type: "text", Text: "**not** bold"}},
			},
		},
		"Titled code expands to title + code children": {
			Input: Block{
				Type:  BlockTitledCode,
				Text:  "x := 1",
				Attrs: map[string]any{"title": "example.go", "language": "go"},
			},
			Expected: document.Block{
				Type: document.BlockNodeTitledCodeBlock,
				Content: []document.Block{
					{Type: document.BlockNodeCodeBlockTitle, Content: []document.Block{{Type: "text", Text: "example.go"}}},
					{Type: document.BlockNodeCodeBlock, Attrs: map[string]any{"language": "go"}, Content: []document.Block{{Type: "text", Text: "x := 1"}}},
				},
			},
		},
		"Mermaid is raw text": {
			Input: Block{Type: BlockMermaid, Text: "graph TD; A-->B;"},
			Expected: document.Block{
				Type:    document.BlockNodeMermaidBlock,
				Content: []document.Block{{Type: "text", Text: "graph TD; A-->B;"}},
			},
		},
		"Horizontal rule has no content": {
			Input:    Block{Type: BlockHorizontalRule},
			Expected: document.Block{Type: document.BlockNodeHorizontalRule},
		},
		"Image carries src and width attrs": {
			Input: Block{Type: BlockImage, Attrs: map[string]any{"src": "http://x", "width": 200}},
			Expected: document.Block{
				Type:  document.BlockNodeImageBlock,
				Attrs: map[string]any{"src": "http://x", "width": 200},
			},
		},
		"Split doc builds left and right wrappers": {
			Input: Block{
				Type:  BlockSplitDoc,
				Attrs: map[string]any{"inversed": true},
				Left: []Block{
					{Type: BlockHeading, Text: "API", Attrs: map[string]any{"level": 1}},
					{Type: BlockParagraph, Text: "intro"},
				},
				Right: []Block{
					{Type: BlockTitledCode, Text: "ok", Attrs: map[string]any{"title": "example"}},
				},
			},
			Expected: document.Block{
				Type:  document.BlockNodeSplitDoc,
				Attrs: map[string]any{"inversed": true},
				Content: []document.Block{
					{
						Type: document.BlockNodeSplitDocLeft,
						Content: []document.Block{
							{Type: document.BlockNodeHeading, Attrs: map[string]any{"level": 1}, Content: []document.Block{{Type: "text", Text: "API"}}},
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "intro"}}},
						},
					},
					{
						Type: document.BlockNodeSplitDocRight,
						Content: []document.Block{
							{
								Type: document.BlockNodeTitledCodeBlock,
								Content: []document.Block{
									{Type: document.BlockNodeCodeBlockTitle, Content: []document.Block{{Type: "text", Text: "example"}}},
									{Type: document.BlockNodeCodeBlock, Content: []document.Block{{Type: "text", Text: "ok"}}},
								},
							},
						},
					},
				},
			},
		},
		"Param list builds header and full item nesting": {
			Input: Block{
				Type:   BlockParamList,
				Header: "Body",
				Params: []ParamItem{
					{Name: "id", Type: "string", Description: "the user id"},
				},
			},
			Expected: document.Block{
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
									{Type: document.BlockNodeParamListItemType, Content: []document.Block{{Type: "text", Text: "string"}}},
								},
							},
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "the user id"}}},
						},
					},
				},
			},
		},
		"Ordered list wraps items in listItem": {
			Input: Block{
				Type:  BlockOrderedList,
				Items: []Block{{Type: BlockParagraph, Text: "one"}},
			},
			Expected: document.Block{
				Type: document.BlockNodeOrderedList,
				Content: []document.Block{
					{
						Type: document.BlockNodeListItem,
						Content: []document.Block{
							{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "one"}}},
						},
					},
				},
			},
		},
		"Heading without level defaults to 1": {
			Input: Block{Type: BlockHeading, Text: "T"},
			Expected: document.Block{
				Type:    document.BlockNodeHeading,
				Attrs:   map[string]any{"level": 1},
				Content: []document.Block{{Type: "text", Text: "T"}},
			},
		},
		"Callout without icon uses the default": {
			Input: Block{Type: BlockCallout, Text: "note"},
			Expected: document.Block{
				Type:  document.BlockNodeCalloutBlock,
				Attrs: map[string]any{"icon": "lucide:text"},
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "note"}}},
				},
			},
		},
		"Callout with items expands each": {
			Input: Block{
				Type:  BlockCallout,
				Attrs: map[string]any{"icon": "lucide:warning"},
				Items: []Block{
					{Type: BlockParagraph, Text: "a"},
					{Type: BlockParagraph, Text: "b"},
				},
			},
			Expected: document.Block{
				Type:  document.BlockNodeCalloutBlock,
				Attrs: map[string]any{"icon": "lucide:warning"},
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "a"}}},
					{Type: document.BlockNodeParagraph, Content: []document.Block{{Type: "text", Text: "b"}}},
				},
			},
		},
		"Callout with invalid item fails": {
			Input: Block{
				Type:  BlockCallout,
				Items: []Block{{Type: "not_a_type"}},
			},
			Err: assert.AnError,
		},
		"Blockquote with invalid item fails": {
			Input: Block{
				Type:  BlockBlockquote,
				Items: []Block{{Type: "not_a_type"}},
			},
			Err: assert.AnError,
		},
		"Bullet list with invalid item fails": {
			Input: Block{
				Type:  BlockBulletList,
				Items: []Block{{Type: "not_a_type"}},
			},
			Err: assert.AnError,
		},
		"Bullet list with invalid nested child fails": {
			Input: Block{
				Type: BlockBulletList,
				Items: []Block{{
					Type:     BlockParagraph,
					Text:     "one",
					Children: []Block{{Type: "not_a_type"}},
				}},
			},
			Err: assert.AnError,
		},
		"Task list with invalid item fails": {
			Input: Block{
				Type:      BlockTaskList,
				TaskItems: []TaskItem{{Block: Block{Type: "not_a_type"}}},
			},
			Err: assert.AnError,
		},
		"Task list with invalid nested child fails": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{{
					Block: Block{
						Type:     BlockParagraph,
						Text:     "one",
						Children: []Block{{Type: "not_a_type"}},
					},
				}},
			},
			Err: assert.AnError,
		},
		"Metric passes attrs through": {
			Input: Block{Type: BlockMetric, Attrs: map[string]any{"query": "up"}},
			Expected: document.Block{
				Type:  document.BlockNodeMetricBlock,
				Attrs: map[string]any{"query": "up"},
			},
		},
		"Metric keeps an active simulation": {
			Input: Block{
				Type:  BlockMetric,
				Attrs: map[string]any{"simulationPreset": "http_latency"},
			},
			Expected: document.Block{
				Type:  document.BlockNodeMetricBlock,
				Attrs: map[string]any{"simulationPreset": "http_latency"},
			},
		},
		// the other half of the legacy round trip: what Compact read
		// back out has to expand into exactly what it came from.
		"Metric keeps a legacy config blob, nulls and unknown attrs": {
			Input: Block{
				Type: BlockMetric,
				UID:  "m1",
				Attrs: map[string]any{
					"config":            map[string]any{"type": "line_chart"},
					"visualizationType": nil,
					"wibble":            42,
				},
			},
			// the harness strips uids, so what is asserted here is that
			// every other attribute survived untouched; the uid's own
			// preservation is covered by the cases that check it.
			Expected: document.Block{
				Type: document.BlockNodeMetricBlock,
				Attrs: map[string]any{
					"config":            map[string]any{"type": "line_chart"},
					"visualizationType": nil,
					"wibble":            42,
				},
			},
		},
		"Metric grid wraps metric items": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{{Type: BlockMetric, Attrs: map[string]any{"query": "up"}}},
			},
			Expected: document.Block{
				Type: document.BlockNodeMetricGrid,
				Content: []document.Block{
					{Type: document.BlockNodeMetricBlock, Attrs: map[string]any{"query": "up"}},
				},
			},
		},
		"Metric grid with invalid item fails": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{{Type: "not_a_type"}},
			},
			Err: assert.AnError,
		},
		"Split doc with invalid left fails": {
			Input: Block{
				Type: BlockSplitDoc,
				Left: []Block{{Type: "not_a_type"}},
			},
			Err: assert.AnError,
		},
		"Split doc with invalid right fails": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockHeading, Text: "T", Attrs: map[string]any{"level": 1}}},
				Right: []Block{{Type: "not_a_type"}},
			},
			Err: assert.AnError,
		},
		"Split doc with empty sides yields empty wrappers": {
			Input: Block{Type: BlockSplitDoc},
			Expected: document.Block{
				Type: document.BlockNodeSplitDoc,
				Content: []document.Block{
					{Type: document.BlockNodeSplitDocLeft},
					{Type: document.BlockNodeSplitDocRight},
				},
			},
		},
		"Image carries title and alt attrs": {
			Input: Block{Type: BlockImage, Attrs: map[string]any{"src": "http://x", "alt": "pic", "title": "shot"}},
			Expected: document.Block{
				Type:  document.BlockNodeImageBlock,
				Attrs: map[string]any{"src": "http://x", "alt": "pic", "title": "shot"},
			},
		},
		"Titled code without title has an empty title node": {
			Input: Block{Type: BlockTitledCode, Text: "x", Attrs: map[string]any{"language": "go"}},
			Expected: document.Block{
				Type: document.BlockNodeTitledCodeBlock,
				Content: []document.Block{
					{Type: document.BlockNodeCodeBlockTitle},
					{Type: document.BlockNodeCodeBlock, Attrs: map[string]any{"language": "go"}, Content: []document.Block{{Type: "text", Text: "x"}}},
				},
			},
		},
		"Unknown block type fails": {
			Input: Block{Type: "not_a_type"},
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := Expand(c.Input)

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err != nil {
				return
			}

			if c.RoundTrip {
				require.NoError(t, Validate(c.Input), "input should validate")

				compacted, cerr := Compact(got)
				require.NoError(t, cerr)

				assert.Equal(t,
					stripUIDsCanonical(c.Input),
					stripUIDsCanonical(compacted),
				)

				return
			}

			if c.Check != nil {
				c.Check(t, got)

				return
			}

			assert.Equal(t, c.Expected, stripUIDsPM(got))
		})
	}
}

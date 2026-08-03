package aiblock

import (
	"testing"

	"github.com/oxynote/heimdall/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripUIDsPM recursively zeroes UID attributes throughout a
// document.Block tree so structural equality assertions don't have
// to know which uids will be generated. The function is destructive
// on the input copy semantics that callers should provide — it
// returns a transformed copy.
func stripUIDsPM(b document.Block) document.Block {
	out := document.Block{
		Type:  b.Type,
		Text:  b.Text,
		Marks: b.Marks,
	}

	if b.Attrs != nil {
		na := make(map[string]any, len(b.Attrs))
		for k, v := range b.Attrs {
			if k == _attrUID {
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

// stripUIDsCanonical strips UID fields throughout an aiblock Block
// tree (Block.UID, TaskItem.UID, ParamItem.UID) so round-trip
// assertions can ignore generated identifiers.
func stripUIDsCanonical(b Block) Block {
	b.UID = ""

	for i, c := range b.Items {
		b.Items[i] = stripUIDsCanonical(c)
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
	tests := map[string]struct {
		Input    Block
		Expected document.Block
	}{
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := Expand(tc.Input)
			require.NoError(t, err)

			assert.Equal(t, tc.Expected, stripUIDsPM(got))
		})
	}
}

func Test_Expand_PreservesUID(t *testing.T) {
	t.Parallel()

	got, err := Expand(Block{Type: BlockParagraph, UID: "preserve-me", Text: "x"})
	require.NoError(t, err)

	uid, ok := got.UID()
	require.True(t, ok)
	assert.Equal(t, "preserve-me", uid)
}

func Test_Expand_GeneratesUID(t *testing.T) {
	t.Parallel()

	got, err := Expand(Block{Type: BlockParagraph, Text: "x"})
	require.NoError(t, err)

	uid, ok := got.UID()
	require.True(t, ok)
	assert.NotEmpty(t, uid)
}

func Test_Expand_RoundTrip(t *testing.T) {
	tests := map[string]struct {
		Input Block
	}{
		"Paragraph":  {Input: Block{Type: BlockParagraph, Text: "hi **there**"}},
		"Heading":    {Input: Block{Type: BlockHeading, Text: "Title", Attrs: map[string]any{"level": 1}}},
		"Blockquote": {Input: Block{Type: BlockBlockquote, Text: "quote"}},
		"Bullet list": {Input: Block{
			Type:  BlockBulletList,
			Items: []Block{{Type: BlockParagraph, Text: "one"}, {Type: BlockParagraph, Text: "two"}},
		}},
		"Task list": {Input: Block{
			Type: BlockTaskList,
			TaskItems: []TaskItem{
				{Checked: true, Block: Block{Type: BlockParagraph, Text: "done"}},
			},
		}},
		"Callout text shorthand": {Input: Block{
			Type:  BlockCallout,
			Text:  "warn",
			Attrs: map[string]any{"icon": "lucide:warning"},
		}},
		"Code": {Input: Block{
			Type:  BlockCode,
			Text:  "x := 1",
			Attrs: map[string]any{"language": "go"},
		}},
		"Titled code": {Input: Block{
			Type:  BlockTitledCode,
			Text:  "x := 1",
			Attrs: map[string]any{"title": "ex.go", "language": "go"},
		}},
		"Mermaid":         {Input: Block{Type: BlockMermaid, Text: "graph TD; A-->B;"}},
		"Horizontal rule": {Input: Block{Type: BlockHorizontalRule}},
		"Image": {Input: Block{
			Type:  BlockImage,
			Attrs: map[string]any{"src": "http://x", "alt": "x", "width": 100},
		}},
		"Figma": {Input: Block{
			Type:  BlockFigma,
			Attrs: map[string]any{"src": "http://figma", "width": 320, "height": 200},
		}},
		"Split doc": {Input: Block{
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
		"Param list": {Input: Block{
			Type:   BlockParamList,
			Header: "Body",
			Params: []ParamItem{
				{Name: "id", Type: "string", Description: "the **user** id"},
				{Name: "limit", Type: "number?", Description: "page size"},
			},
		}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, Validate(tc.Input), "input should validate")

			expanded, err := Expand(tc.Input)
			require.NoError(t, err)

			compacted, err := Compact(expanded)
			require.NoError(t, err)

			assert.Equal(t,
				stripUIDsCanonical(tc.Input),
				stripUIDsCanonical(compacted),
			)
		})
	}
}

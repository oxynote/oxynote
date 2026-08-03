package aiblock

import (
	"testing"

	"github.com/oxynote/heimdall/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Compact(t *testing.T) {
	tests := map[string]struct {
		Input    document.Block
		Expected Block
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := Compact(tc.Input)
			require.NoError(t, err)

			assert.Equal(t, tc.Expected, got)
		})
	}
}

package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

func Test_Block_CollectAttributeValues(t *testing.T) {
	t.Parallel()

	metric := func(id string) Block {
		return Block{
			Type:  BlockMetric,
			Attrs: document.Attributes{document.AttrDataSourceID: id},
		}
	}

	cc := map[string]struct {
		Input    Block
		Expected []string
	}{
		"A block that does not carry the attribute": {
			Input: Block{Type: BlockParagraph, Text: "hello"},
		},
		"A block carrying it unset": {
			Input: Block{
				Type:  BlockMetric,
				Attrs: document.Attributes{document.AttrDataSourceID: nil},
			},
		},
		"A block carrying it empty": {
			Input: metric(""),
		},
		"A block carrying it": {
			Input:    metric("a"),
			Expected: []string{"a"},
		},
		"A value that is not a string": {
			Input: Block{
				Type:  BlockMetric,
				Attrs: document.Attributes{document.AttrDataSourceID: 1},
			},
		},
		"Items report each distinct value once": {
			Input: Block{
				Type:  BlockMetricGrid,
				Items: []Block{metric("a"), metric("b"), metric("a")},
			},
			Expected: []string{"a", "b"},
		},
		"A split_doc reports both sides": {
			Input: Block{
				Type:  BlockSplitDoc,
				Left:  []Block{{Type: BlockHeading, Text: "x"}},
				Right: []Block{metric("a")},
			},
			Expected: []string{"a"},
		},
		"A list item reports what is nested under it": {
			Input: Block{
				Type: BlockBulletList,
				Items: []Block{{
					Type:     BlockParagraph,
					Text:     "x",
					Children: []Block{metric("a")},
				}},
			},
			Expected: []string{"a"},
		},
		"A task item reports its content block": {
			Input: Block{
				Type: BlockTaskList,
				TaskItems: []TaskItem{{
					Block: Block{
						Type:     BlockParagraph,
						Text:     "x",
						Children: []Block{metric("a")},
					},
				}},
			},
			Expected: []string{"a"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, c.Input.CollectAttributeValues(document.AttrDataSourceID))
		})
	}
}

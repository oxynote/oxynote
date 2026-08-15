package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubTree builds a small nested document for the find tests: a
// paragraph "p1" and a list "l1" wrapping an item "li1".
func stubTree() RootBlock {
	return RootBlock{
		Type: "doc",
		Content: []Block{
			{Type: BlockNodeParagraph, Attrs: Attributes{"uid": "p1"}},
			{
				Type:  BlockNodeBulletList,
				Attrs: Attributes{"uid": "l1"},
				Content: []Block{
					{Type: BlockNodeListItem, Attrs: Attributes{"uid": "li1"}},
				},
			},
			{Type: BlockNodeText, Text: "no uid"},
		},
	}
}

func Test_RootBlock_FindByUID(t *testing.T) {
	cc := map[string]struct {
		UID   string
		Found bool
	}{
		"Top-level block":  {UID: "p1", Found: true},
		"Nested block":     {UID: "li1", Found: true},
		"Missing uid":      {UID: "nope", Found: false},
		"Empty target uid": {UID: "", Found: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			found, ok := stubTree().FindByUID(c.UID)
			assert.Equal(t, c.Found, ok)

			if c.Found {
				uid, _ := found.UID()
				assert.Equal(t, c.UID, uid)
			}
		})
	}
}

func Test_Block_FindByUID(t *testing.T) {
	cc := map[string]struct {
		UID   string
		Found bool
	}{
		"Self match":  {UID: "l1", Found: true},
		"Child match": {UID: "li1", Found: true},
		"Missing uid": {UID: "nope", Found: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			found, ok := stubTree().Content[1].FindByUID(c.UID)
			assert.Equal(t, c.Found, ok)

			if c.Found {
				uid, _ := found.UID()
				assert.Equal(t, c.UID, uid)
			}
		})
	}
}

func Test_RootBlock_HasBlock(t *testing.T) {
	t.Parallel()

	assert.True(t, stubTree().HasBlock("li1"))
	assert.False(t, stubTree().HasBlock("nope"))
}

func Test_Block_HasBlock(t *testing.T) {
	t.Parallel()

	assert.True(t, stubTree().Content[1].HasBlock("li1"))
	assert.False(t, stubTree().Content[1].HasBlock("nope"))
}

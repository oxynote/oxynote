package document

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTree builds a small nested document for the find tests: a
// paragraph "p1" and a list "l1" wrapping an item "li1".
func stubTree() RootBlock {
	return RootBlock{
		Type: BlockNodeDoc,
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

// stubMarkedTree builds a tree with comment marks, nodeCommentId
// attrs, and uids for the strip/duplicate tests.
func stubMarkedTree() RootBlock {
	return RootBlock{
		Type: BlockNodeDoc,
		Content: []Block{
			{
				Type:  BlockNodeParagraph,
				Attrs: Attributes{"uid": "p1", "nodeCommentId": "c1", "align": "left"},
				Content: []Block{
					{
						Type: BlockNodeText,
						Text: "hello",
						Marks: []Mark{
							{Type: "bold"},
							{Type: "comment", Attrs: Attributes{"commentId": "c1"}},
						},
					},
				},
			},
		},
	}
}

func Test_Block_UID(t *testing.T) {
	t.Parallel()

	uid, ok := (Block{Attrs: Attributes{"uid": "u1"}}).UID()
	assert.True(t, ok)
	assert.Equal(t, "u1", uid)

	_, ok = (Block{}).UID()
	assert.False(t, ok)

	_, ok = (Block{Attrs: Attributes{"uid": 42}}).UID()
	assert.False(t, ok)
}

func Test_Block_Flatten(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Block    Block
		Expected string
	}{
		"Text leaf returns its text": {
			Block:    Block{Type: BlockNodeText, Text: "hello"},
			Expected: "hello",
		},
		"Adjacent fragments are space separated": {
			Block: Block{
				Type: BlockNodeParagraph,
				Content: []Block{
					{Type: BlockNodeText, Text: "hello"},
					{Type: BlockNodeText, Text: "world"},
				},
			},
			Expected: "hello world",
		},
		"Nested subtrees are concatenated": {
			Block: Block{
				Type: BlockNodeBulletList,
				Content: []Block{
					{
						Type: BlockNodeListItem,
						Content: []Block{
							{Type: BlockNodeText, Text: "one"},
						},
					},
					{
						Type: BlockNodeListItem,
						Content: []Block{
							{Type: BlockNodeText, Text: "two"},
						},
					},
				},
			},
			Expected: "one two",
		},
		"Empty text fragments are skipped": {
			Block: Block{
				Type: BlockNodeParagraph,
				Content: []Block{
					{Type: BlockNodeText, Text: ""},
					{Type: BlockNodeText, Text: "solo"},
				},
			},
			Expected: "solo",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, c.Block.Flatten())
		})
	}
}

func Test_RootBlock_Search(t *testing.T) {
	t.Parallel()

	documentID := xid.New()

	rb := RootBlock{
		Type: BlockNodeDoc,
		Content: []Block{
			{
				Type:  BlockNodeParagraph,
				Attrs: Attributes{"uid": "p1"},
				Content: []Block{
					{Type: BlockNodeText, Text: "first"},
				},
			},
			{
				Type:  BlockNodeBulletList,
				Attrs: Attributes{"uid": "l1"},
				Content: []Block{
					{
						Type:  BlockNodeListItem,
						Attrs: Attributes{"uid": "li1"},
						Content: []Block{
							{Type: BlockNodeText, Text: "nested"},
						},
					},
				},
			},
			// no uid: the text is not indexable.
			{
				Type: BlockNodeParagraph,
				Content: []Block{
					{Type: BlockNodeText, Text: "orphan"},
				},
			},
			// no text: nothing to index.
			{Type: BlockNodeHorizontalRule, Attrs: Attributes{"uid": "hr1"}},
		},
	}

	res := rb.Search("org-1", documentID)

	assert.Equal(t, map[string]search.Block{
		"p1": {
			ID:             "p1",
			OrganizationID: "org-1",
			DocumentID:     documentID,
			Type:           "paragraph",
			Text:           "first",
		},
		"li1": {
			ID:             "li1",
			OrganizationID: "org-1",
			DocumentID:     documentID,
			Type:           "listItem",
			Text:           "nested",
		},
	}, res)
}

func Test_RootBlock_StripCommentMarks(t *testing.T) {
	t.Parallel()

	stripped := stubMarkedTree().StripCommentMarks()

	assert.Equal(t, RootBlock{
		Type: BlockNodeDoc,
		Content: []Block{
			{
				Type:  BlockNodeParagraph,
				Attrs: Attributes{"uid": "p1", "align": "left"},
				Content: []Block{
					{
						Type:  BlockNodeText,
						Text:  "hello",
						Marks: []Mark{{Type: "bold"}},
					},
				},
			},
		},
	}, stripped)
}

func Test_RootBlock_Duplicate(t *testing.T) {
	t.Parallel()

	orig := stubMarkedTree()
	dup := orig.Duplicate()

	require.Len(t, dup.Content, 1)

	p := dup.Content[0]
	assert.Equal(t, Attributes{"align": "left"}, Attributes{"align": p.Attrs["align"]})
	assert.NotContains(t, p.Attrs, "nodeCommentId")

	uid, ok := p.UID()
	require.True(t, ok)
	assert.NotEqual(t, "p1", uid, "uid should be regenerated")
	assert.Len(t, uid, 21)

	require.Len(t, p.Content, 1)
	assert.Equal(t, []Mark{{Type: "bold"}}, p.Content[0].Marks)

	// mutating the duplicate must not touch the original.
	p.Attrs["align"] = "right"
	assert.Equal(t, "left", orig.Content[0].Attrs["align"])
}

func Test_RootBlock_Value_Scan(t *testing.T) {
	t.Parallel()

	orig := stubTree()

	val, err := orig.Value()
	require.NoError(t, err)

	var decoded RootBlock

	require.NoError(t, decoded.Scan(val))
	assert.Equal(t, orig, decoded)

	assert.Error(t, decoded.Scan(42))
	assert.Error(t, decoded.Scan([]byte(`{not json`)))
}

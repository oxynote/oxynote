package document

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTree builds a small nested document for the find tests: a
// paragraph "p1" and a list "l1" wrapping an item "li1" holding a
// paragraph "lp1".
func stubTree() RootBlock {
	return RootBlock{
		Type: BlockNodeDoc,
		Content: []Block{
			{Type: BlockNodeParagraph, Attrs: Attributes{"uid": "p1"}},
			{
				Type:  BlockNodeBulletList,
				Attrs: Attributes{"uid": "l1"},
				Content: []Block{
					{
						Type:  BlockNodeListItem,
						Attrs: Attributes{"uid": "li1"},
						Content: []Block{
							{Type: BlockNodeParagraph, Attrs: Attributes{"uid": "lp1"}},
						},
					},
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

func Test_RootBlock_FindParentTypeByUID(t *testing.T) {
	cc := map[string]struct {
		UID    string
		Parent BlockNodeType
		Found  bool
	}{
		"Top-level block":     {UID: "p1", Parent: BlockNodeDoc, Found: true},
		"Nested block":        {UID: "li1", Parent: BlockNodeBulletList, Found: true},
		"Deeply nested block": {UID: "lp1", Parent: BlockNodeListItem, Found: true},
		"Missing uid":         {UID: "nope"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			parent, ok := stubTree().FindParentTypeByUID(c.UID)
			assert.Equal(t, c.Found, ok)
			assert.Equal(t, c.Parent, parent)
		})
	}
}

func Test_Block_FindParentTypeByUID(t *testing.T) {
	cc := map[string]struct {
		UID    string
		Parent BlockNodeType
		Found  bool
	}{
		"Direct child":              {UID: "li1", Parent: BlockNodeBulletList, Found: true},
		"Grandchild":                {UID: "lp1", Parent: BlockNodeListItem, Found: true},
		"Self match is not a match": {UID: "l1"},
		"Missing uid":               {UID: "nope"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			parent, ok := stubTree().Content[1].FindParentTypeByUID(c.UID)
			assert.Equal(t, c.Found, ok)
			assert.Equal(t, c.Parent, parent)
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
			// marks split the text into multiple fragments; all of
			// them must be indexed, not just the last one.
			{
				Type:  BlockNodeParagraph,
				Attrs: Attributes{"uid": "p2"},
				Content: []Block{
					{Type: BlockNodeText, Text: "plain "},
					{Type: BlockNodeText, Text: "bold", Marks: []Mark{{Type: "bold"}}},
					{Type: BlockNodeText, Text: " tail"},
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
		"p2": {
			ID:             "p2",
			OrganizationID: "org-1",
			DocumentID:     documentID,
			Type:           "paragraph",
			Text:           "plain bold tail",
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

	type tcase struct {
		Input         RootBlock
		OldDocumentID xid.ID
		NewDocumentID xid.ID
		Check         func(t *testing.T, orig, dup RootBlock, files map[string]string)
	}

	cc := map[string]tcase{
		"Uids are regenerated and the original is left alone": {
			Input:         stubMarkedTree(),
			OldDocumentID: xid.New(),
			NewDocumentID: xid.New(),
			Check: func(t *testing.T, orig, dup RootBlock, files map[string]string) {
				assert.Empty(t, files)

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
			},
		},
		"File references are remapped to the new document": func() tcase {
			oldDocumentID, newDocumentID := xid.New(), xid.New()

			return tcase{
				OldDocumentID: oldDocumentID,
				NewDocumentID: newDocumentID,
				Input: RootBlock{
					Type: BlockNodeDoc,
					Content: []Block{
						{
							Type: BlockNodeImageBlock,
							Attrs: Attributes{
								"uid": "img1",
								"src": "https://app.test/core" + fmt.Sprintf(FilePathFormat, oldDocumentID, "img1"),
							},
						},
						{
							Type: BlockNodeImageBlock,
							Attrs: Attributes{
								"uid": "img2",
								"src": "https://cdn.test/photo.png",
							},
						},
						{
							Type: BlockNodeImageBlock,
							Attrs: Attributes{
								"uid": "img3",
								"src": "https://app.test/core" + fmt.Sprintf(FilePathFormat, xid.New(), "img3"),
							},
						},
						{
							Type:  BlockNodeImageBlock,
							Attrs: Attributes{"uid": "img4"},
						},
					},
				},
				Check: func(t *testing.T, orig, dup RootBlock, files map[string]string) {
					// only the image served by the source document is remapped.
					require.Len(t, files, 1)

					newID, ok := files["img1"]
					require.True(t, ok)

					uid, ok := dup.Content[0].UID()
					require.True(t, ok)
					assert.Equal(t, newID, uid)
					assert.Equal(
						t,
						"https://app.test/core"+fmt.Sprintf(FilePathFormat, newDocumentID, newID),
						dup.Content[0].Attrs["src"],
						"the host and path prefix must survive the rewrite",
					)

					// an externally hosted image, an image of another document
					// and an image without a src are all left as they are.
					assert.Equal(t, "https://cdn.test/photo.png", dup.Content[1].Attrs["src"])
					assert.Equal(t, orig.Content[2].Attrs["src"], dup.Content[2].Attrs["src"])
					assert.NotContains(t, dup.Content[3].Attrs, "src")

					// the original is untouched.
					assert.Equal(t, "img1", orig.Content[0].Attrs["uid"])
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			dup, files := c.Input.Duplicate(c.OldDocumentID, c.NewDocumentID)

			c.Check(t, c.Input, dup, files)
		})
	}
}

func Test_RootBlock_RegenerateUIDs(t *testing.T) {
	t.Parallel()

	orig := stubMarkedTree()
	res := orig.RegenerateUIDs()

	require.Len(t, res.Content, 1)
	assert.NotContains(t, res.Content[0].Attrs, "nodeCommentId")

	uid, ok := res.Content[0].UID()
	require.True(t, ok)
	assert.NotEqual(t, "p1", uid)
}

func Test_RootBlock_Value(t *testing.T) {
	t.Parallel()

	orig := stubTree()

	val, err := orig.Value()
	require.NoError(t, err)

	data, ok := val.([]byte)
	require.True(t, ok)

	exp, err := json.Marshal(orig)
	require.NoError(t, err)
	assert.JSONEq(t, string(exp), string(data))
}

func Test_RootBlock_Scan(t *testing.T) {
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

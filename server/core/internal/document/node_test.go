package document

import (
	"encoding/json"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

func Test_RootBlock_HasBlock(t *testing.T) {
	t.Parallel()

	assert.True(t, stubTree().HasBlock("li1"))
	assert.False(t, stubTree().HasBlock("nope"))
}

func Test_Block_FirstChild(t *testing.T) {
	t.Parallel()

	item := stubTree().Content[1].Content[0]

	assert.Equal(t, item.Content[0], item.FirstChild(BlockNodeParagraph))
	assert.Equal(t, Block{}, item.FirstChild(BlockNodeHeading))
}

func Test_Block_PlainText(t *testing.T) {
	t.Parallel()

	b := Block{
		Type: BlockNodeParagraph,
		Content: []Block{
			{Type: BlockNodeText, Text: "a "},
			{Type: BlockNodeImageBlock},
			{Type: BlockNodeText, Text: "b", Marks: []Mark{{Type: "bold"}}},
		},
	}

	assert.Equal(t, "a b", b.PlainText())
	assert.Empty(t, Block{}.PlainText())
}

func Test_Mark_Equal(t *testing.T) {
	t.Parallel()

	link := Mark{Type: "link", Attrs: Attributes{"href": "https://x.test", "title": nil}}

	assert.True(t, link.Equal(Mark{Type: "link", Attrs: Attributes{"href": "https://x.test", "title": nil}}))
	assert.False(t, link.Equal(Mark{Type: "link", Attrs: Attributes{"href": "https://y.test", "title": nil}}))
	assert.False(t, link.Equal(Mark{Type: "bold"}))
	assert.True(t, Mark{Type: "bold"}.Equal(Mark{Type: "bold", Attrs: Attributes{}}))
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

func Test_Mark_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		JSON   string
		Result Mark
		Err    error
	}{
		"Mark without attrs": {
			JSON:   `{"type":"bold"}`,
			Result: Mark{Type: "bold"},
		},
		"Mark with attrs": {
			JSON:   `{"type":"link","attrs":{"href":"https://oxynote.test"}}`,
			Result: Mark{Type: "link", Attrs: Attributes{"href": "https://oxynote.test"}},
		},
		"Mark with attrs that are not an object": {
			JSON:   `{"type":"bold","attrs":true}`,
			Result: Mark{Type: "bold"},
		},
		"Mark with null attrs": {
			JSON:   `{"type":"italic","attrs":null}`,
			Result: Mark{Type: "italic"},
		},
		"Malformed mark": {
			JSON: `{"type":1}`,
			Err:  assert.AnError,
		},
		"Malformed attrs": {
			JSON: `{"type":"link","attrs":{"href":}}`,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var m Mark

			err := json.Unmarshal([]byte(c.JSON), &m)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, m)
		})
	}
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

func Test_FilePath(t *testing.T) {
	cc := map[string]struct {
		ID     string
		Name   string
		Result string
	}{
		"Plain name":      {ID: "f1", Name: "notes.zip", Result: "/files/f1-notes.zip"},
		"Name with space": {ID: "f1", Name: "my notes.zip", Result: "/files/f1-my%20notes.zip"},
		"Name with dash":  {ID: "f1", Name: "a-b.zip", Result: "/files/f1-a-b.zip"},
		"Name with slash": {ID: "f1", Name: "a/b.zip", Result: "/files/f1-a%2Fb.zip"},
		"Non-ASCII name":  {ID: "f1", Name: "résumé.pdf", Result: "/files/f1-r%C3%A9sum%C3%A9.pdf"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			documentID := xid.New()

			assert.Equal(t, "/api/documents/"+documentID.String()+c.Result, FilePath(documentID, c.ID, c.Name))
		})
	}
}

func Test_ParseFileRef(t *testing.T) {
	cc := map[string]struct {
		Ref  string
		ID   string
		Name string
		OK   bool
	}{
		"Id and name":               {Ref: "f1xxxxxxxxxxxxxxxxxxx-notes.zip", ID: "f1xxxxxxxxxxxxxxxxxxx", Name: "notes.zip", OK: true},
		"Name with dashes":          {Ref: "f1xxxxxxxxxxxxxxxxxxx-a-b-c.zip", ID: "f1xxxxxxxxxxxxxxxxxxx", Name: "a-b-c.zip", OK: true},
		"Id with dashes":            {Ref: "f1-x_-xxxxxxxxxxxxxxx-notes.zip", ID: "f1-x_-xxxxxxxxxxxxxxx", Name: "notes.zip", OK: true},
		"Empty name":                {Ref: "f1xxxxxxxxxxxxxxxxxxx-", ID: "f1xxxxxxxxxxxxxxxxxxx", OK: true},
		"No dash after the id":      {Ref: "f1xxxxxxxxxxxxxxxxxxxxnotes.zip"},
		"Id alone":                  {Ref: "f1xxxxxxxxxxxxxxxxxxx"},
		"Short":                     {Ref: "f1-notes.zip"},
		"Id with the wrong charset": {Ref: "f1.................xx-notes.zip"},
		"Empty":                     {Ref: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			id, name, ok := ParseFileRef(c.Ref)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.ID, id)
			assert.Equal(t, c.Name, name)
		})
	}
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
								"uid": "img-1-aaaaaaaaaaaaaaa",
								"src": "https://app.test/core" + FilePath(oldDocumentID, "img-1-aaaaaaaaaaaaaaa", "shot.png"),
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
								"uid": "img-3-aaaaaaaaaaaaaaa",
								"src": "https://app.test/core" + FilePath(xid.New(), "img-3-aaaaaaaaaaaaaaa", "shot.png"),
							},
						},
						{
							Type:  BlockNodeImageBlock,
							Attrs: Attributes{"uid": "img4"},
						},
						{
							Type: BlockNodeFileBlock,
							Attrs: Attributes{
								"uid":         "file-1-aaaaaaaaaaaaaa",
								"src":         "https://app.test/core" + FilePath(oldDocumentID, "file-1-aaaaaaaaaaaaaa", "my notes.zip"),
								"name":        "notes.zip",
								"size":        2048,
								"contentType": "application/zip",
							},
						},
					},
				},
				Check: func(t *testing.T, orig, dup RootBlock, files map[string]string) {
					// only the image and the file served by the source
					// document are remapped.
					require.Len(t, files, 2)

					newID, ok := files["img-1-aaaaaaaaaaaaaaa"]
					require.True(t, ok)
					assert.Len(t, newID, 21)

					// the file gets an id of its own; the block's uid is
					// regenerated separately.
					uid, ok := dup.Content[0].UID()
					require.True(t, ok)
					assert.NotEqual(t, newID, uid)
					assert.NotEqual(t, "img-1-aaaaaaaaaaaaaaa", uid)
					assert.Equal(
						t,
						"https://app.test/core"+FilePath(newDocumentID, newID, "shot.png"),
						dup.Content[0].Attrs["src"],
						"the host and path prefix must survive the rewrite",
					)

					// an externally hosted image, an image of another document
					// and an image without a src are all left as they are.
					assert.Equal(t, "https://cdn.test/photo.png", dup.Content[1].Attrs["src"])
					assert.Equal(t, orig.Content[2].Attrs["src"], dup.Content[2].Attrs["src"])
					assert.NotContains(t, dup.Content[3].Attrs, "src")

					// the file block is remapped the way the image is, keeps
					// the file name segment of its address, and keeps what
					// it shows.
					newFileID, ok := files["file-1-aaaaaaaaaaaaaa"]
					require.True(t, ok)
					assert.Len(t, newFileID, 21)
					assert.Equal(
						t,
						"https://app.test/core"+FilePath(newDocumentID, newFileID, "my notes.zip"),
						dup.Content[4].Attrs["src"],
					)
					assert.Equal(t, "notes.zip", dup.Content[4].Attrs["name"])
					assert.Equal(t, 2048, dup.Content[4].Attrs["size"])
					assert.Equal(t, "application/zip", dup.Content[4].Attrs["contentType"])

					// the original is untouched.
					assert.Equal(t, "img-1-aaaaaaaaaaaaaaa", orig.Content[0].Attrs["uid"])
					assert.Equal(t, "file-1-aaaaaaaaaaaaaa", orig.Content[4].Attrs["uid"])
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			dup, files, uids := c.Input.Duplicate(c.OldDocumentID, c.NewDocumentID)

			c.Check(t, c.Input, dup, files)
			assertUIDsRemapped(t, c.Input, dup, uids)
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

	data, ok := val.([]byte)
	require.True(t, ok)

	var fromString RootBlock

	require.NoError(t, fromString.Scan(string(data)))
	assert.Equal(t, orig, fromString)

	assert.Error(t, decoded.Scan(42))
	assert.Error(t, decoded.Scan([]byte(`{not json`)))
}

// assertUIDsRemapped checks that the uid map pairs every block uid of the
// source with the uid the duplicate carries in its place, and nothing else.
func assertUIDsRemapped(t *testing.T, orig, dup RootBlock, uids map[string]string) {
	t.Helper()

	origUIDs := collectUIDs(orig.Content)
	dupUIDs := collectUIDs(dup.Content)

	require.Len(t, uids, len(origUIDs))
	require.Len(t, dupUIDs, len(origUIDs))

	for i, old := range origUIDs {
		assert.Equal(t, dupUIDs[i], uids[old])
		assert.NotEqual(t, old, uids[old])
	}
}

// collectUIDs lists the uids of the blocks in document order.
func collectUIDs(blocks []Block) []string {
	var uids []string

	for _, b := range blocks {
		if uid, ok := b.UID(); ok {
			uids = append(uids, uid)
		}

		uids = append(uids, collectUIDs(b.Content)...)
	}

	return uids
}

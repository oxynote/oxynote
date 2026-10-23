package search

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubScope builds a scope on a draft branch.
func stubScope() Scope {
	return Scope{
		OrganizationID: "org-1",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "draft",
		BranchDefault:  false,
	}
}

func Test_Block_UID(t *testing.T) {
	t.Parallel()

	scope := stubScope()

	assert.Equal(t, "p1", scope.Block("p1", "paragraph", "hello").UID())
	assert.Equal(t, "other-p1", Block{ID: "other-p1", BranchID: scope.BranchID}.UID())
}

func Test_Block_IsDocumentName(t *testing.T) {
	t.Parallel()

	scope := stubScope()

	assert.True(t, scope.Block(_documentNameUID, "document", "Runbook").IsDocumentName())
	assert.False(t, scope.Block("p1", "paragraph", "hello").IsDocumentName())
}

func Test_Scope_Block(t *testing.T) {
	t.Parallel()

	scope := stubScope()

	assert.Equal(t, Block{
		ID:             scope.BranchID.String() + "-p1",
		OrganizationID: "org-1",
		DocumentID:     scope.DocumentID,
		BranchID:       scope.BranchID,
		BranchName:     "draft",
		BranchDefault:  false,
		Type:           "paragraph",
		Text:           "hello",
	}, scope.Block("p1", "paragraph", "hello"))
}

func Test_Scope_entries(t *testing.T) {
	t.Parallel()

	scope := stubScope()

	metric := scope.Block("m1", "metricBlock", "Pizza Fridays")
	metric.Attrs = map[string]string{
		"dataSourceId":      "ds1",
		"visualizationType": "line_chart",
	}

	code := scope.Block("c1", "codeBlock", "func main() {}")
	code.Attrs = map[string]string{"language": "go"}

	cc := map[string]struct {
		Block  document.Block
		Result map[string]Block
	}{
		"Paragraph": {
			Block: document.Block{
				Type:  document.BlockNodeParagraph,
				Attrs: document.Attributes{"uid": "p1"},
				Content: []document.Block{
					{Type: document.BlockNodeText, Text: "first"},
				},
			},
			Result: map[string]Block{"p1": scope.Block("p1", "paragraph", "first")},
		},
		"Nested list item": {
			Block: document.Block{
				Type:  document.BlockNodeBulletList,
				Attrs: document.Attributes{"uid": "l1"},
				Content: []document.Block{
					{
						Type:  document.BlockNodeListItem,
						Attrs: document.Attributes{"uid": "li1"},
						Content: []document.Block{
							{Type: document.BlockNodeText, Text: "nested"},
						},
					},
				},
			},
			Result: map[string]Block{"li1": scope.Block("li1", "listItem", "nested")},
		},
		// marks split the text into multiple fragments; all of them are
		// indexed, not just the last one.
		"Text split by marks": {
			Block: document.Block{
				Type:  document.BlockNodeParagraph,
				Attrs: document.Attributes{"uid": "p2"},
				Content: []document.Block{
					{Type: document.BlockNodeText, Text: "plain "},
					{Type: document.BlockNodeText, Text: "bold", Marks: []document.Mark{{Type: "bold"}}},
					{Type: document.BlockNodeText, Text: " tail"},
				},
			},
			Result: map[string]Block{"p2": scope.Block("p2", "paragraph", "plain bold tail")},
		},
		// no uid: the text is not indexable.
		"Block without a uid": {
			Block: document.Block{
				Type: document.BlockNodeParagraph,
				Content: []document.Block{
					{Type: document.BlockNodeText, Text: "orphan"},
				},
			},
			Result: map[string]Block{},
		},
		// no text: nothing to index.
		"Block without text": {
			Block:  document.Block{Type: document.BlockNodeHorizontalRule, Attrs: document.Attributes{"uid": "hr1"}},
			Result: map[string]Block{},
		},
		// a metric block's title is an attribute, not a text child.
		"Metric block": {
			Block: document.Block{
				Type: document.BlockNodeMetricBlock,
				Attrs: document.Attributes{
					"uid":               "m1",
					"title":             "Pizza Fridays",
					"dataSourceId":      "ds1",
					"visualizationType": "line_chart",
					"timeRange":         "1h",
				},
			},
			Result: map[string]Block{"m1": metric},
		},
		// a code block keeps its language. An empty or mistyped kept
		// attribute is left out.
		"Code block with a language": {
			Block: document.Block{
				Type:  document.BlockNodeCodeBlock,
				Attrs: document.Attributes{"uid": "c1", "language": "go"},
				Content: []document.Block{
					{Type: document.BlockNodeText, Text: "func main() {}"},
				},
			},
			Result: map[string]Block{"c1": code},
		},
		"Code block with an empty language": {
			Block: document.Block{
				Type:  document.BlockNodeCodeBlock,
				Attrs: document.Attributes{"uid": "c2", "language": ""},
				Content: []document.Block{
					{Type: document.BlockNodeText, Text: "plain"},
				},
			},
			Result: map[string]Block{"c2": scope.Block("c2", "codeBlock", "plain")},
		},
		"Code block with a mistyped language": {
			Block: document.Block{
				Type:  document.BlockNodeCodeBlock,
				Attrs: document.Attributes{"uid": "c3", "language": 42},
				Content: []document.Block{
					{Type: document.BlockNodeText, Text: "odd"},
				},
			},
			Result: map[string]Block{"c3": scope.Block("c3", "codeBlock", "odd")},
		},
		// an untitled metric block has nothing to index.
		"Metric block with an empty title": {
			Block: document.Block{
				Type:  document.BlockNodeMetricBlock,
				Attrs: document.Attributes{"uid": "m2", "title": ""},
			},
			Result: map[string]Block{},
		},
		"Metric block without a title": {
			Block: document.Block{
				Type:  document.BlockNodeMetricBlock,
				Attrs: document.Attributes{"uid": "m3"},
			},
			Result: map[string]Block{},
		},
		// a file block's name is an attribute, not a text child.
		"File block": {
			Block: document.Block{
				Type:  document.BlockNodeFileBlock,
				Attrs: document.Attributes{"uid": "f1", "name": "quarterly-report.pdf", "size": 2048},
			},
			Result: map[string]Block{"f1": scope.Block("f1", "fileBlock", "quarterly-report.pdf")},
		},
		// a file block still uploading has no name to index.
		"File block without a name": {
			Block: document.Block{
				Type:  document.BlockNodeFileBlock,
				Attrs: document.Attributes{"uid": "f2"},
			},
			Result: map[string]Block{},
		},
		// an image block is described by its alt and title attributes.
		"Image block with an alt and a title": {
			Block: document.Block{
				Type:  document.BlockNodeImageBlock,
				Attrs: document.Attributes{"uid": "i1", "alt": "architecture diagram", "title": "Overview"},
			},
			Result: map[string]Block{"i1": scope.Block("i1", "imageBlock", "architecture diagram Overview")},
		},
		"Image block with an alt only": {
			Block: document.Block{
				Type:  document.BlockNodeImageBlock,
				Attrs: document.Attributes{"uid": "i2", "alt": "login flow"},
			},
			Result: map[string]Block{"i2": scope.Block("i2", "imageBlock", "login flow")},
		},
		// an undescribed image has nothing to index.
		"Image block without a description": {
			Block: document.Block{
				Type:  document.BlockNodeImageBlock,
				Attrs: document.Attributes{"uid": "i3", "src": "/x.png"},
			},
			Result: map[string]Block{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res := make(map[string]Block)
			scope.entries(c.Block, res)

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_attrKeys(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type   document.BlockNodeType
		Result []string
	}{
		"Code block": {
			Type:   document.BlockNodeCodeBlock,
			Result: []string{"language"},
		},
		"Metric block": {
			Type:   document.BlockNodeMetricBlock,
			Result: []string{"dataSourceId", "visualizationType"},
		},
		"Block without kept attributes": {
			Type: document.BlockNodeParagraph,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, attrKeys(c.Type))
		})
	}
}

func Test_Entries(t *testing.T) {
	t.Parallel()

	doc := document.Document{
		ID:             xid.New(),
		OrganizationID: "org-1",
		BranchID:       xid.New(),
		BranchName:     document.DefaultBranch,
		DocumentName:   "Runbook",
		Content: document.RootBlock{
			Type: document.BlockNodeDoc,
			Content: []document.Block{
				{
					Type:  document.BlockNodeParagraph,
					Attrs: document.Attributes{"uid": "p1"},
					Content: []document.Block{
						{Type: document.BlockNodeText, Text: "hello"},
					},
				},
			},
		},
		Default: true,
	}

	res := Entries(doc)

	// the content block plus the synthetic document-name block.
	require.Len(t, res, 2)

	nameBlock, ok := res[doc.ID.String()]
	require.True(t, ok)
	assert.Equal(t, Block{
		ID:             doc.BranchID.String() + "-docname",
		OrganizationID: "org-1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		BranchName:     document.DefaultBranch,
		BranchDefault:  true,
		Type:           "document",
		Text:           "Runbook",
	}, nameBlock)

	contentBlock, ok := res["p1"]
	require.True(t, ok)
	assert.Equal(t, doc.BranchID.String()+"-p1", contentBlock.ID)
	assert.Equal(t, doc.BranchID, contentBlock.BranchID)
	assert.True(t, contentBlock.BranchDefault)
	assert.Equal(t, "hello", contentBlock.Text)
}

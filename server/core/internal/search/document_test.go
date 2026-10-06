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

	blocks := []document.Block{
		{
			Type:  document.BlockNodeParagraph,
			Attrs: document.Attributes{"uid": "p1"},
			Content: []document.Block{
				{Type: document.BlockNodeText, Text: "first"},
			},
		},
		{
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
		// marks split the text into multiple fragments; all of them are
		// indexed, not just the last one.
		{
			Type:  document.BlockNodeParagraph,
			Attrs: document.Attributes{"uid": "p2"},
			Content: []document.Block{
				{Type: document.BlockNodeText, Text: "plain "},
				{Type: document.BlockNodeText, Text: "bold", Marks: []document.Mark{{Type: "bold"}}},
				{Type: document.BlockNodeText, Text: " tail"},
			},
		},
		// no uid: the text is not indexable.
		{
			Type: document.BlockNodeParagraph,
			Content: []document.Block{
				{Type: document.BlockNodeText, Text: "orphan"},
			},
		},
		// no text: nothing to index.
		{Type: document.BlockNodeHorizontalRule, Attrs: document.Attributes{"uid": "hr1"}},
		// a metric block's title is an attribute, not a text child.
		{
			Type: document.BlockNodeMetricBlock,
			Attrs: document.Attributes{
				"uid":               "m1",
				"title":             "Pizza Fridays",
				"dataSourceId":      "ds1",
				"visualizationType": "line_chart",
				"timeRange":         "1h",
			},
		},
		// a code block keeps its language. An empty or mistyped kept
		// attribute is left out.
		{
			Type:  document.BlockNodeCodeBlock,
			Attrs: document.Attributes{"uid": "c1", "language": "go"},
			Content: []document.Block{
				{Type: document.BlockNodeText, Text: "func main() {}"},
			},
		},
		{
			Type:  document.BlockNodeCodeBlock,
			Attrs: document.Attributes{"uid": "c2", "language": ""},
			Content: []document.Block{
				{Type: document.BlockNodeText, Text: "plain"},
			},
		},
		{
			Type:  document.BlockNodeCodeBlock,
			Attrs: document.Attributes{"uid": "c3", "language": 42},
			Content: []document.Block{
				{Type: document.BlockNodeText, Text: "odd"},
			},
		},
		// an untitled metric block has nothing to index.
		{
			Type:  document.BlockNodeMetricBlock,
			Attrs: document.Attributes{"uid": "m2", "title": ""},
		},
		{
			Type:  document.BlockNodeMetricBlock,
			Attrs: document.Attributes{"uid": "m3"},
		},
		// a file block's name is an attribute, not a text child.
		{
			Type:  document.BlockNodeFileBlock,
			Attrs: document.Attributes{"uid": "f1", "name": "quarterly-report.pdf", "size": 2048},
		},
		// a file block still uploading has no name to index.
		{
			Type:  document.BlockNodeFileBlock,
			Attrs: document.Attributes{"uid": "f2"},
		},
		// an image block is described by its alt and title attributes.
		{
			Type:  document.BlockNodeImageBlock,
			Attrs: document.Attributes{"uid": "i1", "alt": "architecture diagram", "title": "Overview"},
		},
		{
			Type:  document.BlockNodeImageBlock,
			Attrs: document.Attributes{"uid": "i2", "alt": "login flow"},
		},
		// an undescribed image has nothing to index.
		{
			Type:  document.BlockNodeImageBlock,
			Attrs: document.Attributes{"uid": "i3", "src": "/x.png"},
		},
	}

	scope := stubScope()
	res := make(map[string]Block)

	for _, b := range blocks {
		scope.entries(b, res)
	}

	metric := scope.Block("m1", "metricBlock", "Pizza Fridays")
	metric.Attrs = map[string]string{
		"dataSourceId":      "ds1",
		"visualizationType": "line_chart",
	}

	code := scope.Block("c1", "codeBlock", "func main() {}")
	code.Attrs = map[string]string{"language": "go"}

	assert.Equal(t, map[string]Block{
		"p1":  scope.Block("p1", "paragraph", "first"),
		"li1": scope.Block("li1", "listItem", "nested"),
		"p2":  scope.Block("p2", "paragraph", "plain bold tail"),
		"m1":  metric,
		"c1":  code,
		"c2":  scope.Block("c2", "codeBlock", "plain"),
		"c3":  scope.Block("c3", "codeBlock", "odd"),
		"f1":  scope.Block("f1", "fileBlock", "quarterly-report.pdf"),
		"i1":  scope.Block("i1", "imageBlock", "architecture diagram Overview"),
		"i2":  scope.Block("i2", "imageBlock", "login flow"),
	}, res)
}

func Test_attrKeys(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"language"}, attrKeys(document.BlockNodeCodeBlock))
	assert.Equal(t, []string{"dataSourceId", "visualizationType"}, attrKeys(document.BlockNodeMetricBlock))
	assert.Nil(t, attrKeys(document.BlockNodeParagraph))
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

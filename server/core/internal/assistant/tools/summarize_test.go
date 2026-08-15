package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

// stubNamedDB returns a DB mock whose FetchDocument returns a
// document named name, or err when set.
func stubNamedDB(name string, err error) *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			if err != nil {
				return nil, err
			}

			return &document.Document{
				Branch:         document.Branch{DocumentName: name},
				ID:             id,
				OrganizationID: orgID,
			}, nil
		},
	}
}

func Test_subjectFor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "document", subjectFor(""))
	assert.Equal(t, "'Cat Facts'", subjectFor("'Cat Facts'"))
}

func Test_Manager_parseToolArgs(t *testing.T) {
	t.Parallel()

	m := &Manager{log: slog.New(slog.DiscardHandler)}

	// valid args populate dst
	var dst struct {
		Name string `json:"name"`
	}

	m.parseToolArgs(NameCreateDocument, json.RawMessage(`{"name":"x"}`), &dst)
	assert.Equal(t, "x", dst.Name)

	// malformed args leave dst zeroed
	dst.Name = ""

	m.parseToolArgs(NameCreateDocument, json.RawMessage(`{broken`), &dst)
	assert.Empty(t, dst.Name)
}

func Test_Manager_ToolStatusLabel(t *testing.T) {
	docID := xid.New().String()

	cc := map[string]struct {
		DB     *DBMock
		Name   Name
		Args   string
		Result string
	}{
		"Read summary with document name": {
			DB:     stubNamedDB("Cat Facts", nil),
			Name:   NameReadDocumentSummary,
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Reading Cat Facts",
		},
		"Read block falls back to generic subject": {
			DB:     stubNamedDB("", assert.AnError),
			Name:   NameReadBlock,
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Reading a block in document",
		},
		"Search with query": {
			DB:     &DBMock{},
			Name:   NameSearchDocuments,
			Args:   `{"query":"rate limits"}`,
			Result: `Searching for "rate limits"`,
		},
		"Search without query": {
			DB:     &DBMock{},
			Name:   NameSearchDocuments,
			Args:   `{}`,
			Result: "Searching documents",
		},
		"Create with name": {
			DB:     &DBMock{},
			Name:   NameCreateDocument,
			Args:   `{"name":"Specs"}`,
			Result: `Creating "Specs"`,
		},
		"Create without name": {
			DB:     &DBMock{},
			Name:   NameCreateDocument,
			Args:   `{}`,
			Result: "Creating a document",
		},
		"Delete": {
			DB:     stubNamedDB("Old", nil),
			Name:   NameDeleteDocument,
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Deleting Old",
		},
		"Rename": {
			DB:     stubNamedDB("Old", nil),
			Name:   NameRenameDocument,
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Renaming Old",
		},
		"Move": {
			DB:     stubNamedDB("Old", nil),
			Name:   NameMoveDocument,
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Moving Old",
		},
		"Block edits collapse to updating": {
			DB:     stubNamedDB("Spec", nil),
			Name:   NameUpdateBlockText,
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Updating Spec",
		},
		"Noisy tools have no label": {
			DB:     &DBMock{},
			Name:   NameListDocuments,
			Args:   `{}`,
			Result: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			got := m.ToolStatusLabel(context.Background(), c.Name, json.RawMessage(c.Args))
			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_Manager_Summarize(t *testing.T) {
	docID := xid.New().String()

	cc := map[string]struct {
		DB     *DBMock
		Name   Name
		Args   string
		Result ConfirmActionSummary
	}{
		"Delete resolves document metadata": {
			DB:   stubNamedDB("Cat Facts", nil),
			Name: NameDeleteDocument,
			Args: `{"document_id":"` + docID + `"}`,
			Result: ConfirmActionSummary{
				Tool:         "delete_document",
				DocumentID:   docID,
				DocumentName: "Cat Facts",
				Summary:      "Delete Cat Facts",
			},
		},
		"Create carries no document reference": {
			DB:   &DBMock{},
			Name: NameCreateDocument,
			Args: `{"name":"Specs"}`,
			Result: ConfirmActionSummary{
				Tool:    "create_document",
				Summary: `Create document "Specs"`,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			got := m.Summarize(context.Background(), c.Name, json.RawMessage(c.Args))
			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_Manager_pluckDocumentID(t *testing.T) {
	t.Parallel()

	m := &Manager{log: slog.New(slog.DiscardHandler)}

	// create_document never references an existing document
	assert.Empty(t, m.pluckDocumentID(NameCreateDocument, json.RawMessage(`{"document_id":"x"}`)))

	// document_id is plucked when present
	assert.Equal(t, "abc", m.pluckDocumentID(NameDeleteDocument, json.RawMessage(`{"document_id":"abc"}`)))

	// malformed args degrade to empty
	assert.Empty(t, m.pluckDocumentID(NameDeleteDocument, json.RawMessage(`{broken`)))
}

func Test_Manager_lookupDocumentName(t *testing.T) {
	docID := xid.New().String()

	cc := map[string]struct {
		DB     *DBMock
		ID     string
		Result string
	}{
		"Invalid xid": {
			DB:     &DBMock{},
			ID:     "not-an-xid",
			Result: "",
		},
		"Error returned by db.FetchDocument": {
			DB:     stubNamedDB("", assert.AnError),
			ID:     docID,
			Result: "",
		},
		"Nil document": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, _ xid.ID, _, _ string) (*document.Document, error) {
					return nil, nil //nolint:nilnil // the nil-doc guard is the path under test
				},
			},
			ID:     docID,
			Result: "",
		},
		"Successful lookup": {
			DB:     stubNamedDB("Cat Facts", nil),
			ID:     docID,
			Result: "Cat Facts",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			assert.Equal(t, c.Result, m.lookupDocumentName(context.Background(), c.ID))
		})
	}
}

func Test_Manager_describe(t *testing.T) {
	cc := map[string]struct {
		Name    Name
		Args    string
		DocName string
		Result  string
	}{
		"Create with name":    {Name: NameCreateDocument, Args: `{"name":"Specs"}`, Result: `Create document "Specs"`},
		"Create without name": {Name: NameCreateDocument, Args: `{}`, Result: "Create a new document"},
		"Delete":              {Name: NameDeleteDocument, Args: `{}`, DocName: "Old", Result: "Delete Old"},
		"Rename with name":    {Name: NameRenameDocument, Args: `{"name":"New"}`, DocName: "Old", Result: `Rename Old to "New"`},
		"Rename without name": {Name: NameRenameDocument, Args: `{}`, DocName: "Old", Result: "Rename Old"},
		"Set icon":            {Name: NameSetDocumentIcon, Args: `{"icon":"lucide:cat"}`, DocName: "Old", Result: "Change icon of Old to lucide:cat"},
		"Move to root":        {Name: NameMoveDocument, Args: `{}`, DocName: "Old", Result: "Move Old to the org root"},
		"Move under parent":   {Name: NameMoveDocument, Args: `{"new_parent_id":"p"}`, DocName: "Old", Result: "Move Old under another document"},
		"Insert block":        {Name: NameInsertBlock, Args: `{"position":"after","block":{"type":"callout"}}`, DocName: "Spec", Result: "Insert a callout after a block in Spec"},
		"Append block":        {Name: NameAppendBlock, Args: `{"block":{"type":"paragraph"}}`, DocName: "Spec", Result: "Append a paragraph to Spec"},
		"Prepend block":       {Name: NamePrependBlock, Args: `{"block":{"type":"heading"}}`, DocName: "Spec", Result: "Prepend a heading to Spec"},
		"Replace block":       {Name: NameReplaceBlock, Args: `{"block":{"type":"code"}}`, DocName: "Spec", Result: "Replace a block in Spec with a code block"},
		"Update text with preview": {
			Name:    NameUpdateBlockText,
			Args:    `{"text":"new\ncontent"}`,
			DocName: "Spec",
			Result:  `Update a block in Spec: "new content"`,
		},
		"Update text without preview": {
			Name:    NameUpdateBlockText,
			Args:    `{}`,
			DocName: "Spec",
			Result:  "Update text of a block in Spec",
		},
		"Update attrs with keys": {
			Name:    NameUpdateBlockAttrs,
			Args:    `{"attrs":{"level":2}}`,
			DocName: "Spec",
			Result:  "Update block level in Spec",
		},
		"Update attrs without keys": {
			Name:    NameUpdateBlockAttrs,
			Args:    `{}`,
			DocName: "Spec",
			Result:  "Update block attributes in Spec",
		},
		"Delete block": {Name: NameDeleteBlock, Args: `{}`, DocName: "Spec", Result: "Delete a block in Spec"},
		"Unknown tool falls back to its name": {
			Name:   NameListDocuments,
			Args:   `{}`,
			Result: "list_documents",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler)}

			assert.Equal(t, c.Result, m.describe(c.Name, json.RawMessage(c.Args), c.DocName))
		})
	}
}

func Test_blockKindLabel(t *testing.T) {
	cc := map[string]struct {
		Kind   string
		Result string
	}{
		"Paragraph":       {Kind: "paragraph", Result: "a paragraph"},
		"Heading":         {Kind: "heading", Result: "a heading"},
		"Blockquote":      {Kind: "blockquote", Result: "a blockquote"},
		"Bullet list":     {Kind: "bullet_list", Result: "a bullet list"},
		"Ordered list":    {Kind: "ordered_list", Result: "an ordered list"},
		"Task list":       {Kind: "task_list", Result: "a task list"},
		"Callout":         {Kind: "callout", Result: "a callout"},
		"Code":            {Kind: "code", Result: "a code block"},
		"Titled code":     {Kind: "titled_code", Result: "a titled code block"},
		"Mermaid":         {Kind: "mermaid", Result: "a mermaid diagram"},
		"Horizontal rule": {Kind: "horizontal_rule", Result: "a divider"},
		"Image":           {Kind: "image", Result: "an image"},
		"Figma":           {Kind: "figma", Result: "a figma embed"},
		"Metric":          {Kind: "metric", Result: "a metric"},
		"Metric grid":     {Kind: "metric_grid", Result: "a metric grid"},
		"Split doc":       {Kind: "split_doc", Result: "a split documentation block"},
		"Param list":      {Kind: "split_doc_param_list", Result: "a parameter list"},
		"Empty":           {Kind: "", Result: "a block"},
		"Unknown":         {Kind: "wibble", Result: "a wibble block"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, blockKindLabel(c.Kind))
		})
	}
}

func Test_textPreview(t *testing.T) {
	cc := map[string]struct {
		Input  string
		MaxLen int
		Result string
	}{
		"Newlines collapse":  {Input: "a\nb\nc", MaxLen: 10, Result: "a b c"},
		"Whitespace trimmed": {Input: "  x  ", MaxLen: 10, Result: "x"},
		"Long text elided":   {Input: "abcdef", MaxLen: 3, Result: "abc…"},
		"Empty input":        {Input: "", MaxLen: 3, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, textPreview(c.Input, c.MaxLen))
		})
	}
}

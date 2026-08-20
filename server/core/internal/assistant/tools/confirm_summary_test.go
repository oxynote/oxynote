package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Set_Confirm(t *testing.T) {
	t.Parallel()

	// every write describes itself for the confirm card, from the same
	// arguments it was about to receive and without performing them.
	cc := map[string]struct {
		Name    Name
		Args    string
		DocName string
		Result  string
	}{
		"Create with name":      {Name: NameCreateDocument, Args: `{"name":"Specs"}`, Result: `Create document "Specs"`},
		"Create without name":   {Name: NameCreateDocument, Args: `{}`, Result: "Create a new document"},
		"Delete":                {Name: NameDeleteDocument, Args: `{}`, DocName: "Old", Result: "Delete Old"},
		"Rename with name":      {Name: NameRenameDocument, Args: `{"name":"New"}`, DocName: "Old", Result: `Rename Old to "New"`},
		"Rename without name":   {Name: NameRenameDocument, Args: `{}`, DocName: "Old", Result: "Rename Old"},
		"Set icon":              {Name: NameSetDocumentIcon, Args: `{"icon":"lucide:cat"}`, DocName: "Old", Result: "Change icon of Old to lucide:cat"},
		"Set icon without icon": {Name: NameSetDocumentIcon, Args: `{}`, DocName: "Old", Result: "Change icon of Old"},
		"Move to root":          {Name: NameMoveDocument, Args: `{}`, DocName: "Old", Result: "Move Old to the org root"},
		"Move under parent":     {Name: NameMoveDocument, Args: `{"new_parent_id":"p"}`, DocName: "Old", Result: "Move Old under another document"},
		"Insert block":          {Name: NameInsertBlock, Args: `{"position":"after","block":{"type":"callout"}}`, DocName: "Spec", Result: "Insert a callout after a block in Spec"},
		"Insert block without position": {
			Name:    NameInsertBlock,
			Args:    `{"block":{"type":"callout"}}`,
			DocName: "Spec",
			Result:  "Insert a callout in Spec",
		},
		"Append block":  {Name: NameAppendBlock, Args: `{"block":{"type":"paragraph"}}`, DocName: "Spec", Result: "Append a paragraph to Spec"},
		"Prepend block": {Name: NamePrependBlock, Args: `{"block":{"type":"heading"}}`, DocName: "Spec", Result: "Prepend a heading to Spec"},
		"Replace block": {Name: NameReplaceBlock, Args: `{"block":{"type":"code"}}`, DocName: "Spec", Result: "Replace a block in Spec with a code block"},
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
		"Update attrs with two keys": {
			Name:    NameUpdateBlockAttrs,
			Args:    `{"attrs":{"level":2,"icon":"lucide:cat"}}`,
			DocName: "Spec",
			Result:  "Update block icon, level in Spec",
		},
		"Update attrs without keys": {
			Name:    NameUpdateBlockAttrs,
			Args:    `{}`,
			DocName: "Spec",
			Result:  "Update block attributes in Spec",
		},
		"Delete block": {Name: NameDeleteBlock, Args: `{}`, DocName: "Spec", Result: "Delete a block in Spec"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput()
			inp.db = &DBMock{
				FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
					return &document.Document{
						Branch:         document.Branch{DocumentName: c.DocName},
						ID:             id,
						OrganizationID: orgID,
					}, nil
				},
			}

			args := json.RawMessage(c.Args)
			if c.DocName != "" {
				// the summary resolves the target by id, so give it one.
				args = withDocumentID(t, args)
			}

			cf, ok := unwrap(New(inp).tools[c.Name]).(Confirmer)
			require.True(t, ok, "%s must describe its write", c.Name)

			assert.Equal(t, c.Result, cf.Confirm(context.Background(), args).Summary)
		})
	}
}

// withDocumentID adds a resolvable document id to a set of tool args.
func withDocumentID(t *testing.T, args json.RawMessage) json.RawMessage {
	t.Helper()

	var m map[string]any

	require.NoError(t, json.Unmarshal(args, &m))

	if m == nil {
		m = map[string]any{}
	}

	m["document_id"] = xid.New().String()

	out, err := json.Marshal(m)
	require.NoError(t, err)

	return out
}

func Test_Set_Confirm_carriesTheTarget(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	inp := testInput()
	inp.db = &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			return &document.Document{
				Branch:         document.Branch{DocumentName: "Runbook"},
				ID:             id,
				OrganizationID: orgID,
			}, nil
		},
	}

	cf, ok := unwrap(New(inp).tools[NameDeleteBlock]).(Confirmer)
	require.True(t, ok)

	got := cf.Confirm(context.Background(), json.RawMessage(`{"document_id":"`+docID.String()+`"}`))

	// the card names the tool and the document it targets, so the user
	// knows what they are approving.
	assert.Equal(t, string(NameDeleteBlock), got.Tool)
	assert.Equal(t, docID.String(), got.DocumentID)
	assert.Equal(t, "Runbook", got.DocumentName)
}

func Test_Set_Confirm_createNamesNoTarget(t *testing.T) {
	t.Parallel()

	cf, ok := unwrap(New(testInput()).tools[NameCreateDocument]).(Confirmer)
	require.True(t, ok)

	got := cf.Confirm(context.Background(), json.RawMessage(`{"name":"Specs"}`))

	// a document that does not exist yet has no id to resolve.
	assert.Empty(t, got.DocumentID)
	assert.Empty(t, got.DocumentName)
	assert.Equal(t, `Create document "Specs"`, got.Summary)
}

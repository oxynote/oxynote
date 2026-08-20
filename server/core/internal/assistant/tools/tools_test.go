package tools

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allToolNames is every tool the assistant offers, in the order the
// registry builds them.
func allToolNames() []Name {
	return []Name{
		NameListDocuments,
		NameGetDocument,
		NameReadDocumentSummary,
		NameReadBlock,
		NameSearchDocuments,
		NameCreateDocument,
		NameDeleteDocument,
		NameRenameDocument,
		NameSetDocumentIcon,
		NameMoveDocument,
		NameInsertBlock,
		NameAppendBlock,
		NamePrependBlock,
		NameReplaceBlock,
		NameUpdateBlockText,
		NameUpdateBlockAttrs,
		NameDeleteBlock,
	}
}

func Test_New(t *testing.T) {
	t.Parallel()

	s := New(testInput())
	require.NotNil(t, s)

	// every tool the model is told about has to be reachable by the
	// name it was told, or the agent's dispatch finds nothing.
	require.Len(t, s.tools, len(allToolNames()))

	for _, name := range allToolNames() {
		it, ok := s.tools[name]
		require.True(t, ok, "%s is missing from the registry", name)

		info, err := it.Info(context.Background())
		require.NoError(t, err)
		assert.Equal(t, string(name), info.Name)
	}
}

func Test_Set_Tools(t *testing.T) {
	t.Parallel()

	ts := New(testInput()).Tools()
	assert.Len(t, ts, len(allToolNames()))

	seen := map[string]bool{}

	for _, bt := range ts {
		info, err := bt.Info(context.Background())
		require.NoError(t, err)

		assert.False(t, seen[info.Name], "%s appears twice", info.Name)
		seen[info.Name] = true
	}
}

func Test_Set_WriteNames(t *testing.T) {
	t.Parallel()

	got := New(testInput()).WriteNames()

	want := []string{
		string(NameCreateDocument),
		string(NameDeleteDocument),
		string(NameRenameDocument),
		string(NameSetDocumentIcon),
		string(NameMoveDocument),
		string(NameInsertBlock),
		string(NameAppendBlock),
		string(NamePrependBlock),
		string(NameReplaceBlock),
		string(NameUpdateBlockText),
		string(NameUpdateBlockAttrs),
		string(NameDeleteBlock),
	}

	slices.Sort(got)
	slices.Sort(want)

	// the reads are absent: a stale read can always be taken again,
	// while what the model changed has to stay in its context.
	assert.Equal(t, want, got)
}

func Test_Set_WriteNames_isACopy(t *testing.T) {
	t.Parallel()

	s := New(testInput())

	got := s.WriteNames()
	got[0] = "clobbered"

	assert.NotEqual(t, "clobbered", s.WriteNames()[0])
}

func Test_Set_Label(t *testing.T) {
	t.Parallel()

	// every tool decides for itself what the user is told while it
	// runs, so every tool is asked here.
	cc := map[Name]struct {
		Args   string
		Result string
	}{
		NameListDocuments:       {Args: `{}`},
		NameGetDocument:         {Args: `{"document_id":"d"}`},
		NameSetDocumentIcon:     {Args: `{"document_id":"d","icon":"lucide:cat"}`},
		NameReadDocumentSummary: {Args: `{"document_id":"d"}`, Result: "Reading document"},
		NameReadBlock:           {Args: `{"document_id":"d","block_uid":"b"}`, Result: "Reading a block in document"},
		NameSearchDocuments:     {Args: `{"query":"rate limit"}`, Result: `Searching for "rate limit"`},
		NameCreateDocument:      {Args: `{"name":"Runbook"}`, Result: `Creating "Runbook"`},
		NameDeleteDocument:      {Args: `{"document_id":"d"}`, Result: "Deleting document"},
		NameRenameDocument:      {Args: `{"document_id":"d","name":"n"}`, Result: "Renaming document"},
		NameMoveDocument:        {Args: `{"document_id":"d"}`, Result: "Moving document"},
		NameInsertBlock:         {Args: `{"document_id":"d"}`, Result: "Updating document"},
		NameAppendBlock:         {Args: `{"document_id":"d"}`, Result: "Updating document"},
		NamePrependBlock:        {Args: `{"document_id":"d"}`, Result: "Updating document"},
		NameReplaceBlock:        {Args: `{"document_id":"d"}`, Result: "Updating document"},
		NameUpdateBlockText:     {Args: `{"document_id":"d"}`, Result: "Updating document"},
		NameUpdateBlockAttrs:    {Args: `{"document_id":"d"}`, Result: "Updating document"},
		NameDeleteBlock:         {Args: `{"document_id":"d"}`, Result: "Updating document"},
	}

	require.Len(t, cc, len(allToolNames()), "every tool must state its label")

	for name, c := range cc {
		t.Run(string(name), func(t *testing.T) {
			t.Parallel()

			s := New(testInput())

			assert.Equal(t, c.Result, s.Label(context.Background(), name, json.RawMessage(c.Args)))
		})
	}
}

func Test_Set_Label_edgeCases(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name   Name
		Args   string
		Result string
	}{
		"Search without a query": {
			Name: NameSearchDocuments, Args: `{}`, Result: "Searching documents",
		},
		"Create without a name": {
			Name: NameCreateDocument, Args: `{}`, Result: "Creating a document",
		},
		"Malformed args degrade the label rather than failing": {
			Name: NameSearchDocuments, Args: `{broken`, Result: "Searching documents",
		},
		"A tool the set does not own is silent": {
			Name: Name("read_tool_output"), Args: `{}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := New(testInput())

			assert.Equal(t, c.Result, s.Label(context.Background(), c.Name, json.RawMessage(c.Args)))
		})
	}
}

func Test_Set_Label_namesTheDocument(t *testing.T) {
	t.Parallel()

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

	got := New(inp).Label(
		context.Background(),
		NameReadDocumentSummary,
		json.RawMessage(`{"document_id":"`+xid.New().String()+`"}`),
	)

	// a resolvable document is named, so the pill says what is being
	// read rather than "document".
	assert.Equal(t, "Reading Runbook", got)
}

func Test_unwrap(t *testing.T) {
	t.Parallel()

	s := New(testInput())

	// a read is stored bare, so unwrapping it changes nothing
	read := s.tools[NameReadBlock]
	assert.Equal(t, read, unwrap(read))

	// a write is stored gated, and unwrapping reaches the tool itself
	write := s.tools[NameInsertBlock]
	assert.NotEqual(t, write, unwrap(write))
	assert.IsType(t, &insertBlock{}, unwrap(write))
}

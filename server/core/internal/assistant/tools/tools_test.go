package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _writeNames enumerates every confirmed write tool.
var _writeNames = []Name{
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

// _readNames enumerates every unconfirmed read tool.
var _readNames = []Name{
	NameListDocuments,
	NameGetDocument,
	NameReadDocumentSummary,
	NameReadBlock,
	NameSearchDocuments,
}

func Test_IsWrite(t *testing.T) {
	t.Parallel()

	for _, name := range _writeNames {
		assert.True(t, IsWrite(name), string(name))
	}

	for _, name := range _readNames {
		assert.False(t, IsWrite(name), string(name))
	}

	// unknown names default to read so a typo never bypasses confirm
	assert.False(t, IsWrite("wibble"))
}

func Test_IsValid(t *testing.T) {
	t.Parallel()

	for _, name := range append(append([]Name{}, _readNames...), _writeNames...) {
		assert.True(t, IsValid(name), string(name))
	}

	assert.False(t, IsValid("wibble"))
	assert.False(t, IsValid(""))
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	db := &DBMock{}
	search := &SearcherMock{}
	applier := &EditApplierMock{}
	tree := &TreeNotifierMock{}

	m := NewManager(log, db, search, applier, tree, "org", "user")
	require.NotNil(t, m)

	assert.NotNil(t, m.log)
	assert.Same(t, db, m.db)
	assert.Same(t, search, m.search)
	assert.Same(t, applier, m.applier)
	assert.Same(t, tree, m.tree)
	assert.Equal(t, "org", m.orgID)
	assert.Equal(t, "user", m.userID)
}

func Test_Manager_Execute(t *testing.T) {
	// every known tool must be routed by the dispatcher; malformed
	// args make each routed tool fail fast, proving the call reached
	// it without needing per-tool collaborator wiring.
	for _, name := range append(append([]Name{}, _readNames...), _writeNames...) {
		t.Run("Routes "+string(name), func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: &DBMock{}}

			_, err := m.Execute(context.Background(), name, json.RawMessage(`{broken`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), string(name))
		})
	}

	t.Run("Unknown tool", func(t *testing.T) {
		t.Parallel()

		m := &Manager{log: slog.New(slog.DiscardHandler)}

		_, err := m.Execute(context.Background(), "wibble", json.RawMessage(`{}`))
		require.Error(t, err)
	})

	t.Run("Successful dispatch", func(t *testing.T) {
		t.Parallel()

		m := &Manager{
			log: slog.New(slog.DiscardHandler),
			db: &DBMock{
				FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
					return nil, nil
				},
			},
			orgID: "org",
		}

		res, err := m.Execute(context.Background(), NameListDocuments, json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.JSONEq(t, `{"documents":null}`, string(res))
	})
}

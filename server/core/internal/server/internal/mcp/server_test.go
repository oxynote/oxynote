package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// _readToolNames is every read tool the MCP surface serves to a token
// holding only documents:read.
var _readToolNames = []string{
	"list_documents",
	"get_document",
	"read_document_summary",
	"read_block",
	"search_documents",
}

// _writeToolNames is every write tool the MCP surface serves to a
// token holding documents:write.
var _writeToolNames = []string{
	"create_document",
	"delete_document",
	"rename_document",
	"set_document_icon",
	"move_document",
	"insert_block",
	"append_block",
	"prepend_block",
	"replace_block",
	"update_block_text",
	"update_block_attrs",
	"delete_block",
}

func Test_Handler_server(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Scopes []string
		Names  []string
	}{
		"No scopes lists nothing": {
			Scopes: []string{},
			Names:  []string{},
		},
		"Read scope lists the read tools": {
			Scopes: []string{ScopeRead},
			Names:  _readToolNames,
		},
		"Write scope lists the write tools": {
			Scopes: []string{ScopeWrite},
			Names:  _writeToolNames,
		},
		"Both scopes list all seventeen": {
			Scopes: []string{ScopeRead, ScopeWrite},
			Names:  append(append([]string{}, _readToolNames...), _writeToolNames...),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := prepHandler(t, c.Scopes, stubToolsDB(), &DBMock{})

			// the SDK lists tools sorted by name, so compare as sets.
			assert.ElementsMatch(t, c.Names, listedToolNames(t, hdl))
		})
	}
}

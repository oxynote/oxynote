package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// checkReadToolShape asserts a listed tool reaches the wire as its own
// description: the tool's prose, the tool's argument schema, and the
// annotations a client gates calls on.
func checkReadToolShape(t *testing.T, defs map[string]map[string]any) {
	t.Helper()

	get := defs["get_document"]
	require.NotNil(t, get)

	assert.NotEmpty(t, get["description"])

	schema, ok := get["inputSchema"].(map[string]any)
	require.True(t, ok, "inputSchema: %v", get["inputSchema"])
	assert.Equal(t, "object", schema["type"])
	assert.Contains(t, schema["properties"], "document_id")
	assert.Equal(t, []any{"document_id"}, schema["required"])

	ann, ok := get["annotations"].(map[string]any)
	require.True(t, ok, "annotations: %v", get["annotations"])
	assert.Equal(t, true, ann["readOnlyHint"])
	assert.Equal(t, false, ann["openWorldHint"])

	// a tool with no required arguments says nothing about them. A null
	// here is not a valid "required" and a strict client may reject it.
	list := defs["list_documents"]
	require.NotNil(t, list)

	schema, ok = list["inputSchema"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, schema, "required")
}

func Test_Handler_server(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Scopes []string
		Names  []string
		Check  func(t *testing.T, defs map[string]map[string]any)
	}{
		"No scopes lists nothing": {
			Scopes: []string{},
			Names:  []string{},
		},
		"Read scope lists the read tools": {
			Scopes: []string{ScopeRead},
			Names:  _readToolNames,
			Check:  checkReadToolShape,
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

			defs := listedTools(t, hdl)

			names := make([]string, 0, len(defs))
			for name := range defs {
				names = append(names, name)
			}

			assert.ElementsMatch(t, c.Names, names)

			if c.Check != nil {
				c.Check(t, defs)
			}
		})
	}
}

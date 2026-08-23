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

// _dataSourceToolNames is every data-source tool the MCP surface serves
// to a token holding data-sources:read.
var _dataSourceToolNames = []string{
	"list_data_sources",
	"get_prometheus_metadata",
	"list_prometheus_label_names",
	"list_prometheus_label_values",
	"list_prometheus_series",
	"query_prometheus",
	"get_sql_metadata",
	"get_sql_query_labels",
	"query_sql",
}

// checkDataSourceToolShape asserts a data-source tool reaches the wire
// as a read that reaches outside Oxynote.
func checkDataSourceToolShape(t *testing.T, defs map[string]map[string]any) {
	t.Helper()

	query := defs["query_prometheus"]
	require.NotNil(t, query)

	assert.NotEmpty(t, query["description"])

	schema, ok := query["inputSchema"].(map[string]any)
	require.True(t, ok, "inputSchema: %v", query["inputSchema"])
	assert.Contains(t, schema["properties"], "data_source_id")
	assert.Equal(t, []any{"data_source_id", "query"}, schema["required"])

	ann, ok := query["annotations"].(map[string]any)
	require.True(t, ok, "annotations: %v", query["annotations"])
	assert.Equal(t, true, ann["readOnlyHint"])

	// the connection points at a system Oxynote does not own, so the
	// world a data-source tool works in is open.
	assert.Equal(t, true, ann["openWorldHint"])
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
			Scopes: []string{ScopeDocumentRead},
			Names:  _readToolNames,
			Check:  checkReadToolShape,
		},
		"Write scope lists the write tools": {
			Scopes: []string{ScopeDocumentWrite},
			Names:  _writeToolNames,
		},
		"Both document scopes list all seventeen and no data-source tool": {
			Scopes: []string{ScopeDocumentRead, ScopeDocumentWrite},
			Names:  append(append([]string{}, _readToolNames...), _writeToolNames...),
		},
		"Data-source scope lists only the data-source tools": {
			Scopes: []string{ScopeDataSourceRead},
			Names:  _dataSourceToolNames,
			Check:  checkDataSourceToolShape,
		},
		"Read and data-source scopes list both groups": {
			Scopes: []string{ScopeDocumentRead, ScopeDataSourceRead},
			Names:  append(append([]string{}, _readToolNames...), _dataSourceToolNames...),
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

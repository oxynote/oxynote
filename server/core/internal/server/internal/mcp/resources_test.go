package mcp

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_addResources(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	childID := xid.New()

	// listResources runs resources/list through the handler and
	// returns the (uri, name) pairs.
	listResources := func(t *testing.T, hdl http.Handler) map[string]string {
		t.Helper()

		code, payload := rpc(t, hdl, `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
		require.Equal(t, http.StatusOK, code)
		require.NotNil(t, payload)

		result, ok := payload["result"].(map[string]any)
		require.True(t, ok, "payload: %v", payload)

		rawResources, ok := result["resources"].([]any)
		require.True(t, ok)

		out := make(map[string]string, len(rawResources))

		for _, rr := range rawResources {
			rm, rok := rr.(map[string]any)
			require.True(t, rok)

			uri, uok := rm["uri"].(string)
			require.True(t, uok)

			name, nok := rm["name"].(string)
			require.True(t, nok)

			out[uri] = name
		}

		return out
	}

	t.Run("Error returned by db.FetchDocumentTree", func(t *testing.T) {
		t.Parallel()

		db := &DBMock{
			FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
				return nil, assert.AnError
			},
		}

		hdl := prepHandler(t, []string{ScopeRead}, stubToolsDB(), db)

		// the listing failure degrades to an empty resource list, not
		// a failed request.
		assert.Empty(t, listResources(t, hdl))
	})

	t.Run("Documents listed depth-first", func(t *testing.T) {
		t.Parallel()

		db := &DBMock{
			FetchDocumentTreeFunc: func(_ context.Context, organizationID string) (document.Summaries, error) {
				require.Equal(t, "org1", organizationID)

				return document.Summaries{{
					ID:           parentID,
					DocumentName: "Parent",
					Children: document.Summaries{{
						ID:           childID,
						DocumentName: "Child",
					}},
				}}, nil
			},
		}

		hdl := prepHandler(t, []string{ScopeRead}, stubToolsDB(), db)

		assert.Equal(t, map[string]string{
			_resourceURIPrefix + parentID.String(): "Parent",
			_resourceURIPrefix + childID.String():  "Child",
		}, listResources(t, hdl))
	})

	t.Run("Write-only tokens get no resources", func(t *testing.T) {
		t.Parallel()

		db := &DBMock{}

		hdl := prepHandler(t, []string{ScopeWrite}, stubToolsDB(), db)

		// without the read scope no resources are registered and the
		// document tree is never even fetched.
		assert.Empty(t, listResources(t, hdl))
		assert.Empty(t, db.FetchDocumentTreeCalls())
	})
}

func Test_Handler_readDocument(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	cc := map[string]struct {
		URI     string
		ToolsDB *toolsMock.DB
		Text    string
		Err     error
	}{
		"URI outside the document scheme": {
			URI:     "file:///etc/passwd",
			ToolsDB: stubToolsDB(),
			Err:     assert.AnError,
		},
		"URI without an id": {
			URI:     _resourceURIPrefix,
			ToolsDB: stubToolsDB(),
			Err:     assert.AnError,
		},
		"Error returned by the summary tool": {
			URI: _resourceURIPrefix + docID.String(),
			ToolsDB: &toolsMock.DB{
				FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		"Successful read": {
			URI: _resourceURIPrefix + docID.String(),
			ToolsDB: &toolsMock.DB{
				FetchMainBranchContentFunc: func(_ context.Context, id xid.ID, organizationID string) (document.Content, error) {
					require.Equal(t, docID, id)
					require.Equal(t, "org1", organizationID)

					return document.Content{DocumentName: "Doc"}, nil
				},
			},
			Text: `"document_name":"Doc"`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{log: discardLog()}

			res, err := hdl.readDocument(stubSet(c.ToolsDB))(context.Background(), &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{URI: c.URI},
			})

			if c.Err != nil {
				require.Error(t, err)
				assert.Nil(t, res)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Len(t, res.Contents, 1)

			assert.Equal(t, c.URI, res.Contents[0].URI)
			assert.Equal(t, _documentMIMEType, res.Contents[0].MIMEType)
			assert.Contains(t, res.Contents[0].Text, c.Text)
		})
	}
}

func Test_flatten(t *testing.T) {
	t.Parallel()

	a, b, c, d := xid.New(), xid.New(), xid.New(), xid.New()

	tree := document.Summaries{
		{
			ID: a,
			Children: document.Summaries{
				{ID: b, Children: document.Summaries{{ID: c}}},
			},
		},
		{ID: d},
	}

	got := flatten(tree)
	require.Len(t, got, 4)

	ids := make([]xid.ID, 0, len(got))
	for _, s := range got {
		ids = append(ids, s.ID)
	}

	assert.Equal(t, []xid.ID{a, b, c, d}, ids)
}

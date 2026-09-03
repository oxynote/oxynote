package mcp

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBranches answers a branch listing with the default branch and a
// draft.
func stubBranches(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
	return []document.BranchSummary{
		{BranchID: _stubMainBranchID, BranchName: document.DefaultBranch, Default: true},
		{BranchID: _stubDraftBranchID, BranchName: "draft"},
	}, nil
}

// stubDraftDocument answers a branch-id fetch with the draft branch of
// the given document.
func stubDraftDocument(docID xid.ID) func(context.Context, xid.ID, string) (*document.Document, error) {
	return func(_ context.Context, branchID xid.ID, _ string) (*document.Document, error) {
		if branchID != _stubDraftBranchID {
			return nil, errutil.ErrNotFound
		}

		return &document.Document{
			ID:           docID,
			BranchID:     branchID,
			BranchName:   "draft",
			DocumentName: "Doc",
		}, nil
	}
}

// _stubMainBranchID and _stubDraftBranchID are the ids of the two
// branches the stubs list.
var (
	_stubMainBranchID  = xid.New()
	_stubDraftBranchID = xid.New()
)

func Test_Handler_addResources(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	parentBranchID := xid.New()
	childBranchID := xid.New()
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

		hdl := prepHandler(t, []string{ScopeDocumentRead}, stubToolsDB(), db)

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
					ID:              parentID,
					DocumentName:    "Parent",
					DefaultBranchID: parentBranchID,
					Children: document.Summaries{{
						ID:              childID,
						DocumentName:    "Child",
						DefaultBranchID: childBranchID,
					}},
				}}, nil
			},
		}

		hdl := prepHandler(t, []string{ScopeDocumentRead}, stubToolsDB(), db)

		// each document is listed on its default branch.
		assert.Equal(t, map[string]string{
			_resourceURIPrefix + parentID.String() + _resourceBranchSegment + parentBranchID.String(): "Parent",
			_resourceURIPrefix + childID.String() + _resourceBranchSegment + childBranchID.String():   "Child",
		}, listResources(t, hdl))
	})

	t.Run("Write-only tokens get no resources", func(t *testing.T) {
		t.Parallel()

		db := &DBMock{}

		hdl := prepHandler(t, []string{ScopeDocumentWrite}, stubToolsDB(), db)

		// without the read scope no resources are registered and the
		// document tree is never even fetched.
		assert.Empty(t, listResources(t, hdl))
		assert.Empty(t, db.FetchDocumentTreeCalls())
	})
}

func Test_Handler_readDocument(t *testing.T) {
	t.Parallel()

	docID := xid.New()
	unknownBranchID := xid.New()

	cc := map[string]struct {
		URI     string
		ToolsDB *toolsMock.DB
		Text    string
		Err     error

		// Passthru asserts that the error is the tool's own rather
		// than a resource-not-found translation.
		Passthru bool
	}{
		"URI outside the document scheme": {
			URI:     "file:///etc/passwd",
			ToolsDB: stubToolsDB(),
			Err:     mcp.ResourceNotFoundError("file:///etc/passwd"),
		},
		"URI without an id": {
			URI:     _resourceURIPrefix,
			ToolsDB: stubToolsDB(),
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix),
		},
		"URI without a branch maps to resource not found": {
			URI:     _resourceURIPrefix + docID.String(),
			ToolsDB: stubToolsDB(),
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + docID.String()),
		},
		"Internal tool failure passes through": {
			URI: _resourceURIPrefix + docID.String() + _resourceBranchSegment + _stubDraftBranchID.String(),
			ToolsDB: &toolsMock.DB{
				FetchDocumentByBranchIDFunc: stubDraftDocument(docID),
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				},
			},
			Err:      assert.AnError,
			Passthru: true,
		},
		"Unknown branch maps to resource not found": {
			URI: _resourceURIPrefix + docID.String() + _resourceBranchSegment + unknownBranchID.String(),
			ToolsDB: &toolsMock.DB{
				FetchDocumentByBranchIDFunc: stubDraftDocument(docID),
				FetchDocumentBranchesFunc:   stubBranches,
			},
			Err: mcp.ResourceNotFoundError(_resourceURIPrefix + docID.String() + _resourceBranchSegment + unknownBranchID.String()),
		},
		"Branch segment that is not an id": {
			URI:     _resourceURIPrefix + docID.String() + _resourceBranchSegment + "draft",
			ToolsDB: stubToolsDB(),
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + docID.String() + _resourceBranchSegment + "draft"),
		},
		"Branch URI without a document id": {
			URI:     _resourceURIPrefix + _resourceBranchSegment + _stubDraftBranchID.String(),
			ToolsDB: stubToolsDB(),
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + _resourceBranchSegment + _stubDraftBranchID.String()),
		},
		"Successful read": {
			URI: _resourceURIPrefix + docID.String() + _resourceBranchSegment + _stubDraftBranchID.String(),
			ToolsDB: &toolsMock.DB{
				FetchDocumentByBranchIDFunc: stubDraftDocument(docID),
				FetchDocumentBranchesFunc:   stubBranches,
			},
			Text: `"branch":{"id":"` + _stubDraftBranchID.String() + `","name":"draft","protected":false,"default":false}`,
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
				testutil.AssertEqualError(t, c.Err, err)

				if c.Passthru {
					assert.NotEqual(t, mcp.ResourceNotFoundError(c.URI), err)
				}

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

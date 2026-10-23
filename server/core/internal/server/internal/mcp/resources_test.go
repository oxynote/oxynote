package mcp

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	cc := map[string]struct {
		Scopes                 []string
		DB                     *DBMock
		Result                 map[string]string
		FetchDocumentTreeCalls int
	}{
		// the listing failure degrades to an empty resource list, not a
		// failed request.
		"Error returned by db.FetchDocumentTree": {
			Scopes: []string{ScopeDocumentRead},
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Result:                 map[string]string{},
			FetchDocumentTreeCalls: _serverBuilds,
		},
		// each document is listed on its default branch.
		"Documents listed depth-first": {
			Scopes: []string{ScopeDocumentRead},
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
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
			},
			Result: map[string]string{
				_resourceURIPrefix + parentID.String() + _resourceBranchSegment + parentBranchID.String(): "Parent",
				_resourceURIPrefix + childID.String() + _resourceBranchSegment + childBranchID.String():   "Child",
			},
			FetchDocumentTreeCalls: _serverBuilds,
		},
		// without the read scope no resources are registered and the
		// document tree is never even fetched.
		"Write-only tokens get no resources": {
			Scopes: []string{ScopeDocumentWrite},
			DB:     &DBMock{},
			Result: map[string]string{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := &ManagerMock{
				ToolSetFunc: func(string, string) *tools.Set {
					return stubSet(&toolsMock.DB{})
				},
			}
			hdl, _ := prepHandler(t, stubSession(t, c.Scopes), man, c.DB)

			code, payload := rpc(t, hdl, `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
			require.Equal(t, http.StatusOK, code)
			require.NotNil(t, payload)

			result, ok := payload["result"].(map[string]any)
			require.True(t, ok, "payload: %v", payload)

			rawResources, ok := result["resources"].([]any)
			require.True(t, ok)

			got := make(map[string]string, len(rawResources))

			for _, rr := range rawResources {
				rm, rok := rr.(map[string]any)
				require.True(t, rok)

				uri, uok := rm["uri"].(string)
				require.True(t, uok)

				name, nok := rm["name"].(string)
				require.True(t, nok)

				got[uri] = name
			}

			assert.Equal(t, c.Result, got)

			calls := c.DB.FetchDocumentTreeCalls()
			require.Len(t, calls, c.FetchDocumentTreeCalls)

			for _, call := range calls {
				assert.Equal(t, "org1", call.OrganizationID)
			}
		})
	}
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

		FetchDocumentByBranchIDCalls int
		FetchDocumentBranchesCalls   int
		FetchBranchTagsCalls         int
	}{
		"URI outside the document scheme": {
			URI:     "file:///etc/passwd",
			ToolsDB: &toolsMock.DB{},
			Err:     mcp.ResourceNotFoundError("file:///etc/passwd"),
		},
		"URI without an id": {
			URI:     _resourceURIPrefix,
			ToolsDB: &toolsMock.DB{},
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix),
		},
		"URI without a branch maps to resource not found": {
			URI:     _resourceURIPrefix + docID.String(),
			ToolsDB: &toolsMock.DB{},
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + docID.String()),
		},
		"Document segment that is not an id": {
			URI:     _resourceURIPrefix + "doc" + _resourceBranchSegment + _stubDraftBranchID.String(),
			ToolsDB: &toolsMock.DB{},
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + "doc" + _resourceBranchSegment + _stubDraftBranchID.String()),
		},
		"Internal tool failure passes through": {
			URI: resourceURI(docID, _stubDraftBranchID),
			ToolsDB: &toolsMock.DB{
				FetchDocumentByBranchIDFunc: stubDraftDocument(docID),
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				},
			},
			Err:                          assert.AnError,
			Passthru:                     true,
			FetchDocumentByBranchIDCalls: 1,
			FetchDocumentBranchesCalls:   1,
		},
		"Unknown branch maps to resource not found": {
			URI: resourceURI(docID, unknownBranchID),
			ToolsDB: &toolsMock.DB{
				FetchDocumentByBranchIDFunc: stubDraftDocument(docID),
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return []document.BranchSummary{
						{BranchID: _stubMainBranchID, BranchName: document.DefaultBranch, Default: true},
						{BranchID: _stubDraftBranchID, BranchName: "draft"},
					}, nil
				},
			},
			Err:                          mcp.ResourceNotFoundError(resourceURI(docID, unknownBranchID)),
			FetchDocumentByBranchIDCalls: 1,
			FetchDocumentBranchesCalls:   1,
		},
		"Branch segment that is not an id": {
			URI:     _resourceURIPrefix + docID.String() + _resourceBranchSegment + "draft",
			ToolsDB: &toolsMock.DB{},
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + docID.String() + _resourceBranchSegment + "draft"),
		},
		"Branch URI without a document id": {
			URI:     _resourceURIPrefix + _resourceBranchSegment + _stubDraftBranchID.String(),
			ToolsDB: &toolsMock.DB{},
			Err:     mcp.ResourceNotFoundError(_resourceURIPrefix + _resourceBranchSegment + _stubDraftBranchID.String()),
		},
		"Successful read": {
			URI: resourceURI(docID, _stubDraftBranchID),
			ToolsDB: &toolsMock.DB{
				FetchDocumentByBranchIDFunc: stubDraftDocument(docID),
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return []document.BranchSummary{
						{BranchID: _stubMainBranchID, BranchName: document.DefaultBranch, Default: true},
						{BranchID: _stubDraftBranchID, BranchName: "draft"},
					}, nil
				},
			},
			Text:                         `{"id":"` + _stubDraftBranchID.String() + `","name":"draft","protected":false,"default":false`,
			FetchDocumentByBranchIDCalls: 1,
			FetchDocumentBranchesCalls:   1,
			FetchBranchTagsCalls:         1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{log: slog.New(slog.DiscardHandler)}

			res, err := hdl.readDocument(stubSet(c.ToolsDB))(context.Background(), &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{URI: c.URI},
			})

			assert.Len(t, c.ToolsDB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Len(t, c.ToolsDB.FetchDocumentBranchesCalls(), c.FetchDocumentBranchesCalls)
			assert.Len(t, c.ToolsDB.FetchBranchTagsCalls(), c.FetchBranchTagsCalls)

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

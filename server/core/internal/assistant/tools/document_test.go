package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDocumentInput answers every document and branch lookup with the
// test document, holding the stub content, and accepts every write.
func stubDocumentInput() *InputMock {
	return &InputMock{
		FetchDocumentFunc: func(id xid.ID) (*document.Document, error) {
			doc := stubBranchDocument(id, "org", _stubMainBranchID, document.DefaultBranch)
			doc.Content = stubContentRoot()

			return doc, nil
		},
		FetchBranchFunc: func(documentID, branchID xid.ID) (*document.Document, error) {
			name := document.DefaultBranch
			if branchID == _stubBranchID {
				name = _stubBranchName
			}

			doc := stubBranchDocument(documentID, "org", branchID, name)
			doc.Content = stubContentRoot()

			return doc, nil
		},
		FetchDocumentBranchesFunc: func(xid.ID) ([]document.BranchSummary, error) {
			return stubBranches(false, false)(context.Background(), _testDocID, "org")
		},
		OrganizationIDFunc: func() string { return "org" },
		UserIDFunc:         func() string { return "user" },
	}
}

func Test_listDocuments_Info(t *testing.T) {
	t.Parallel()

	info := listDocuments{}.Info()

	assert.Equal(t, Traits{Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameListDocuments, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Contains(t, info.Properties, "parent_id")

	// the whole tree is the default, so nothing is required.
	assert.Empty(t, info.Required)
}

func Test_listDocuments_Validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, listDocuments{}.Validate())
}

func Test_listDocuments_Exec(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	childID := xid.New()
	parentBranchID := xid.New()
	childBranchID := xid.New()

	branches := map[xid.ID][]document.BranchSummary{
		parentID: {{BranchID: parentBranchID, DocumentID: parentID, BranchName: "main", Default: true, Protected: true}},
		childID:  {{BranchID: childBranchID, DocumentID: childID, BranchName: "main", Default: true}},
	}
	child := document.Summaries{{ID: childID, DocumentName: "Child", DefaultBranchID: childBranchID}}
	tree := document.Summaries{{ID: parentID, DocumentName: "Parent", DefaultBranchID: parentBranchID, Children: child}}

	childNode := docTreeNode{ID: childID, Name: "Child", Branches: []branchInfo{{ID: childBranchID, Name: "main", Default: true}}, Children: []docTreeNode{}}
	parentNode := docTreeNode{
		ID:       parentID,
		Name:     "Parent",
		Branches: []branchInfo{{ID: parentBranchID, Name: "main", Protected: true, Default: true}},
		Children: []docTreeNode{childNode},
	}

	stub := func() *InputMock {
		return &InputMock{
			FetchDocumentTreeFunc:       func() (document.Summaries, error) { return tree, nil },
			FetchDocumentChildrenFunc:   func(null.Value[xid.ID]) (document.Summaries, error) { return child, nil },
			FetchBranchesByDocumentFunc: func() (map[xid.ID][]document.BranchSummary, error) { return branches, nil },
		}
	}

	cc := map[string]struct {
		Input         *InputMock
		Tool          listDocuments
		TreeFetches   int
		ChildFetches  int
		BranchFetches int
		Result        any
		Err           error
	}{
		"Error returned by Input.FetchDocumentTree": {
			Input: &InputMock{
				FetchDocumentTreeFunc: func() (document.Summaries, error) { return nil, assert.AnError },
			},
			TreeFetches: 1,
			Err:         assert.AnError,
		},
		"Error returned by Input.FetchBranchesByDocument": {
			Input: func() *InputMock {
				inp := stub()
				inp.FetchBranchesByDocumentFunc = func() (map[xid.ID][]document.BranchSummary, error) {
					return nil, assert.AnError
				}

				return inp
			}(),
			TreeFetches:   1,
			BranchFetches: 1,
			Err:           assert.AnError,
		},
		"Whole tree": {
			Input:         stub(),
			TreeFetches:   1,
			BranchFetches: 1,
			Result:        listDocumentsResult{Documents: []docTreeNode{parentNode}},
		},
		"Children of one parent": {
			Input:         stub(),
			Tool:          listDocuments{ParentID: null.ValueFrom(parentID)},
			ChildFetches:  1,
			BranchFetches: 1,
			Result:        listDocumentsResult{Documents: []docTreeNode{childNode}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchDocumentTreeCalls(), c.TreeFetches)
			assert.Len(t, c.Input.FetchDocumentChildrenCalls(), c.ChildFetches)
			assert.Len(t, c.Input.FetchBranchesByDocumentCalls(), c.BranchFetches)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_newDocTreeNodes(t *testing.T) {
	t.Parallel()

	id, child := xid.New(), xid.New()
	mainID, draftID := xid.New(), xid.New()

	cc := map[string]struct {
		Summaries document.Summaries
		Branches  map[xid.ID][]document.BranchSummary
		Result    []docTreeNode
	}{
		"No documents": {
			Result: []docTreeNode{},
		},
		// a document whose branches are not listed has none, never null.
		"Document with its branches and a child": {
			Summaries: document.Summaries{{
				ID:              id,
				DocumentName:    "Parent",
				DefaultBranchID: mainID,
				Children:        document.Summaries{{ID: child, DocumentName: "Child"}},
			}},
			Branches: map[xid.ID][]document.BranchSummary{
				id: {
					{BranchID: mainID, DocumentID: id, BranchName: "main", Default: true, Protected: true},
					{BranchID: draftID, DocumentID: id, BranchName: "draft"},
				},
			},
			Result: []docTreeNode{{
				ID:   id,
				Name: "Parent",
				Branches: []branchInfo{
					{ID: mainID, Name: "main", Protected: true, Default: true},
					{ID: draftID, Name: "draft"},
				},
				Children: []docTreeNode{{ID: child, Name: "Child", Branches: []branchInfo{}, Children: []docTreeNode{}}},
			}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newDocTreeNodes(c.Summaries, c.Branches))
		})
	}
}

func Test_getDocument_Info(t *testing.T) {
	t.Parallel()

	info := getDocument{}.Info()

	assert.Equal(t, Traits{Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameGetDocument, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id"}, info.Required)
	assert.Contains(t, info.Properties, "branch_id")
}

func Test_getDocument_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   getDocument
		Result string
	}{
		"Default branch": {
			Input:  stubDocumentInput(),
			Tool:   getDocument{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}},
			Result: "Reading Runbook",
		},
		"Another branch is named": {
			Input:  stubDocumentInput(),
			Tool:   getDocument{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubBranchID}},
			Result: "Reading Runbook on branch draft",
		},
		// a document that does not resolve makes no line.
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Tool: getDocument{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.Title(c.Input))
			assert.Len(t, c.Input.FetchBranchCalls(), 1)
		})
	}
}

func Test_getDocument_Exec(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	updated := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

	withInput := func(edit func(inp *InputMock)) *InputMock {
		inp := stubDocumentInput()
		edit(inp)

		return inp
	}

	branches := []branchInfo{
		{ID: _stubMainBranchID, Name: "main", Default: true, UpdatedAt: "0001-01-01T00:00:00Z"},
		{ID: _stubBranchID, Name: _stubBranchName, UpdatedAt: "0001-01-01T00:00:00Z"},
	}

	cc := map[string]struct {
		Input          *InputMock
		Tool           getDocument
		BranchListings int
		TagFetches     int
		Result         any
		Err            error
	}{
		"Error returned by Input.FetchBranch": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchBranchFunc = func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError }
			}),
			Err: assert.AnError,
		},
		"Error returned by Input.FetchDocumentBranches": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchDocumentBranchesFunc = func(xid.ID) ([]document.BranchSummary, error) { return nil, assert.AnError }
			}),
			BranchListings: 1,
			Err:            assert.AnError,
		},
		"Error returned by Input.FetchBranchTags": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchBranchTagsFunc = func(xid.ID, xid.ID) ([]tag.Tag, error) { return nil, assert.AnError }
			}),
			BranchListings: 1,
			TagFetches:     1,
			Err:            assert.AnError,
		},
		"Metadata branches tags and content are returned": {
			Input: withInput(func(inp *InputMock) {
				fetch := inp.FetchBranchFunc
				inp.FetchBranchFunc = func(documentID, branchID xid.ID) (*document.Document, error) {
					doc, err := fetch(documentID, branchID)
					doc.Icon = "lucide:rocket"
					doc.ParentID = null.ValueFrom(parentID)
					doc.UpdatedAt = updated

					return doc, err
				}
				inp.FetchBranchTagsFunc = func(xid.ID, xid.ID) ([]tag.Tag, error) {
					return []tag.Tag{{ID: _testTagID, TagName: _stubTagName, Color: _stubTagColor}}, nil
				}
			}),
			BranchListings: 1,
			TagFetches:     1,
			Result: getDocumentResult{
				DocumentID: _testDocID,
				Name:       _stubDocumentName,
				Icon:       "lucide:rocket",
				ParentID:   null.ValueFrom(parentID),
				UpdatedAt:  "2026-11-08T12:00:00Z",
				Branches:   branches,
				Tags:       []documentTag{{ID: _testTagID, Name: _stubTagName, ColorName: _stubTagColor}},
				Content:    markup.Render(stubContentRoot().Content),
			},
		},
		"Block narrows the content to it": {
			Input:          stubDocumentInput(),
			Tool:           getDocument{BlockUID: "cp"},
			BranchListings: 1,
			TagFetches:     1,
			Result: getDocumentResult{
				DocumentID: _testDocID,
				Name:       _stubDocumentName,
				UpdatedAt:  "0001-01-01T00:00:00Z",
				Branches:   branches,
				Tags:       []documentTag{},
				Content:    "<p id=\"cp\">inside</p>\n",
			},
		},
		"Block the document does not hold": {
			Input:          stubDocumentInput(),
			Tool:           getDocument{BlockUID: "zzz"},
			BranchListings: 1,
			TagFetches:     1,
			Err:            fmt.Errorf("block zzz: %w", _errUnknownBlock),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.Tool.docTarget = docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)
			assert.Len(t, c.Input.FetchDocumentBranchesCalls(), c.BranchListings)
			assert.Len(t, c.Input.FetchBranchTagsCalls(), c.TagFetches)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_createDocument_Info(t *testing.T) {
	t.Parallel()

	info := createDocument{}.Info()

	assert.Equal(t, Traits{Write: true, Role: org.RoleEditor}, info.Traits)

	assert.Equal(t, NameCreateDocument, info.Name)
	assert.Equal(t, []string{"name"}, info.Required)
	assert.Contains(t, info.Properties, document.AttrIcon)
}

func Test_createDocument_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool        createDocument
		ErrContains string
		Err         error
	}{
		"Valid arguments": {Tool: createDocument{Name: "n"}},
		"Name is required": {
			Tool:        createDocument{},
			ErrContains: "name is required",
			Err:         assert.AnError,
		},
		"Name of only white space": {
			Tool:        createDocument{Name: "   "},
			ErrContains: "document name cannot be empty",
			Err:         assert.AnError,
		},
		"Name over 200 characters": {
			Tool:        createDocument{Name: strings.Repeat("a", 201)},
			ErrContains: "cannot be longer than 200",
			Err:         assert.AnError,
		},
		"Icon the picker offers": {Tool: createDocument{Name: "n", Icon: "mingcute:rocket-fill"}},
		// an icon the picker does not offer is refused, naming close ones.
		"Icon the picker does not offer": {
			Tool:        createDocument{Name: "n", Icon: "lucide:rocket"},
			ErrContains: "mingcute:rocket-fill",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Tool.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
			}
		})
	}
}

func Test_createDocument_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool   createDocument
		Result ActionSummary
	}{
		// a document that does not exist yet has no id to name.
		"Document to create has no id": {
			Tool:   createDocument{Name: "Runbook"},
			Result: ActionSummary{Tool: NameCreateDocument, Summary: `Create document "Runbook"`},
		},
		// the card names the document as it will be stored.
		"Name with white space around it": {
			Tool:   createDocument{Name: "  Runbook \n"},
			Result: ActionSummary{Tool: NameCreateDocument, Summary: `Create document "Runbook"`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(&InputMock{})
			require.NoError(t, err)

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_createDocument_Exec(t *testing.T) {
	t.Parallel()

	parentID := xid.New()

	cc := map[string]struct {
		Input *InputMock
		Tool  createDocument
		Err   error
	}{
		"Name of only white space": {
			Input: stubDocumentInput(),
			Tool:  createDocument{Name: "   "},
			Err:   assert.AnError,
		},
		"Error returned by Input.CreateDocument": {
			Input: &InputMock{CreateDocumentFunc: func(document.Document) error { return assert.AnError }},
			Tool:  createDocument{Name: "Runbook"},
			Err:   assert.AnError,
		},
		"Created under a parent": {
			Input: stubDocumentInput(),
			Tool:  createDocument{Name: "Runbook", Icon: "mingcute:rocket-fill", ParentID: null.ValueFrom(parentID)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// the document is stamped with the session's pair and the
			// arguments, and the result names the ids it got.
			ff := c.Input.CreateDocumentCalls()
			require.Len(t, ff, 1)

			doc := ff[0].Doc
			assert.Equal(t, "org", doc.OrganizationID)
			assert.Equal(t, "Runbook", doc.DocumentName)
			assert.Equal(t, "mingcute:rocket-fill", doc.Icon)
			assert.Equal(t, null.ValueFrom(parentID), doc.ParentID)
			assert.Equal(t, createDocumentResult{DocumentID: doc.ID, BranchID: doc.BranchID}, res)
		})
	}
}

func Test_createDocument_input(t *testing.T) {
	t.Parallel()

	parentID := null.ValueFrom(xid.New())

	cc := map[string]struct {
		Tool   createDocument
		Result document.CreateInput
	}{
		"Icon given": {
			Tool:   createDocument{Name: "n", Icon: "mingcute:rocket-fill", ParentID: parentID},
			Result: document.CreateInput{Name: "n", Icon: "mingcute:rocket-fill", ParentID: parentID},
		},
		"Icon omitted falls back to the default": {
			Tool:   createDocument{Name: "n"},
			Result: document.CreateInput{Name: "n", Icon: _defaultDocumentIcon},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.input())
		})
	}
}

func Test_deleteDocument_Info(t *testing.T) {
	t.Parallel()

	info := deleteDocument{}.Info()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true, Role: org.RoleEditor}, info.Traits)

	assert.Equal(t, NameDeleteDocument, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{"document_id"}, info.Required)
}

func Test_deleteDocument_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool        deleteDocument
		ErrContains string
		Err         error
	}{
		"Valid arguments": {Tool: deleteDocument{DocumentID: _testDocID}},
		"Document id is required": {
			Tool:        deleteDocument{},
			ErrContains: "document_id is required",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Tool.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
			}
		})
	}
}

func Test_deleteDocument_Summary(t *testing.T) {
	t.Parallel()

	withCount := func(n int, err error) *InputMock {
		inp := stubDocumentInput()
		inp.DescendantCountFunc = func(xid.ID) (int, error) { return n, err }

		return inp
	}

	summary := func(text string) ActionSummary {
		return ActionSummary{Tool: NameDeleteDocument, DocumentID: _testDocID, DocumentName: _stubDocumentName, Summary: text}
	}

	cc := map[string]struct {
		Input  *InputMock
		Counts int
		Result ActionSummary
		Err    error
	}{
		"Document without nested pages": {
			Input:  withCount(0, nil),
			Counts: 1,
			Result: summary("Delete Runbook"),
		},
		// the delete cascades, so a card naming only the document would
		// have the user approve a subtree they were never shown.
		"Nested pages are counted": {
			Input:  withCount(3, nil),
			Counts: 1,
			Result: summary("Delete Runbook and the 3 pages nested under it"),
		},
		"One nested page reads as one": {
			Input:  withCount(1, nil),
			Counts: 1,
			Result: summary("Delete Runbook and the 1 page nested under it"),
		},
		// a count that cannot be read still names the document. The
		// count is worth having, never worth failing the confirmation
		// over.
		"Error returned by Input.DescendantCount": {
			Input:  withCount(0, assert.AnError),
			Counts: 1,
			Result: summary("Delete Runbook"),
		},
		"Error returned by Input.FetchDocument": {
			Input: &InputMock{
				FetchDocumentFunc: func(xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteDocument{DocumentID: _testDocID}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchDocumentCalls(), 1)
			assert.Len(t, c.Input.DescendantCountCalls(), c.Counts)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_deleteDocument_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.DeleteDocument": {
			Input: &InputMock{DeleteDocumentFunc: func(xid.ID) error { return assert.AnError }},
			Err:   assert.AnError,
		},
		"Deleted": {
			Input:  &InputMock{},
			Result: deleteDocumentResult{DocumentID: _testDocID, Deleted: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteDocument{DocumentID: _testDocID}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.DeleteDocumentCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testDocID, ff[0].ID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_updateDocument_Info(t *testing.T) {
	t.Parallel()

	info := updateDocument{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Role: org.RoleEditor}, info.Traits)

	assert.Equal(t, NameUpdateDocument, info.Name)
	assert.Equal(t, []string{"document_id"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Contains(t, info.Properties, document.AttrIcon)
	assert.Contains(t, info.Properties, "parent_id")
}

func Test_updateDocument_Validate(t *testing.T) {
	t.Parallel()

	root := ""
	bad := "nope"

	cc := map[string]struct {
		Tool        updateDocument
		ErrContains string
		Err         error
	}{
		"Valid arguments": {Tool: updateDocument{DocumentID: _testDocID, Name: "n"}},
		"Document id is required": {
			Tool:        updateDocument{Name: "n"},
			ErrContains: "document_id is required",
			Err:         assert.AnError,
		},
		"Name of only white space": {
			Tool:        updateDocument{DocumentID: _testDocID, Name: "  "},
			ErrContains: "document name cannot be empty",
			Err:         assert.AnError,
		},
		"Name over 200 characters": {
			Tool:        updateDocument{DocumentID: _testDocID, Name: strings.Repeat("a", 201)},
			ErrContains: "cannot be longer than 200",
			Err:         assert.AnError,
		},
		// any one change is enough, and the root is a change.
		"Icon alone is a change": {Tool: updateDocument{DocumentID: _testDocID, Icon: "mingcute:rocket-fill"}},
		"Root alone is a change": {Tool: updateDocument{DocumentID: _testDocID, ParentID: &root}},
		// a call that changes nothing is refused, named by what it takes.
		"Nothing to change": {
			Tool:        updateDocument{DocumentID: _testDocID},
			ErrContains: "at least one of name, icon or parent_id",
			Err:         assert.AnError,
		},
		// a parent that is not an id is refused before anything is looked
		// up.
		"Parent id is not a valid xid": {
			Tool:        updateDocument{DocumentID: _testDocID, ParentID: &bad},
			ErrContains: "parent_id",
			Err:         assert.AnError,
		},
		// an icon the picker does not offer is refused, naming close ones.
		"Icon the picker does not offer": {
			Tool:        updateDocument{DocumentID: _testDocID, Icon: "lucide:rocket"},
			ErrContains: "mingcute:rocket-fill",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Tool.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
			}
		})
	}
}

func Test_updateDocument_Summary(t *testing.T) {
	t.Parallel()

	root := ""

	cc := map[string]struct {
		Input  *InputMock
		Tool   updateDocument
		Result string
		Err    error
	}{
		"Error returned by Input.FetchDocument": {
			Input: &InputMock{
				FetchDocumentFunc: func(xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Tool: updateDocument{Name: "Playbook"},
			Err:  assert.AnError,
		},
		// the card names the document as it will be stored.
		"Rename with white space around the name": {
			Input:  stubDocumentInput(),
			Tool:   updateDocument{Name: "  Playbook "},
			Result: `Rename Runbook to "Playbook"`,
		},
		"All three": {
			Input:  stubDocumentInput(),
			Tool:   updateDocument{Name: "Playbook", Icon: "mingcute:rocket-fill", ParentID: &root},
			Result: `Rename Runbook to "Playbook", set the icon to mingcute:rocket-fill and move it to the org root`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.Tool.DocumentID = _testDocID

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchDocumentCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameUpdateDocument,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_updateDocument_Exec(t *testing.T) {
	t.Parallel()

	root := ""
	parentID := xid.New()
	parent := parentID.String()

	withInput := func(edit func(inp *InputMock)) *InputMock {
		inp := stubDocumentInput()
		edit(inp)

		return inp
	}

	cc := map[string]struct {
		Input   *InputMock
		Tool    updateDocument
		Renames int
		Moves   int
		Parent  null.Value[xid.ID]
		Result  any
		Err     error
	}{
		"Error returned by Input.FetchDocument": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchDocumentFunc = func(xid.ID) (*document.Document, error) { return nil, assert.AnError }
			}),
			Tool: updateDocument{Name: "Playbook"},
			Err:  assert.AnError,
		},
		"Error returned by Input.RenameDocument": {
			Input: withInput(func(inp *InputMock) {
				inp.RenameDocumentFunc = func(*document.Document, string, string) error { return assert.AnError }
			}),
			Tool:    updateDocument{Name: "Playbook"},
			Renames: 1,
			Err:     assert.AnError,
		},
		"Error returned by Input.MoveDocument": {
			Input: withInput(func(inp *InputMock) {
				inp.MoveDocumentFunc = func(*document.Document, null.Value[xid.ID]) error { return assert.AnError }
			}),
			Tool:    updateDocument{ParentID: &root},
			Renames: 1,
			Moves:   1,
			Err:     assert.AnError,
		},
		// the name is stored trimmed, and the result says so.
		"Renamed": {
			Input:   stubDocumentInput(),
			Tool:    updateDocument{Name: " Playbook "},
			Renames: 1,
			Result:  updateDocumentResult{DocumentID: _testDocID, Name: "Playbook"},
		},
		"Moved to the root": {
			Input:   stubDocumentInput(),
			Tool:    updateDocument{ParentID: &root},
			Renames: 1,
			Moves:   1,
			Result:  updateDocumentResult{DocumentID: _testDocID, ParentID: &root},
		},
		"Renamed and moved under a parent": {
			Input:   stubDocumentInput(),
			Tool:    updateDocument{Name: "Playbook", ParentID: &parent},
			Renames: 1,
			Moves:   1,
			Parent:  null.ValueFrom(parentID),
			Result:  updateDocumentResult{DocumentID: _testDocID, Name: "Playbook", ParentID: &parent},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.Tool.DocumentID = _testDocID

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.RenameDocumentCalls(), c.Renames)
			assert.Len(t, c.Input.MoveDocumentCalls(), c.Moves)

			if moves := c.Input.MoveDocumentCalls(); len(moves) == 1 {
				assert.Equal(t, c.Parent, moves[0].ParentID)
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_updateDocument_parent(t *testing.T) {
	t.Parallel()

	root := ""
	id := xid.New()
	named := id.String()

	cc := map[string]struct {
		Tool   updateDocument
		Result null.Value[xid.ID]
		OK     bool
	}{
		"Absent parent is no move":   {Tool: updateDocument{}},
		"Empty parent is the root":   {Tool: updateDocument{ParentID: &root}, OK: true},
		"Parent id names the parent": {Tool: updateDocument{ParentID: &named}, Result: null.ValueFrom(id), OK: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			parent, ok := c.Tool.parent()
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, parent)
		})
	}
}

func Test_updateDocument_changes(t *testing.T) {
	t.Parallel()

	root := ""
	named := xid.New().String()

	cc := map[string]struct {
		Tool   updateDocument
		Result []string
	}{
		"Name":   {Tool: updateDocument{Name: "Playbook"}, Result: []string{`rename Runbook to "Playbook"`}},
		"Icon":   {Tool: updateDocument{Icon: "i"}, Result: []string{"set the icon to i"}},
		"Root":   {Tool: updateDocument{ParentID: &root}, Result: []string{"move it to the org root"}},
		"Parent": {Tool: updateDocument{ParentID: &named}, Result: []string{"move it under another document"}},
		"All three": {
			Tool:   updateDocument{Name: "Playbook", Icon: "i", ParentID: &named},
			Result: []string{`rename Runbook to "Playbook"`, "set the icon to i", "move it under another document"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.changes("Runbook"))
		})
	}
}

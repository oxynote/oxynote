package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_listDocumentsArgs_Validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, listDocumentsArgs{}.Validate())
}

func Test_listDocuments_Info(t *testing.T) {
	t.Parallel()

	info := listDocuments{}.Info()

	assert.Equal(t, Traits{}, info.Traits)

	assert.Equal(t, NameListDocuments, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Contains(t, info.Properties, "parent_id")

	// the whole tree is the default, so nothing is required.
	assert.Empty(t, info.Required)
}

func Test_listDocuments_Execute(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	childID := xid.New()
	parentBranchID := xid.New()
	childBranchID := xid.New()

	stubTreeDB := func() *DBMock {
		return &DBMock{
			FetchDocumentBranchesByOrganizationIDFunc: func(context.Context, string) ([]document.BranchSummary, error) {
				return []document.BranchSummary{
					{BranchID: parentBranchID, DocumentID: parentID, BranchName: "main", Default: true, Protected: true},
					{BranchID: childBranchID, DocumentID: childID, BranchName: "main", Default: true},
				}, nil
			},
			FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
				return document.Summaries{{
					ID:              parentID,
					DocumentName:    "Parent",
					DefaultBranchID: parentBranchID,
					Icon:            "lucide:file",
					Children:        document.Summaries{{ID: childID, DocumentName: "Child", DefaultBranchID: childBranchID}},
				}}, nil
			},
			FetchDocumentTreeByDocumentParentIDFunc: func(
				_ context.Context, pid null.Value[xid.ID], _ string,
			) (document.Summaries, error) {
				if pid.V != parentID {
					return nil, assert.AnError
				}

				return document.Summaries{{ID: childID, DocumentName: "Child", DefaultBranchID: childBranchID}}, nil
			},
		}
	}

	parentBranches := `"branches":[{"id":"` + parentBranchID.String() + `","name":"main","protected":true,"default":true}]`
	childBranches := `"branches":[{"id":"` + childBranchID.String() + `","name":"main","protected":false,"default":true}]`

	cc := map[string]struct {
		DB            *DBMock
		Args          string
		TreeFetches   int
		ChildFetches  int
		BranchFetches int
		Result        string
		Err           error
	}{
		"Malformed arguments": {DB: stubTreeDB(), Args: `{`, Err: assert.AnError},
		"Parent id is not a valid xid": {
			DB:   stubTreeDB(),
			Args: `{"parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Empty parent id is refused": {
			DB:   stubTreeDB(),
			Args: `{"parent_id":""}`,
			Err:  assert.AnError,
		},
		"Null parent id lists the whole tree": {
			DB:            stubTreeDB(),
			Args:          `{"parent_id":null}`,
			TreeFetches:   1,
			BranchFetches: 1,
			Result: `{"documents":[{"id":"` + parentID.String() + `","name":"Parent",` + parentBranches + `,` +
				`"children":[{"id":"` + childID.String() + `","name":"Child",` + childBranches + `}]}]}`,
		},
		"Error returned by db.FetchDocumentTree": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Args:        `{}`,
			TreeFetches: 1,
			Err:         assert.AnError,
		},
		"Error returned by db.FetchDocumentBranchesByOrganizationID": {
			DB: func() *DBMock {
				db := stubTreeDB()
				db.FetchDocumentBranchesByOrganizationIDFunc = func(context.Context, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				}

				return db
			}(),
			Args:          `{}`,
			TreeFetches:   1,
			BranchFetches: 1,
			Err:           assert.AnError,
		},
		"Whole tree": {
			DB:            stubTreeDB(),
			Args:          `{}`,
			TreeFetches:   1,
			BranchFetches: 1,
			Result: `{"documents":[{"id":"` + parentID.String() + `","name":"Parent",` + parentBranches + `,` +
				`"children":[{"id":"` + childID.String() + `","name":"Child",` + childBranches + `}]}]}`,
		},
		"Children of one parent": {
			DB:            stubTreeDB(),
			Args:          `{"parent_id":"` + parentID.String() + `"}`,
			ChildFetches:  1,
			BranchFetches: 1,
			Result:        `{"documents":[{"id":"` + childID.String() + `","name":"Child",` + childBranches + `}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listDocuments{}.Execute(testDeps(c.DB, nil, nil).newInput(context.Background(), NameListDocuments, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentTreeCalls(), c.TreeFetches)
			assert.Len(t, c.DB.FetchDocumentTreeByDocumentParentIDCalls(), c.ChildFetches)
			assert.Len(t, c.DB.FetchDocumentBranchesByOrganizationIDCalls(), c.BranchFetches)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_getDocument_Info(t *testing.T) {
	t.Parallel()

	info := getDocument{}.Info()

	assert.Equal(t, Traits{}, info.Traits)

	assert.Equal(t, NameGetDocument, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id"}, info.Required)
	assert.Contains(t, info.Properties, "branch_id")
}

func Test_getDocument_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB            *DBMock
		Args          string
		BranchFetches int
		Result        string
	}{
		"Default branch": {
			DB:            stubDocumentDB(),
			Args:          `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches: 1,
			Result:        "Reading Runbook",
		},
		"Another branch is named": {
			DB:            stubDocumentDB(),
			Args:          `{` + targetArgs(_stubBranchID) + `}`,
			BranchFetches: 1,
			Result:        "Reading Runbook on branch draft",
		},
		// a document that does not resolve, or arguments that cannot be
		// read, make no line.
		"Error returned by db.FetchDocumentByBranchID": {
			DB:            failingDocumentDB(),
			Args:          requiredArgs(t, NameGetDocument),
			BranchFetches: 1,
		},
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res := getDocument{}.Title(testDeps(c.DB, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(c.Args)))

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_getDocument_Execute(t *testing.T) {
	t.Parallel()

	parentID := xid.New()

	// a document under a parent, protected, with the stub content.
	nested := stubContentDB(nil)
	fetch := nested.FetchDocumentByBranchIDFunc
	nested.FetchDocumentByBranchIDFunc = func(ctx context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
		doc, err := fetch(ctx, branchID, orgID)
		if err != nil {
			return nil, err
		}

		doc.Icon = "lucide:rocket"
		doc.ParentID = null.ValueFrom(parentID)
		doc.Protected = true

		return doc, nil
	}

	branchless := stubContentDB(nil)
	branchless.FetchDocumentBranchesFunc = func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
		return nil, assert.AnError
	}

	untaggable := stubContentDB(nil)
	untaggable.FetchBranchTagsFunc = func(context.Context, string, xid.ID, xid.ID) ([]tag.Tag, error) {
		return nil, assert.AnError
	}

	tagged := stubContentDB(nil)
	tagged.FetchBranchTagsFunc = func(_ context.Context, _ string, documentID, branchID xid.ID) ([]tag.Tag, error) {
		if documentID != _testDocID || branchID != _stubBranchID {
			return nil, assert.AnError
		}

		return []tag.Tag{{ID: _testTagID, TagName: _stubTagName, Color: _stubTagColor}}, nil
	}

	cc := map[string]struct {
		DB             *DBMock
		Args           string
		BranchFetches  int
		BranchListings int
		TagFetches     int
		Contains       []string
		Omits          []string
		Err            error
	}{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Document id is not a valid xid": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:            stubContentDB(assert.AnError),
			Args:          `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches: 1,
			Err:           assert.AnError,
		},
		"Error returned by db.FetchDocumentBranches": {
			DB:             branchless,
			Args:           `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches:  1,
			BranchListings: 1,
			Err:            assert.AnError,
		},
		"Error returned by db.FetchBranchTags": {
			DB:             untaggable,
			Args:           `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Err:            assert.AnError,
		},
		"Metadata branches and content are returned": {
			DB:             nested,
			Args:           `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Contains: []string{
				`"name":"Runbook"`,
				`"icon":"lucide:rocket"`,
				`"parent_id":"` + parentID.String() + `"`,
				`"branches":[{"id":"` + _stubMainBranchID.String() + `","name":"main","protected":false,"default":true,"updated_at":`,
				`{"id":"` + _stubBranchID.String() + `","name":"draft","protected":false,"default":false,"updated_at":`,
				`"tags":[]`,
				`"content":"<p id=\"a\">`,
			},
		},
		"Another branch is read": {
			DB:             stubContentDB(nil),
			Args:           `{` + targetArgs(_stubBranchID) + `}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Contains:       []string{`<p id=\"a\">hello</p>`},
		},
		"Tags of the branch read are listed": {
			DB:             tagged,
			Args:           `{` + targetArgs(_stubBranchID) + `}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Contains: []string{
				`"tags":[{"id":"` + _testTagID.String() + `","name":"Production","color":"green"}]`,
			},
		},
		"Block narrows the content to it": {
			DB:             stubContentDB(nil),
			Args:           `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"cp"}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Contains:       []string{`"content":"<p id=\"cp\">inside</p>\n"`},
			Omits:          []string{`id=\"a\"`},
		},
		"Block the document does not hold": {
			DB:             stubContentDB(nil),
			Args:           `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz"}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Err:            assert.AnError,
		},
		"Root document omits the parent": {
			DB:             stubContentDB(nil),
			Args:           `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches:  1,
			BranchListings: 1,
			TagFetches:     1,
			Contains:       []string{`"name":"Runbook"`},
			Omits:          []string{`"parent_id"`, `"branch_id"`, `"branch":`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := getDocument{}.Execute(testDeps(c.DB, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.BranchListings)
			assert.Len(t, c.DB.FetchBranchTagsCalls(), c.TagFetches)

			if err != nil {
				return
			}

			for _, want := range c.Contains {
				assert.Contains(t, res, want)
			}

			for _, absent := range c.Omits {
				assert.NotContains(t, res, absent)
			}
		})
	}
}

func Test_createDocumentArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args        createDocumentArgs
		ErrContains string
		Err         error
	}{
		"Valid arguments": {Args: createDocumentArgs{Name: "n"}},
		"Name is required": {
			Args:        createDocumentArgs{},
			ErrContains: "name is required",
			Err:         assert.AnError,
		},
		"Icon the picker offers": {Args: createDocumentArgs{Name: "n", Icon: "mingcute:rocket-fill"}},
		// an icon the picker does not offer is refused, naming close ones.
		"Icon the picker does not offer": {
			Args:        createDocumentArgs{Name: "n", Icon: "lucide:rocket"},
			ErrContains: "mingcute:rocket-fill",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Args.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
			}
		})
	}
}

func Test_createDocument_Info(t *testing.T) {
	t.Parallel()

	info := createDocument{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameCreateDocument, info.Name)
	assert.Equal(t, []string{"name"}, info.Required)
	assert.Contains(t, info.Properties, document.AttrIcon)
}

func Test_createDocument_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	cc := map[string]struct {
		Args   string
		Result ActionSummary
		Err    error
	}{
		// a document that does not exist yet has no id to name.
		"Document to create has no id": {
			Args:   `{"name":"Runbook"}`,
			Result: ActionSummary{Tool: NameCreateDocument, Summary: `Create document "Runbook"`},
		},
		"Name is required": {Args: `{}`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := createDocument{}.Summary(d.newInput(context.Background(), NameCreateDocument, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_createDocument_Execute(t *testing.T) {
	t.Parallel()

	parentID := xid.New()

	type tcase struct {
		DB           *DBMock
		Tx           *TxMock
		Args         string
		ParentChecks int
		Begins       int
		Inserts      int
		Upserts      int
		Jobs         int
		Commits      int
		Rollbacks    int
		Notify       int
		Err          error
	}

	stubDB := func(tx *TxMock, beginErr error) *DBMock {
		return &DBMock{
			BeginTxFunc: testutil.StubBeginTx(tx, beginErr),
		}
	}

	cc := map[string]tcase{
		"Malformed arguments": func() tcase {
			tx := &TxMock{}

			return tcase{DB: stubDB(tx, nil), Tx: tx, Args: `{`, Err: assert.AnError}
		}(),
		"Name is required": func() tcase {
			tx := &TxMock{}

			return tcase{DB: stubDB(tx, nil), Tx: tx, Args: `{}`, Err: assert.AnError}
		}(),
		"Parent id is not a valid xid": func() tcase {
			tx := &TxMock{}

			return tcase{DB: stubDB(tx, nil), Tx: tx, Args: `{"name":"Runbook","parent_id":"nope"}`, Err: assert.AnError}
		}(),
		"Empty parent id is refused": func() tcase {
			tx := &TxMock{}

			return tcase{DB: stubDB(tx, nil), Tx: tx, Args: `{"name":"Runbook","parent_id":""}`, Err: assert.AnError}
		}(),
		"Error returned by db.CheckDocumentExists": func() tcase {
			tx := &TxMock{}
			db := stubDB(tx, nil)
			db.CheckDocumentExistsFunc = func(context.Context, xid.ID, string) error {
				return assert.AnError
			}

			return tcase{
				DB:           db,
				Tx:           tx,
				Args:         `{"name":"Runbook","parent_id":"` + parentID.String() + `"}`,
				ParentChecks: 1,
				Err:          assert.AnError,
			}
		}(),
		"Error returned by db.BeginTx": func() tcase {
			tx := &TxMock{}

			return tcase{DB: stubDB(tx, assert.AnError), Tx: tx, Args: `{"name":"Runbook"}`, Begins: 1, Err: assert.AnError}
		}(),
		"Created at the root": func() tcase {
			tx := &TxMock{}

			return tcase{
				DB:        stubDB(tx, nil),
				Tx:        tx,
				Args:      `{"name":"Runbook"}`,
				Begins:    1,
				Inserts:   1,
				Upserts:   1,
				Jobs:      1,
				Commits:   1,
				Rollbacks: 1,
				Notify:    1,
			}
		}(),
		"Created under a parent": func() tcase {
			tx := &TxMock{}

			return tcase{
				DB:           stubDB(tx, nil),
				Tx:           tx,
				Args:         `{"name":"Runbook","icon":"mingcute:rocket-fill","parent_id":"` + parentID.String() + `"}`,
				ParentChecks: 1,
				Begins:       1,
				Inserts:      1,
				Upserts:      1,
				Jobs:         1,
				Commits:      1,
				Rollbacks:    1,
				Notify:       1,
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			queue := &SearchQueueMock{}
			tree := &TreeNotifierMock{}

			d := testDeps(c.DB, nil, tree)
			d.searchQueue = queue

			res, err := createDocument{}.Execute(d.newInput(context.Background(), NameCreateDocument, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.ParentChecks)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begins)
			assert.Len(t, c.Tx.InsertDocumentCalls(), c.Inserts)
			assert.Len(t, c.Tx.UpsertDocumentMaintainersCalls(), c.Upserts)
			assert.Len(t, queue.EnqueueCalls(), c.Jobs)
			assert.Len(t, c.Tx.CommitCalls(), c.Commits)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollbacks)
			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			assert.Contains(t, res, `"document_id"`)
			assert.Contains(t, res, `"branch_id"`)
		})
	}
}

func Test_deleteDocumentArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args        deleteDocumentArgs
		ErrContains string
		Err         error
	}{
		"Valid arguments": {Args: deleteDocumentArgs{DocumentID: _testDocID}},
		"Document id is required": {
			Args:        deleteDocumentArgs{},
			ErrContains: "document_id is required",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Args.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
			}
		})
	}
}

func Test_deleteDocument_Info(t *testing.T) {
	t.Parallel()

	info := deleteDocument{}.Info()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, info.Traits)

	assert.Equal(t, NameDeleteDocument, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{"document_id"}, info.Required)
}

func Test_deleteDocument_Summary(t *testing.T) {
	t.Parallel()

	doc := `{"document_id":"` + _testDocID.String() + `"}`

	withKids := stubDocumentDB()
	withKids.FetchDocumentTreeFunc = func(context.Context, string) (document.Summaries, error) {
		return document.Summaries{{
			ID: _testDocID,
			Children: document.Summaries{
				{ID: xid.New()},
				{ID: xid.New(), Children: document.Summaries{{ID: xid.New()}}},
			},
		}}, nil
	}

	withOneKid := stubDocumentDB()
	withOneKid.FetchDocumentTreeFunc = func(context.Context, string) (document.Summaries, error) {
		return document.Summaries{{
			ID:       _testDocID,
			Children: document.Summaries{{ID: xid.New()}},
		}}, nil
	}

	treeFails := stubDocumentDB()
	treeFails.FetchDocumentTreeFunc = func(context.Context, string) (document.Summaries, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB              *DBMock
		Args            string
		DocumentFetches int
		TreeFetches     int
		Result          ActionSummary
		Err             error
	}{
		"Document without nested pages": {
			DB:              stubDocumentDB(),
			Args:            doc,
			DocumentFetches: 1,
			TreeFetches:     1,
			Result: ActionSummary{
				Tool:         NameDeleteDocument,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      "Delete Runbook",
			},
		},
		// the delete cascades, so a card naming only the document would
		// have the user approve a subtree they were never shown.
		"Nested pages are counted": {
			DB:              withKids,
			Args:            doc,
			DocumentFetches: 1,
			TreeFetches:     1,
			Result: ActionSummary{
				Tool:         NameDeleteDocument,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      "Delete Runbook and the 3 pages nested under it",
			},
		},
		"One nested page reads as one": {
			DB:              withOneKid,
			Args:            doc,
			DocumentFetches: 1,
			TreeFetches:     1,
			Result: ActionSummary{
				Tool:         NameDeleteDocument,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      "Delete Runbook and the 1 page nested under it",
			},
		},
		// a tree that cannot be read still names the document: the count
		// is worth having, never worth failing the confirmation over.
		"Error returned by db.FetchDocumentTree": {
			DB:              treeFails,
			Args:            doc,
			DocumentFetches: 1,
			TreeFetches:     1,
			Result: ActionSummary{
				Tool:         NameDeleteDocument,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      "Delete Runbook",
			},
		},
		// the document it names has to resolve; the failure is passed on
		// rather than described around.
		"Error returned by db.FetchDocument": {
			DB:              failingDocumentDB(),
			Args:            requiredArgs(t, NameDeleteDocument),
			DocumentFetches: 1,
			Err:             assert.AnError,
		},
		// unreadable arguments are refused before anything is looked up.
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteDocument{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameDeleteDocument, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentCalls(), c.DocumentFetches)
			assert.Len(t, c.DB.FetchDocumentTreeCalls(), c.TreeFetches)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_deleteDocument_Execute(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	type tcase struct {
		DB              *DBMock
		Tx              *TxMock
		Args            string
		DocumentFetches int
		Begins          int
		Deletes         int
		Jobs            int
		Commits         int
		Rollbacks       int
		Notify          int
		Err             error
	}

	// the delete reports one destroyed document, or fails with
	// deleteErr.
	stubTx := func(deleteErr error) *TxMock {
		return &TxMock{
			DeleteDocumentFunc: func(_ context.Context, id xid.ID, _ string) ([]xid.ID, error) {
				return []xid.ID{id}, deleteErr
			},
		}
	}

	stubDB := func(tx *TxMock, beginErr error) *DBMock {
		db := stubDocumentDB()
		db.BeginTxFunc = testutil.StubBeginTx(tx, beginErr)

		return db
	}

	cc := map[string]tcase{
		"Malformed arguments": func() tcase {
			tx := stubTx(nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Args: `{`, Err: assert.AnError}
		}(),
		"Document id is not a valid xid": func() tcase {
			tx := stubTx(nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Args: `{"document_id":"nope"}`, Err: assert.AnError}
		}(),
		"Error returned by db.BeginTx": func() tcase {
			tx := stubTx(nil)

			return tcase{
				DB:              stubDB(tx, assert.AnError),
				Tx:              tx,
				Args:            `{"document_id":"` + docID.String() + `"}`,
				DocumentFetches: 1,
				Begins:          1,
				Err:             assert.AnError,
			}
		}(),
		"Error returned by Tx.DeleteDocument": func() tcase {
			tx := stubTx(assert.AnError)

			return tcase{
				DB:              stubDB(tx, nil),
				Tx:              tx,
				Args:            `{"document_id":"` + docID.String() + `"}`,
				DocumentFetches: 1,
				Begins:          1,
				Deletes:         1,
				Rollbacks:       1,
				Err:             assert.AnError,
			}
		}(),
		"Deleted": func() tcase {
			tx := stubTx(nil)

			return tcase{
				DB:              stubDB(tx, nil),
				Tx:              tx,
				Args:            `{"document_id":"` + docID.String() + `"}`,
				DocumentFetches: 1,
				Begins:          1,
				Deletes:         1,
				Jobs:            1,
				Commits:         1,
				Rollbacks:       1,
				Notify:          1,
			}
		}(),
		"Deleted even when the parent cannot be captured": func() tcase {
			tx := stubTx(nil)
			db := stubDB(tx, nil)
			db.FetchDocumentFunc = func(context.Context, xid.ID, string, string) (*document.Document, error) {
				return nil, assert.AnError
			}
			db.FetchDocumentByBranchIDFunc = func(context.Context, xid.ID, string) (*document.Document, error) {
				return nil, assert.AnError
			}

			return tcase{
				DB:              db,
				Tx:              tx,
				Args:            `{"document_id":"` + docID.String() + `"}`,
				DocumentFetches: 1,
				Begins:          1,
				Deletes:         1,
				Jobs:            1,
				Commits:         1,
				Rollbacks:       1,
				Notify:          1,
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			queue := &SearchQueueMock{}
			tree := &TreeNotifierMock{}

			d := testDeps(c.DB, nil, tree)
			d.searchQueue = queue

			res, err := deleteDocument{}.Execute(d.newInput(context.Background(), NameDeleteDocument, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentCalls(), c.DocumentFetches)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begins)
			assert.Len(t, c.Tx.DeleteDocumentCalls(), c.Deletes)
			assert.Len(t, queue.EnqueueCalls(), c.Jobs)
			assert.Len(t, c.Tx.CommitCalls(), c.Commits)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollbacks)
			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"document_id":"`+docID.String()+`","deleted":true}`, res)
		})
	}
}

func Test_updateDocumentArgs_Validate(t *testing.T) {
	t.Parallel()

	root := ""
	bad := "nope"

	cc := map[string]struct {
		Args        updateDocumentArgs
		ErrContains string
		Err         error
	}{
		"Valid arguments": {Args: updateDocumentArgs{DocumentID: _testDocID, Name: "n"}},
		"Document id is required": {
			Args:        updateDocumentArgs{Name: "n"},
			ErrContains: "document_id is required",
			Err:         assert.AnError,
		},
		// any one change is enough, and the root is a change.
		"Icon alone is a change": {Args: updateDocumentArgs{DocumentID: _testDocID, Icon: "mingcute:rocket-fill"}},
		"Root alone is a change": {Args: updateDocumentArgs{DocumentID: _testDocID, ParentID: &root}},
		// a call that changes nothing is refused, named by what it takes.
		"Nothing to change": {
			Args:        updateDocumentArgs{DocumentID: _testDocID},
			ErrContains: "at least one of name, icon or parent_id",
			Err:         assert.AnError,
		},
		// a parent that is not an id is refused before anything is looked
		// up.
		"Parent id is not a valid xid": {
			Args:        updateDocumentArgs{DocumentID: _testDocID, ParentID: &bad},
			ErrContains: "parent_id",
			Err:         assert.AnError,
		},
		// an icon the picker does not offer is refused, naming close ones.
		"Icon the picker does not offer": {
			Args:        updateDocumentArgs{DocumentID: _testDocID, Icon: "lucide:rocket"},
			ErrContains: "mingcute:rocket-fill",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Args.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
			}
		})
	}
}

func Test_updateDocumentArgs_parent(t *testing.T) {
	t.Parallel()

	root := ""
	id := xid.New()
	named := id.String()

	cc := map[string]struct {
		Args   updateDocumentArgs
		Result null.Value[xid.ID]
		OK     bool
	}{
		"Absent parent is no move":   {Args: updateDocumentArgs{}},
		"Empty parent is the root":   {Args: updateDocumentArgs{ParentID: &root}, OK: true},
		"Parent id names the parent": {Args: updateDocumentArgs{ParentID: &named}, Result: null.ValueFrom(id), OK: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			parent, ok := c.Args.parent()
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, parent)
		})
	}
}

func Test_updateDocumentArgs_changes(t *testing.T) {
	t.Parallel()

	root := ""
	named := xid.New().String()

	cc := map[string]struct {
		Args   updateDocumentArgs
		Result []string
	}{
		"Name":   {Args: updateDocumentArgs{Name: "Playbook"}, Result: []string{`rename Runbook to "Playbook"`}},
		"Icon":   {Args: updateDocumentArgs{Icon: "i"}, Result: []string{"set the icon to i"}},
		"Root":   {Args: updateDocumentArgs{ParentID: &root}, Result: []string{"move it to the org root"}},
		"Parent": {Args: updateDocumentArgs{ParentID: &named}, Result: []string{"move it under another document"}},
		"All three": {
			Args:   updateDocumentArgs{Name: "Playbook", Icon: "i", ParentID: &named},
			Result: []string{`rename Runbook to "Playbook"`, "set the icon to i", "move it under another document"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Args.changes("Runbook"))
		})
	}
}

func Test_updateDocument_Info(t *testing.T) {
	t.Parallel()

	info := updateDocument{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true}, info.Traits)

	assert.Equal(t, NameUpdateDocument, info.Name)
	assert.Equal(t, []string{"document_id"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Contains(t, info.Properties, document.AttrIcon)
	assert.Contains(t, info.Properties, "parent_id")
}

func Test_updateDocument_Summary(t *testing.T) {
	t.Parallel()

	doc := `{"document_id":"` + _testDocID.String() + `"`

	cc := map[string]struct {
		DB              *DBMock
		Args            string
		DocumentFetches int
		Result          string
		Err             error
	}{
		"Rename":    {DB: stubDocumentDB(), Args: doc + `,"name":"Playbook"}`, DocumentFetches: 1, Result: `Rename Runbook to "Playbook"`},
		"Icon":      {DB: stubDocumentDB(), Args: doc + `,"icon":"mingcute:rocket-fill"}`, DocumentFetches: 1, Result: "Set the icon to mingcute:rocket-fill"},
		"Root":      {DB: stubDocumentDB(), Args: doc + `,"parent_id":""}`, DocumentFetches: 1, Result: "Move it to the org root"},
		"Parent":    {DB: stubDocumentDB(), Args: doc + `,"parent_id":"` + xid.New().String() + `"}`, DocumentFetches: 1, Result: "Move it under another document"},
		"Two":       {DB: stubDocumentDB(), Args: doc + `,"name":"Playbook","icon":"mingcute:rocket-fill"}`, DocumentFetches: 1, Result: `Rename Runbook to "Playbook" and set the icon to mingcute:rocket-fill`},
		"All three": {DB: stubDocumentDB(), Args: doc + `,"name":"Playbook","icon":"mingcute:rocket-fill","parent_id":""}`, DocumentFetches: 1, Result: `Rename Runbook to "Playbook", set the icon to mingcute:rocket-fill and move it to the org root`},
		// a call that changes nothing is refused before a card is built.
		"Nothing to change": {DB: stubDocumentDB(), Args: doc + `}`, Err: assert.AnError},
		// the document it names has to resolve; the failure is passed on
		// rather than described around.
		"Error returned by db.FetchDocument": {
			DB:              failingDocumentDB(),
			Args:            doc + `,"name":"n"}`,
			DocumentFetches: 1,
			Err:             assert.AnError,
		},
		// unreadable arguments are refused before anything is looked up.
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateDocument{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameUpdateDocument, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentCalls(), c.DocumentFetches)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got.Summary)
		})
	}
}

func Test_updateDocument_Execute(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	doc := `{"document_id":"` + _testDocID.String() + `"`

	moveDB := func(exists error, cycle bool, cycleErr, updateErr error) *DBMock {
		db := stubDocumentDB()
		db.CheckDocumentExistsFunc = func(context.Context, xid.ID, string) error { return exists }
		db.CheckDocumentCycleFunc = func(context.Context, xid.ID, xid.ID, string) (bool, error) {
			return cycle, cycleErr
		}
		db.UpdateDocumentParentIDFunc = func(context.Context, xid.ID, null.Value[xid.ID], string) error {
			return updateErr
		}

		return db
	}

	cc := map[string]struct {
		DB              *DBMock
		Applier         *EditApplierMock
		Args            string
		DocumentFetches int
		BranchFetches   int
		Ops             int
		ParentChecks    int
		CycleChecks     int
		ParentUpdates   int
		Notify          int
		Result          string
		Err             error
	}{
		"Malformed arguments": {DB: stubDocumentDB(), Applier: stubApplier(), Args: `{`, Err: assert.AnError},
		"Nothing to change": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Args:    doc + `}`,
			Err:     assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:              failingDocumentDB(),
			Applier:         stubApplier(),
			Args:            doc + `,"name":"Playbook"}`,
			DocumentFetches: 1,
			Err:             assert.AnError,
		},
		"Error returned by applier.Apply": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			Args:            doc + `,"name":"Playbook"}`,
			DocumentFetches: 1,
			BranchFetches:   1,
			Ops:             1,
			Err:             assert.AnError,
		},
		"Renamed": {
			DB:              stubDocumentDB(),
			Applier:         stubApplier(),
			Args:            doc + `,"name":"Playbook"}`,
			DocumentFetches: 1,
			BranchFetches:   1,
			Ops:             1,
			Notify:          1,
			Result:          `{"document_id":"` + _testDocID.String() + `","name":"Playbook"}`,
		},
		"Icon set": {
			DB:              stubDocumentDB(),
			Applier:         stubApplier(),
			Args:            doc + `,"icon":"mingcute:rocket-fill"}`,
			DocumentFetches: 1,
			BranchFetches:   1,
			Ops:             1,
			Notify:          1,
			Result:          `{"document_id":"` + _testDocID.String() + `","icon":"mingcute:rocket-fill"}`,
		},
		// name and icon travel in one batch, so the live document
		// changes once.
		"Renamed and icon set": {
			DB:              stubDocumentDB(),
			Applier:         stubApplier(),
			Args:            doc + `,"name":"Playbook","icon":"mingcute:rocket-fill"}`,
			DocumentFetches: 1,
			BranchFetches:   1,
			Ops:             2,
			Notify:          1,
			Result:          `{"document_id":"` + _testDocID.String() + `","name":"Playbook","icon":"mingcute:rocket-fill"}`,
		},
		"Error returned by db.CheckDocumentExists": {
			DB:              moveDB(assert.AnError, false, nil, nil),
			Applier:         stubApplier(),
			Args:            doc + `,"parent_id":"` + parentID.String() + `"}`,
			DocumentFetches: 1,
			ParentChecks:    1,
			Err:             assert.AnError,
		},
		"Cycle is refused": {
			DB:              moveDB(nil, true, nil, nil),
			Applier:         stubApplier(),
			Args:            doc + `,"parent_id":"` + parentID.String() + `"}`,
			DocumentFetches: 1,
			ParentChecks:    1,
			CycleChecks:     1,
			Err:             assert.AnError,
		},
		"Error returned by db.UpdateDocumentParentID": {
			DB:              moveDB(nil, false, nil, assert.AnError),
			Applier:         stubApplier(),
			Args:            doc + `,"parent_id":"` + parentID.String() + `"}`,
			DocumentFetches: 1,
			ParentChecks:    1,
			CycleChecks:     1,
			ParentUpdates:   1,
			Err:             assert.AnError,
		},
		// source and destination are the same (both root), so one
		// notification covers both.
		"Moved to the root": {
			DB:              moveDB(nil, false, nil, nil),
			Applier:         stubApplier(),
			Args:            doc + `,"parent_id":""}`,
			DocumentFetches: 1,
			ParentUpdates:   1,
			Notify:          1,
			Result:          `{"document_id":"` + _testDocID.String() + `","parent_id":""}`,
		},
		"Moved under a parent": {
			DB:              moveDB(nil, false, nil, nil),
			Applier:         stubApplier(),
			Args:            doc + `,"parent_id":"` + parentID.String() + `"}`,
			DocumentFetches: 1,
			ParentChecks:    1,
			CycleChecks:     1,
			ParentUpdates:   1,
			Notify:          2,
			Result:          `{"document_id":"` + _testDocID.String() + `","parent_id":"` + parentID.String() + `"}`,
		},
		"Renamed and moved": {
			DB:              moveDB(nil, false, nil, nil),
			Applier:         stubApplier(),
			Args:            doc + `,"name":"Playbook","parent_id":"` + parentID.String() + `"}`,
			DocumentFetches: 1,
			BranchFetches:   1,
			Ops:             1,
			ParentChecks:    1,
			CycleChecks:     1,
			ParentUpdates:   1,
			Notify:          3,
			Result:          `{"document_id":"` + _testDocID.String() + `","name":"Playbook","parent_id":"` + parentID.String() + `"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := updateDocument{}.Execute(
				testDeps(c.DB, c.Applier, tree).newInput(context.Background(), NameUpdateDocument, json.RawMessage(c.Args)),
			)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentCalls(), c.DocumentFetches)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())
			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.ParentChecks)
			assert.Len(t, c.DB.CheckDocumentCycleCalls(), c.CycleChecks)
			assert.Len(t, c.DB.UpdateDocumentParentIDCalls(), c.ParentUpdates)
			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			// every requested name or icon change travels in the one
			// batch sent to the live document.
			if c.Ops == 0 {
				assert.Empty(t, c.Applier.ApplyCalls())
			} else {
				require.Len(t, c.Applier.ApplyCalls(), 1)
				assert.Len(t, c.Applier.ApplyCalls()[0].Ops, c.Ops)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_summariesToTree(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []docTreeNode{}, summariesToTree(nil, nil))

	id := xid.New()
	child := xid.New()

	mainID, draftID := xid.New(), xid.New()

	got := summariesToTree(document.Summaries{{
		ID:              id,
		DocumentName:    "Parent",
		DefaultBranchID: mainID,
		Icon:            "lucide:file",
		Children:        document.Summaries{{ID: child, DocumentName: "Child"}},
	}}, map[xid.ID][]document.BranchSummary{
		id: {
			{BranchID: mainID, DocumentID: id, BranchName: "main", Default: true, Protected: true},
			{BranchID: draftID, DocumentID: id, BranchName: "draft"},
		},
	})

	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.Equal(t, "Parent", got[0].Name)
	assert.Equal(t, []branchInfo{
		{ID: mainID, Name: "main", Protected: true, Default: true},
		{ID: draftID, Name: "draft"},
	}, got[0].Branches)
	require.Len(t, got[0].Children, 1)
	assert.Equal(t, child, got[0].Children[0].ID)

	// a document whose branches are not listed has none, never null.
	assert.Equal(t, []branchInfo{}, got[0].Children[0].Branches)
}

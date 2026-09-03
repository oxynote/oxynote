package tools

import (
	"context"
	"reflect"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCreateDB builds a DB whose transaction accepts every write, for
// the tools that create a document.
func stubCreateDB(checkErr error) *DBMock {
	return &DBMock{
		CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
			return checkErr
		},
		BeginTxFunc: func(_ context.Context, dest any) error {
			reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(&TxMock{
				InsertDocumentFunc: func(context.Context, document.Document) error { return nil },
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return nil
				},
				InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
					return nil
				},
			}))

			return nil
		},
	}
}

func Test_listDocumentsArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, listDocumentsArgs{}, nil)
}

func Test_listDocuments_Info(t *testing.T) {
	t.Parallel()

	info := listDocuments{}.Info()

	assert.Equal(t, NameListDocuments, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Contains(t, info.Properties, "parent_id")

	// the whole tree is the default, so nothing is required.
	assert.Empty(t, info.Required)
}

func Test_listDocuments_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, listDocuments{}.Traits())
}

func Test_listDocuments_Title(t *testing.T) {
	t.Parallel()

	got, err := listDocuments{}.Title(testInput(testDeps(nil, nil, nil), NameListDocuments, `{}`))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_listDocuments_Execute(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	childID := xid.New()
	parentBranchID := xid.New()
	childBranchID := xid.New()

	treeDB := &DBMock{
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

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments": {DB: treeDB, Args: `{`, Err: assert.AnError},
		"Parent id is not a valid xid": {
			DB:   treeDB,
			Args: `{"parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Empty parent id is refused": {
			DB:   treeDB,
			Args: `{"parent_id":""}`,
			Err:  assert.AnError,
		},
		"Null parent id lists the whole tree": {
			DB:   treeDB,
			Args: `{"parent_id":null}`,
			Result: `{"documents":[{"id":"` + parentID.String() + `","name":"Parent","default_branch_id":"` + parentBranchID.String() + `",` +
				`"children":[{"id":"` + childID.String() + `","name":"Child","default_branch_id":"` + childBranchID.String() + `"}]}]}`,
		},
		"Error returned by db.FetchDocumentTree": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Args: `{}`,
			Err:  assert.AnError,
		},
		"Whole tree": {
			DB:   treeDB,
			Args: `{}`,
			Result: `{"documents":[{"id":"` + parentID.String() + `","name":"Parent","default_branch_id":"` + parentBranchID.String() + `",` +
				`"children":[{"id":"` + childID.String() + `","name":"Child","default_branch_id":"` + childBranchID.String() + `"}]}]}`,
		},
		"Children of one parent": {
			DB:     treeDB,
			Args:   `{"parent_id":"` + parentID.String() + `"}`,
			Result: `{"documents":[{"id":"` + childID.String() + `","name":"Child","default_branch_id":"` + childBranchID.String() + `"}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listDocuments{}.Execute(testInput(testDeps(c.DB, nil, nil), NameListDocuments, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_getDocumentArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, getDocumentArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID}, map[string]Args{
		"document_id": getDocumentArgs{},
	})
}

func Test_getDocument_Info(t *testing.T) {
	t.Parallel()

	info := getDocument{}.Info()

	assert.Equal(t, NameGetDocument, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyBranchID}, info.Required)
	assert.Contains(t, info.Properties, _keyBranchID)
}

func Test_getDocument_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, getDocument{}.Traits())
}

func Test_getDocument_Title(t *testing.T) {
	t.Parallel()

	got, err := getDocument{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameGetDocument,
		`{`+targetArgs(_stubMainBranchID)+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Reading Runbook", got)

	// a branch other than the default one is named.
	got, err = getDocument{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameGetDocument,
		`{`+targetArgs(_stubBranchID)+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Reading Runbook on branch draft", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = getDocument{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameGetDocument, requiredArgs(t, NameGetDocument)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = getDocument{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameGetDocument, `{`))
	require.Error(t, err)
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

	cc := map[string]struct {
		DB       *DBMock
		Args     string
		Contains []string
		Omits    []string
		Err      error
	}{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Document id is not a valid xid": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentBranches": {
			DB:   branchless,
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  assert.AnError,
		},
		"Metadata, branches and rows are returned": {
			DB:   nested,
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Contains: []string{
				`"name":"Runbook"`,
				`"icon":"lucide:rocket"`,
				`"parent_id":"` + parentID.String() + `"`,
				`"protected":true`,
				`"branch":{"id":"` + _stubMainBranchID.String() + `","name":"main","protected":true,"default":true}`,
				`"branches":[{"id":"` + _stubMainBranchID.String() + `","name":"main","protected":false,"default":true,"updated_at":`,
				`{"id":"` + _stubBranchID.String() + `","name":"draft","protected":false,"default":false,"updated_at":`,
				`"uid":"a"`,
				`"kind":"paragraph"`,
			},
		},
		"Another branch is read": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubBranchID) + `}`,
			Contains: []string{`"branch":{"id":"` + _stubBranchID.String() + `","name":"draft","protected":false,"default":false}`, `"uid":"a"`},
		},
		"Root document omits the parent": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `}`,
			Contains: []string{`"name":"Runbook"`, `"protected":false`},
			Omits:    []string{`"parent_id"`, `"branch_id"`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := getDocument{}.Execute(testInput(testDeps(c.DB, nil, nil), NameGetDocument, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

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

	assertValidate(t, createDocumentArgs{Name: "n"}, map[string]Args{
		"name": createDocumentArgs{},
	})
}

func Test_createDocument_Info(t *testing.T) {
	t.Parallel()

	info := createDocument{}.Info()

	assert.Equal(t, NameCreateDocument, info.Name)
	assert.Equal(t, []string{"name"}, info.Required)
	assert.Contains(t, info.Properties, document.AttrIcon)
}

func Test_createDocument_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, createDocument{}.Traits())
}

func Test_createDocument_Title(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	got, err := createDocument{}.Title(testInput(d, NameCreateDocument, `{"name":"Runbook"}`))
	require.NoError(t, err)
	assert.Equal(t, `Creating "Runbook"`, got)

	_, err = createDocument{}.Title(testInput(d, NameCreateDocument, `{}`))
	require.Error(t, err)
}

func Test_createDocument_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	// a document that does not exist yet has no id to name.
	got, err := createDocument{}.Summary(testInput(d, NameCreateDocument, `{"name":"Runbook"}`))
	require.NoError(t, err)
	assert.Equal(t, NameCreateDocument, got.Tool)
	assert.Equal(t, `Create document "Runbook"`, got.Summary)
	assert.Empty(t, got.DocumentID)

	_, err = createDocument{}.Summary(testInput(d, NameCreateDocument, `{}`))
	require.Error(t, err)
}

func Test_createDocument_Execute(t *testing.T) {
	t.Parallel()

	parentID := xid.New()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubCreateDB(nil), Args: `{`, Err: assert.AnError},
		"Name is required":    {DB: stubCreateDB(nil), Args: `{}`, Err: assert.AnError},
		"Parent id is not a valid xid": {
			DB:   stubCreateDB(nil),
			Args: `{"name":"Runbook","parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Empty parent id is refused": {
			DB:   stubCreateDB(nil),
			Args: `{"name":"Runbook","parent_id":""}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentExists": {
			DB:   stubCreateDB(assert.AnError),
			Args: `{"name":"Runbook","parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by the transaction": {
			DB: &DBMock{
				BeginTxFunc: func(context.Context, any) error { return assert.AnError },
			},
			Args: `{"name":"Runbook"}`,
			Err:  assert.AnError,
		},
		"Created at the root": {
			DB:     stubCreateDB(nil),
			Args:   `{"name":"Runbook"}`,
			Notify: 1,
		},
		"Created under a parent": {
			DB:     stubCreateDB(nil),
			Args:   `{"name":"Runbook","icon":"lucide:rocket","parent_id":"` + parentID.String() + `"}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := createDocument{}.Execute(
				testInput(testDeps(c.DB, nil, tree), NameCreateDocument, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

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

	assertValidate(t, deleteDocumentArgs{DocumentID: _testDocID}, map[string]Args{
		"document_id": deleteDocumentArgs{},
	})
}

func Test_deleteDocument_Info(t *testing.T) {
	t.Parallel()

	info := deleteDocument{}.Info()

	assert.Equal(t, NameDeleteDocument, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{_keyDocumentID}, info.Required)
}

func Test_deleteDocument_Traits(t *testing.T) {
	t.Parallel()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, deleteDocument{}.Traits())
}

func Test_deleteDocument_Title(t *testing.T) {
	t.Parallel()

	got, err := deleteDocument{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameDeleteDocument,
		`{"document_id":"`+_testDocID.String()+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Deleting Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = deleteDocument{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameDeleteDocument, requiredArgs(t, NameDeleteDocument)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = deleteDocument{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameDeleteDocument, `{`))
	require.Error(t, err)
}

func Test_deleteDocument_Summary(t *testing.T) {
	t.Parallel()

	got, err := deleteDocument{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameDeleteDocument,
		`{"document_id":"`+_testDocID.String()+`"}`,
	))
	require.NoError(t, err)

	assert.Equal(t, NameDeleteDocument, got.Tool)
	assert.Equal(t, _testDocID, got.DocumentID)
	assert.Equal(t, "Runbook", got.DocumentName)
	assert.Equal(t, "Delete Runbook", got.Summary)

	// the delete cascades, so a card naming only the document would
	// have the user approve a subtree they were never shown.
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

	got, err = deleteDocument{}.Summary(testInput(
		testDeps(withKids, nil, nil), NameDeleteDocument,
		`{"document_id":"`+_testDocID.String()+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Delete Runbook and the 3 pages nested under it", got.Summary)

	// one reads as one.
	withOneKid := stubDocumentDB()
	withOneKid.FetchDocumentTreeFunc = func(context.Context, string) (document.Summaries, error) {
		return document.Summaries{{
			ID:       _testDocID,
			Children: document.Summaries{{ID: xid.New()}},
		}}, nil
	}

	got, err = deleteDocument{}.Summary(testInput(
		testDeps(withOneKid, nil, nil), NameDeleteDocument,
		`{"document_id":"`+_testDocID.String()+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Delete Runbook and the 1 page nested under it", got.Summary)

	// a tree that cannot be read still names the document: the count is
	// worth having, never worth failing the confirmation over.
	treeFails := stubDocumentDB()
	treeFails.FetchDocumentTreeFunc = func(context.Context, string) (document.Summaries, error) {
		return nil, assert.AnError
	}

	got, err = deleteDocument{}.Summary(testInput(
		testDeps(treeFails, nil, nil), NameDeleteDocument,
		`{"document_id":"`+_testDocID.String()+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Delete Runbook", got.Summary)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = deleteDocument{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameDeleteDocument, requiredArgs(t, NameDeleteDocument)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = deleteDocument{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameDeleteDocument, `{`))
	require.Error(t, err)
}

func Test_deleteDocument_Execute(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	// stubDeleteDB wires the given DB (nil for a bare one) with a
	// transaction whose delete reports one destroyed document, or fails
	// with deleteErr.
	stubDeleteDB := func(db *DBMock, deleteErr error) *DBMock {
		if db == nil {
			db = &DBMock{}
		}

		db.BeginTxFunc = func(_ context.Context, dest any) error {
			reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(&TxMock{
				DeleteDocumentFunc: func(_ context.Context, id xid.ID, _ string) ([]xid.ID, error) {
					return []xid.ID{id}, deleteErr
				},
				InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
					return nil
				},
			}))

			return nil
		}

		return db
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: &DBMock{}, Args: `{`, Err: assert.AnError},
		"Document id is not a valid xid": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by Tx.DeleteDocument": {
			DB:   stubDeleteDB(nil, assert.AnError),
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:     stubDeleteDB(stubDocumentDB(), nil),
			Args:   `{"document_id":"` + docID.String() + `"}`,
			Notify: 1,
		},
		"Deleted even when the parent cannot be captured": {
			DB: stubDeleteDB(&DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			}, nil),
			Args:   `{"document_id":"` + docID.String() + `"}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := deleteDocument{}.Execute(
				testInput(testDeps(c.DB, nil, tree), NameDeleteDocument, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

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

	assertValidate(t, updateDocumentArgs{DocumentID: _testDocID, Name: "n"}, map[string]Args{
		"document_id": updateDocumentArgs{Name: "n"},
	})

	// any one change is enough, and the root is a change.
	require.NoError(t, updateDocumentArgs{DocumentID: _testDocID, Icon: "i"}.Validate())
	require.NoError(t, updateDocumentArgs{DocumentID: _testDocID, ParentID: &root}.Validate())

	// a call that changes nothing is refused, named by what it takes.
	err := updateDocumentArgs{DocumentID: _testDocID}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of name, icon or parent_id")

	// a parent that is not an id is refused before anything is looked up.
	err = updateDocumentArgs{DocumentID: _testDocID, ParentID: &bad}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parent_id")
}

func Test_updateDocumentArgs_parent(t *testing.T) {
	t.Parallel()

	root := ""
	id := xid.New()
	named := id.String()

	// absent: no move.
	_, ok := updateDocumentArgs{}.parent()
	assert.False(t, ok)

	// empty: the root.
	parent, ok := updateDocumentArgs{ParentID: &root}.parent()
	assert.True(t, ok)
	assert.False(t, parent.Valid)

	// an id: that parent.
	parent, ok = updateDocumentArgs{ParentID: &named}.parent()
	assert.True(t, ok)
	assert.Equal(t, null.ValueFrom(id), parent)
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

	assert.Equal(t, NameUpdateDocument, info.Name)
	assert.Equal(t, []string{_keyDocumentID}, info.Required)
	assert.Contains(t, info.Properties, _keyName)
	assert.Contains(t, info.Properties, document.AttrIcon)
	assert.Contains(t, info.Properties, "parent_id")
}

func Test_updateDocument_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, updateDocument{}.Traits())
}

func Test_updateDocument_Title(t *testing.T) {
	t.Parallel()

	got, err := updateDocument{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameUpdateDocument,
		`{"document_id":"`+_testDocID.String()+`","name":"Playbook"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = updateDocument{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateDocument,
		`{"document_id":"`+_testDocID.String()+`","name":"Playbook"}`))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateDocument{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateDocument, `{`))
	require.Error(t, err)
}

func Test_updateDocument_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)
	doc := `{"document_id":"` + _testDocID.String() + `"`

	cc := map[string]struct {
		Args   string
		Result string
	}{
		"Rename":    {Args: doc + `,"name":"Playbook"}`, Result: `Rename Runbook to "Playbook"`},
		"Icon":      {Args: doc + `,"icon":"mingcute:rocket-fill"}`, Result: "Set the icon to mingcute:rocket-fill"},
		"Root":      {Args: doc + `,"parent_id":""}`, Result: "Move it to the org root"},
		"Parent":    {Args: doc + `,"parent_id":"` + xid.New().String() + `"}`, Result: "Move it under another document"},
		"Two":       {Args: doc + `,"name":"Playbook","icon":"i"}`, Result: `Rename Runbook to "Playbook" and set the icon to i`},
		"All three": {Args: doc + `,"name":"Playbook","icon":"i","parent_id":""}`, Result: `Rename Runbook to "Playbook", set the icon to i and move it to the org root`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateDocument{}.Summary(testInput(d, NameUpdateDocument, c.Args))
			require.NoError(t, err)
			assert.Equal(t, c.Result, got.Summary)
		})
	}

	// a call that changes nothing is refused before a card is built.
	_, err := updateDocument{}.Summary(testInput(d, NameUpdateDocument, doc+`}`))
	require.Error(t, err)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = updateDocument{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateDocument, doc+`,"name":"n"}`))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateDocument{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateDocument, `{`))
	require.Error(t, err)
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
		DB      *DBMock
		Applier *EditApplierMock
		Args    string
		Ops     int
		Notify  int
		Result  string
		Err     error
	}{
		"Malformed arguments": {DB: stubDocumentDB(), Applier: stubApplier(), Args: `{`, Err: assert.AnError},
		"Nothing to change": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Args:    doc + `}`,
			Err:     assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:      failingDocumentDB(),
			Applier: stubApplier(),
			Args:    doc + `,"name":"Playbook"}`,
			Err:     assert.AnError,
		},
		"Error returned by the edit": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, bool) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			Args: doc + `,"name":"Playbook"}`,
			Err:  assert.AnError,
		},
		"Renamed": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Args:    doc + `,"name":"Playbook"}`,
			Ops:     1,
			Notify:  1,
			Result:  `{"document_id":"` + _testDocID.String() + `","name":"Playbook"}`,
		},
		"Icon set": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Args:    doc + `,"icon":"i"}`,
			Ops:     1,
			Notify:  1,
			Result:  `{"document_id":"` + _testDocID.String() + `","icon":"i"}`,
		},
		// name and icon travel in one batch, so the live document
		// changes once.
		"Renamed and icon set": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Args:    doc + `,"name":"Playbook","icon":"i"}`,
			Ops:     2,
			Notify:  1,
			Result:  `{"document_id":"` + _testDocID.String() + `","name":"Playbook","icon":"i"}`,
		},
		"Error returned by db.CheckDocumentExists": {
			DB:      moveDB(assert.AnError, false, nil, nil),
			Applier: stubApplier(),
			Args:    doc + `,"parent_id":"` + parentID.String() + `"}`,
			Err:     assert.AnError,
		},
		"Cycle is refused": {
			DB:      moveDB(nil, true, nil, nil),
			Applier: stubApplier(),
			Args:    doc + `,"parent_id":"` + parentID.String() + `"}`,
			Err:     assert.AnError,
		},
		"Error returned by db.UpdateDocumentParentID": {
			DB:      moveDB(nil, false, nil, assert.AnError),
			Applier: stubApplier(),
			Args:    doc + `,"parent_id":"` + parentID.String() + `"}`,
			Err:     assert.AnError,
		},
		// source and destination are the same (both root), so one
		// notification covers both.
		"Moved to the root": {
			DB:      moveDB(nil, false, nil, nil),
			Applier: stubApplier(),
			Args:    doc + `,"parent_id":""}`,
			Notify:  1,
			Result:  `{"document_id":"` + _testDocID.String() + `","parent_id":""}`,
		},
		"Moved under a parent": {
			DB:      moveDB(nil, false, nil, nil),
			Applier: stubApplier(),
			Args:    doc + `,"parent_id":"` + parentID.String() + `"}`,
			Notify:  2,
			Result:  `{"document_id":"` + _testDocID.String() + `","parent_id":"` + parentID.String() + `"}`,
		},
		"Renamed and moved": {
			DB:      moveDB(nil, false, nil, nil),
			Applier: stubApplier(),
			Args:    doc + `,"name":"Playbook","parent_id":"` + parentID.String() + `"}`,
			Ops:     1,
			Notify:  2,
			Result:  `{"document_id":"` + _testDocID.String() + `","name":"Playbook","parent_id":"` + parentID.String() + `"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := updateDocument{}.Execute(
				testInput(testDeps(c.DB, c.Applier, tree), NameUpdateDocument, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			// every requested name or icon change is in the one batch
			// that reached the live document.
			if c.Ops == 0 {
				assert.Empty(t, c.Applier.ApplyCalls())
			} else {
				require.Len(t, c.Applier.ApplyCalls(), 1)
				assert.Len(t, c.Applier.ApplyCalls()[0].Ops, c.Ops)
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_summariesToTree(t *testing.T) {
	t.Parallel()

	assert.Nil(t, summariesToTree(nil))

	id := xid.New()
	child := xid.New()

	branch := xid.New()

	got := summariesToTree(document.Summaries{{
		ID:              id,
		DocumentName:    "Parent",
		DefaultBranchID: branch,
		Icon:            "lucide:file",
		Children:        document.Summaries{{ID: child, DocumentName: "Child"}},
	}})

	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.Equal(t, "Parent", got[0].Name)
	assert.Equal(t, branch, got[0].DefaultBranchID)
	require.Len(t, got[0].Children, 1)
	assert.Equal(t, child, got[0].Children[0].ID)
}

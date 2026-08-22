package tools

import (
	"context"
	"reflect"
	"testing"

	"github.com/guregu/null/v5"
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

	treeDB := &DBMock{
		FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
			return document.Summaries{{
				ID:           parentID,
				DocumentName: "Parent",
				Icon:         "lucide:file",
				Children:     document.Summaries{{ID: childID, DocumentName: "Child"}},
			}}, nil
		},
		FetchDocumentTreeByDocumentParentIDFunc: func(
			_ context.Context, pid null.Value[xid.ID], _ string,
		) (document.Summaries, error) {
			if pid.V != parentID {
				return nil, assert.AnError
			}

			return document.Summaries{{ID: childID, DocumentName: "Child"}}, nil
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
			Result: `{"documents":[{"id":"` + parentID.String() + `","name":"Parent","icon":"lucide:file",` +
				`"children":[{"id":"` + childID.String() + `","name":"Child","icon":""}]}]}`,
		},
		"Children of one parent": {
			DB:     treeDB,
			Args:   `{"parent_id":"` + parentID.String() + `"}`,
			Result: `{"documents":[{"id":"` + childID.String() + `","name":"Child","icon":""}]}`,
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

func Test_getDocument_Info(t *testing.T) {
	t.Parallel()

	info := getDocument{}.Info()

	assert.Equal(t, NameGetDocument, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID}, info.Required)
}

func Test_getDocument_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, getDocument{}.Traits())
}

func Test_getDocument_Title(t *testing.T) {
	t.Parallel()

	got, err := getDocument{}.Title(testInput(testDeps(nil, nil, nil), NameGetDocument, `{}`))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_getDocument_Execute(t *testing.T) {
	t.Parallel()

	docID := xid.New()
	branchID := xid.New()
	parentID := xid.New()

	cc := map[string]struct {
		DB       *DBMock
		Args     string
		Contains []string
		Err      error
	}{
		"Malformed arguments": {DB: &DBMock{}, Args: `{`, Err: assert.AnError},
		"Document id is not a valid xid": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Metadata is returned": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, id xid.ID, _, _ string) (*document.Document, error) {
					return &document.Document{
						Branch: document.Branch{
							BranchID:     branchID,
							BranchName:   "main",
							DocumentName: "Runbook",
							Icon:         "lucide:rocket",
						},
						ID:       id,
						ParentID: null.ValueFrom(parentID),
					}, nil
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Contains: []string{
				`"name":"Runbook"`,
				`"icon":"lucide:rocket"`,
				`"branch_id":"` + branchID.String() + `"`,
				`"parent_id":"` + parentID.String() + `"`,
			},
		},
		"Root document omits the parent": {
			DB:       stubDocumentDB(),
			Args:     `{"document_id":"` + docID.String() + `"}`,
			Contains: []string{`"name":"Runbook"`},
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

			if cn == "Root document omits the parent" {
				assert.NotContains(t, res, `"parent_id"`)
			}
		})
	}
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
	got, err = createDocument{}.Title(testInput(d, NameCreateDocument, `{}`))
	require.NoError(t, err)
	assert.Equal(t, "Creating a document", got)
}

func Test_createDocument_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	// a document that does not exist yet has no id to name.
	got, err := createDocument{}.Summary(testInput(d, NameCreateDocument, `{"name":"Runbook"}`))
	require.NoError(t, err)
	assert.Equal(t, string(NameCreateDocument), got.Tool)
	assert.Equal(t, `Create document "Runbook"`, got.Summary)
	assert.Empty(t, got.DocumentID)

	got, err = createDocument{}.Summary(testInput(d, NameCreateDocument, `{}`))
	require.NoError(t, err)
	assert.Equal(t, "Create a new document", got.Summary)
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

func Test_deleteDocument_Info(t *testing.T) {
	t.Parallel()

	info := deleteDocument{}.Info()

	assert.Equal(t, NameDeleteDocument, info.Name)
	assert.Contains(t, info.Description, "destructive")
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
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Deleting Runbook", got)
}

func Test_deleteDocument_Summary(t *testing.T) {
	t.Parallel()

	got, err := deleteDocument{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameDeleteDocument,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)

	assert.Equal(t, string(NameDeleteDocument), got.Tool)
	assert.Equal(t, _testDocID, got.DocumentID)
	assert.Equal(t, "Runbook", got.DocumentName)
	assert.Equal(t, "Delete Runbook", got.Summary)
}

func Test_deleteDocument_Execute(t *testing.T) {
	t.Parallel()

	docID := xid.New()

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
		"Error returned by db.DeleteDocument": {
			DB: &DBMock{
				DeleteDocumentFunc: func(context.Context, xid.ID, string) error {
					return assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:     stubDocumentDB(),
			Args:   `{"document_id":"` + docID.String() + `"}`,
			Notify: 1,
		},
		"Deleted even when the parent cannot be captured": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
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

func Test_renameDocument_Info(t *testing.T) {
	t.Parallel()

	info := renameDocument{}.Info()

	assert.Equal(t, NameRenameDocument, info.Name)
	assert.Equal(t, []string{_keyDocumentID, "name"}, info.Required)
}

func Test_renameDocument_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, renameDocument{}.Traits())
}

func Test_renameDocument_Title(t *testing.T) {
	t.Parallel()

	got, err := renameDocument{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameRenameDocument,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Renaming Runbook", got)
}

func Test_renameDocument_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	got, err := renameDocument{}.Summary(testInput(d, NameRenameDocument,
		`{"document_id":"`+_testDocID+`","name":"Playbook"}`))
	require.NoError(t, err)
	assert.Equal(t, `Rename Runbook to "Playbook"`, got.Summary)

	// a missing name still produces a readable card.
	got, err = renameDocument{}.Summary(testInput(d, NameRenameDocument,
		`{"document_id":"`+_testDocID+`"}`))
	require.NoError(t, err)
	assert.Equal(t, "Rename Runbook", got.Summary)
}

func Test_renameDocument_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Name is required": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by the edit": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"document_id":"` + _testDocID + `","name":"Playbook"}`,
			Err:  assert.AnError,
		},
		"Renamed": {
			DB:     stubDocumentDB(),
			Args:   `{"document_id":"` + _testDocID + `","name":"Playbook"}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := renameDocument{}.Execute(
				testInput(testDeps(c.DB, stubApplier(), tree), NameRenameDocument, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

func Test_setDocumentIcon_Info(t *testing.T) {
	t.Parallel()

	info := setDocumentIcon{}.Info()

	assert.Equal(t, NameSetDocumentIcon, info.Name)
	assert.Equal(t, []string{_keyDocumentID, document.AttrIcon}, info.Required)
}

func Test_setDocumentIcon_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, setDocumentIcon{}.Traits())
}

func Test_setDocumentIcon_Title(t *testing.T) {
	t.Parallel()

	got, err := setDocumentIcon{}.Title(
		testInput(testDeps(nil, nil, nil), NameSetDocumentIcon, `{}`))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_setDocumentIcon_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	got, err := setDocumentIcon{}.Summary(testInput(d, NameSetDocumentIcon,
		`{"document_id":"`+_testDocID+`","icon":"lucide:rocket"}`))
	require.NoError(t, err)
	assert.Equal(t, "Change icon of Runbook to lucide:rocket", got.Summary)

	got, err = setDocumentIcon{}.Summary(testInput(d, NameSetDocumentIcon,
		`{"document_id":"`+_testDocID+`"}`))
	require.NoError(t, err)
	assert.Equal(t, "Change icon of Runbook", got.Summary)
}

func Test_setDocumentIcon_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Icon is required": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by the edit": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"document_id":"` + _testDocID + `","icon":"lucide:rocket"}`,
			Err:  assert.AnError,
		},
		"Icon set": {
			DB:     stubDocumentDB(),
			Args:   `{"document_id":"` + _testDocID + `","icon":"lucide:rocket"}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := setDocumentIcon{}.Execute(
				testInput(testDeps(c.DB, stubApplier(), tree), NameSetDocumentIcon, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

func Test_moveDocument_Info(t *testing.T) {
	t.Parallel()

	info := moveDocument{}.Info()

	assert.Equal(t, NameMoveDocument, info.Name)
	assert.Equal(t, []string{_keyDocumentID}, info.Required)
	assert.Contains(t, info.Properties, "new_parent_id")
}

func Test_moveDocument_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, moveDocument{}.Traits())
}

func Test_moveDocument_Title(t *testing.T) {
	t.Parallel()

	got, err := moveDocument{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameMoveDocument,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Moving Runbook", got)
}

func Test_moveDocument_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	got, err := moveDocument{}.Summary(testInput(d, NameMoveDocument,
		`{"document_id":"`+_testDocID+`"}`))
	require.NoError(t, err)
	assert.Equal(t, "Move Runbook to the org root", got.Summary)

	got, err = moveDocument{}.Summary(testInput(d, NameMoveDocument,
		`{"document_id":"`+_testDocID+`","new_parent_id":"`+xid.New().String()+`"}`))
	require.NoError(t, err)
	assert.Equal(t, "Move Runbook under another document", got.Summary)
}

func Test_moveDocument_Execute(t *testing.T) {
	t.Parallel()

	docID := xid.New()
	parentID := xid.New()

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
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: moveDB(nil, false, nil, nil), Args: `{`, Err: assert.AnError},
		"Document id is not a valid xid": {
			DB:   moveDB(nil, false, nil, nil),
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"New parent id is not a valid xid": {
			DB:   moveDB(nil, false, nil, nil),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentExists": {
			DB:   moveDB(assert.AnError, false, nil, nil),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentCycle": {
			DB:   moveDB(nil, false, assert.AnError, nil),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Cycle is refused": {
			DB:   moveDB(nil, true, nil, nil),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.UpdateDocumentParentID": {
			DB:   moveDB(nil, false, nil, assert.AnError),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Moved to the root": {
			// source and destination are the same (both root), so the
			// duplicate notification is skipped.
			DB:     moveDB(nil, false, nil, nil),
			Args:   `{"document_id":"` + docID.String() + `"}`,
			Notify: 1,
		},
		"Moved under a parent": {
			DB:     moveDB(nil, false, nil, nil),
			Args:   `{"document_id":"` + docID.String() + `","new_parent_id":"` + parentID.String() + `"}`,
			Notify: 2,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			res, err := moveDocument{}.Execute(
				testInput(testDeps(c.DB, nil, tree), NameMoveDocument, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			assert.Contains(t, res, `"document_id":"`+docID.String()+`"`)
		})
	}
}

func Test_summariesToTree(t *testing.T) {
	t.Parallel()

	assert.Nil(t, summariesToTree(nil))

	id := xid.New()
	child := xid.New()

	got := summariesToTree(document.Summaries{{
		ID:           id,
		DocumentName: "Parent",
		Icon:         "lucide:file",
		Children:     document.Summaries{{ID: child, DocumentName: "Child"}},
	}})

	require.Len(t, got, 1)
	assert.Equal(t, id.String(), got[0].ID)
	assert.Equal(t, "Parent", got[0].Name)
	require.Len(t, got[0].Children, 1)
	assert.Equal(t, child.String(), got[0].Children[0].ID)
}

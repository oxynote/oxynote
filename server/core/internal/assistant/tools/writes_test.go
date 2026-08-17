package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Manager_applyEdit(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	stubDB := func(err error) *DBMock {
		return &DBMock{
			FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
				if err != nil {
					return nil, err
				}

				return &document.Document{
					Branch:         document.Branch{BranchID: branchID},
					ID:             id,
					OrganizationID: orgID,
				}, nil
			},
		}
	}

	cc := map[string]struct {
		DB         *DBMock
		Applier    *EditApplierMock
		DocumentID string
		ApplyCalls int
		RespJSON   string
		Err        error
	}{
		"Invalid document id": {
			DB:         stubDB(nil),
			Applier:    &EditApplierMock{},
			DocumentID: "not-an-xid",
			Err:        assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:         stubDB(assert.AnError),
			Applier:    &EditApplierMock{},
			DocumentID: docID.String(),
			Err:        assert.AnError,
		},
		"Error returned by EditApplier.Apply": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		"Partial failure surfaces per-op errors": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{
						Applied: 1,
						Errors:  []edit.OpError{{Index: 1, Message: "uid not found"}},
					}, nil
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			RespJSON:   `{"applied":1,"errors":[{"index":1,"message":"uid not found"}]}`,
		},
		"Successful apply": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			RespJSON:   `{"applied":1,"errors":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				log:     slog.New(slog.DiscardHandler),
				db:      c.DB,
				applier: c.Applier,
				orgID:   "org",
				userID:  "user",
			}

			res, err := m.applyEdit(context.Background(), c.DocumentID, []edit.Operation{edit.Delete("target")})
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Applier.ApplyCalls()
			require.Len(t, ff, c.ApplyCalls)

			if c.ApplyCalls > 0 {
				assert.Equal(t, docID.String(), ff[0].DocumentID)
				assert.Equal(t, branchID.String(), ff[0].BranchID)
				assert.Len(t, ff[0].Ops, 1)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

// stubEditManager builds a Manager whose db resolves documents to
// branchID and whose applier succeeds, for the edit-based tools.
func stubEditManager(db *DBMock, applier *EditApplierMock, tree *TreeNotifierMock) *Manager {
	m := &Manager{
		log:     slog.New(slog.DiscardHandler),
		db:      db,
		applier: applier,
		orgID:   "org",
		userID:  "user",
	}

	if tree != nil {
		m.tree = tree
	}

	return m
}

// stubResolvingDB returns a DB mock whose FetchDocument succeeds
// with the given parent, or fails with err.
func stubResolvingDB(branchID xid.ID, parent null.Value[xid.ID], err error) *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			if err != nil {
				return nil, err
			}

			return &document.Document{
				Branch:         document.Branch{BranchID: branchID},
				ID:             id,
				ParentID:       parent,
				OrganizationID: orgID,
			}, nil
		},
	}
}

// stubOKApplier returns an applier that reports one applied op.
func stubOKApplier() *EditApplierMock {
	return &EditApplierMock{
		ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
			return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
		},
	}
}

func Test_Manager_resolveDoc(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		DB         *DBMock
		DocumentID string
		Result     docRef
		Err        error
	}{
		"Invalid document id": {
			DB:         &DBMock{},
			DocumentID: "nope",
			Err:        assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:         stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError),
			DocumentID: docID.String(),
			Err:        assert.AnError,
		},
		"Successful resolve": {
			DB:         stubResolvingDB(branchID, null.Value[xid.ID]{}, nil),
			DocumentID: docID.String(),
			Result: docRef{
				DocumentID: docID.String(),
				BranchID:   branchID.String(),
				OrgID:      "org",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(c.DB, nil, nil)

			ref, err := m.resolveDoc(context.Background(), c.DocumentID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, ref)
		})
	}
}

func Test_Manager_createDocument(t *testing.T) {
	parentID := xid.New()

	type tcase struct {
		DB          *DBMock
		Tree        *TreeNotifierMock
		Args        string
		Icon        string
		NotifyCalls int
		Err         error
	}

	okDB := func(checkErr, insertErr, upsertErr error) *DBMock {
		return &DBMock{
			CheckDocumentExistsFunc: func(_ context.Context, _ xid.ID, _ string) error {
				return checkErr
			},
			InsertDocumentFunc: func(_ context.Context, _ document.Document) error {
				return insertErr
			},
			UpsertDocumentMaintainersFunc: func(_ context.Context, _ xid.ID, _ string, _ []string) error {
				return upsertErr
			},
		}
	}

	cc := map[string]tcase{
		"Malformed args": {
			DB:   &DBMock{},
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing name": {
			DB:   &DBMock{},
			Args: `{}`,
			Err:  assert.AnError,
		},
		"Invalid parent id": {
			DB:   &DBMock{},
			Args: `{"name":"Doc","parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentExists": {
			DB:   okDB(assert.AnError, nil, nil),
			Args: `{"name":"Doc","parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.InsertDocument": {
			DB:   okDB(nil, assert.AnError, nil),
			Args: `{"name":"Doc"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.UpsertDocumentMaintainers": {
			DB:   okDB(nil, nil, assert.AnError),
			Args: `{"name":"Doc"}`,
			Err:  assert.AnError,
		},
		"Successful creation with default icon": {
			DB:          okDB(nil, nil, nil),
			Tree:        &TreeNotifierMock{},
			Args:        `{"name":"Doc"}`,
			Icon:        "lucide:file",
			NotifyCalls: 1,
		},
		"Successful creation with explicit icon and parent": {
			DB:          okDB(nil, nil, nil),
			Tree:        &TreeNotifierMock{},
			Args:        `{"name":"Doc","icon":"lucide:cat","parent_id":"` + parentID.String() + `"}`,
			Icon:        "lucide:cat",
			NotifyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(c.DB, nil, c.Tree)

			res, err := m.createDocument(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if c.Tree != nil {
				assert.Len(t, c.Tree.NotifyTreeChangeCalls(), c.NotifyCalls)
			}

			if err != nil {
				return
			}

			ff := c.DB.InsertDocumentCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Icon, ff[0].Doc.Icon)
			assert.Equal(t, "Doc", ff[0].Doc.DocumentName)

			var out struct {
				DocumentID string `json:"document_id"`
				BranchID   string `json:"branch_id"`
			}

			require.NoError(t, json.Unmarshal(res, &out))
			assert.NotEmpty(t, out.DocumentID)
			assert.NotEmpty(t, out.BranchID)
		})
	}
}

func Test_Manager_deleteDocument(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	type tcase struct {
		DB          *DBMock
		Tree        *TreeNotifierMock
		Args        string
		NotifyCalls int
		Err         error
	}

	cc := map[string]tcase{
		"Malformed args": {
			DB:   &DBMock{},
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Invalid document id": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.DeleteDocument": {
			DB: &DBMock{
				FetchDocumentFunc: stubResolvingDB(branchID, null.Value[xid.ID]{}, nil).FetchDocumentFunc,
				DeleteDocumentFunc: func(_ context.Context, _ xid.ID, _ string) error {
					return assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Successful delete despite parent lookup failure": {
			DB: &DBMock{
				FetchDocumentFunc: stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError).FetchDocumentFunc,
				DeleteDocumentFunc: func(_ context.Context, _ xid.ID, _ string) error {
					return nil
				},
			},
			Tree:        &TreeNotifierMock{},
			Args:        `{"document_id":"` + docID.String() + `"}`,
			NotifyCalls: 1,
		},
		"Successful delete": {
			DB: &DBMock{
				FetchDocumentFunc: stubResolvingDB(branchID, null.ValueFrom(xid.New()), nil).FetchDocumentFunc,
				DeleteDocumentFunc: func(_ context.Context, _ xid.ID, _ string) error {
					return nil
				},
			},
			Tree:        &TreeNotifierMock{},
			Args:        `{"document_id":"` + docID.String() + `"}`,
			NotifyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(c.DB, nil, c.Tree)

			res, err := m.deleteDocument(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if c.Tree != nil {
				assert.Len(t, c.Tree.NotifyTreeChangeCalls(), c.NotifyCalls)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"document_id":"`+docID.String()+`","deleted":true}`, string(res))
		})
	}
}

func Test_Manager_renameDocument(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		DB          *DBMock
		Applier     *EditApplierMock
		Tree        *TreeNotifierMock
		Args        string
		NotifyCalls int
		Err         error
	}{
		"Malformed args": {
			DB:      &DBMock{},
			Applier: &EditApplierMock{},
			Args:    `{broken`,
			Err:     assert.AnError,
		},
		"Missing name": {
			DB:      &DBMock{},
			Applier: &EditApplierMock{},
			Args:    `{"document_id":"` + docID.String() + `"}`,
			Err:     assert.AnError,
		},
		"Error returned by applyEdit": {
			DB:      stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError),
			Applier: &EditApplierMock{},
			Args:    `{"document_id":"` + docID.String() + `","name":"New"}`,
			Err:     assert.AnError,
		},
		"Successful rename": {
			DB:          stubResolvingDB(branchID, null.Value[xid.ID]{}, nil),
			Applier:     stubOKApplier(),
			Tree:        &TreeNotifierMock{},
			Args:        `{"document_id":"` + docID.String() + `","name":"New"}`,
			NotifyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(c.DB, c.Applier, c.Tree)

			res, err := m.renameDocument(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if c.Tree != nil {
				assert.Len(t, c.Tree.NotifyTreeChangeCalls(), c.NotifyCalls)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_setDocumentIcon(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		DB      *DBMock
		Applier *EditApplierMock
		Args    string
		Err     error
	}{
		"Malformed args": {
			DB:      &DBMock{},
			Applier: &EditApplierMock{},
			Args:    `{broken`,
			Err:     assert.AnError,
		},
		"Missing icon": {
			DB:      &DBMock{},
			Applier: &EditApplierMock{},
			Args:    `{"document_id":"` + docID.String() + `"}`,
			Err:     assert.AnError,
		},
		"Error returned by applyEdit": {
			DB:      stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError),
			Applier: &EditApplierMock{},
			Args:    `{"document_id":"` + docID.String() + `","icon":"lucide:cat"}`,
			Err:     assert.AnError,
		},
		"Successful icon change": {
			DB:      stubResolvingDB(branchID, null.Value[xid.ID]{}, nil),
			Applier: stubOKApplier(),
			Args:    `{"document_id":"` + docID.String() + `","icon":"lucide:cat"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(c.DB, c.Applier, nil)

			res, err := m.setDocumentIcon(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_moveDocument(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()
	oldParent := xid.New()
	newParent := xid.New()

	okDB := func(fetchErr, checkErr, updateErr error) *DBMock {
		db := stubResolvingDB(branchID, null.ValueFrom(oldParent), fetchErr)

		db.CheckDocumentExistsFunc = func(_ context.Context, _ xid.ID, _ string) error {
			return checkErr
		}
		db.CheckDocumentCycleFunc = func(_ context.Context, _, _ xid.ID, _ string) (bool, error) {
			return false, nil
		}
		db.UpdateDocumentParentIDFunc = func(_ context.Context, _ xid.ID, _ null.Value[xid.ID], _ string) error {
			return updateErr
		}

		return db
	}

	cc := map[string]struct {
		DB          *DBMock
		Args        string
		NotifyCalls int
		RespJSON    string
		Err         error
	}{
		"Malformed args": {
			DB:   &DBMock{},
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Invalid document id": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:   okDB(assert.AnError, nil, nil),
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Invalid new parent id": {
			DB:   okDB(nil, nil, nil),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentExists": {
			DB:   okDB(nil, assert.AnError, nil),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + newParent.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentCycle": {
			DB: func() *DBMock {
				db := okDB(nil, nil, nil)
				db.CheckDocumentCycleFunc = func(_ context.Context, _, _ xid.ID, _ string) (bool, error) {
					return false, assert.AnError
				}

				return db
			}(),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + newParent.String() + `"}`,
			Err:  assert.AnError,
		},
		"New parent is the document or its descendant": {
			DB: func() *DBMock {
				db := okDB(nil, nil, nil)
				db.CheckDocumentCycleFunc = func(_ context.Context, _, _ xid.ID, _ string) (bool, error) {
					return true, nil
				}

				return db
			}(),
			Args: `{"document_id":"` + docID.String() + `","new_parent_id":"` + newParent.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.UpdateDocumentParentID": {
			DB:   okDB(nil, nil, assert.AnError),
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Move to the org root notifies both subtrees": {
			DB:          okDB(nil, nil, nil),
			Args:        `{"document_id":"` + docID.String() + `"}`,
			NotifyCalls: 2,
			RespJSON:    `{"document_id":"` + docID.String() + `"}`,
		},
		"Move under a new parent": {
			DB:          okDB(nil, nil, nil),
			Args:        `{"document_id":"` + docID.String() + `","new_parent_id":"` + newParent.String() + `"}`,
			NotifyCalls: 2,
			RespJSON:    `{"document_id":"` + docID.String() + `","new_parent_id":"` + newParent.String() + `"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			m := stubEditManager(c.DB, nil, tree)

			res, err := m.moveDocument(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.NotifyCalls)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

func Test_Manager_insertBlock(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args    string
		Content document.RootBlock
		Err     error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing reference uid": {
			Args: `{"document_id":"` + docID.String() + `","block":{"type":"paragraph"}}`,
			Err:  assert.AnError,
		},
		"Invalid block": {
			Args: `{"document_id":"` + docID.String() + `","reference_block_uid":"r","position":"after","block":{"type":"heading"}}`,
			Err:  assert.AnError,
		},
		// a macro internal next to a root block would land at the root,
		// where the editor schema has no place for it.
		"Macro internal next to a root block": {
			Args: `{"document_id":"` + docID.String() + `","reference_block_uid":"r","position":"after","block":{"type":"titled_code","text":"x","attrs":{"title":"t"}}}`,
			Content: document.RootBlock{
				Type: document.BlockNodeDoc,
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Attrs: document.Attributes{"uid": "r"}},
				},
			},
			Err: assert.AnError,
		},
		"Macro internal next to a nested block": {
			Args: `{"document_id":"` + docID.String() + `","reference_block_uid":"r","position":"after","block":{"type":"titled_code","text":"x","attrs":{"title":"t"}}}`,
			Content: document.RootBlock{
				Type: document.BlockNodeDoc,
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Attrs: document.Attributes{"uid": "other"}},
				},
			},
		},
		"Invalid position": {
			Args: `{"document_id":"` + docID.String() + `","reference_block_uid":"r","position":"sideways","block":{"type":"paragraph"}}`,
			Err:  assert.AnError,
		},
		"Insert before": {
			Args: `{"document_id":"` + docID.String() + `","reference_block_uid":"r","position":"before","block":{"type":"paragraph","text":"x"}}`,
		},
		"Insert after": {
			Args: `{"document_id":"` + docID.String() + `","reference_block_uid":"r","position":"after","block":{"type":"paragraph","text":"x"}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubResolvingDB(branchID, null.Value[xid.ID]{}, nil)
			db.FetchMainBranchContentFunc = func(context.Context, xid.ID, string) (document.Content, error) {
				return document.Content{Content: c.Content}, nil
			}

			m := stubEditManager(db, stubOKApplier(), nil)

			res, err := m.insertBlock(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_appendBlock(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args string
		Err  error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Root-restricted block is rejected": {
			Args: `{"document_id":"` + docID.String() + `","block":{"type":"titled_code","text":"x","attrs":{"title":"t"}}}`,
			Err:  assert.AnError,
		},
		"Successful append": {
			Args: `{"document_id":"` + docID.String() + `","block":{"type":"paragraph","text":"x"}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := m.appendBlock(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_prependBlock(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args string
		Err  error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Root-restricted block is rejected": {
			Args: `{"document_id":"` + docID.String() + `","block":{"type":"metric"}}`,
			Err:  assert.AnError,
		},
		"Successful prepend": {
			Args: `{"document_id":"` + docID.String() + `","block":{"type":"paragraph","text":"x"}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := m.prependBlock(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_replaceBlock(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args    string
		Content document.RootBlock
		Err     error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing block uid": {
			Args: `{"document_id":"` + docID.String() + `","block":{"type":"paragraph"}}`,
			Err:  assert.AnError,
		},
		"Invalid block": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b","block":{"type":"heading"}}`,
			Err:  assert.AnError,
		},
		// the replacement lands where the target sits, so replacing a root
		// block with a macro internal puts it at the root.
		"Macro internal replacing a root block": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b","block":{"type":"titled_code","text":"x","attrs":{"title":"t"}}}`,
			Content: document.RootBlock{
				Type: document.BlockNodeDoc,
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Attrs: document.Attributes{"uid": "b"}},
				},
			},
			Err: assert.AnError,
		},
		"Successful replace": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b","block":{"type":"paragraph","text":"x"}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubResolvingDB(branchID, null.Value[xid.ID]{}, nil)
			db.FetchMainBranchContentFunc = func(context.Context, xid.ID, string) (document.Content, error) {
				return document.Content{Content: c.Content}, nil
			}

			m := stubEditManager(db, stubOKApplier(), nil)

			res, err := m.replaceBlock(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_updateBlockText(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args string
		Err  error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing block uid": {
			Args: `{"document_id":"` + docID.String() + `","text":"x"}`,
			Err:  assert.AnError,
		},
		"Successful update": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b","text":"x"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := m.updateBlockText(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_updateBlockAttrs(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args string
		Err  error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing block uid": {
			Args: `{"document_id":"` + docID.String() + `","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Empty attrs": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b","attrs":{}}`,
			Err:  assert.AnError,
		},
		"Successful update": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b","attrs":{"level":2}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := m.updateBlockAttrs(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_deleteBlock(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		Args string
		Err  error
	}{
		"Malformed args": {
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing block uid": {
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Successful delete": {
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := stubEditManager(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := m.deleteBlock(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, string(res))
		})
	}
}

func Test_Manager_notifyTreeChange(t *testing.T) {
	t.Parallel()

	// nil notifier is a silent no-op
	m := stubEditManager(&DBMock{}, nil, nil)
	m.notifyTreeChange(null.Value[xid.ID]{})

	// configured notifier receives the parent
	tree := &TreeNotifierMock{}
	m = stubEditManager(&DBMock{}, nil, tree)

	parent := null.ValueFrom(xid.New())
	m.notifyTreeChange(parent)

	ff := tree.NotifyTreeChangeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, parent, ff[0].ParentID)
}

func Test_Manager_notifyTreeChangeForDocument(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		DB          *DBMock
		DocumentID  string
		NotifyCalls int
	}{
		"Invalid document id skips": {
			DB:          &DBMock{},
			DocumentID:  "nope",
			NotifyCalls: 0,
		},
		"Fetch failure skips": {
			DB:          stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError),
			DocumentID:  docID.String(),
			NotifyCalls: 0,
		},
		"Successful notify": {
			DB:          stubResolvingDB(branchID, null.ValueFrom(xid.New()), nil),
			DocumentID:  docID.String(),
			NotifyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			m := stubEditManager(c.DB, nil, tree)

			m.notifyTreeChangeForDocument(context.Background(), c.DocumentID)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.NotifyCalls)
		})
	}
}

package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_moveDocument_InvokableRun(t *testing.T) {
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
		"Move within the same parent notifies once": {
			DB:          okDB(nil, nil, nil),
			Args:        `{"document_id":"` + docID.String() + `","new_parent_id":"` + oldParent.String() + `"}`,
			NotifyCalls: 1,
			RespJSON:    `{"document_id":"` + docID.String() + `","new_parent_id":"` + oldParent.String() + `"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			inp := stubEditInput(c.DB, nil, tree)

			res, err := (&moveDocument{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.NotifyCalls)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

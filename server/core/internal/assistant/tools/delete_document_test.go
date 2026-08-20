package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_deleteDocument_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(c.DB, nil, c.Tree)

			res, err := (&deleteDocument{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Tree != nil {
				assert.Len(t, c.Tree.NotifyTreeChangeCalls(), c.NotifyCalls)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"document_id":"`+docID.String()+`","deleted":true}`, res)
		})
	}
}

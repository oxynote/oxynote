package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_renameDocument_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(c.DB, c.Applier, c.Tree)

			res, err := (&renameDocument{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Tree != nil {
				assert.Len(t, c.Tree.NotifyTreeChangeCalls(), c.NotifyCalls)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_setDocumentIcon_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(c.DB, c.Applier, nil)

			res, err := (&setDocumentIcon{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

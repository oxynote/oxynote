package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_updateBlockText_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := (&updateBlockText{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

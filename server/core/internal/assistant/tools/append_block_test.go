package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_appendBlock_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(stubResolvingDB(branchID, null.Value[xid.ID]{}, nil), stubOKApplier(), nil)

			res, err := (&appendBlock{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

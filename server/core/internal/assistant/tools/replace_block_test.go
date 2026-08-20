package tools

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_replaceBlock_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(db, stubOKApplier(), nil)

			res, err := (&replaceBlock{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

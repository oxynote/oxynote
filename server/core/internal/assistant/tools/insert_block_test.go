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

func Test_insertBlock_InvokableRun(t *testing.T) {
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

			inp := stubEditInput(db, stubOKApplier(), nil)

			res, err := (&insertBlock{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
		})
	}
}

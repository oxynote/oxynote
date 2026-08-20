package tools

import (
	"context"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_readBlock_InvokableRun(t *testing.T) {
	docID := xid.New()

	stubContent := func(blocks ...document.Block) *DBMock {
		return &DBMock{
			FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
				return document.Content{
					Content: document.RootBlock{Type: document.BlockNodeDoc, Content: blocks},
				}, nil
			},
		}
	}

	cc := map[string]struct {
		DB       *DBMock
		Args     string
		RespJSON string
		Err      error
	}{
		"Malformed args": {
			DB:   &DBMock{},
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Invalid document id": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Missing block uid": {
			DB:   &DBMock{},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Block not found": {
			DB:   stubContent(pmBlock(document.BlockNodeParagraph, "other", nil)),
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Unsupported block type fails to compact": {
			DB:   stubContent(pmBlock("weirdNode", "b1", nil)),
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Successful read": {
			DB:       stubContent(pmBlock(document.BlockNodeParagraph, "b1", nil, pmText("hello"))),
			Args:     `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			RespJSON: `{"type":"paragraph","uid":"b1","text":"hello"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := (&readBlock{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

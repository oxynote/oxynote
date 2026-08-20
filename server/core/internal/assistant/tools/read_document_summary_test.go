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

func Test_readDocumentSummary_InvokableRun(t *testing.T) {
	docID := xid.New()

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
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Successful summary": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{
						DocumentName: "Cat Facts",
						Content: document.RootBlock{
							Type: document.BlockNodeDoc,
							Content: []document.Block{
								pmBlock(document.BlockNodeParagraph, "p1", nil, pmText("hello")),
							},
						},
					}, nil
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			RespJSON: `{"document_id":"` + docID.String() + `","document_name":"Cat Facts",` +
				`"blocks":[{"uid":"p1","kind":"paragraph","text":"hello","depth":0}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := (&readDocumentSummary{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

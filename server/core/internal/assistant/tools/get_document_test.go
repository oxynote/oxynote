package tools

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_getDocument_InvokableRun(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()
	parentID := xid.New()
	updated := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

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
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, _ xid.ID, _, _ string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Successful fetch with parent": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, id xid.ID, _, _ string) (*document.Document, error) {
					return &document.Document{
						Branch: document.Branch{
							BranchID:     branchID,
							BranchName:   "main",
							DocumentName: "Cat Facts",
							Icon:         "lucide:cat",
							Protected:    true,
							UpdatedAt:    updated,
						},
						ID:       id,
						ParentID: null.ValueFrom(parentID),
					}, nil
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			RespJSON: `{"id":"` + docID.String() + `","name":"Cat Facts","icon":"lucide:cat",` +
				`"parent_id":"` + parentID.String() + `","branch_id":"` + branchID.String() + `",` +
				`"branch_name":"main","protected":true,"updated_at":"2026-08-15T10:00:00Z"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := (&getDocument{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

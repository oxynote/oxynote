package tools

import (
	"context"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Manager_applyEdit(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	stubDB := func(err error) *DBMock {
		return &DBMock{
			FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
				if err != nil {
					return nil, err
				}

				return &document.Document{
					Branch:         document.Branch{BranchID: branchID},
					ID:             id,
					OrganizationID: orgID,
				}, nil
			},
		}
	}

	cc := map[string]struct {
		DB         *DBMock
		Applier    *EditApplierMock
		DocumentID string
		ApplyCalls int
		RespJSON   string
		Err        error
	}{
		"Invalid document id": {
			DB:         stubDB(nil),
			Applier:    &EditApplierMock{},
			DocumentID: "not-an-xid",
			Err:        assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:         stubDB(assert.AnError),
			Applier:    &EditApplierMock{},
			DocumentID: docID.String(),
			Err:        assert.AnError,
		},
		"Error returned by EditApplier.Apply": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		"Partial failure surfaces per-op errors": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{
						Applied: 1,
						Errors:  []edit.OpError{{Index: 1, Message: "uid not found"}},
					}, nil
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			RespJSON:   `{"applied":1,"errors":[{"index":1,"message":"uid not found"}]}`,
		},
		"Successful apply": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			RespJSON:   `{"applied":1,"errors":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				log:     slog.New(slog.DiscardHandler),
				db:      c.DB,
				applier: c.Applier,
				orgID:   "org",
				userID:  "user",
			}

			res, err := m.applyEdit(context.Background(), c.DocumentID, []edit.Operation{edit.Delete("target")})
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Applier.ApplyCalls()
			require.Len(t, ff, c.ApplyCalls)

			if c.ApplyCalls > 0 {
				assert.Equal(t, docID.String(), ff[0].DocumentID)
				assert.Equal(t, branchID.String(), ff[0].BranchID)
				assert.Len(t, ff[0].Ops, 1)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

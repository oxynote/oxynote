package block

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/simulation"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testSession is the authenticated session used across handler tests.
var _testSession = auth.Session{
	UserID:               "user-1",
	ActiveOrganizationID: "org-1",
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	runner := &RunnerMock{}

	hdl := NewHandler(log, runner)
	require.NotNil(t, hdl)
	assert.Equal(t, log, hdl.log)
	assert.Same(t, runner, hdl.runner)
}

func Test_Handler_RunBlock(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	cc := map[string]struct {
		Result      any
		Err         error
		OmitSession bool
		DocumentID  string
		BranchID    string
		BlockUID    string
		Code        int
		Body        string
		Ran         bool
	}{
		"Not authenticated": {
			OmitSession: true,
			DocumentID:  documentID.String(),
			BranchID:    branchID.String(),
			BlockUID:    "block-1",
			Code:        http.StatusUnauthorized,
			Body:        `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Missing document id": {
			BranchID: branchID.String(),
			BlockUID: "block-1",
			Code:     http.StatusNotFound,
			Body:     `{"code":"general","message":"not found"}`,
		},
		"Missing branch id": {
			DocumentID: documentID.String(),
			BlockUID:   "block-1",
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Missing block uid": {
			DocumentID: documentID.String(),
			BranchID:   branchID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Error returned by the runner": {
			Err:        assert.AnError,
			DocumentID: documentID.String(),
			BranchID:   branchID.String(),
			BlockUID:   "block-1",
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Ran:        true,
		},
		// what a run reports is the block kind's own payload, which the
		// handler passes through unread.
		"A metric block that keeps simulating": {
			Result:     simulation.Result{},
			DocumentID: documentID.String(),
			BranchID:   branchID.String(),
			BlockUID:   "block-1",
			Code:       http.StatusOK,
			Body:       `{"cleared":false}`,
			Ran:        true,
		},
		"A metric block whose real data has arrived": {
			Result:     simulation.Result{Cleared: true},
			DocumentID: documentID.String(),
			BranchID:   branchID.String(),
			BlockUID:   "block-1",
			Code:       http.StatusOK,
			Body:       `{"cleared":true}`,
			Ran:        true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runner := &RunnerMock{
				RunFunc: func(context.Context, xid.ID, xid.ID, string, string) (any, error) {
					return c.Result, c.Err
				},
			}
			hdl := NewHandler(slog.New(slog.DiscardHandler), runner)

			req := httptest.NewRequest("POST", "http://test.com/", http.NoBody)

			if !c.OmitSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), _testSession))
			}

			// an empty value reads back exactly like an absent one, which
			// is how the cases omit a parameter.
			ctx := testutil.AddChiCtx(req.Context(), "documentId", c.DocumentID)
			ctx = testutil.AddChiCtx(ctx, "branchId", c.BranchID)
			ctx = testutil.AddChiCtx(ctx, "blockUid", c.BlockUID)

			rec := httptest.NewRecorder()
			hdl.RunBlock(rec, req.WithContext(ctx))

			assert.Equal(t, c.Code, rec.Code)
			assert.JSONEq(t, c.Body, rec.Body.String())

			if !c.Ran {
				assert.Empty(t, runner.RunCalls())

				return
			}

			require.Len(t, runner.RunCalls(), 1)

			call := runner.RunCalls()[0]
			assert.Equal(t, documentID, call.DocumentID)
			assert.Equal(t, branchID, call.BranchID)
			assert.Equal(t, c.BlockUID, call.BlockUID)
			assert.Equal(t, _testSession.ActiveOrganizationID, call.OrganizationID)
		})
	}
}

package activity

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	activityCore "github.com/oxynote/oxynote/server/core/internal/document/activity"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
}

func Test_Handler_FetchDocumentActivity(t *testing.T) {
	documentID, branchID, entryID, revisionID, commentID := xid.New(), xid.New(), xid.New(), xid.New(), xid.New()
	at := time.Date(2026, 11, 27, 10, 14, 3, 0, time.UTC)

	doc := &document.Document{ID: documentID}

	page := activityCore.Page{
		Entries: []activityCore.Entry{
			{
				ID:             entryID,
				OrganizationID: "org1",
				DocumentID:     documentID,
				BranchID:       branchID,
				Actor:          activityCore.Actor{Kind: activityCore.ActorKindUser, UserID: null.StringFrom("u2")},
				Type:           activityCore.TypeCommentCreated,
				Category:       activityCore.CategoryComment,
				Target:         activityCore.NewTarget(activityCore.TargetKindCommentRevision, revisionID.String()),
				Metadata:       activityCore.Metadata{activityCore.MetaKeyCommentID: commentID.String()},
				Subject: comment.Revision{
					ID:             revisionID,
					CommentID:      commentID,
					OrganizationID: "org1",
					Content:        comment.Content{"text": "hello"},
					UserID:         null.StringFrom("u2"),
					CreatedAt:      at,
				},
				CreatedAt: at,
				UpdatedAt: at,
			},
		},
		Next: null.ValueFrom(entryID),
	}

	cc := map[string]struct {
		DB                           *DBMock
		NoSession                    bool
		Query                        string
		OmitDoc                      bool
		OmitBranch                   bool
		RespCode                     int
		RespJSON                     string
		FetchDocumentByBranchID      int
		FetchDocumentActivityEntries int
		ActivityQuery                activityCore.Query
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Unknown category": {
			DB:       &DBMock{},
			Query:    "?category=edit",
			RespCode: http.StatusBadRequest,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{ID: xid.New()}, nil
				},
			},
			RespCode:                http.StatusNotFound,
			RespJSON:                `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by DB.FetchDocumentActivityEntries": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return doc, nil
				},
				FetchDocumentActivityEntriesFunc: func(context.Context, xid.ID, string, activityCore.Query) (activityCore.Page, error) {
					return activityCore.Page{}, assert.AnError
				},
			},
			RespCode:                     http.StatusInternalServerError,
			FetchDocumentByBranchID:      1,
			FetchDocumentActivityEntries: 1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return doc, nil
				},
				FetchDocumentActivityEntriesFunc: func(context.Context, xid.ID, string, activityCore.Query) (activityCore.Page, error) {
					return page, nil
				},
			},
			Query:    "?category=comment&category=review&before=" + entryID.String() + "&limit=1",
			RespCode: http.StatusOK,
			RespJSON: fmt.Sprintf(`{
				"entries": [{
					"id": %[1]q,
					"documentId": %[2]q,
					"branchId": %[3]q,
					"actor": {"kind": "user", "userId": "u2", "clientId": null},
					"type": "comment.created",
					"category": "comment",
					"target": {"kind": "comment-revision", "id": %[4]q},
					"metadata": {"commentId": %[5]q},
					"subject": {
						"id": %[4]q,
						"commentId": %[5]q,
						"organizationId": "org1",
						"content": {"text": "hello"},
						"userId": "u2",
						"createdAt": "2026-11-27T10:14:03Z"
					},
					"createdAt": "2026-11-27T10:14:03Z",
					"updatedAt": "2026-11-27T10:14:03Z"
				}],
				"next": %[1]q
			}`, entryID, documentID, branchID, revisionID, commentID),
			FetchDocumentByBranchID:      1,
			FetchDocumentActivityEntries: 1,
			ActivityQuery: activityCore.Query{
				Categories: []activityCore.Category{activityCore.CategoryComment, activityCore.CategoryReview},
				Before:     entryID,
				Limit:      1,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)
			ctx := req.Context()

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			if !c.OmitDoc {
				ctx = testutil.AddChiCtx(ctx, "documentId", documentID.String())
			}

			if !c.OmitBranch {
				ctx = testutil.AddChiCtx(ctx, "branchId", branchID.String())
			}

			rec := httptest.NewRecorder()

			NewHandler(slog.New(slog.DiscardHandler), c.DB).FetchDocumentActivity(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			bb := c.DB.FetchDocumentByBranchIDCalls()
			require.Len(t, bb, c.FetchDocumentByBranchID)

			if c.FetchDocumentByBranchID > 0 {
				assert.Equal(t, branchID, bb[0].BranchID)
				assert.Equal(t, "org1", bb[0].OrganizationID)
			}

			ff := c.DB.FetchDocumentActivityEntriesCalls()
			require.Len(t, ff, c.FetchDocumentActivityEntries)

			if c.FetchDocumentActivityEntries == 0 {
				return
			}

			assert.Equal(t, branchID, ff[0].BranchID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Equal(t, c.ActivityQuery, ff[0].Qr)
		})
	}
}

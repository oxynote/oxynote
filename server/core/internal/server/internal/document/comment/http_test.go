package comment

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	commentCore "github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/notification"
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

// fixed IDs used across test requests.
var (
	_documentID = xid.New()
	_commentID  = xid.New()
	_replyID    = xid.New()
	_branchID   = xid.New()
)

// fakePublisher captures published notifications.
type fakePublisher struct {
	calls []struct {
		OrganizationID string
		Core           notification.Core
		UserIDs        []string
	}
}

func (p *fakePublisher) PublishNotifications(organizationID string, nc notification.Core, userIDs ...string) {
	p.calls = append(p.calls, struct {
		OrganizationID string
		Core           notification.Core
		UserIDs        []string
	}{organizationID, nc, userIDs})
}

// capturedChange records comment change callback invocations.
type capturedChange struct {
	calls []ChangeMessage
}

// newTestHandler builds a Handler with the callback capture attached.
func newTestHandler(db DB, pub *fakePublisher, cpt *capturedChange) *Handler {
	hdl := &Handler{
		log:      slog.New(slog.DiscardHandler),
		db:       db,
		notifPub: pub,
	}

	if cpt != nil {
		hdl.comments.changeCallback = func(_ string, _ xid.ID, msg ChangeMessage) {
			cpt.calls = append(cpt.calls, msg)
		}
	}

	return hdl
}

// withTx wires the DB mock's BeginTx to hand out the provided Tx mock.
func withTx(db *DBMock, tx *TxMock, err error) *DBMock {
	db.BeginTxFunc = func(_ context.Context, dest any) error {
		if err != nil {
			return err
		}

		*dest.(*Tx) = tx

		return nil
	}

	return db
}

// newRequest builds a request carrying a session and the standard chi
// parameters unless the respective omit flags are set.
func newRequest(method, body string, noSession, omitDoc, omitComment, omitReply bool) *http.Request {
	req := httptest.NewRequest(method, "http://test.com/", http.NoBody)

	if body != "" {
		req = httptest.NewRequest(method, "http://test.com/", strings.NewReader(body))
	}

	ctx := req.Context()

	if !noSession {
		ctx = auth.AddSessionToContext(ctx, auth.Session{
			UserID:               "u1",
			ActiveOrganizationID: "org1",
		})
	}

	if !omitDoc {
		ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
	}

	if !omitComment {
		ctx = testutil.AddChiCtx(ctx, "commentId", _commentID.String())
	}

	if !omitReply {
		ctx = testutil.AddChiCtx(ctx, "replyId", _replyID.String())
	}

	return req.WithContext(ctx)
}

// storedComment builds a stored comment owned by the given user.
func storedComment(userID string, replies ...commentCore.Reply) *commentCore.Comment {
	return &commentCore.Comment{
		ID:             _commentID,
		DocumentID:     _documentID,
		OrganizationID: "org1",
		BranchID:       _branchID,
		AnchorBlockID:  null.StringFrom("block1"),
		UserID:         null.StringFrom(userID),
		Replies:        replies,
	}
}

// storedReply builds a stored reply owned by the given user.
func storedReply(userID string) commentCore.Reply {
	return commentCore.Reply{
		ID:             _replyID,
		CommentID:      _commentID,
		OrganizationID: "org1",
		UserID:         null.StringFrom(userID),
	}
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	pub := &fakePublisher{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, pub)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, pub, hdl.notifPub)
}

func Test_Handler_CreateDocumentComment(t *testing.T) {
	validBody := `{"content":{"text":"hello"},"branchId":"` + _branchID.String() + `"}`

	cc := map[string]struct {
		DB          *DBMock
		NoSession   bool
		OmitDoc     bool
		Body        string
		RespCode    int
		Inserted    int
		Changes     []ChangeMessage
		NotifyUsers [][]string
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Invalid JSON body": {
			DB:       &DBMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Comment insertion error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID, Branch: documentCore.Branch{BranchID: _branchID}}, nil
				},
				InsertDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Inserted: 1,
		},
		// the comment is already stored and announced by the time the
		// maintainers are read, so a failure there costs notifications, not
		// the request.
		"Maintainer fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID, Branch: documentCore.Branch{BranchID: _branchID}}, nil
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusCreated,
			Inserted: 1,
			Changes:  []ChangeMessage{{Type: ChangeTypeCreated}},
		},
		"Branch belongs to another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: xid.New(), Branch: documentCore.Branch{BranchID: _branchID}}, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Successful creation notifies other maintainers": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID, Branch: documentCore.Branch{BranchID: _branchID}}, nil
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u2", "u2", "u3"}, nil
				},
			},
			Body:        validBody,
			RespCode:    http.StatusCreated,
			Inserted:    1,
			Changes:     []ChangeMessage{{Type: ChangeTypeCreated}},
			NotifyUsers: [][]string{{"u2", "u3"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			pub := &fakePublisher{}
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, pub, cpt)

			rec := httptest.NewRecorder()

			hdl.CreateDocumentComment(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertDocumentCommentCalls(), c.Inserted)
			require.Len(t, cpt.calls, len(c.Changes))

			for i, ch := range c.Changes {
				assert.Equal(t, ch.Type, cpt.calls[i].Type)
			}

			require.Len(t, pub.calls, len(c.NotifyUsers))

			for i, users := range c.NotifyUsers {
				assert.Equal(t, "org1", pub.calls[i].OrganizationID)
				assert.Equal(t, users, pub.calls[i].UserIDs)
			}
		})
	}
}

func Test_Handler_CreateDocumentCommentReply(t *testing.T) {
	validBody := `{"content":{"text":"reply"}}`

	cc := map[string]struct {
		DB          *DBMock
		NoSession   bool
		OmitDoc     bool
		OmitComment bool
		Body        string
		RespCode    int
		Inserted    int
		Changes     []ChangeMessage
		NotifyUsers [][]string
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Missing comment ID parameter": {
			DB:          &DBMock{},
			OmitComment: true,
			Body:        validBody,
			RespCode:    http.StatusNotFound,
		},
		"Comment fetch error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Reply insertion error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
				InsertDocumentCommentReplyFunc: func(context.Context, commentCore.Reply) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Inserted: 1,
		},
		"Successful reply notifies author and other repliers": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2", storedReply("u1"), storedReply("u3")), nil
				},
			},
			Body:        validBody,
			RespCode:    http.StatusCreated,
			Inserted:    1,
			Changes:     []ChangeMessage{{Type: ChangeTypeUpdated}},
			NotifyUsers: [][]string{{"u2", "u3"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			pub := &fakePublisher{}
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, pub, cpt)

			rec := httptest.NewRecorder()

			hdl.CreateDocumentCommentReply(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, c.OmitComment, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertDocumentCommentReplyCalls(), c.Inserted)
			require.Len(t, cpt.calls, len(c.Changes))

			for i, ch := range c.Changes {
				assert.Equal(t, ch.Type, cpt.calls[i].Type)
				assert.Equal(t, _commentID, cpt.calls[i].CommentID)
			}

			require.Len(t, pub.calls, len(c.NotifyUsers))

			for i, users := range c.NotifyUsers {
				assert.Equal(t, users, pub.calls[i].UserIDs)
			}
		})
	}
}

func Test_Handler_FetchDocumentComment(t *testing.T) {
	cc := map[string]struct {
		DB          *DBMock
		NoSession   bool
		OmitDoc     bool
		OmitComment bool
		RespCode    int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Missing comment ID parameter": {
			DB:          &DBMock{},
			OmitComment: true,
			RespCode:    http.StatusNotFound,
		},
		"Comment fetch error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newTestHandler(c.DB, &fakePublisher{}, nil)

			rec := httptest.NewRecorder()

			hdl.FetchDocumentComment(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, c.OmitComment, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _commentID.String())

				ff := c.DB.FetchDocumentCommentCalls()
				require.Len(t, ff, 1)
				assert.Equal(t, _commentID, ff[0].ID)
				assert.Equal(t, _documentID, ff[0].DocumentID)
				assert.Equal(t, "org1", ff[0].OrganizationID)
			}
		})
	}
}

func Test_Handler_FetchDocumentComments(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Query     string
		RespCode  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Query:     "?branchId=" + _branchID.String(),
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid branch ID query parameter": {
			DB:       &DBMock{},
			Query:    "?branchId=bogus",
			RespCode: http.StatusBadRequest,
		},
		"Comments fetch error": {
			DB: &DBMock{
				FetchDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string) ([]commentCore.Comment, error) {
					return nil, errors.New("boom")
				},
			},
			Query:    "?branchId=" + _branchID.String(),
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string) ([]commentCore.Comment, error) {
					return []commentCore.Comment{*storedComment("u2")}, nil
				},
			},
			Query:    "?branchId=" + _branchID.String(),
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newTestHandler(c.DB, &fakePublisher{}, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchDocumentComments(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _commentID.String())

				ff := c.DB.FetchDocumentCommentsByBranchIDCalls()
				require.Len(t, ff, 1)
				assert.Equal(t, _branchID, ff[0].BranchID)
			}
		})
	}
}

func Test_Handler_UpdateDocumentComment(t *testing.T) {
	validBody := `{"content":{"text":"updated"}}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		Body      string
		RespCode  int
		Updated   int
		Changes   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Comment fetch error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Not the comment author": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusForbidden,
		},
		"Anonymous comment author": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					c := storedComment("u1")
					c.UserID = null.String{}

					return c, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusForbidden,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Comment update error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				UpdateDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusOK,
			Updated:  1,
			Changes:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentComment(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UpdateDocumentCommentCalls(), c.Updated)
			assert.Len(t, cpt.calls, c.Changes)
		})
	}
}

func Test_Handler_UpdateDocumentCommentReply(t *testing.T) {
	validBody := `{"content":{"text":"updated"}}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitReply bool
		Body      string
		RespCode  int
		Updated   int
		Changes   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing reply ID parameter": {
			DB:        &DBMock{},
			OmitReply: true,
			Body:      validBody,
			RespCode:  http.StatusNotFound,
		},
		"Reply fetch error": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Not the reply author": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u2")

					return &r, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusForbidden,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Reply update error": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
				UpdateDocumentCommentReplyFunc: func(context.Context, commentCore.Reply) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusOK,
			Updated:  1,
			Changes:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentCommentReply(rec, newRequest(http.MethodPut, c.Body, c.NoSession, false, false, c.OmitReply))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UpdateDocumentCommentReplyCalls(), c.Updated)
			assert.Len(t, cpt.calls, c.Changes)
		})
	}
}

func Test_Handler_ResolveDocumentComment(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		RespCode  int
		Deleted   int
		Changes   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Comment fetch error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Another user's comment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			RespCode: http.StatusForbidden,
		},
		"Comment deletion error": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				DeleteDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful resolution": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
			},
			RespCode: http.StatusOK,
			Deleted:  1,
			Changes:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.ResolveDocumentComment(rec, newRequest(http.MethodPost, "", c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.DeleteDocumentCommentCalls(), c.Deleted)
			require.Len(t, cpt.calls, c.Changes)

			if c.Changes > 0 {
				assert.Equal(t, ChangeTypeDeleted, cpt.calls[0].Type)
			}
		})
	}
}

func Test_Handler_DeleteDocumentComment(t *testing.T) {
	cc := map[string]struct {
		Tx         *TxMock
		BeginErr   error
		NoSession  bool
		OmitDoc    bool
		RespCode   int
		Committed  int
		ChangeType ChangeType
	}{
		"No session in context": {
			Tx:        &TxMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			Tx:       &TxMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Transaction start error": {
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			RespCode: http.StatusInternalServerError,
		},
		"Comment fetch error": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Not the comment author": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			RespCode: http.StatusForbidden,
		},
		"Comment deletion error without replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				DeleteDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Comment replacement error with replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1", storedReply("u2")), nil
				},
				ReplaceDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Reply deletion error with replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1", storedReply("u2")), nil
				},
				DeleteDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Commit error": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful deletion without replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
			},
			RespCode:   http.StatusNoContent,
			Committed:  1,
			ChangeType: ChangeTypeDeleted,
		},
		"Successful deletion promotes first reply": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1", storedReply("u2")), nil
				},
			},
			RespCode:   http.StatusNoContent,
			Committed:  1,
			ChangeType: ChangeTypeUpdated,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			hdl := newTestHandler(withTx(&DBMock{}, c.Tx, c.BeginErr), &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentComment(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)

			if c.ChangeType == "" {
				assert.Empty(t, cpt.calls)
				return
			}

			require.Len(t, cpt.calls, 1)
			assert.Equal(t, c.ChangeType, cpt.calls[0].Type)

			if c.ChangeType == ChangeTypeUpdated {
				assert.Len(t, c.Tx.ReplaceDocumentCommentCalls(), 1)
				assert.Len(t, c.Tx.DeleteDocumentCommentReplyCalls(), 1)
			} else {
				assert.Len(t, c.Tx.DeleteDocumentCommentCalls(), 1)
			}
		})
	}
}

func Test_Handler_DeleteDocumentCommentReply(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitReply bool
		RespCode  int
		Deleted   int
		Changes   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing reply ID parameter": {
			DB:        &DBMock{},
			OmitReply: true,
			RespCode:  http.StatusNotFound,
		},
		"Reply fetch error": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Not the reply author": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u2")

					return &r, nil
				},
			},
			RespCode: http.StatusForbidden,
		},
		"Reply deletion error": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
				DeleteDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
			},
			RespCode: http.StatusOK,
			Deleted:  1,
			Changes:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentCommentReply(rec, newRequest(http.MethodDelete, "", c.NoSession, false, false, c.OmitReply))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.DeleteDocumentCommentReplyCalls(), c.Deleted)
			require.Len(t, cpt.calls, c.Changes)

			if c.Changes > 0 {
				assert.Equal(t, ChangeTypeUpdated, cpt.calls[0].Type)
			}
		})
	}
}

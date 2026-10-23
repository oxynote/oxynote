package comment

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// _docContent is comment content in the editor's shape, reading "hello".
const _docContent = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`

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
	validBody := `{"content":` + _docContent + `,"branchId":"` + _branchID.String() + `"}`

	cc := map[string]struct {
		DB                 *DBMock
		NoSession          bool
		OmitDoc            bool
		Body               string
		RespCode           int
		BranchFetched      int
		Inserted           int
		MaintainersFetched int
		Changes            []ChangeMessage
		NotifyUsers        [][]string
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
		"Empty comment content": {
			DB:       &DBMock{},
			Body:     `{"branchId":"` + _branchID.String() + `"}`,
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, assert.AnError
				},
			},
			Body:          validBody,
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
		},
		"Error returned by DB.InsertDocumentComment": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID, BranchID: _branchID}, nil
				},
				InsertDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return assert.AnError
				},
			},
			Body:          validBody,
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
			Inserted:      1,
		},
		// the comment is already stored and announced by the time the
		// maintainers are read, so a failure there costs notifications, not
		// the request.
		"Error returned by DB.FetchDocumentMaintainers": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID, BranchID: _branchID}, nil
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, assert.AnError
				},
			},
			Body:               validBody,
			RespCode:           http.StatusCreated,
			BranchFetched:      1,
			Inserted:           1,
			MaintainersFetched: 1,
			Changes:            []ChangeMessage{{Type: ChangeTypeCreated}},
		},
		"Branch belongs to another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: xid.New(), BranchID: _branchID}, nil
				},
			},
			Body:          validBody,
			RespCode:      http.StatusNotFound,
			BranchFetched: 1,
		},
		"Successful creation notifies other maintainers": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID, BranchID: _branchID}, nil
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u2", "u2", "u3"}, nil
				},
			},
			Body:               validBody,
			RespCode:           http.StatusCreated,
			BranchFetched:      1,
			Inserted:           1,
			MaintainersFetched: 1,
			Changes:            []ChangeMessage{{Type: ChangeTypeCreated}},
			NotifyUsers:        [][]string{{"u2", "u3"}},
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
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetched)
			assert.Len(t, c.DB.InsertDocumentCommentCalls(), c.Inserted)
			assert.Len(t, c.DB.FetchDocumentMaintainersCalls(), c.MaintainersFetched)
			require.Len(t, cpt.calls, len(c.Changes))

			for i, ch := range c.Changes {
				assert.Equal(t, ch.Type, cpt.calls[i].Type)
			}

			require.Len(t, pub.calls, len(c.NotifyUsers))

			for i, users := range c.NotifyUsers {
				assert.Equal(t, "org1", pub.calls[i].OrganizationID)
				assert.Equal(t, users, pub.calls[i].UserIDs)
				assert.Equal(t, notification.NotificationDocumentNewComment, pub.calls[i].Core.Code)
				assert.Equal(t, "hello", pub.calls[i].Core.Metadata[notification.MetaKeyCommentExcerpt])
			}
		})
	}
}

func Test_Handler_CreateDocumentCommentReply(t *testing.T) {
	validBody := `{"content":` + _docContent + `}`

	cc := map[string]struct {
		DB          *DBMock
		NoSession   bool
		OmitDoc     bool
		OmitComment bool
		Body        string
		RespCode    int
		Fetched     int
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
		"Error returned by DB.FetchDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Empty reply content": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     `{"content":{}}`,
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.InsertDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
				InsertDocumentCommentReplyFunc: func(context.Context, commentCore.Reply) error {
					return assert.AnError
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
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
			Fetched:     1,
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
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)
			assert.Len(t, c.DB.InsertDocumentCommentReplyCalls(), c.Inserted)
			require.Len(t, cpt.calls, len(c.Changes))

			for i, ch := range c.Changes {
				assert.Equal(t, ch.Type, cpt.calls[i].Type)
				assert.Equal(t, _commentID, cpt.calls[i].CommentID)
			}

			require.Len(t, pub.calls, len(c.NotifyUsers))

			for i, users := range c.NotifyUsers {
				assert.Equal(t, users, pub.calls[i].UserIDs)
				assert.Equal(t, notification.NotificationDocumentNewCommentReply, pub.calls[i].Core.Code)
				assert.Equal(t, "hello", pub.calls[i].Core.Metadata[notification.MetaKeyCommentExcerpt])
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
		Fetched     int
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
		"Error returned by DB.FetchDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			RespCode: http.StatusOK,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newTestHandler(c.DB, &fakePublisher{}, nil)

			rec := httptest.NewRecorder()

			hdl.FetchDocumentComment(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, c.OmitComment, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)

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
		DB            *DBMock
		NoSession     bool
		OmitDoc       bool
		Query         string
		RespCode      int
		BranchFetched int
		Fetched       int
		Resolved      bool
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Query:     "?branchId=" + _branchID.String(),
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Query:    "?branchId=" + _branchID.String(),
			RespCode: http.StatusNotFound,
		},
		"Invalid branch ID query parameter": {
			DB:       &DBMock{},
			Query:    "?branchId=bogus",
			RespCode: http.StatusBadRequest,
		},
		"Invalid resolved query parameter": {
			DB:       &DBMock{},
			Query:    "?branchId=" + _branchID.String() + "&resolved=bogus",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, assert.AnError
				},
			},
			Query:         "?branchId=" + _branchID.String(),
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: xid.New()}, nil
				},
			},
			Query:         "?branchId=" + _branchID.String(),
			RespCode:      http.StatusNotFound,
			BranchFetched: 1,
		},
		"Error returned by DB.FetchDocumentCommentsByBranchID": {
			DB: &DBMock{
				FetchDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string, bool) ([]commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			Query:         "?branchId=" + _branchID.String(),
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
			Fetched:       1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string, bool) ([]commentCore.Comment, error) {
					return []commentCore.Comment{*storedComment("u2")}, nil
				},
			},
			Query:         "?branchId=" + _branchID.String(),
			RespCode:      http.StatusOK,
			BranchFetched: 1,
			Fetched:       1,
		},
		"Successful fetch of open comments": {
			DB: &DBMock{
				FetchDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string, bool) ([]commentCore.Comment, error) {
					return []commentCore.Comment{*storedComment("u2")}, nil
				},
			},
			Query:         "?branchId=" + _branchID.String() + "&resolved=false",
			RespCode:      http.StatusOK,
			BranchFetched: 1,
			Fetched:       1,
		},
		"Successful fetch of resolved comments": {
			DB: &DBMock{
				FetchDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string, bool) ([]commentCore.Comment, error) {
					return []commentCore.Comment{*storedComment("u2")}, nil
				},
			},
			Query:         "?branchId=" + _branchID.String() + "&resolved=true",
			RespCode:      http.StatusOK,
			BranchFetched: 1,
			Fetched:       1,
			Resolved:      true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.DB.FetchDocumentByBranchIDFunc == nil {
				c.DB.FetchDocumentByBranchIDFunc = func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return &documentCore.Document{ID: _documentID}, nil
				}
			}

			hdl := newTestHandler(c.DB, &fakePublisher{}, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			ctx := req.Context()

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			if !c.OmitDoc {
				ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
			}

			rec := httptest.NewRecorder()

			hdl.FetchDocumentComments(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetched)
			assert.Len(t, c.DB.FetchDocumentCommentsByBranchIDCalls(), c.Fetched)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _commentID.String())

				ff := c.DB.FetchDocumentCommentsByBranchIDCalls()
				require.Len(t, ff, 1)
				assert.Equal(t, _branchID, ff[0].BranchID)
				assert.Equal(t, c.Resolved, ff[0].Resolved)
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
		Fetched   int
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
		"Error returned by DB.FetchDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Not the comment author": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusForbidden,
			Fetched:  1,
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
			Fetched:  1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.UpdateDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				UpdateDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return assert.AnError
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
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
			Fetched:  1,
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
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)
			assert.Len(t, c.DB.UpdateDocumentCommentCalls(), c.Updated)
			assert.Len(t, cpt.calls, c.Changes)
		})
	}
}

func Test_Handler_UpdateDocumentCommentReply(t *testing.T) {
	validBody := `{"content":{"text":"updated"}}`

	cc := map[string]struct {
		DB             *DBMock
		NoSession      bool
		OmitReply      bool
		Body           string
		RespCode       int
		CommentFetched int
		ReplyFetched   int
		Updated        int
		Changes        int
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
		// the document-scoped comment fetch is what ties the reply to the
		// path document, so its failure ends the request before the reply
		// is ever read.
		"Error returned by DB.FetchDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
		},
		"Error returned by DB.FetchDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					return nil, assert.AnError
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Not the reply author": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u2")

					return &r, nil
				},
			},
			Body:           validBody,
			RespCode:       http.StatusForbidden,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
			},
			Body:           "{",
			RespCode:       http.StatusBadRequest,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Error returned by DB.UpdateDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
				UpdateDocumentCommentReplyFunc: func(context.Context, commentCore.Reply) error {
					return assert.AnError
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Updated:        1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
			},
			Body:           validBody,
			RespCode:       http.StatusOK,
			CommentFetched: 1,
			ReplyFetched:   1,
			Updated:        1,
			Changes:        1,
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
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.CommentFetched)
			assert.Len(t, c.DB.FetchDocumentCommentReplyCalls(), c.ReplyFetched)
			assert.Len(t, c.DB.UpdateDocumentCommentReplyCalls(), c.Updated)
			assert.Len(t, cpt.calls, c.Changes)
		})
	}
}

func Test_Handler_UpdateDocumentCommentStatus(t *testing.T) {
	cc := map[string]struct {
		DB          *DBMock
		NoSession   bool
		OmitDoc     bool
		Body        string
		RespCode    int
		Fetched     int
		Updated     int
		Changes     int
		NotifyUsers [][]string
		Check       func(t *testing.T, c commentCore.Comment)
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      `{"resolved":true}`,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Body:     `{"resolved":true}`,
			RespCode: http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			Body:     `{"resolved":true}`,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Missing resolved field": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     `{}`,
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.UpdateDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
				UpdateDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return assert.AnError
				},
			},
			Body:     `{"resolved":true}`,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Updated:  1,
		},
		"Successful resolution notifies author and repliers": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					c := storedComment("u2", storedReply("u1"), storedReply("u3"))

					if err := json.Unmarshal([]byte(_docContent), &c.Content); err != nil {
						return nil, err
					}

					return c, nil
				},
			},
			Body:        `{"resolved":true}`,
			RespCode:    http.StatusOK,
			Fetched:     1,
			Updated:     1,
			Changes:     1,
			NotifyUsers: [][]string{{"u2", "u3"}},
			Check: func(t *testing.T, c commentCore.Comment) {
				assert.True(t, c.Resolved)
				assert.Equal(t, null.StringFrom("u1"), c.ResolvedBy)
				assert.True(t, c.ResolvedAt.Valid)
			},
		},
		"Successful unresolution": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					c := storedComment("u2")
					c.Resolved = true
					c.ResolvedBy = null.StringFrom("u2")
					c.ResolvedAt = null.TimeFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

					return c, nil
				},
			},
			Body:     `{"resolved":false}`,
			RespCode: http.StatusOK,
			Fetched:  1,
			Updated:  1,
			Changes:  1,
			Check: func(t *testing.T, c commentCore.Comment) {
				assert.False(t, c.Resolved)
				assert.False(t, c.ResolvedBy.Valid)
				assert.False(t, c.ResolvedAt.Valid)
			},
		},
		"Already resolved": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					c := storedComment("u2")
					c.Resolved = true
					c.ResolvedBy = null.StringFrom("u2")
					c.ResolvedAt = null.TimeFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

					return c, nil
				},
			},
			Body:     `{"resolved":true}`,
			RespCode: http.StatusOK,
			Fetched:  1,
		},
		"Already unresolved": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			Body:     `{"resolved":false}`,
			RespCode: http.StatusOK,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			pub := &fakePublisher{}
			hdl := newTestHandler(c.DB, pub, cpt)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentCommentStatus(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)
			require.Len(t, pub.calls, len(c.NotifyUsers))

			for i, users := range c.NotifyUsers {
				assert.Equal(t, "org1", pub.calls[i].OrganizationID)
				assert.Equal(t, users, pub.calls[i].UserIDs)
				assert.Equal(t, notification.NotificationDocumentCommentResolved, pub.calls[i].Core.Code)
				assert.Equal(t, "u1", pub.calls[i].Core.Metadata[notification.MetaKeyUserID])
				assert.Equal(t, "hello", pub.calls[i].Core.Metadata[notification.MetaKeyCommentExcerpt])
			}

			uu := c.DB.UpdateDocumentCommentCalls()
			require.Len(t, uu, c.Updated)
			require.Len(t, cpt.calls, c.Changes)

			if c.Changes > 0 {
				assert.Equal(t, ChangeTypeUpdated, cpt.calls[0].Type)
			}

			if c.Check != nil {
				c.Check(t, uu[0].C)
			}

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _commentID.String())
			}
		})
	}
}

func Test_Handler_DeleteDocumentComment(t *testing.T) {
	cc := map[string]struct {
		Tx           *TxMock
		BeginErr     error
		NoSession    bool
		OmitDoc      bool
		RespCode     int
		Begun        int
		Fetched      int
		Replaced     int
		ReplyDeleted int
		Deleted      int
		Committed    int
		RolledBack   int
		ChangeType   ChangeType
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
		"Error returned by DB.BeginTx": {
			Tx:       &TxMock{},
			BeginErr: assert.AnError,
			RespCode: http.StatusInternalServerError,
			Begun:    1,
		},
		"Error returned by Tx.FetchDocumentComment": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			RolledBack: 1,
		},
		"Not the comment author": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2"), nil
				},
			},
			RespCode:   http.StatusForbidden,
			Begun:      1,
			Fetched:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.DeleteDocumentComment": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				DeleteDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.ReplaceDocumentComment": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1", storedReply("u2")), nil
				},
				ReplaceDocumentCommentFunc: func(context.Context, commentCore.Comment) error {
					return assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			Replaced:   1,
			RolledBack: 1,
		},
		"Error returned by Tx.DeleteDocumentCommentReply": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1", storedReply("u2")), nil
				},
				DeleteDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return assert.AnError
				},
			},
			RespCode:     http.StatusInternalServerError,
			Begun:        1,
			Fetched:      1,
			Replaced:     1,
			ReplyDeleted: 1,
			RolledBack:   1,
		},
		"Error returned by Tx.Commit": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Committed:  1,
			RolledBack: 1,
		},
		"Successful deletion without replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1"), nil
				},
			},
			RespCode:   http.StatusNoContent,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Committed:  1,
			RolledBack: 1,
			ChangeType: ChangeTypeDeleted,
		},
		"Successful deletion promotes first reply": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u1", storedReply("u2")), nil
				},
			},
			RespCode:     http.StatusNoContent,
			Begun:        1,
			Fetched:      1,
			Replaced:     1,
			ReplyDeleted: 1,
			Committed:    1,
			RolledBack:   1,
			ChangeType:   ChangeTypeUpdated,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			db := &DBMock{
				BeginTxFunc: testutil.StubBeginTx(c.Tx, c.BeginErr),
			}
			hdl := newTestHandler(db, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentComment(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, db.BeginTxCalls(), c.Begun)
			assert.Len(t, c.Tx.FetchDocumentCommentCalls(), c.Fetched)
			assert.Len(t, c.Tx.ReplaceDocumentCommentCalls(), c.Replaced)
			assert.Len(t, c.Tx.DeleteDocumentCommentReplyCalls(), c.ReplyDeleted)
			assert.Len(t, c.Tx.DeleteDocumentCommentCalls(), c.Deleted)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.RolledBack)

			if c.ChangeType == "" {
				assert.Empty(t, cpt.calls)
				return
			}

			require.Len(t, cpt.calls, 1)
			assert.Equal(t, c.ChangeType, cpt.calls[0].Type)
		})
	}
}

func Test_Handler_DeleteDocumentCommentReply(t *testing.T) {
	cc := map[string]struct {
		DB             *DBMock
		NoSession      bool
		OmitReply      bool
		RespCode       int
		CommentFetched int
		ReplyFetched   int
		Deleted        int
		Changes        int
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
		// the document-scoped comment fetch is what ties the reply to the
		// path document, so its failure ends the request before the reply
		// is ever read.
		"Error returned by DB.FetchDocumentComment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return nil, assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
		},
		"Error returned by DB.FetchDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					return nil, assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Not the reply author": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u2")

					return &r, nil
				},
			},
			RespCode:       http.StatusForbidden,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Error returned by DB.DeleteDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
				DeleteDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Deleted:        1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
					r := storedReply("u1")

					return &r, nil
				},
			},
			RespCode:       http.StatusOK,
			CommentFetched: 1,
			ReplyFetched:   1,
			Deleted:        1,
			Changes:        1,
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
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.CommentFetched)
			assert.Len(t, c.DB.FetchDocumentCommentReplyCalls(), c.ReplyFetched)
			assert.Len(t, c.DB.DeleteDocumentCommentReplyCalls(), c.Deleted)
			require.Len(t, cpt.calls, c.Changes)

			if c.Changes > 0 {
				assert.Equal(t, ChangeTypeUpdated, cpt.calls[0].Type)
			}
		})
	}
}

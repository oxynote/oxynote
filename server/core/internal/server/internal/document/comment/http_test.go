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
	"github.com/oxynote/oxynote/server/core/internal/document/activity"
	commentCore "github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
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
	_revisionID = xid.New()
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

// stubTx returns the transaction the database mock begins, an empty one
// when the case sets none.
func stubTx(db *DBMock, tx *TxMock, beginErr error) *TxMock {
	if tx == nil {
		tx = &TxMock{}
	}

	db.BeginTxFunc = testutil.StubBeginTx[sqlutil.Tx](tx, beginErr)

	return tx
}

// stubRevision stores a comment's text as the revision _revisionID.
func stubRevision(context.Context, commentCore.Comment) (commentCore.Revision, error) {
	return commentCore.Revision{ID: _revisionID}, nil
}

// stubNewestRevision returns _revisionID as the comment's newest revision.
func stubNewestRevision(context.Context, xid.ID, string) (commentCore.Revision, error) {
	return commentCore.Revision{ID: _revisionID}, nil
}

// stubReplyRevision stores a reply's text as the revision _revisionID.
func stubReplyRevision(_ context.Context, r commentCore.Reply) (commentCore.ReplyRevision, error) {
	return commentCore.ReplyRevision{ID: _revisionID, ReplyID: r.ID}, nil
}

// stubNewestReplyRevision returns _revisionID as the reply's newest
// revision.
func stubNewestReplyRevision(_ context.Context, replyID xid.ID, _ string) (commentCore.ReplyRevision, error) {
	return commentCore.ReplyRevision{ID: _revisionID, ReplyID: replyID}, nil
}

// assertEntry checks the one activity entry the transaction wrote, made by
// the session user on the comment's branch.
func assertEntry(t *testing.T, tx *TxMock, tp activity.Type, target activity.Target, md activity.Metadata) {
	t.Helper()

	ee := tx.InsertDocumentActivityEntriesCalls()
	require.Len(t, ee, 1)
	require.Len(t, ee[0].Entries, 1)

	e := ee[0].Entries[0]
	assert.Equal(t, activity.UserActor("u1"), e.Actor)
	assert.Equal(t, tp, e.Type)
	assert.Equal(t, target, e.Target)
	assert.Equal(t, md, e.Metadata)
	assert.Equal(t, "org1", e.OrganizationID)
	assert.Equal(t, _documentID, e.DocumentID)
	assert.Equal(t, _branchID, e.BranchID)
}

// commentMetadata is the metadata of a comment entry.
func commentMetadata(commentID xid.ID) activity.Metadata {
	return activity.Metadata{activity.MetaKeyCommentID: commentID.String()}
}

// replyMetadata is the metadata of an entry of the reply.
func replyMetadata(replyID xid.ID) activity.Metadata {
	return activity.Metadata{
		activity.MetaKeyCommentID: _commentID.String(),
		activity.MetaKeyReplyID:   replyID.String(),
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

	stubBranch := func(context.Context, xid.ID, string) (*documentCore.Document, error) {
		return &documentCore.Document{ID: _documentID, BranchID: _branchID}, nil
	}

	cc := map[string]struct {
		DB                 *DBMock
		Tx                 *TxMock
		BeginErr           error
		NoSession          bool
		OmitDoc            bool
		Body               string
		RespCode           int
		BranchFetched      int
		Begun              int
		Inserted           int
		Entries            int
		Committed          int
		RolledBack         int
		MaintainersFetched int
		Changes            []ChangeMessage
		NotifyUsers        [][]string
		EntryType          activity.Type
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
			DB:            &DBMock{FetchDocumentByBranchIDFunc: stubBranch},
			Body:          `{"branchId":"` + _branchID.String() + `"}`,
			RespCode:      http.StatusBadRequest,
			BranchFetched: 1,
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
		"Error returned by DB.BeginTx": {
			DB:            &DBMock{FetchDocumentByBranchIDFunc: stubBranch},
			BeginErr:      assert.AnError,
			Body:          validBody,
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
			Begun:         1,
		},
		"Error returned by Tx.InsertDocumentComment": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: stubBranch},
			Tx: &TxMock{
				InsertDocumentCommentFunc: func(context.Context, commentCore.Comment) (commentCore.Revision, error) {
					return commentCore.Revision{}, assert.AnError
				},
			},
			Body:          validBody,
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
			Begun:         1,
			Inserted:      1,
			RolledBack:    1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: stubBranch},
			Tx: &TxMock{
				InsertDocumentCommentFunc: stubRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			Body:          validBody,
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
			Begun:         1,
			Inserted:      1,
			Entries:       1,
			RolledBack:    1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: stubBranch},
			Tx: &TxMock{
				InsertDocumentCommentFunc: stubRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Body:          validBody,
			RespCode:      http.StatusInternalServerError,
			BranchFetched: 1,
			Begun:         1,
			Inserted:      1,
			Entries:       1,
			Committed:     1,
			RolledBack:    1,
		},
		// the comment is already stored and announced by the time the
		// maintainers are read, so a failure there costs notifications, not
		// the request.
		"Error returned by DB.FetchDocumentMaintainers": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch,
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, assert.AnError
				},
			},
			Tx:                 &TxMock{InsertDocumentCommentFunc: stubRevision},
			Body:               validBody,
			RespCode:           http.StatusCreated,
			BranchFetched:      1,
			Begun:              1,
			Inserted:           1,
			Entries:            1,
			Committed:          1,
			RolledBack:         1,
			MaintainersFetched: 1,
			Changes:            []ChangeMessage{{Type: ChangeTypeCreated}},
		},
		"Successful creation notifies other maintainers": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch,
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u2", "u2", "u3"}, nil
				},
			},
			Tx:                 &TxMock{InsertDocumentCommentFunc: stubRevision},
			Body:               validBody,
			RespCode:           http.StatusCreated,
			BranchFetched:      1,
			Begun:              1,
			Inserted:           1,
			Entries:            1,
			Committed:          1,
			RolledBack:         1,
			MaintainersFetched: 1,
			Changes:            []ChangeMessage{{Type: ChangeTypeCreated}},
			NotifyUsers:        [][]string{{"u2", "u3"}},
			EntryType:          activity.TypeCommentCreated,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := stubTx(c.DB, c.Tx, c.BeginErr)
			pub := &fakePublisher{}
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, pub, cpt)

			rec := httptest.NewRecorder()

			hdl.CreateDocumentComment(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetched)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			require.Len(t, tx.InsertDocumentCommentCalls(), c.Inserted)
			assert.Len(t, tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.RolledBack)
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

			if c.EntryType == "" {
				return
			}

			assertEntry(
				t,
				tx,
				c.EntryType,
				activity.NewTarget(activity.TargetKindCommentRevision, _revisionID.String()),
				commentMetadata(tx.InsertDocumentCommentCalls()[0].C.ID),
			)
		})
	}
}

func Test_Handler_CreateDocumentCommentReply(t *testing.T) {
	validBody := `{"content":` + _docContent + `}`

	stubComment := func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
		return storedComment("u2"), nil
	}

	cc := map[string]struct {
		DB          *DBMock
		Tx          *TxMock
		BeginErr    error
		NoSession   bool
		OmitDoc     bool
		OmitComment bool
		Body        string
		RespCode    int
		Fetched     int
		Begun       int
		Inserted    int
		Entries     int
		Committed   int
		RolledBack  int
		Changes     []ChangeMessage
		NotifyUsers [][]string
		EntryType   activity.Type
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
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Empty reply content": {
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			Body:     `{"content":{}}`,
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.BeginTx": {
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			BeginErr: assert.AnError,
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Begun:    1,
		},
		"Error returned by Tx.InsertDocumentCommentReply": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				InsertDocumentCommentReplyFunc: func(context.Context, commentCore.Reply) (commentCore.ReplyRevision, error) {
					return commentCore.ReplyRevision{}, assert.AnError
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Inserted:   1,
			RolledBack: 1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				InsertDocumentCommentReplyFunc: stubReplyRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Inserted:   1,
			Entries:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				InsertDocumentCommentReplyFunc: stubReplyRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Inserted:   1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
		},
		"Successful reply notifies author and other repliers": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					return storedComment("u2", storedReply("u1"), storedReply("u3")), nil
				},
			},
			Tx:          &TxMock{InsertDocumentCommentReplyFunc: stubReplyRevision},
			Body:        validBody,
			RespCode:    http.StatusCreated,
			Fetched:     1,
			Begun:       1,
			Inserted:    1,
			Entries:     1,
			Committed:   1,
			RolledBack:  1,
			Changes:     []ChangeMessage{{Type: ChangeTypeUpdated}},
			NotifyUsers: [][]string{{"u2", "u3"}},
			EntryType:   activity.TypeCommentReplied,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := stubTx(c.DB, c.Tx, c.BeginErr)
			pub := &fakePublisher{}
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, pub, cpt)

			rec := httptest.NewRecorder()

			hdl.CreateDocumentCommentReply(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, c.OmitComment, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			require.Len(t, tx.InsertDocumentCommentReplyCalls(), c.Inserted)
			assert.Len(t, tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.RolledBack)
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

			if c.EntryType == "" {
				return
			}

			assertEntry(
				t,
				tx,
				c.EntryType,
				activity.NewTarget(activity.TargetKindReplyRevision, _revisionID.String()),
				replyMetadata(tx.InsertDocumentCommentReplyCalls()[0].R.ID),
			)
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

	stubComment := func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
		return storedComment("u1"), nil
	}

	cc := map[string]struct {
		DB         *DBMock
		Tx         *TxMock
		BeginErr   error
		NoSession  bool
		OmitDoc    bool
		Body       string
		RespCode   int
		Fetched    int
		Begun      int
		Updated    int
		Entries    int
		Committed  int
		RolledBack int
		Changes    int
		EntryType  activity.Type
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
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.BeginTx": {
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			BeginErr: assert.AnError,
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Begun:    1,
		},
		"Error returned by Tx.UpdateDocumentComment": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentFunc: func(context.Context, commentCore.Comment) (commentCore.Revision, error) {
					return commentCore.Revision{}, assert.AnError
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentFunc: stubRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Entries:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentFunc: stubRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
		},
		"Successful update": {
			DB:         &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx:         &TxMock{UpdateDocumentCommentFunc: stubRevision},
			Body:       validBody,
			RespCode:   http.StatusOK,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
			Changes:    1,
			EntryType:  activity.TypeCommentEdited,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := stubTx(c.DB, c.Tx, c.BeginErr)
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentComment(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			assert.Len(t, tx.UpdateDocumentCommentCalls(), c.Updated)
			assert.Len(t, tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.RolledBack)
			assert.Len(t, cpt.calls, c.Changes)

			if c.EntryType == "" {
				return
			}

			assertEntry(
				t,
				tx,
				c.EntryType,
				activity.NewTarget(activity.TargetKindCommentRevision, _revisionID.String()),
				commentMetadata(_commentID),
			)
		})
	}
}

func Test_Handler_UpdateDocumentCommentReply(t *testing.T) {
	validBody := `{"content":{"text":"updated"}}`

	stubComment := func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
		return storedComment("u2"), nil
	}

	stubReply := func(userID string) func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
		return func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
			r := storedReply(userID)

			return &r, nil
		}
	}

	cc := map[string]struct {
		DB             *DBMock
		Tx             *TxMock
		BeginErr       error
		NoSession      bool
		OmitReply      bool
		Body           string
		RespCode       int
		CommentFetched int
		ReplyFetched   int
		Begun          int
		Updated        int
		Entries        int
		Committed      int
		RolledBack     int
		Changes        int
		EntryType      activity.Type
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
				FetchDocumentCommentFunc: stubComment,
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
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u2"),
			},
			Body:           validBody,
			RespCode:       http.StatusForbidden,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Body:           "{",
			RespCode:       http.StatusBadRequest,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			BeginErr:       assert.AnError,
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
		},
		"Error returned by Tx.UpdateDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				UpdateDocumentCommentReplyFunc: func(context.Context, commentCore.Reply) (commentCore.ReplyRevision, error) {
					return commentCore.ReplyRevision{}, assert.AnError
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Updated:        1,
			RolledBack:     1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				UpdateDocumentCommentReplyFunc: stubReplyRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Updated:        1,
			Entries:        1,
			RolledBack:     1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				UpdateDocumentCommentReplyFunc: stubReplyRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Updated:        1,
			Entries:        1,
			Committed:      1,
			RolledBack:     1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx:             &TxMock{UpdateDocumentCommentReplyFunc: stubReplyRevision},
			Body:           validBody,
			RespCode:       http.StatusOK,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Updated:        1,
			Entries:        1,
			Committed:      1,
			RolledBack:     1,
			Changes:        1,
			EntryType:      activity.TypeCommentReplyEdited,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := stubTx(c.DB, c.Tx, c.BeginErr)
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentCommentReply(rec, newRequest(http.MethodPut, c.Body, c.NoSession, false, false, c.OmitReply))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.CommentFetched)
			assert.Len(t, c.DB.FetchDocumentCommentReplyCalls(), c.ReplyFetched)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			assert.Len(t, tx.UpdateDocumentCommentReplyCalls(), c.Updated)
			assert.Len(t, tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.RolledBack)
			assert.Len(t, cpt.calls, c.Changes)

			if c.EntryType == "" {
				return
			}

			assertEntry(
				t,
				tx,
				c.EntryType,
				activity.NewTarget(activity.TargetKindReplyRevision, _revisionID.String()),
				replyMetadata(_replyID),
			)
		})
	}
}

func Test_Handler_UpdateDocumentCommentStatus(t *testing.T) {
	stubComment := func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
		return storedComment("u2"), nil
	}

	// changed reports the status write as one that changed the comment.
	changed := func(context.Context, commentCore.Comment) (bool, error) {
		return true, nil
	}

	cc := map[string]struct {
		DB          *DBMock
		Tx          *TxMock
		BeginErr    error
		NoSession   bool
		OmitDoc     bool
		Body        string
		RespCode    int
		Fetched     int
		Begun       int
		Updated     int
		Newest      int
		Entries     int
		Committed   int
		RolledBack  int
		Changes     int
		NotifyUsers [][]string
		EntryType   activity.Type
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
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Missing resolved field": {
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			Body:     `{}`,
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.BeginTx": {
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			BeginErr: assert.AnError,
			Body:     `{"resolved":true}`,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Begun:    1,
		},
		"Error returned by Tx.UpdateDocumentCommentStatus": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentStatusFunc: func(context.Context, commentCore.Comment) (bool, error) {
					return false, assert.AnError
				},
			},
			Body:       `{"resolved":true}`,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.FetchNewestDocumentCommentRevision": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentStatusFunc: changed,
				FetchNewestDocumentCommentRevisionFunc: func(context.Context, xid.ID, string) (commentCore.Revision, error) {
					return commentCore.Revision{}, assert.AnError
				},
			},
			Body:       `{"resolved":true}`,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Newest:     1,
			RolledBack: 1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentStatusFunc:        changed,
				FetchNewestDocumentCommentRevisionFunc: stubNewestRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			Body:       `{"resolved":true}`,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Newest:     1,
			Entries:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx: &TxMock{
				UpdateDocumentCommentStatusFunc:        changed,
				FetchNewestDocumentCommentRevisionFunc: stubNewestRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Body:       `{"resolved":true}`,
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Newest:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
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
			Tx:          &TxMock{UpdateDocumentCommentStatusFunc: changed, FetchNewestDocumentCommentRevisionFunc: stubNewestRevision},
			Body:        `{"resolved":true}`,
			RespCode:    http.StatusOK,
			Fetched:     1,
			Begun:       1,
			Updated:     1,
			Newest:      1,
			Entries:     1,
			Committed:   1,
			RolledBack:  1,
			Changes:     1,
			NotifyUsers: [][]string{{"u2", "u3"}},
			EntryType:   activity.TypeCommentResolved,
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
			Tx:         &TxMock{UpdateDocumentCommentStatusFunc: changed, FetchNewestDocumentCommentRevisionFunc: stubNewestRevision},
			Body:       `{"resolved":false}`,
			RespCode:   http.StatusOK,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			Newest:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
			Changes:    1,
			EntryType:  activity.TypeCommentReopened,
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
		"Status a concurrent request already set records nothing": {
			DB:         &DBMock{FetchDocumentCommentFunc: stubComment},
			Tx:         &TxMock{},
			Body:       `{"resolved":true}`,
			RespCode:   http.StatusOK,
			Fetched:    1,
			Begun:      1,
			Updated:    1,
			RolledBack: 1,
		},
		"Already unresolved": {
			DB:       &DBMock{FetchDocumentCommentFunc: stubComment},
			Body:     `{"resolved":false}`,
			RespCode: http.StatusOK,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := stubTx(c.DB, c.Tx, c.BeginErr)
			cpt := &capturedChange{}
			pub := &fakePublisher{}
			hdl := newTestHandler(c.DB, pub, cpt)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentCommentStatus(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.Fetched)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			require.Len(t, tx.FetchNewestDocumentCommentRevisionCalls(), c.Newest)
			assert.Len(t, tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.RolledBack)
			require.Len(t, pub.calls, len(c.NotifyUsers))

			for i, users := range c.NotifyUsers {
				assert.Equal(t, "org1", pub.calls[i].OrganizationID)
				assert.Equal(t, users, pub.calls[i].UserIDs)
				assert.Equal(t, notification.NotificationDocumentCommentResolved, pub.calls[i].Core.Code)
				assert.Equal(t, "u1", pub.calls[i].Core.Metadata[notification.MetaKeyUserID])
				assert.Equal(t, "hello", pub.calls[i].Core.Metadata[notification.MetaKeyCommentExcerpt])
			}

			uu := tx.UpdateDocumentCommentStatusCalls()
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

			if c.EntryType == "" {
				return
			}

			assert.Equal(t, _commentID, tx.FetchNewestDocumentCommentRevisionCalls()[0].CommentID)
			assertEntry(
				t,
				tx,
				c.EntryType,
				activity.NewTarget(activity.TargetKindCommentRevision, _revisionID.String()),
				commentMetadata(_commentID),
			)
		})
	}
}

func Test_Handler_DeleteDocumentComment(t *testing.T) {
	stubComment := func(replies ...commentCore.Reply) func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
		return func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
			return storedComment("u1", replies...), nil
		}
	}

	cc := map[string]struct {
		Tx         *TxMock
		BeginErr   error
		NoSession  bool
		OmitDoc    bool
		RespCode   int
		Begun      int
		Fetched    int
		Deleted    int
		Newest     int
		Entries    int
		Committed  int
		RolledBack int
		ChangeType ChangeType
		EntryType  activity.Type
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
		"Already deleted comment": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					c := storedComment("u1", storedReply("u2"))
					c.DeletedAt = null.TimeFrom(time.Now())

					return c, nil
				},
			},
			RespCode:   http.StatusNotFound,
			Begun:      1,
			Fetched:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.DeleteDocumentComment": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: stubComment(),
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
		"Error returned by Tx.FetchNewestDocumentCommentRevision": {
			Tx: &TxMock{
				FetchDocumentCommentFunc: stubComment(),
				FetchNewestDocumentCommentRevisionFunc: func(context.Context, xid.ID, string) (commentCore.Revision, error) {
					return commentCore.Revision{}, assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Newest:     1,
			RolledBack: 1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			Tx: &TxMock{
				FetchDocumentCommentFunc:               stubComment(),
				FetchNewestDocumentCommentRevisionFunc: stubNewestRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Newest:     1,
			Entries:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.Commit": {
			Tx: &TxMock{
				FetchDocumentCommentFunc:               stubComment(),
				FetchNewestDocumentCommentRevisionFunc: stubNewestRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Newest:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
		},
		"Successful deletion without replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc:               stubComment(),
				FetchNewestDocumentCommentRevisionFunc: stubNewestRevision,
			},
			RespCode:   http.StatusNoContent,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Newest:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
			ChangeType: ChangeTypeDeleted,
			EntryType:  activity.TypeCommentDeleted,
		},
		"Successful deletion keeps a thread with replies": {
			Tx: &TxMock{
				FetchDocumentCommentFunc:               stubComment(storedReply("u2")),
				FetchNewestDocumentCommentRevisionFunc: stubNewestRevision,
			},
			RespCode:   http.StatusNoContent,
			Begun:      1,
			Fetched:    1,
			Deleted:    1,
			Newest:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
			ChangeType: ChangeTypeUpdated,
			EntryType:  activity.TypeCommentDeleted,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cpt := &capturedChange{}
			db := &DBMock{}
			stubTx(db, c.Tx, c.BeginErr)
			hdl := newTestHandler(db, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentComment(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, false, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, db.BeginTxCalls(), c.Begun)
			assert.Len(t, c.Tx.FetchDocumentCommentCalls(), c.Fetched)
			require.Len(t, c.Tx.DeleteDocumentCommentCalls(), c.Deleted)
			require.Len(t, c.Tx.FetchNewestDocumentCommentRevisionCalls(), c.Newest)
			assert.Len(t, c.Tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.RolledBack)

			if c.Deleted > 0 {
				deleted := c.Tx.DeleteDocumentCommentCalls()[0]
				assert.Equal(t, _commentID, deleted.ID)
				assert.Equal(t, _documentID, deleted.DocumentID)
				assert.Equal(t, "org1", deleted.OrganizationID)
			}

			if c.EntryType != "" {
				assert.Equal(t, _commentID, c.Tx.FetchNewestDocumentCommentRevisionCalls()[0].CommentID)
				assertEntry(
					t,
					c.Tx,
					c.EntryType,
					activity.NewTarget(activity.TargetKindCommentRevision, _revisionID.String()),
					commentMetadata(_commentID),
				)
			}

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
	stubComment := func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
		return storedComment("u2", storedReply("u1"), storedReply("u3")), nil
	}

	stubReply := func(userID string) func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
		return func(context.Context, xid.ID, xid.ID, string) (*commentCore.Reply, error) {
			r := storedReply(userID)

			return &r, nil
		}
	}

	cc := map[string]struct {
		DB             *DBMock
		Tx             *TxMock
		BeginErr       error
		NoSession      bool
		OmitReply      bool
		RespCode       int
		CommentFetched int
		ReplyFetched   int
		Begun          int
		Deleted        int
		Newest         int
		Entries        int
		Committed      int
		RolledBack     int
		ChangeType     ChangeType
		EntryType      activity.Type
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
				FetchDocumentCommentFunc: stubComment,
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
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u2"),
			},
			RespCode:       http.StatusForbidden,
			CommentFetched: 1,
			ReplyFetched:   1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			BeginErr:       assert.AnError,
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
		},
		"Error returned by Tx.DeleteDocumentCommentReply": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				DeleteDocumentCommentReplyFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Deleted:        1,
			RolledBack:     1,
		},
		"Error returned by Tx.FetchNewestDocumentCommentReplyRevision": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				FetchNewestDocumentCommentReplyRevisionFunc: func(context.Context, xid.ID, string) (commentCore.ReplyRevision, error) {
					return commentCore.ReplyRevision{}, assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Deleted:        1,
			Newest:         1,
			RolledBack:     1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				FetchNewestDocumentCommentReplyRevisionFunc: stubNewestReplyRevision,
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Deleted:        1,
			Newest:         1,
			Entries:        1,
			RolledBack:     1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx: &TxMock{
				FetchNewestDocumentCommentReplyRevisionFunc: stubNewestReplyRevision,
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			RespCode:       http.StatusInternalServerError,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Deleted:        1,
			Newest:         1,
			Entries:        1,
			Committed:      1,
			RolledBack:     1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchDocumentCommentFunc:      stubComment,
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx:             &TxMock{FetchNewestDocumentCommentReplyRevisionFunc: stubNewestReplyRevision},
			RespCode:       http.StatusOK,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Deleted:        1,
			Newest:         1,
			Entries:        1,
			Committed:      1,
			RolledBack:     1,
			ChangeType:     ChangeTypeUpdated,
			EntryType:      activity.TypeCommentReplyDeleted,
		},
		"Successful deletion of the last reply under a deleted comment": {
			DB: &DBMock{
				FetchDocumentCommentFunc: func(context.Context, xid.ID, xid.ID, string) (*commentCore.Comment, error) {
					c := storedComment("u2", storedReply("u1"))
					c.DeletedAt = null.TimeFrom(time.Now())

					return c, nil
				},
				FetchDocumentCommentReplyFunc: stubReply("u1"),
			},
			Tx:             &TxMock{FetchNewestDocumentCommentReplyRevisionFunc: stubNewestReplyRevision},
			RespCode:       http.StatusOK,
			CommentFetched: 1,
			ReplyFetched:   1,
			Begun:          1,
			Deleted:        1,
			Newest:         1,
			Entries:        1,
			Committed:      1,
			RolledBack:     1,
			ChangeType:     ChangeTypeDeleted,
			EntryType:      activity.TypeCommentReplyDeleted,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := stubTx(c.DB, c.Tx, c.BeginErr)
			cpt := &capturedChange{}
			hdl := newTestHandler(c.DB, &fakePublisher{}, cpt)

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentCommentReply(rec, newRequest(http.MethodDelete, "", c.NoSession, false, false, c.OmitReply))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCommentCalls(), c.CommentFetched)
			assert.Len(t, c.DB.FetchDocumentCommentReplyCalls(), c.ReplyFetched)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			require.Len(t, tx.DeleteDocumentCommentReplyCalls(), c.Deleted)
			require.Len(t, tx.FetchNewestDocumentCommentReplyRevisionCalls(), c.Newest)
			assert.Len(t, tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.RolledBack)

			if c.Deleted > 0 {
				deleted := tx.DeleteDocumentCommentReplyCalls()[0]
				assert.Equal(t, _replyID, deleted.ID)
				assert.Equal(t, _commentID, deleted.CommentID)
				assert.Equal(t, "org1", deleted.OrganizationID)
			}

			if c.EntryType != "" {
				assert.Equal(t, _replyID, tx.FetchNewestDocumentCommentReplyRevisionCalls()[0].ReplyID)
				assertEntry(
					t,
					tx,
					c.EntryType,
					activity.NewTarget(activity.TargetKindReplyRevision, _revisionID.String()),
					replyMetadata(_replyID),
				)
			}

			if c.ChangeType == "" {
				assert.Empty(t, cpt.calls)
				return
			}

			require.Len(t, cpt.calls, 1)
			assert.Equal(t, c.ChangeType, cpt.calls[0].Type)
		})
	}
}

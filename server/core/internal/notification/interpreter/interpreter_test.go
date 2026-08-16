package interpreter

import (
	"context"
	"fmt"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testDocID identifies the stub document in tests.
var _testDocID = xid.New()

// _testBranchID identifies the stub branch in tests.
var _testBranchID = xid.New()

// _testDocURL is the expected URL of the stub document.
var _testDocURL = fmt.Sprintf("https://app.test/acme/my-doc-%s", _testDocID)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// stubDoc creates a stub document for tests.
func stubDoc() *document.Document {
	return &document.Document{
		Branch: document.Branch{
			BranchName:   "main",
			DocumentName: "My Doc",
		},
		ID: _testDocID,
	}
}

// stubDB creates a database mock whose calls all succeed.
func stubDB() *DBMock {
	return &DBMock{
		FetchDocumentByBranchIDFunc: func(_ context.Context, _ xid.ID, _ string) (*document.Document, error) {
			return stubDoc(), nil
		},
		FetchOrganizationSlugFunc: func(_ context.Context, _ string) (string, error) {
			return "acme", nil
		},
		FetchUserNameFunc: func(_ context.Context, _ string) (string, error) {
			return "Alice", nil
		},
	}
}

// stubNotification wraps a notification core into a full notification.
func stubNotification(nc notification.Core) notification.Notification {
	return notification.Notification{
		Core:           nc,
		OrganizationID: "org1",
	}
}

func Test_NewInterpreter(t *testing.T) {
	t.Parallel()

	db := &DBMock{}

	i := NewInterpreter(db, NewSlackFormatter(), "https://app.test")

	require.NotNil(t, i)
	assert.Equal(t, db, i.db)
	assert.Equal(t, NewSlackFormatter(), i.fm)
	assert.Equal(t, "https://app.test", i.appURL)
}

func Test_Interpreter_InterpretNotification(t *testing.T) {
	cc := map[string]struct {
		N      notification.Notification
		Result *Message
		Err    error
	}{
		"Unknown notification code": {
			N: stubNotification(notification.Core{
				Code: notification.Code("bogus"),
			}),
			Err: ErrUnknownNotificationCode,
		},
		"Document review request": {
			N: stubNotification(notification.NewDocumentReviewRequestNotification(
				"user1",
				_testDocID,
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Your review was requested on the main branch of <%s|My Doc>", _testDocURL),
			},
		},
		"Document hook triggered": {
			N: stubNotification(notification.NewDocumentHookTriggeredNotification(
				_testDocID,
				hook.TypeScheduledReminder,
				null.String{},
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("<%s|My Doc> may be outdated — Scheduled Reminder", _testDocURL),
			},
		},
		"Document new comment": {
			N: stubNotification(notification.NewDocumentNewCommentNotification(
				"user1",
				_testDocID,
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Alice left a comment on <%s|My Doc>", _testDocURL),
			},
		},
		"Document new comment reply": {
			N: stubNotification(notification.NewDocumentNewCommentReplyNotification(
				"user1",
				_testDocID,
				xid.New(),
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Alice replied to a comment on <%s|My Doc>", _testDocURL),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			i := NewInterpreter(stubDB(), NewSlackFormatter(), "https://app.test")

			res, err := i.InterpretNotification(context.Background(), c.N)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Interpreter_interpretDocumentReviewRequestNotification(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		N      notification.Notification
		Result *Message
		Err    error
	}{
		"Invalid branch metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code:     notification.NotificationDocumentReviewRequest,
				Metadata: notification.Metadata{},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchDocumentByBranchIDFunc = func(_ context.Context, _ xid.ID, _ string) (*document.Document, error) {
					return nil, assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentReviewRequestNotification(
				"user1",
				_testDocID,
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Error returned by db.FetchOrganizationSlug": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchOrganizationSlugFunc = func(_ context.Context, _ string) (string, error) {
					return "", assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentReviewRequestNotification(
				"user1",
				_testDocID,
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Successful interpretation": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentReviewRequestNotification(
				"user1",
				_testDocID,
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Your review was requested on the main branch of <%s|My Doc>", _testDocURL),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			i := NewInterpreter(c.DB, NewSlackFormatter(), "https://app.test")

			res, err := i.interpretDocumentReviewRequestNotification(context.Background(), c.N)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)

			ff := c.DB.FetchDocumentByBranchIDCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testBranchID, ff[0].BranchID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
		})
	}
}

func Test_Interpreter_interpretDocumentHookTriggeredNotification(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		N      notification.Notification
		Result *Message
		Err    error
	}{
		"Invalid type metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentHookTriggered,
				Metadata: notification.Metadata{
					"type": "not-a-hook-type",
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Invalid block metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentHookTriggered,
				Metadata: notification.Metadata{
					"type":    hook.TypeScheduledReminder,
					"blockId": "not-a-null-string",
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Invalid branch metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentHookTriggered,
				Metadata: notification.Metadata{
					"type":    hook.TypeScheduledReminder,
					"blockId": null.String{},
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchDocumentByBranchIDFunc = func(_ context.Context, _ xid.ID, _ string) (*document.Document, error) {
					return nil, assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentHookTriggeredNotification(
				_testDocID,
				hook.TypeScheduledReminder,
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Error returned by db.FetchOrganizationSlug": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchOrganizationSlugFunc = func(_ context.Context, _ string) (string, error) {
					return "", assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentHookTriggeredNotification(
				_testDocID,
				hook.TypeScheduledReminder,
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Successful interpretation": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentHookTriggeredNotification(
				_testDocID,
				hook.TypeScheduledReminder,
				null.String{},
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("<%s|My Doc> may be outdated — Scheduled Reminder", _testDocURL),
			},
		},
		"Successful interpretation with block": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentHookTriggeredNotification(
				_testDocID,
				hook.TypeScheduledReminder,
				null.StringFrom("blk1"),
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("<%s#blk1|a block in My Doc> may be outdated — Scheduled Reminder", _testDocURL),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			i := NewInterpreter(c.DB, NewSlackFormatter(), "https://app.test")

			res, err := i.interpretDocumentHookTriggeredNotification(context.Background(), c.N)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Interpreter_interpretDocumentNewCommentNotification(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		N      notification.Notification
		Result *Message
		Err    error
	}{
		"Invalid user metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code:     notification.NotificationDocumentNewComment,
				Metadata: notification.Metadata{},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Invalid anchor block metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentNewComment,
				Metadata: notification.Metadata{
					"userId":        "user1",
					"anchorBlockId": "not-a-null-string",
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Invalid branch metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentNewComment,
				Metadata: notification.Metadata{
					"userId":        "user1",
					"anchorBlockId": null.String{},
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchDocumentByBranchIDFunc = func(_ context.Context, _ xid.ID, _ string) (*document.Document, error) {
					return nil, assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentNewCommentNotification(
				"user1",
				_testDocID,
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Error returned by db.FetchUserName": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchUserNameFunc = func(_ context.Context, _ string) (string, error) {
					return "", assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentNewCommentNotification(
				"user1",
				_testDocID,
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Error returned by db.FetchOrganizationSlug": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchOrganizationSlugFunc = func(_ context.Context, _ string) (string, error) {
					return "", assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentNewCommentNotification(
				"user1",
				_testDocID,
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Successful interpretation": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentNewCommentNotification(
				"user1",
				_testDocID,
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Alice left a comment on <%s|My Doc>", _testDocURL),
			},
		},
		"Successful interpretation with anchor block": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentNewCommentNotification(
				"user1",
				_testDocID,
				xid.New(),
				null.StringFrom("blk1"),
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Alice left a comment on <%s#blk1|My Doc>", _testDocURL),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			i := NewInterpreter(c.DB, NewSlackFormatter(), "https://app.test")

			res, err := i.interpretDocumentNewCommentNotification(context.Background(), c.N)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)

			ff := c.DB.FetchUserNameCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "user1", ff[0].UserID)
		})
	}
}

func Test_Interpreter_interpretDocumentNewCommentReplyNotification(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		N      notification.Notification
		Result *Message
		Err    error
	}{
		"Invalid user metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code:     notification.NotificationDocumentNewCommentReply,
				Metadata: notification.Metadata{},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Invalid anchor block metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentNewCommentReply,
				Metadata: notification.Metadata{
					"userId":        "user1",
					"anchorBlockId": "not-a-null-string",
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Invalid branch metadata": {
			DB: stubDB(),
			N: stubNotification(notification.Core{
				Code: notification.NotificationDocumentNewCommentReply,
				Metadata: notification.Metadata{
					"userId":        "user1",
					"anchorBlockId": null.String{},
				},
			}),
			Err: ErrInvalidNotificationMetadata,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchDocumentByBranchIDFunc = func(_ context.Context, _ xid.ID, _ string) (*document.Document, error) {
					return nil, assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentNewCommentReplyNotification(
				"user1",
				_testDocID,
				xid.New(),
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Error returned by db.FetchUserName": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchUserNameFunc = func(_ context.Context, _ string) (string, error) {
					return "", assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentNewCommentReplyNotification(
				"user1",
				_testDocID,
				xid.New(),
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Error returned by db.FetchOrganizationSlug": {
			DB: func() *DBMock {
				db := stubDB()
				db.FetchOrganizationSlugFunc = func(_ context.Context, _ string) (string, error) {
					return "", assert.AnError
				}

				return db
			}(),
			N: stubNotification(notification.NewDocumentNewCommentReplyNotification(
				"user1",
				_testDocID,
				xid.New(),
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Err: assert.AnError,
		},
		"Successful interpretation": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentNewCommentReplyNotification(
				"user1",
				_testDocID,
				xid.New(),
				xid.New(),
				null.String{},
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Alice replied to a comment on <%s|My Doc>", _testDocURL),
			},
		},
		"Successful interpretation with anchor block": {
			DB: stubDB(),
			N: stubNotification(notification.NewDocumentNewCommentReplyNotification(
				"user1",
				_testDocID,
				xid.New(),
				xid.New(),
				null.StringFrom("blk1"),
				_testBranchID,
			)),
			Result: &Message{
				Text: fmt.Sprintf("Alice replied to a comment on <%s#blk1|My Doc>", _testDocURL),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			i := NewInterpreter(c.DB, NewSlackFormatter(), "https://app.test")

			res, err := i.interpretDocumentNewCommentReplyNotification(context.Background(), c.N)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_metaBranchID(t *testing.T) {
	cc := map[string]struct {
		Metadata notification.Metadata
		ID       xid.ID
		OK       bool
	}{
		"Typed branch ID": {
			Metadata: notification.Metadata{"branchId": _testBranchID},
			ID:       _testBranchID,
			OK:       true,
		},
		"String branch ID": {
			Metadata: notification.Metadata{"branchId": _testBranchID.String()},
			ID:       _testBranchID,
			OK:       true,
		},
		"Invalid string branch ID": {
			Metadata: notification.Metadata{"branchId": "not-an-id"},
		},
		"Unexpected branch ID type": {
			Metadata: notification.Metadata{"branchId": 123},
		},
		"Missing branch ID": {
			Metadata: notification.Metadata{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			id, ok := metaBranchID(notification.Notification{
				Core: notification.Core{Metadata: c.Metadata},
			})

			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.ID, id)
		})
	}
}

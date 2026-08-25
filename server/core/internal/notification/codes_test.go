package notification

import (
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_NewDocumentReviewRequestNotification(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	nc := NewDocumentReviewRequestNotification("user1", documentID, branchID)

	assert.Equal(t, Core{
		Code: NotificationDocumentReviewRequest,
		Metadata: Metadata{
			MetaKeyUserID:     "user1",
			MetaKeyDocumentID: documentID,
			MetaKeyBranchID:   branchID,
		},
	}, nc)
}

func Test_NewDocumentHookTriggeredNotification(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	nc := NewDocumentHookTriggeredNotification(
		documentID,
		hook.TypeScheduledReminder,
		null.StringFrom("blk1"),
		branchID,
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentHookTriggered,
		Metadata: Metadata{
			MetaKeyDocumentID: documentID,
			MetaKeyBlockID:    null.StringFrom("blk1"),
			MetaKeyType:       hook.TypeScheduledReminder,
			MetaKeyBranchID:   branchID,
		},
	}, nc)
}

func Test_NewDocumentNewCommentNotification(t *testing.T) {
	t.Parallel()

	documentID, commentID, branchID := xid.New(), xid.New(), xid.New()

	nc := NewDocumentNewCommentNotification(
		"user1",
		documentID,
		commentID,
		null.StringFrom("blk1"),
		branchID,
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentNewComment,
		Metadata: Metadata{
			MetaKeyUserID:        "user1",
			MetaKeyDocumentID:    documentID,
			MetaKeyCommentID:     commentID,
			MetaKeyAnchorBlockID: null.StringFrom("blk1"),
			MetaKeyBranchID:      branchID,
		},
	}, nc)
}

func Test_NewDocumentNewCommentReplyNotification(t *testing.T) {
	t.Parallel()

	documentID, commentID, commentReplyID, branchID := xid.New(), xid.New(), xid.New(), xid.New()

	nc := NewDocumentNewCommentReplyNotification(
		"user1",
		documentID,
		commentID,
		commentReplyID,
		null.StringFrom("blk1"),
		branchID,
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentNewCommentReply,
		Metadata: Metadata{
			MetaKeyUserID:         "user1",
			MetaKeyDocumentID:     documentID,
			MetaKeyCommentID:      commentID,
			MetaKeyCommentReplyID: commentReplyID,
			MetaKeyAnchorBlockID:  null.StringFrom("blk1"),
			MetaKeyBranchID:       branchID,
		},
	}, nc)
}

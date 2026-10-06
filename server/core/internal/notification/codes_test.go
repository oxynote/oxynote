package notification

import (
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
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
		hook.TypeGithubTracking,
		null.StringFrom("blk1"),
		branchID,
		processor.Settings(`{"repository":"repo"}`),
		null.IntFrom(2),
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentHookTriggered,
		Metadata: Metadata{
			MetaKeyDocumentID:                 documentID,
			MetaKeyBlockID:                    null.StringFrom("blk1"),
			MetaKeyType:                       hook.TypeGithubTracking,
			MetaKeyBranchID:                   branchID,
			MetaKeyHookSettings:               processor.Settings(`{"repository":"repo"}`),
			MetaKeyGithubTrackingChangedPaths: null.IntFrom(2),
		},
	}, nc)
}

func Test_NewDocumentHookNeedsAttentionNotification(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	nc := NewDocumentHookNeedsAttentionNotification(
		documentID,
		hook.TypeGithubTracking,
		null.StringFrom("blk1"),
		branchID,
		processor.StatusMissingRepository,
		processor.Settings(`{"repository":"repo"}`),
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentHookNeedsAttention,
		Metadata: Metadata{
			MetaKeyDocumentID:   documentID,
			MetaKeyBlockID:      null.StringFrom("blk1"),
			MetaKeyType:         hook.TypeGithubTracking,
			MetaKeyBranchID:     branchID,
			MetaKeyStatus:       processor.StatusMissingRepository,
			MetaKeyHookSettings: processor.Settings(`{"repository":"repo"}`),
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
		"Worth a second look",
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentNewComment,
		Metadata: Metadata{
			MetaKeyUserID:         "user1",
			MetaKeyDocumentID:     documentID,
			MetaKeyCommentID:      commentID,
			MetaKeyAnchorBlockID:  null.StringFrom("blk1"),
			MetaKeyBranchID:       branchID,
			MetaKeyCommentExcerpt: "Worth a second look",
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
		"Agreed",
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
			MetaKeyCommentExcerpt: "Agreed",
		},
	}, nc)
}

func Test_NewDocumentCommentResolvedNotification(t *testing.T) {
	t.Parallel()

	documentID, commentID, branchID := xid.New(), xid.New(), xid.New()

	nc := NewDocumentCommentResolvedNotification(
		"user1",
		documentID,
		commentID,
		null.StringFrom("blk1"),
		branchID,
		"Worth a second look",
	)

	assert.Equal(t, Core{
		Code: NotificationDocumentCommentResolved,
		Metadata: Metadata{
			MetaKeyUserID:         "user1",
			MetaKeyDocumentID:     documentID,
			MetaKeyCommentID:      commentID,
			MetaKeyAnchorBlockID:  null.StringFrom("blk1"),
			MetaKeyBranchID:       branchID,
			MetaKeyCommentExcerpt: "Worth a second look",
		},
	}, nc)
}

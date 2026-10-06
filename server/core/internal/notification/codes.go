// Package notification defines user notifications and their publication.
package notification

import (
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/rs/xid"
)

// Code represents a notification code.
type Code string

// Metadata keys used by the notification constructors. Readers interpreting
// a stored notification must name keys through these constants too — a raw
// literal would compile clean and silently drift on a rename.
const (
	MetaKeyUserID                     = "userId"
	MetaKeyDocumentID                 = "documentId"
	MetaKeyBranchID                   = "branchId"
	MetaKeyBlockID                    = "blockId"
	MetaKeyType                       = "type"
	MetaKeyCommentID                  = "commentId"
	MetaKeyCommentReplyID             = "commentReplyId"
	MetaKeyAnchorBlockID              = "anchorBlockId"
	MetaKeyStatus                     = "status"
	MetaKeyHookSettings               = "hookSettings"
	MetaKeyGithubTrackingChangedPaths = "githubTrackingChangedPaths"
	MetaKeyCommentExcerpt             = "commentExcerpt"
)

const (
	// NotificationDocumentReviewRequest is the notification code for document review requests.
	NotificationDocumentReviewRequest Code = "notification.document.review_request"

	// NotificationDocumentHookTriggered is the notification code for document hook triggers.
	NotificationDocumentHookTriggered Code = "notification.document.hook_triggered"

	// NotificationDocumentHookNeedsAttention is the notification code for
	// a document hook that can no longer check its target.
	NotificationDocumentHookNeedsAttention Code = "notification.document.hook_needs_attention"

	// NotificationDocumentNewComment is the notification code for new document comments.
	NotificationDocumentNewComment Code = "notification.document.new_comment"

	// NotificationDocumentNewCommentReply is the notification code for new replies to document comments.
	NotificationDocumentNewCommentReply Code = "notification.document.new_comment_reply"

	// NotificationDocumentCommentResolved is the notification code for resolved document comments.
	NotificationDocumentCommentResolved Code = "notification.document.comment_resolved"
)

// NewDocumentReviewRequestNotification creates a new document review request notification core.
func NewDocumentReviewRequestNotification(userID string, documentID, branchID xid.ID) Core {
	return Core{
		Code: NotificationDocumentReviewRequest,
		Metadata: map[string]any{
			MetaKeyUserID:     userID,
			MetaKeyDocumentID: documentID,
			MetaKeyBranchID:   branchID,
		},
	}
}

// NewDocumentHookTriggeredNotification creates a new document hook triggered
// notification core. It keeps the settings the hook had when it triggered.
// The changed paths are null for a hook that counts none.
func NewDocumentHookTriggeredNotification(
	documentID xid.ID,
	tp hook.Type,
	blockID null.String,
	branchID xid.ID,
	settings processor.Settings,
	changedPaths null.Int,
) Core {
	return Core{
		Code: NotificationDocumentHookTriggered,
		Metadata: map[string]any{
			MetaKeyDocumentID:                 documentID,
			MetaKeyBlockID:                    blockID,
			MetaKeyType:                       tp,
			MetaKeyBranchID:                   branchID,
			MetaKeyHookSettings:               settings,
			MetaKeyGithubTrackingChangedPaths: changedPaths,
		},
	}
}

// NewDocumentHookNeedsAttentionNotification creates a new notification
// core for a document hook that can no longer check its target. It keeps
// the settings the hook had at that time.
func NewDocumentHookNeedsAttentionNotification(
	documentID xid.ID,
	tp hook.Type,
	blockID null.String,
	branchID xid.ID,
	status processor.Status,
	settings processor.Settings,
) Core {
	return Core{
		Code: NotificationDocumentHookNeedsAttention,
		Metadata: map[string]any{
			MetaKeyDocumentID:   documentID,
			MetaKeyBlockID:      blockID,
			MetaKeyType:         tp,
			MetaKeyBranchID:     branchID,
			MetaKeyStatus:       status,
			MetaKeyHookSettings: settings,
		},
	}
}

// NewDocumentNewCommentNotification creates a new document new comment
// notification core. The excerpt is the start of the comment's text.
func NewDocumentNewCommentNotification(
	userID string,
	documentID, commentID xid.ID,
	anchorBlockID null.String,
	branchID xid.ID,
	excerpt string,
) Core {
	return Core{
		Code: NotificationDocumentNewComment,
		Metadata: map[string]any{
			MetaKeyUserID:         userID,
			MetaKeyDocumentID:     documentID,
			MetaKeyCommentID:      commentID,
			MetaKeyAnchorBlockID:  anchorBlockID,
			MetaKeyBranchID:       branchID,
			MetaKeyCommentExcerpt: excerpt,
		},
	}
}

// NewDocumentNewCommentReplyNotification creates a new document new comment
// reply notification core. The excerpt is the start of the reply's text.
func NewDocumentNewCommentReplyNotification(
	userID string,
	documentID, commentID, commentReplyID xid.ID,
	anchorBlockID null.String,
	branchID xid.ID,
	excerpt string,
) Core {
	return Core{
		Code: NotificationDocumentNewCommentReply,
		Metadata: map[string]any{
			MetaKeyUserID:         userID,
			MetaKeyDocumentID:     documentID,
			MetaKeyCommentID:      commentID,
			MetaKeyCommentReplyID: commentReplyID,
			MetaKeyAnchorBlockID:  anchorBlockID,
			MetaKeyBranchID:       branchID,
			MetaKeyCommentExcerpt: excerpt,
		},
	}
}

// NewDocumentCommentResolvedNotification creates a new document comment
// resolved notification core. The user is the one who resolved the comment,
// and the excerpt is the start of the comment's text.
func NewDocumentCommentResolvedNotification(
	userID string,
	documentID, commentID xid.ID,
	anchorBlockID null.String,
	branchID xid.ID,
	excerpt string,
) Core {
	return Core{
		Code: NotificationDocumentCommentResolved,
		Metadata: map[string]any{
			MetaKeyUserID:         userID,
			MetaKeyDocumentID:     documentID,
			MetaKeyCommentID:      commentID,
			MetaKeyAnchorBlockID:  anchorBlockID,
			MetaKeyBranchID:       branchID,
			MetaKeyCommentExcerpt: excerpt,
		},
	}
}

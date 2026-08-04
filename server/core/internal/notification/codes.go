package notification

import (
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/rs/xid"
)

// NotificationCode represents a notification code.
type NotificationCode string

const (
	// NotificationDocumentReviewRequest is the notification code for document review requests.
	NotificationDocumentReviewRequest NotificationCode = "notification.document.review_request"

	// NotificationDocumentHookTriggered is the notification code for document hook triggers.
	NotificationDocumentHookTriggered NotificationCode = "notification.document.hook_triggered"

	// NotificationDocumentNewComment is the notification code for new document comments.
	NotificationDocumentNewComment NotificationCode = "notification.document.new_comment"

	// NotificationDocumentNewCommentReply is the notification code for new replies to document comments.
	NotificationDocumentNewCommentReply NotificationCode = "notification.document.new_comment_reply"
)

// NewDocumentReviewRequestNotification creates a new document review request notification core.
func NewDocumentReviewRequestNotification(userID string, documentID xid.ID, branchID xid.ID) NotificationCore {
	return NotificationCore{
		Code: NotificationDocumentReviewRequest,
		Metadata: map[string]any{
			"userId":     userID,
			"documentId": documentID,
			"branchId":   branchID,
		},
	}
}

// NewDocumentHookTriggeredNotification creates a new document hook triggered notification core.
func NewDocumentHookTriggeredNotification(documentID xid.ID, tp hook.Type, blockId null.String, branchID xid.ID) NotificationCore {
	return NotificationCore{
		Code: NotificationDocumentHookTriggered,
		Metadata: map[string]any{
			"documentId": documentID,
			"blockId":    blockId,
			"type":       tp,
			"branchId":   branchID,
		},
	}
}

// NewDocumentNewCommentNotification creates a new document new comment notification core.
func NewDocumentNewCommentNotification(
	userID string,
	documentID, commentID xid.ID,
	anchorBlockID null.String,
	branchID xid.ID,
) NotificationCore {
	return NotificationCore{
		Code: NotificationDocumentNewComment,
		Metadata: map[string]any{
			"userId":        userID,
			"documentId":    documentID,
			"commentId":     commentID,
			"anchorBlockId": anchorBlockID,
			"branchId":      branchID,
		},
	}
}

// NewDocumentNewCommentReplyNotification creates a new document new comment reply notification core.
func NewDocumentNewCommentReplyNotification(
	userID string,
	documentID, commentID, commentReplyID xid.ID,
	anchorBlockID null.String,
	branchID xid.ID,
) NotificationCore {
	return NotificationCore{
		Code: NotificationDocumentNewCommentReply,
		Metadata: map[string]any{
			"userId":         userID,
			"documentId":     documentID,
			"commentId":      commentID,
			"commentReplyId": commentReplyID,
			"anchorBlockId":  anchorBlockID,
			"branchId":       branchID,
		},
	}
}

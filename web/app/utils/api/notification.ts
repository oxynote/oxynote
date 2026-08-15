export const WS_NOTIFICATION_CREATION_TOPIC = "creation@notifications"

export interface NotificationsParams {
	limit: number
	page: number
}

export interface NotificationsCountParams {
	read: boolean
}

export interface Notification {
	id: string
	userId: string
	organizationId: string
	code: NotificationCode
	metadata: NotificationMetadata
	read: boolean
	createdAt: Date | string
}

export enum NotificationCode {
	DocumentReviewRequest = "notification.document.review_request",
	DocumentHookTrigerred = "notification.document.hook_triggered",
	DocumentNewComment = "notification.document.new_comment",
	DocumentNewCommentReply = "notification.document.new_comment_reply",
}

export type NotificationMetadata =
	| NotificationMetadataDocumentReviewRequest
	| NotificationMetadataDocumentHookTriggered
	| NotificationMetadataDocumentNewComment
	| NotificationMetadataDocumentNewCommentReply

export interface NotificationMetadataDocumentReviewRequest {
	documentId: string
	branchId: string
}

export interface NotificationMetadataDocumentHookTriggered {
	documentId: string
	branchId: string
	blockId: string | null
	type: DocumentHookType
}

export interface NotificationMetadataDocumentNewComment {
	userId: string
	documentId: string
	branchId: string
	commentId: string
	anchorBlockId: string | null
}

export interface NotificationMetadataDocumentNewCommentReply {
	userId: string
	documentId: string
	branchId: string
	commentId: string
	commentReplyId: string
	anchorBlockId: string | null
}

export interface NotificationsResponse {
	notifications: Notification[]
	pageCount: number
}

export interface MarkNotificationsReadRequest {
	ids: string[] // empty means mark all as read
}

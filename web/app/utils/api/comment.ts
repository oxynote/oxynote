import { compareAsc } from "date-fns"

export function makeWsDocumentCommentChangeTopic(docId: string): string {
	return `change@documents.${docId}.comments`
}

export interface DocumentComment {
	id: string
	organizationId: string
	documentId: string
	branchId: string
	anchorBlockId: string
	userId: string
	resolved: boolean
	resolvedBy?: string | null
	content: Record<string, any>
	createdAt: Date | string
	updatedAt?: Date | string | null
	replies?: DocumentCommentReply[]

	// present only on comments anchored to deleted diff content
	diffDeletionContext?: DocumentCommentDiffDeletionContext | null
}

export interface DocumentCommentReply {
	id: string
	organizationId: string
	commentId: string
	userId: string
	content: Record<string, any>
	createdAt: Date | string
	updatedAt?: Date | string | null
}

// text anchor within a specific node of deleted content. used to
// reconstruct the exact text range that was commented on when the
// deleted node is displayed in the diff view.
export interface DocumentDiffDeletedContentTextAnchor {
	// the uid attribute of the deleted node in the original (published)
	// document. used to locate the node in the diff editor's position
	// map when re-injecting comment marks.
	nodeUid: string
	// character offset from the start of the node where the commented
	// text begins (ProseMirror position relative to the node's start).
	fromOffset: number
	// character offset from the start of the node where the commented
	// text ends (exclusive, same coordinate system as fromOffset).
	toOffset: number
	// when true, the from boundary of the reinjected mark should
	// extend leftward through adjacent removed/added text until it
	// reaches a comment mark with the same comment ID. if no matching
	// mark is found the boundary stays at fromOffset.
	snapFrom?: boolean
	// when true, the to boundary of the reinjected mark should
	// extend rightward through adjacent removed/added text until it
	// reaches a comment mark with the same comment ID. if no matching
	// mark is found the boundary stays at toOffset.
	snapTo?: boolean
}

// groups all metadata needed to anchor a comment to deleted content.
// present only on comments that reference content removed in a draft.
export interface DocumentCommentDiffDeletionContext {
	// for text comments: ordered list of text anchors within deleted
	// nodes. each entry identifies one node and the text range within
	// it. a single user selection spanning multiple consecutive deleted
	// nodes produces multiple entries.
	//
	// for node comments: omitted — the anchorBlockId on the parent
	// DocumentComment already identifies the deleted node.
	textAnchors?: DocumentDiffDeletedContentTextAnchor[]
}

export function promoteFirstDocumentReplyToComment(
	comment: DocumentComment,
): DocumentComment | null {
	// sort mutates in place, so this keeps sorting comment.replies itself
	// whenever there are replies to sort
	const replies = comment.replies ?? []

	const firstReply = replies.sort((a, b) =>
		compareAsc(new Date(a.createdAt), new Date(b.createdAt)),
	)[0]
	if (!firstReply) {
		return null
	}

	return {
		...comment,
		userId: firstReply.userId,
		content: firstReply.content,
		createdAt: firstReply.createdAt,
		updatedAt: firstReply.updatedAt,
		replies: replies.filter((r) => r.id !== firstReply.id),
	}
}

export type DocumentCommentResponse = DocumentComment

export type DocumentCommentsResponse = DocumentComment[]

export type DocumentCommentRepliesResponse = DocumentCommentReply[]

export interface DocumentCommentCreateRequest {
	content: Record<string, any>
	anchorBlockID: string
	branchId: string

	// set when creating a comment on diff mode deleted content
	diffDeletionContext?: DocumentCommentDiffDeletionContext
}

export type DocumentCommentCreateResponse = DocumentComment

export interface DocumentCommentReplyCreateRequest {
	content: Record<string, any>
}

export type DocumentCommentReplyCreateResponse = DocumentCommentReply

export type DocumentCommentUpdateRequest = DocumentCommentCreateRequest

export type DocumentCommentReplyUpdateRequest =
	DocumentCommentReplyCreateRequest

import { describe, it } from "vitest"
import {
	makeWsDocumentCommentChangeTopic,
	promoteFirstDocumentReplyToComment,
	type DocumentComment,
	type DocumentCommentReply,
} from "./comment"

function comment(replies?: DocumentCommentReply[]): DocumentComment {
	return {
		id: "c1",
		organizationId: "o1",
		documentId: "d1",
		branchId: "b1",
		anchorBlockId: "blk1",
		userId: "author",
		resolved: false,
		content: { text: "original" },
		createdAt: "2024-06-01T10:00:00Z",
		replies,
	}
}

function reply(
	id: string,
	userId: string,
	createdAt: string,
): DocumentCommentReply {
	return {
		id,
		organizationId: "o1",
		commentId: "c1",
		userId,
		content: { text: `reply-${id}` },
		createdAt,
		updatedAt: null,
	}
}

describe("makeWsDocumentCommentChangeTopic", () => {
	it("builds the comment change topic for the document", ({ expect }) => {
		expect(makeWsDocumentCommentChangeTopic("d1")).toBe(
			"change@documents.d1.comments",
		)
	})
})

describe("promoteFirstDocumentReplyToComment", () => {
	it("promotes the earliest reply into the comment", ({ expect }) => {
		const later = reply("r2", "u2", "2024-06-02T10:00:00Z")
		const earliest = reply("r1", "u1", "2024-06-01T12:00:00Z")

		const result = promoteFirstDocumentReplyToComment(
			comment([later, earliest]),
		)

		expect(result).toMatchObject({
			id: "c1",
			userId: "u1",
			content: { text: "reply-r1" },
			createdAt: "2024-06-01T12:00:00Z",
			replies: [later],
		})
	})

	it("returns null when the comment has no replies", ({ expect }) => {
		expect(promoteFirstDocumentReplyToComment(comment([]))).toBeNull()
	})

	it("returns null when the replies are missing entirely", ({ expect }) => {
		expect(promoteFirstDocumentReplyToComment(comment(undefined))).toBeNull()
	})
})

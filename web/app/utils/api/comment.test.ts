import { describe, it } from "vitest"
import {
	makeWsDocumentCommentChangeTopic,
	markDocumentCommentDeleted,
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

describe("markDocumentCommentDeleted", () => {
	it("keeps the thread with its replies and drops the comment's text", ({
		expect,
	}) => {
		const replies = [reply("r1", "u1", "2024-06-01T12:00:00Z")]

		const result = markDocumentCommentDeleted(comment(replies))

		expect(result).toEqual({
			...comment(replies),
			content: null,
			deletedAt: result?.deletedAt,
		})
		expect(typeof result?.deletedAt).toBe("string")
	})

	it("returns null when the comment has no replies", ({ expect }) => {
		expect(markDocumentCommentDeleted(comment([]))).toBeNull()
	})

	it("returns null when the replies are missing entirely", ({ expect }) => {
		expect(markDocumentCommentDeleted(comment(undefined))).toBeNull()
	})
})

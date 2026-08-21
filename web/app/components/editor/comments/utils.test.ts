import { describe, it, vi } from "vitest"
import {
	createPendingCommentId,
	deleteCommentFromEditor,
	deletePendingCommentData,
	isPendingCommentId,
	pendingCommentBelongsToActiveUser,
} from "./utils"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"
import {
	commented,
	commentShape,
	doc,
	makeEditor,
	nodeCommentShape,
	paragraph,
	text,
} from "./test-helpers"

describe("createPendingCommentId", () => {
	it("returns ids in the pending-<userId>-<counter> format", ({ expect }) => {
		expect(createPendingCommentId("u1")).toMatch(/^pending-u1-\d+$/)
	})

	it("returns a different id on every call", ({ expect }) => {
		expect(createPendingCommentId("u1")).not.toBe(createPendingCommentId("u1"))
	})
})

describe("isPendingCommentId", () => {
	it.for([
		{
			name: "accepts a generated pending id",
			id: "pending-u1-1",
			expected: true,
		},
		{ name: "rejects a saved comment id", id: "c123", expected: false },
		{ name: "rejects the bare pending prefix", id: "pending", expected: false },
		{ name: "rejects an empty id", id: "", expected: false },
	])("$name", ({ id, expected }, { expect }) => {
		expect(isPendingCommentId(id)).toBe(expected)
	})
})

describe("pendingCommentBelongsToActiveUser", () => {
	it.for([
		{
			name: "accepts the active user's pending id",
			id: "pending-u1-1",
			userId: "u1",
			expected: true,
		},
		{
			name: "rejects another user's pending id",
			id: "pending-u2-1",
			userId: "u1",
			expected: false,
		},
		{
			name: "rejects a user id that only shares a prefix",
			id: "pending-user12-1",
			userId: "user1",
			expected: false,
		},
		{
			name: "rejects a saved comment id",
			id: "c123",
			userId: "u1",
			expected: false,
		},
	])("$name", ({ id, userId, expected }, { expect }) => {
		expect(pendingCommentBelongsToActiveUser(id, userId)).toBe(expected)
	})
})

describe("deletePendingCommentData", () => {
	it("does nothing for an empty active user id", ({ expect }) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph(null, commented("marked", "pending-u1-1"))),
		)
		const onDelete = vi.fn()

		deletePendingCommentData(editor, "", onDelete)

		expect(dispatched).toHaveLength(0)
		expect(onDelete).toHaveBeenCalledTimes(0)
	})

	it("removes the active user's pending marks and node comments together", ({
		expect,
	}) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph(null, commented("marked", "pending-u1-1")),
				paragraph({ nodeCommentId: "pending-u1-2" }, text("block")),
			),
		)
		const onDelete = vi.fn()

		deletePendingCommentData(editor, "u1", onDelete)

		expect(commentShape(state().doc)).toEqual([
			["marked", null],
			["block", null],
		])
		expect(nodeCommentShape(state().doc)).toEqual([
			["marked", null],
			["block", null],
		])
		expect(dispatched).toHaveLength(2)
		expect(onDelete).toHaveBeenCalledTimes(2)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-1")
		expect(onDelete).toHaveBeenCalledWith("pending-u1-2")
	})

	it("keeps the excluded pending comment in both forms", ({ expect }) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph(null, commented("marked", "pending-u1-1")),
				paragraph({ nodeCommentId: "pending-u1-2" }, text("block")),
			),
		)
		const onDelete = vi.fn()

		deletePendingCommentData(editor, "u1", onDelete, false, "pending-u1-1")

		expect(commentShape(state().doc)).toEqual([
			["marked", "pending-u1-1"],
			["block", null],
		])
		expect(nodeCommentShape(state().doc)).toEqual([
			["marked", null],
			["block", null],
		])
		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-2")
	})
})

describe("deleteCommentFromEditor", () => {
	it("removes only the mark of the given text comment", ({ expect }) => {
		const { editor, dispatched, state } = makeEditor(
			doc(paragraph(null, commented("one", "c1"), commented("two", "c2"))),
		)

		deleteCommentFromEditor(editor, "c1", true)

		expect(commentShape(state().doc)).toEqual([
			["one", null],
			["two", "c2"],
		])
		expect(dispatched).toHaveLength(1)
		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBeUndefined()
	})

	it("removes the mark from every text node it spans", ({ expect }) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph(null, commented("first", "c1")),
				paragraph(null, commented("second", "c1")),
			),
		)

		deleteCommentFromEditor(editor, "c1", true)

		expect(commentShape(state().doc)).toEqual([
			["first", null],
			["second", null],
		])
	})

	it("clears the node attribute of the given node comment", ({ expect }) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph({ nodeCommentId: "c1" }, text("target")),
				paragraph({ nodeCommentId: "c2" }, text("other")),
			),
		)

		deleteCommentFromEditor(editor, "c1", false)

		expect(nodeCommentShape(state().doc)).toEqual([
			["target", null],
			["other", "c2"],
		])
	})

	it("does not dispatch when the comment is not in the document", ({
		expect,
	}) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph(null, commented("one", "c1"))),
		)

		deleteCommentFromEditor(editor, "missing", true)

		expect(dispatched).toHaveLength(0)
	})

	it("marks the transaction as a diff comment transaction in the diff editor", ({
		expect,
	}) => {
		const { editor, dispatched, state } = makeEditor(
			doc(paragraph(null, commented("one", "c1"))),
		)

		deleteCommentFromEditor(editor, "c1", true, true)

		expect(commentShape(state().doc)).toEqual([["one", null]])
		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBe(true)
	})
})

import { EditorState } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import {
	deletePendingNodeComments,
	findNodeCommentAtPos,
	findNodeCommentById,
	NodeComment,
	reinjectDeletedContentNodeComments,
	type NodeCommentStorage,
} from "./node-comment-extension"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"
import { DiffStatus } from "../diff/position-map"
import {
	doc,
	makeComment,
	makeEditor,
	nodeCommentShape,
	paragraph,
	runCommand,
	text,
	wrapper,
} from "./test-helpers"

// the tested commands only read this.storage, so a minimal bound
// context stands in for the editor-managed extension instance
function nodeCommands() {
	const addCommands = NodeComment.config.addCommands

	if (!addCommands) {
		throw new Error("NodeComment declares no commands")
	}

	const storage: NodeCommentStorage = {
		forcedHighlights: new Set<string>(),
		updateOverlays: vi.fn(),
	}

	const commands = addCommands.call({ storage } as never)

	return { commands, storage }
}

describe("NodeComment", () => {
	describe("addNodeComment", () => {
		it("sets the node comment id on the node at the position", ({ expect }) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(
				commands.addNodeComment?.(0, { nodeCommentId: "c1" }),
				state,
			)

			expect(res.result).toBe(true)
			expect(nodeCommentShape(res.state.doc)).toEqual([["hello", "c1"]])
		})

		it("returns false when no node exists at the position", ({ expect }) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(
				commands.addNodeComment?.(state.doc.content.size, {
					nodeCommentId: "c1",
				}),
				state,
			)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})

		it("reports success without changing the transaction when not dispatching", ({
			expect,
		}) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(
				commands.addNodeComment?.(0, { nodeCommentId: "c1" }),
				state,
				{ dispatch: false },
			)

			expect(res.result).toBe(true)
			expect(res.tr.steps).toHaveLength(0)
		})

		it("reports failure when not dispatching and no node exists at the position", ({
			expect,
		}) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(
				commands.addNodeComment?.(state.doc.content.size, {
					nodeCommentId: "c1",
				}),
				state,
				{ dispatch: false },
			)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})
	})

	describe("removeNodeComment", () => {
		it("clears the node comment id at the position", ({ expect }) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph({ nodeCommentId: "c1" }, text("hello"))),
			})

			const res = runCommand(commands.removeNodeComment?.(0), state)

			expect(res.result).toBe(true)
			expect(nodeCommentShape(res.state.doc)).toEqual([["hello", null]])
		})

		it("returns false when no node exists at the position", ({ expect }) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph({ nodeCommentId: "c1" }, text("hello"))),
			})

			const res = runCommand(
				commands.removeNodeComment?.(state.doc.content.size),
				state,
			)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})

		it("reports success without changing the transaction when not dispatching", ({
			expect,
		}) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph({ nodeCommentId: "c1" }, text("hello"))),
			})

			const res = runCommand(commands.removeNodeComment?.(0), state, {
				dispatch: false,
			})

			expect(res.result).toBe(true)
			expect(res.tr.steps).toHaveLength(0)
		})
	})

	describe("updateNodeCommentId", () => {
		it("moves the id to the new value on the first matching node", ({
			expect,
		}) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(
					paragraph({ nodeCommentId: "old" }, text("first")),
					paragraph({ nodeCommentId: "old" }, text("second")),
				),
			})

			const res = runCommand(
				commands.updateNodeCommentId?.("old", "new"),
				state,
			)

			expect(res.result).toBe(true)
			expect(nodeCommentShape(res.state.doc)).toEqual([
				["first", "new"],
				["second", "old"],
			])
		})

		it("returns false when no node carries the old id", ({ expect }) => {
			const { commands } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("first"))),
			})

			const res = runCommand(
				commands.updateNodeCommentId?.("old", "new"),
				state,
			)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})
	})

	describe("hasNodeComment", () => {
		// positions: the saved paragraph opens at 0, the pending one at
		// 7, and the uncommented one at 16
		const state = EditorState.create({
			doc: doc(
				paragraph({ nodeCommentId: "c1" }, text("saved")),
				paragraph({ nodeCommentId: "pending-u1-1" }, text("pending")),
				paragraph(null, text("none")),
			),
		})

		it.for([
			{ name: "accepts a node with a saved comment", pos: 0, expected: true },
			{
				name: "rejects a node with a pending comment",
				pos: 7,
				expected: false,
			},
			{ name: "rejects a node without a comment", pos: 16, expected: false },
			{
				name: "rejects a position without a node",
				pos: state.doc.content.size,
				expected: false,
			},
		])("$name", ({ pos, expected }, { expect }) => {
			const { commands } = nodeCommands()

			const res = runCommand(commands.hasNodeComment?.(pos), state)

			expect(res.result).toBe(expected)
		})
	})

	describe("setNodeCommentForcedHighlight", () => {
		it("adds the id to the forced highlights and refreshes", ({ expect }) => {
			const { commands, storage } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(
				commands.setNodeCommentForcedHighlight?.("c1", true),
				state,
			)

			expect(res.result).toBe(true)
			expect(storage.forcedHighlights).toEqual(new Set(["c1"]))
			expect(storage.updateOverlays).toHaveBeenCalledTimes(1)
		})

		it("removes the id when active is false", ({ expect }) => {
			const { commands, storage } = nodeCommands()
			storage.forcedHighlights.add("c1")
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(
				commands.setNodeCommentForcedHighlight?.("c1", false),
				state,
			)

			expect(res.result).toBe(true)
			expect(storage.forcedHighlights).toEqual(new Set())
			expect(storage.updateOverlays).toHaveBeenCalledTimes(1)
		})
	})

	describe("refreshNodeCommentOverlays", () => {
		it("invokes the stored overlay updater", ({ expect }) => {
			const { commands, storage } = nodeCommands()
			const state = EditorState.create({
				doc: doc(paragraph(null, text("hello"))),
			})

			const res = runCommand(commands.refreshNodeCommentOverlays?.(), state)

			expect(res.result).toBe(true)
			expect(storage.updateOverlays).toHaveBeenCalledTimes(1)
		})
	})
})

describe("deletePendingNodeComments", () => {
	it("clears only the active user's pending node comments", ({ expect }) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph({ nodeCommentId: "pending-u1-1" }, text("mine")),
				paragraph({ nodeCommentId: "pending-u2-1" }, text("theirs")),
				paragraph({ nodeCommentId: "c1" }, text("saved")),
			),
		)
		const onDelete = vi.fn()

		deletePendingNodeComments(editor, "u1", onDelete)

		expect(nodeCommentShape(state().doc)).toEqual([
			["mine", null],
			["theirs", "pending-u2-1"],
			["saved", "c1"],
		])
		expect(dispatched).toHaveLength(1)
		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBeUndefined()
		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-1")
	})

	it("keeps the excluded pending comment", ({ expect }) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph({ nodeCommentId: "pending-u1-1" }, text("kept")),
				paragraph({ nodeCommentId: "pending-u1-2" }, text("gone")),
			),
		)
		const onDelete = vi.fn()

		deletePendingNodeComments(editor, "u1", onDelete, false, "pending-u1-1")

		expect(nodeCommentShape(state().doc)).toEqual([
			["kept", "pending-u1-1"],
			["gone", null],
		])
		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-2")
	})

	it("does not dispatch when nothing is pending", ({ expect }) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph({ nodeCommentId: "c1" }, text("saved"))),
		)
		const onDelete = vi.fn()

		deletePendingNodeComments(editor, "u1", onDelete)

		expect(dispatched).toHaveLength(0)
		expect(onDelete).toHaveBeenCalledTimes(0)
	})

	it("marks the transaction as a diff comment transaction in the diff editor", ({
		expect,
	}) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph({ nodeCommentId: "pending-u1-1" }, text("mine"))),
		)

		deletePendingNodeComments(editor, "u1", undefined, true)

		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBe(true)
	})
})

describe("findNodeCommentAtPos", () => {
	it("returns the node and id at the position", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(paragraph({ nodeCommentId: "c1" }, text("hi"))),
		})

		const match = findNodeCommentAtPos(state, 0)

		expect(match?.pos).toBe(0)
		expect(match?.nodeCommentId).toBe("c1")
		expect(match?.node.textContent).toBe("hi")
	})

	it("returns null when the node has no comment", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(paragraph(null, text("hi"))),
		})

		expect(findNodeCommentAtPos(state, 0)).toBeNull()
	})

	it("returns null when no node exists at the position", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(paragraph({ nodeCommentId: "c1" }, text("hi"))),
		})

		expect(findNodeCommentAtPos(state, state.doc.content.size)).toBeNull()
	})
})

describe("findNodeCommentById", () => {
	it("returns the position of the matching node", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(
				paragraph(null, text("one")),
				paragraph({ nodeCommentId: "c2" }, text("two")),
			),
		})

		const match = findNodeCommentById(state, "c2")

		expect(match?.pos).toBe(5)
		expect(match?.nodeCommentId).toBe("c2")
		expect(match?.node.textContent).toBe("two")
	})

	it("returns the first of several matching nodes", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(
				paragraph({ nodeCommentId: "c2" }, text("first")),
				paragraph({ nodeCommentId: "c2" }, text("second")),
			),
		})

		expect(findNodeCommentById(state, "c2")?.pos).toBe(0)
	})

	it("returns null when no node matches", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(paragraph({ nodeCommentId: "c1" }, text("one"))),
		})

		expect(findNodeCommentById(state, "missing")).toBeNull()
	})
})

describe("reinjectDeletedContentNodeComments", () => {
	it("sets the comment id on the removed node matched by anchorBlockId", ({
		expect,
	}) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph(null, text("intro")),
				paragraph({ uid: "u1", diffStatus: DiffStatus.Removed }, text("gone")),
			),
		)
		const comment = makeComment({
			id: "c1",
			anchorBlockId: "u1",
			diffDeletionContext: {},
		})

		reinjectDeletedContentNodeComments(editor, [], [comment])

		expect(nodeCommentShape(state().doc)).toEqual([
			["intro", null],
			["gone", "c1"],
		])
		expect(dispatched).toHaveLength(1)
		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBe(true)
	})

	it("targets a transparent wrapper whose children are all removed", ({
		expect,
	}) => {
		const { editor, state } = makeEditor(
			doc(
				wrapper(
					{ uid: "w1" },
					paragraph({ diffStatus: DiffStatus.Removed }, text("a")),
					paragraph({ diffStatus: DiffStatus.Removed }, text("b")),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			anchorBlockId: "w1",
			diffDeletionContext: {},
		})

		reinjectDeletedContentNodeComments(editor, [], [comment])

		expect(nodeCommentShape(state().doc)).toEqual([
			["ab", "c1"],
			["a", null],
			["b", null],
		])
	})

	it("skips a wrapper containing a non-removed child", ({ expect }) => {
		const { editor, dispatched } = makeEditor(
			doc(
				wrapper(
					{ uid: "w1" },
					paragraph({ diffStatus: DiffStatus.Removed }, text("a")),
					paragraph(null, text("b")),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			anchorBlockId: "w1",
			diffDeletionContext: {},
		})

		reinjectDeletedContentNodeComments(editor, [], [comment])

		expect(dispatched).toHaveLength(0)
	})

	it("skips comments with text anchors or without deletion context", ({
		expect,
	}) => {
		const { editor, dispatched } = makeEditor(
			doc(
				paragraph({ uid: "u1", diffStatus: DiffStatus.Removed }, text("gone")),
			),
		)
		const comments = [
			makeComment({ id: "c1", anchorBlockId: "u1" }),
			makeComment({
				id: "c2",
				anchorBlockId: "u1",
				diffDeletionContext: {
					textAnchors: [{ nodeUid: "u1", fromOffset: 1, toOffset: 2 }],
				},
			}),
		]

		reinjectDeletedContentNodeComments(editor, [], comments)

		expect(dispatched).toHaveLength(0)
	})

	it("keeps an existing node comment id", ({ expect }) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph(
					{
						uid: "u1",
						diffStatus: DiffStatus.Removed,
						nodeCommentId: "existing",
					},
					text("gone"),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			anchorBlockId: "u1",
			diffDeletionContext: {},
		})

		reinjectDeletedContentNodeComments(editor, [], [comment])

		expect(nodeCommentShape(state().doc)).toEqual([["gone", "existing"]])
		expect(dispatched).toHaveLength(0)
	})

	it("does nothing when no removed nodes exist", ({ expect }) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph({ uid: "u1" }, text("kept"))),
		)
		const comment = makeComment({
			id: "c1",
			anchorBlockId: "u1",
			diffDeletionContext: {},
		})

		reinjectDeletedContentNodeComments(editor, [], [comment])

		expect(dispatched).toHaveLength(0)
	})
})

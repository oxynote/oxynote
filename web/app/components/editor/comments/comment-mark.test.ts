import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import {
	CommentMark,
	deletePendingCommentMarks,
	findCommentMarkAtPos,
	findCommentMarkById,
	mergedOffsetToOriginalOffset,
	reinjectDeletedContentMarks,
	type CommentMarkStorage,
} from "./comment-mark"
import { COMMENT_MARK_NAME } from "../mark-names"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"
import { DiffStatus } from "../diff/position-map"
import {
	added,
	commented,
	commentedAdded,
	commentShape,
	doc,
	hardBreak,
	makeComment,
	makeEditor,
	paragraph,
	runCommand,
	text,
} from "./test-helpers"

// the tested commands only read this.name and this.storage, so a
// minimal bound context stands in for the editor-managed mark instance
function markCommands() {
	const addCommands = CommentMark.config.addCommands

	if (!addCommands) {
		throw new Error("CommentMark declares no commands")
	}

	const storage: CommentMarkStorage = {
		forcedHighlights: new Set<string>(),
		updateForcedHighlightStyle: vi.fn(),
		refreshIndicators: vi.fn(),
	}

	const commands = addCommands.call({
		name: COMMENT_MARK_NAME,
		storage,
	} as never)

	return { commands, storage }
}

// positions: paragraph opens at 0, "plain " spans [1, 7), the marked
// "inside" spans [7, 13), and " tail" spans [13, 18)
function markedState() {
	return EditorState.create({
		doc: doc(
			paragraph(null, text("plain "), commented("inside", "c1"), text(" tail")),
		),
	})
}

// addCommentMark applies the mark through a command chain that the
// command manager builds; the stub records the setMark call and
// reports success the way the real chain does
function chainStub() {
	const calls: { name: string; attrs: unknown }[] = []

	function chain() {
		return {
			setMark: (name: string, attrs: unknown) => {
				calls.push({ name, attrs })

				return { run: () => true }
			},
		}
	}

	return { chain, calls }
}

describe("CommentMark", () => {
	describe("addCommentMark", () => {
		it("marks the selection through the command chain", ({ expect }) => {
			const { commands } = markCommands()
			const base = markedState()
			const state = EditorState.create({
				doc: base.doc,
				selection: TextSelection.create(base.doc, 1, 7),
			})
			const { chain, calls } = chainStub()
			vi.stubGlobal("requestAnimationFrame", vi.fn())

			const res = runCommand(
				commands.addCommentMark?.({ commentId: "c2" }),
				state,
				{ chain },
			)

			expect(res.result).toBe(true)
			expect(calls).toEqual([
				{ name: COMMENT_MARK_NAME, attrs: { commentId: "c2" } },
			])
		})

		it("clears the browser selection once the mark is applied", ({
			expect,
		}) => {
			const { commands } = markCommands()
			const base = markedState()
			const state = EditorState.create({
				doc: base.doc,
				selection: TextSelection.create(base.doc, 1, 7),
			})
			const { chain } = chainStub()
			const raf = vi.fn()
			vi.stubGlobal("requestAnimationFrame", raf)

			runCommand(commands.addCommentMark?.({ commentId: "c2" }), state, {
				chain,
			})

			expect(raf).toHaveBeenCalledTimes(1)
		})

		it("leaves the browser selection alone when probing without dispatch", ({
			expect,
		}) => {
			const { commands } = markCommands()
			const base = markedState()
			const state = EditorState.create({
				doc: base.doc,
				selection: TextSelection.create(base.doc, 1, 7),
			})
			const { chain, calls } = chainStub()
			const raf = vi.fn()
			vi.stubGlobal("requestAnimationFrame", raf)

			const res = runCommand(
				commands.addCommentMark?.({ commentId: "c2" }),
				state,
				{ chain, dispatch: false },
			)

			expect(res.result).toBe(true)
			expect(calls).toHaveLength(1)
			expect(raf).toHaveBeenCalledTimes(0)
		})

		it("returns false for a collapsed selection", ({ expect }) => {
			const { commands } = markCommands()
			const base = markedState()
			const state = EditorState.create({
				doc: base.doc,
				selection: TextSelection.create(base.doc, 9),
			})
			const { chain, calls } = chainStub()
			const raf = vi.fn()
			vi.stubGlobal("requestAnimationFrame", raf)

			const res = runCommand(
				commands.addCommentMark?.({ commentId: "c2" }),
				state,
				{ chain },
			)

			expect(res.result).toBe(false)
			expect(calls).toHaveLength(0)
			expect(raf).toHaveBeenCalledTimes(0)
		})
	})

	describe("updateCommentMarkId", () => {
		it("rewrites the mark range to the new id", ({ expect }) => {
			const { commands } = markCommands()

			const res = runCommand(
				commands.updateCommentMarkId?.("c1", "c2"),
				markedState(),
			)

			expect(res.result).toBe(true)
			expect(commentShape(res.state.doc)).toEqual([
				["plain ", null],
				["inside", "c2"],
				[" tail", null],
			])
		})

		it("returns false when no mark carries the old id", ({ expect }) => {
			const { commands } = markCommands()

			const res = runCommand(
				commands.updateCommentMarkId?.("missing", "c2"),
				markedState(),
			)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})
	})

	describe("removeCommentMark", () => {
		it("removes the mark across the selection when no id is given", ({
			expect,
		}) => {
			const { commands } = markCommands()
			const base = markedState()
			const state = EditorState.create({
				doc: base.doc,
				selection: TextSelection.create(base.doc, 7, 13),
			})

			const res = runCommand(commands.removeCommentMark?.(), state)

			expect(res.result).toBe(true)
			expect(commentShape(res.state.doc)).toEqual([["plain inside tail", null]])
		})

		it("returns false for a collapsed selection when no id is given", ({
			expect,
		}) => {
			const { commands } = markCommands()
			const base = markedState()
			const state = EditorState.create({
				doc: base.doc,
				selection: TextSelection.create(base.doc, 9),
			})

			const res = runCommand(commands.removeCommentMark?.(), state)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})

		it("removes every range carrying the given id", ({ expect }) => {
			const { commands } = markCommands()
			const state = EditorState.create({
				doc: doc(
					paragraph(null, commented("first", "c1")),
					paragraph(null, commented("second", "c1"), commented("kept", "c2")),
				),
			})

			const res = runCommand(commands.removeCommentMark?.("c1"), state)

			expect(res.result).toBe(true)
			expect(commentShape(res.state.doc)).toEqual([
				["first", null],
				["second", null],
				["kept", "c2"],
			])
		})

		it("returns false when the id matches no mark", ({ expect }) => {
			const { commands } = markCommands()

			const res = runCommand(
				commands.removeCommentMark?.("missing"),
				markedState(),
			)

			expect(res.result).toBe(false)
			expect(commentShape(res.state.doc)).toEqual([
				["plain ", null],
				["inside", "c1"],
				[" tail", null],
			])
		})
	})

	describe("setCommentMarkForcedHighlight", () => {
		it("adds the id to the forced highlights and refreshes", ({ expect }) => {
			const { commands, storage } = markCommands()

			const res = runCommand(
				commands.setCommentMarkForcedHighlight?.("c1", true),
				markedState(),
			)

			expect(res.result).toBe(true)
			expect(storage.forcedHighlights).toEqual(new Set(["c1"]))
			expect(storage.updateForcedHighlightStyle).toHaveBeenCalledTimes(1)
			expect(storage.refreshIndicators).toHaveBeenCalledTimes(1)
		})

		it("removes the id when active is false", ({ expect }) => {
			const { commands, storage } = markCommands()
			storage.forcedHighlights.add("c1")

			const res = runCommand(
				commands.setCommentMarkForcedHighlight?.("c1", false),
				markedState(),
			)

			expect(res.result).toBe(true)
			expect(storage.forcedHighlights).toEqual(new Set())
			expect(storage.updateForcedHighlightStyle).toHaveBeenCalledTimes(1)
			expect(storage.refreshIndicators).toHaveBeenCalledTimes(1)
		})
	})

	describe("refreshTextCommentIndicators", () => {
		it("invokes the stored refresh callback", ({ expect }) => {
			const { commands, storage } = markCommands()

			const res = runCommand(
				commands.refreshTextCommentIndicators?.(),
				markedState(),
			)

			expect(res.result).toBe(true)
			expect(storage.refreshIndicators).toHaveBeenCalledTimes(1)
		})

		it("returns true when no refresh callback is registered", ({ expect }) => {
			const { commands, storage } = markCommands()
			storage.refreshIndicators = null

			const res = runCommand(
				commands.refreshTextCommentIndicators?.(),
				markedState(),
			)

			expect(res.result).toBe(true)
		})
	})
})

describe("findCommentMarkAtPos", () => {
	it("returns the full range of the mark at the position", ({ expect }) => {
		expect(findCommentMarkAtPos(markedState(), 9)).toEqual({
			from: 7,
			to: 13,
			attrs: { commentId: "c1" },
		})
	})

	// the mark is non-inclusive, so the resolved position's own marks
	// miss it at either boundary and the range fallback resolves it
	it.for([
		{ name: "resolves the mark at its start boundary", pos: 7 },
		{ name: "resolves the mark at its end boundary", pos: 13 },
	])("$name", ({ pos }, { expect }) => {
		expect(findCommentMarkAtPos(markedState(), pos)).toEqual({
			from: 7,
			to: 13,
			attrs: { commentId: "c1" },
		})
	})

	it("returns null on unmarked text", ({ expect }) => {
		expect(findCommentMarkAtPos(markedState(), 2)).toBeNull()
	})

	it.for([
		{ name: "returns null for a negative position", pos: -1 },
		{ name: "returns null for a position past the document", pos: 100 },
	])("$name", ({ pos }, { expect }) => {
		expect(findCommentMarkAtPos(markedState(), pos)).toBeNull()
	})
})

describe("findCommentMarkById", () => {
	it("returns the range spanning adjacent nodes with the same id", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: doc(
				paragraph(
					null,
					commented("ab", "c1"),
					commented("cd", "c1"),
					text("ef"),
				),
			),
		})

		expect(findCommentMarkById(state, "c1")).toEqual({
			from: 1,
			to: 5,
			attrs: { commentId: "c1" },
		})
	})

	it("finds a mark in a later paragraph", ({ expect }) => {
		const state = EditorState.create({
			doc: doc(
				paragraph(null, text("one")),
				paragraph(null, commented("two", "c9")),
			),
		})

		expect(findCommentMarkById(state, "c9")).toEqual({
			from: 6,
			to: 9,
			attrs: { commentId: "c9" },
		})
	})

	it("returns null for an unknown id", ({ expect }) => {
		expect(findCommentMarkById(markedState(), "missing")).toBeNull()
	})
})

describe("deletePendingCommentMarks", () => {
	it("removes only the active user's pending marks", ({ expect }) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph(
					null,
					commented("mine", "pending-u1-1"),
					commented("theirs", "pending-u2-1"),
					commented("saved", "c1"),
				),
			),
		)
		const onDelete = vi.fn()

		deletePendingCommentMarks(editor, "u1", onDelete)

		expect(commentShape(state().doc)).toEqual([
			["mine", null],
			["theirs", "pending-u2-1"],
			["saved", "c1"],
		])
		expect(dispatched).toHaveLength(1)
		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBeUndefined()
		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-1")
	})

	it("reports a mark split across text nodes once", ({ expect }) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph(
					null,
					commented("split ", "pending-u1-1"),
					commentedAdded("across ", "pending-u1-1"),
					commented("nodes", "pending-u1-1"),
				),
			),
		)
		const onDelete = vi.fn()

		deletePendingCommentMarks(editor, "u1", onDelete)

		expect(commentShape(state().doc)).toEqual([
			["split ", null],
			["across ", null],
			["nodes", null],
		])
		expect(dispatched).toHaveLength(1)
		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-1")
	})

	it("keeps the excluded pending mark", ({ expect }) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph(
					null,
					commented("kept", "pending-u1-1"),
					commented("gone", "pending-u1-2"),
				),
			),
		)
		const onDelete = vi.fn()

		deletePendingCommentMarks(editor, "u1", onDelete, false, "pending-u1-1")

		expect(commentShape(state().doc)).toEqual([
			["kept", "pending-u1-1"],
			["gone", null],
		])
		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledWith("pending-u1-2")
	})

	it("does not dispatch when there is nothing to remove", ({ expect }) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph(null, commented("saved", "c1"))),
		)
		const onDelete = vi.fn()

		deletePendingCommentMarks(editor, "u1", onDelete)

		expect(dispatched).toHaveLength(0)
		expect(onDelete).toHaveBeenCalledTimes(0)
	})

	it("marks the transaction as a diff comment transaction in the diff editor", ({
		expect,
	}) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph(null, commented("mine", "pending-u1-1"))),
		)

		deletePendingCommentMarks(editor, "u1", undefined, true)

		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBe(true)
	})
})

describe("mergedOffsetToOriginalOffset", () => {
	// merged text "abcXYde" where "XY" is added: the original text is
	// "abcde", so offsets inside or right after the added run collapse
	// to the position after "abc"
	const node = paragraph(null, text("abc"), added("XY"), text("de"))

	it.for([
		[1, 1],
		[2, 2],
		[4, 4],
		[5, 4],
		[6, 4],
		[7, 5],
		[8, 6],
	])(
		"maps merged offset %i to original offset %i",
		([merged, expected], { expect }) => {
			expect(mergedOffsetToOriginalOffset(node, merged ?? 0)).toBe(expected)
		},
	)

	it("counts a non-text child in both coordinate systems", ({ expect }) => {
		const withBreak = paragraph(
			null,
			text("ab"),
			hardBreak(),
			added("XY"),
			text("cd"),
		)

		expect(mergedOffsetToOriginalOffset(withBreak, 6)).toBe(4)
	})
})

describe("reinjectDeletedContentMarks", () => {
	it("re-applies a mark inside a removed node from its text anchor", ({
		expect,
	}) => {
		const { editor, dispatched, state } = makeEditor(
			doc(
				paragraph(null, text("intro")),
				paragraph(
					{ uid: "u1", diffStatus: DiffStatus.Removed },
					text("deleted text"),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			diffDeletionContext: {
				textAnchors: [{ nodeUid: "u1", fromOffset: 1, toOffset: 8 }],
			},
		})

		reinjectDeletedContentMarks(editor, [], [comment])

		expect(commentShape(state().doc)).toEqual([
			["intro", null],
			["deleted", "c1"],
			[" text", null],
		])
		expect(dispatched).toHaveLength(1)
		expect(dispatched[0]?.getMeta(DIFF_COMMENT_TX_META)).toBe(true)
	})

	it("maps anchor offsets past added text inside a modified node", ({
		expect,
	}) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph(
					{ uid: "m1", diffStatus: DiffStatus.Modified },
					text("abc"),
					added("XY"),
					text("de"),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			diffDeletionContext: {
				textAnchors: [{ nodeUid: "m1", fromOffset: 1, toOffset: 4 }],
			},
		})

		reinjectDeletedContentMarks(editor, [], [comment])

		expect(commentShape(state().doc)).toEqual([
			["abc", "c1"],
			["XY", null],
			["de", null],
		])
	})

	it("extends a snapping end boundary to the neighboring existing mark", ({
		expect,
	}) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph(
					{ uid: "m2", diffStatus: DiffStatus.Modified },
					text("ab"),
					added("XY"),
					commented("cd", "c1"),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			diffDeletionContext: {
				textAnchors: [
					{ nodeUid: "m2", fromOffset: 1, toOffset: 3, snapTo: true },
				],
			},
		})

		reinjectDeletedContentMarks(editor, [], [comment])

		expect(commentShape(state().doc)).toEqual([
			["ab", "c1"],
			["XY", "c1"],
			["cd", "c1"],
		])
	})

	it("extends a snapping start boundary to the neighboring existing mark", ({
		expect,
	}) => {
		const { editor, state } = makeEditor(
			doc(
				paragraph(
					{ uid: "m3", diffStatus: DiffStatus.Modified },
					commented("ab", "c1"),
					added("XY"),
					text("cd"),
				),
			),
		)
		const comment = makeComment({
			id: "c1",
			diffDeletionContext: {
				textAnchors: [
					{ nodeUid: "m3", fromOffset: 3, toOffset: 5, snapFrom: true },
				],
			},
		})

		reinjectDeletedContentMarks(editor, [], [comment])

		expect(commentShape(state().doc)).toEqual([
			["ab", "c1"],
			["XY", "c1"],
			["cd", "c1"],
		])
	})

	it("skips comments without anchors and anchors that cannot resolve", ({
		expect,
	}) => {
		const { editor, dispatched } = makeEditor(
			doc(
				paragraph({ uid: "u1", diffStatus: DiffStatus.Removed }, text("gone")),
			),
		)
		const comments = [
			makeComment({ id: "c1" }),
			makeComment({
				id: "c2",
				diffDeletionContext: {
					textAnchors: [{ nodeUid: "unknown", fromOffset: 1, toOffset: 2 }],
				},
			}),
			makeComment({
				id: "c3",
				diffDeletionContext: {
					textAnchors: [
						{ nodeUid: "u1", fromOffset: 3, toOffset: 3 },
						{ nodeUid: "u1", fromOffset: 1, toOffset: 50 },
					],
				},
			}),
		]

		reinjectDeletedContentMarks(editor, [], comments)

		expect(dispatched).toHaveLength(0)
	})

	it("ignores unchanged nodes even when the uid matches", ({ expect }) => {
		const { editor, dispatched } = makeEditor(
			doc(paragraph({ uid: "u1" }, text("kept"))),
		)
		const comment = makeComment({
			id: "c1",
			diffDeletionContext: {
				textAnchors: [{ nodeUid: "u1", fromOffset: 1, toOffset: 3 }],
			},
		})

		reinjectDeletedContentMarks(editor, [], [comment])

		expect(dispatched).toHaveLength(0)
	})
})

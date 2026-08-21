import type { JSONContent } from "@tiptap/core"
import { Editor } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node } from "@tiptap/pm/model"
import { Fragment, Slice } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { describe, it, vi } from "vitest"
import { findPluginByKey } from "../test-helpers"
import type { UniqueIDOptions } from "./extension"
import { UniqueID } from "./extension"

function paragraph(text = "", id: string | null = null): JSONContent {
	return {
		type: "paragraph",
		attrs: { id },
		content: text === "" ? [] : [{ type: "text", text }],
	}
}

function heading(text: string, id: string | null = null): JSONContent {
	return {
		type: "heading",
		attrs: { id, level: 1 },
		content: [{ type: "text", text }],
	}
}

function blockquote(...content: JSONContent[]): JSONContent {
	return { type: "blockquote", content }
}

interface Harness {
	plugin: Plugin
	state: EditorState
	nodeFromJSON: (json: JSONContent) => Node
}

// builds a state whose only plugin is the uniqueID plugin, with a
// deterministic id generator, so applying a transaction runs its
// appendTransaction hook exactly like in a live editor
function harness(
	content: JSONContent[],
	options: Partial<UniqueIDOptions> = {},
): Harness {
	let counter = 0

	const editor = new Editor({
		extensions: [
			Document,
			Paragraph,
			Text,
			Heading,
			Blockquote,
			HorizontalRule,
			UniqueID.configure({
				types: ["paragraph", "heading"],
				generateID: () => `gen-${++counter}`,
				...options,
			}),
		],
		// json content, because the default empty-string content would
		// send the headless editor down the window-dependent html path
		content: { type: "doc", content },
	})

	const plugin = findPluginByKey(editor, "uniqueID")

	return {
		plugin,
		state: EditorState.create({ doc: editor.state.doc, plugins: [plugin] }),
		nodeFromJSON: (json) => editor.schema.nodeFromJSON(json),
	}
}

// collects the id attribute of every top-level block
function ids(doc: Node): (string | null | undefined)[] {
	const result: (string | null | undefined)[] = []

	doc.forEach((node) => {
		result.push(node.attrs.id as string | null | undefined)
	})

	return result
}

// the paste and drop handlers under test ignore their view/event
// arguments or only read a couple of properties, so loose stand-ins
// are enough
function fakeView(parentElement: object | null = null): EditorView {
	return { dom: { parentElement } } as unknown as EditorView
}

function dropEvent(effectAllowed: string): DragEvent {
	return { dataTransfer: { effectAllowed } } as unknown as DragEvent
}

function pasteProps(plugin: Plugin) {
	const { transformPasted, handleDOMEvents } = plugin.props
	const paste = handleDOMEvents?.paste
	const drop = handleDOMEvents?.drop

	if (!transformPasted || !paste || !drop) {
		throw new Error("uniqueID plugin props missing")
	}

	return {
		transform: (slice: Slice) =>
			transformPasted.call(plugin, slice, fakeView(), false),
		paste: () =>
			paste.call(plugin, fakeView(), new Event("paste") as ClipboardEvent),
		drop: (view: EditorView, event: DragEvent) =>
			drop.call(plugin, view, event),
	}
}

describe("UniqueID", () => {
	it("assigns a generated id to an inserted node of a configured type", ({
		expect,
	}) => {
		const { state, nodeFromJSON } = harness([paragraph("a", "p1")])

		const { state: next } = state.applyTransaction(
			state.tr.insert(state.doc.content.size, nodeFromJSON(paragraph("b"))),
		)

		expect(ids(next.doc)).toEqual(["p1", "gen-1"])
	})

	it("leaves inserted nodes of other types without ids", ({ expect }) => {
		const { state, nodeFromJSON } = harness([paragraph("a", "p1")])

		const { state: next, transactions } = state.applyTransaction(
			state.tr.insert(
				state.doc.content.size,
				nodeFromJSON({ type: "horizontalRule" }),
			),
		)

		expect(ids(next.doc)).toEqual(["p1", undefined])
		expect(transactions).toHaveLength(1)
	})

	it("keeps the existing id of an inserted node", ({ expect }) => {
		const { state, nodeFromJSON } = harness([paragraph("a", "p1")])

		const { state: next, transactions } = state.applyTransaction(
			state.tr.insert(
				state.doc.content.size,
				nodeFromJSON(paragraph("b", "keep")),
			),
		)

		expect(ids(next.doc)).toEqual(["p1", "keep"])
		expect(transactions).toHaveLength(1)
	})

	it("skips transactions applied by the collaboration plugin", ({ expect }) => {
		const { state, nodeFromJSON } = harness([paragraph("a", "p1")])

		const { state: next } = state.applyTransaction(
			state.tr
				.insert(state.doc.content.size, nodeFromJSON(paragraph("b")))
				.setMeta("y-sync$", true),
		)

		expect(ids(next.doc)).toEqual(["p1", null])
	})

	it("skips transactions rejected by filterTransaction", ({ expect }) => {
		const { state, nodeFromJSON } = harness([paragraph("a", "p1")], {
			filterTransaction: (tr) => !tr.getMeta("external"),
		})

		const { state: next } = state.applyTransaction(
			state.tr
				.insert(state.doc.content.size, nodeFromJSON(paragraph("b")))
				.setMeta("external", true),
		)

		expect(ids(next.doc)).toEqual(["p1", null])
	})

	it("ignores transactions that do not change the document", ({ expect }) => {
		const { state } = harness([paragraph("a")])

		const { state: next, transactions } = state.applyTransaction(
			state.tr.setSelection(TextSelection.create(state.doc, 1)),
		)

		expect(ids(next.doc)).toEqual([null])
		expect(transactions).toHaveLength(1)
	})

	it("moves the id to the content half when a paragraph is split at its start", ({
		expect,
	}) => {
		const { state } = harness([paragraph("hello", "abc")])

		const { state: next } = state.applyTransaction(state.tr.split(1))

		expect(ids(next.doc)).toEqual(["gen-1", "abc"])
		expect(next.doc.child(0).textContent).toBe("")
		expect(next.doc.child(1).textContent).toBe("hello")
	})

	it("does not swap ids when the following node already owns a different id", ({
		expect,
	}) => {
		const { state, nodeFromJSON } = harness([paragraph("x", "a")])

		const { state: next, transactions } = state.applyTransaction(
			state.tr.replaceWith(0, state.doc.content.size, [
				nodeFromJSON(paragraph("", "a")),
				nodeFromJSON(paragraph("y", "b")),
			]),
		)

		expect(ids(next.doc)).toEqual(["a", "b"])
		expect(transactions).toHaveLength(1)
	})

	it("regenerates the id of the trailing half when a non-paragraph node is split", ({
		expect,
	}) => {
		const { state } = harness([heading("Hi", "h1")])

		const { state: next } = state.applyTransaction(state.tr.split(1))

		expect(ids(next.doc)).toEqual(["h1", "gen-1"])
	})

	describe("transformPasted", () => {
		function slice(harnessRef: Harness, ...content: JSONContent[]): Slice {
			return new Slice(
				Fragment.from(content.map((json) => harnessRef.nodeFromJSON(json))),
				0,
				0,
			)
		}

		// collects [typeName, id] pairs of a fragment's nodes, depth first.
		// Nodes outside the configured types have no id attribute at all,
		// so their id reads as undefined
		function sliceIds(
			fragment: Fragment,
		): [string, string | null | undefined][] {
			const result: [string, string | null | undefined][] = []

			fragment.forEach((node) => {
				if (!node.isText) {
					result.push([
						node.type.name,
						node.attrs.id as string | null | undefined,
					])
					result.push(...sliceIds(node.content))
				}
			})

			return result
		}

		it("keeps pasted ids when no paste event was seen", ({ expect }) => {
			const h = harness([paragraph()])
			const { transform } = pasteProps(h.plugin)

			const result = transform(slice(h, paragraph("a", "id1")))

			expect(sliceIds(result.content)).toEqual([["paragraph", "id1"]])
		})

		it("strips ids from pasted nodes of configured types, including nested ones", ({
			expect,
		}) => {
			const h = harness([paragraph()])
			const { transform, paste } = pasteProps(h.plugin)

			paste()
			const result = transform(
				slice(h, paragraph("a", "id1"), blockquote(paragraph("b", "id2"))),
			)

			expect(sliceIds(result.content)).toEqual([
				["paragraph", null],
				["blockquote", undefined],
				["paragraph", null],
			])
			expect(result.content.child(0).textContent).toBe("a")
			expect(result.content.child(1).textContent).toBe("b")
		})

		it("strips ids only for the first transform after a paste", ({
			expect,
		}) => {
			const h = harness([paragraph()])
			const { transform, paste } = pasteProps(h.plugin)

			paste()
			transform(slice(h, paragraph("a", "id1")))
			const second = transform(slice(h, paragraph("a", "id1")))

			expect(sliceIds(second.content)).toEqual([["paragraph", "id1"]])
		})

		it("strips ids from content dropped from another source", ({ expect }) => {
			const h = harness([paragraph()])
			const { transform, drop } = pasteProps(h.plugin)

			drop(fakeView({}), dropEvent("move"))
			const result = transform(slice(h, paragraph("a", "id1")))

			expect(sliceIds(result.content)).toEqual([["paragraph", null]])
		})

		it("keeps ids for a move drag within the same editor", ({ expect }) => {
			const h = harness([paragraph()])
			const { transform, drop } = pasteProps(h.plugin)
			const parentElement = { contains: () => true }
			const view = fakeView(parentElement)
			const addEventListener = vi.fn()
			const removeEventListener = vi.fn()

			vi.stubGlobal("window", { addEventListener, removeEventListener })

			if (!h.plugin.spec.view) {
				throw new Error("uniqueID plugin view missing")
			}

			// mounting the plugin view registers the dragstart listener that
			// records the drag source; fire it as if the drag started inside
			// this editor
			const pluginView = h.plugin.spec.view.call(h.plugin, view)
			const dragstart = addEventListener.mock.calls[0]?.[1] as (
				event: DragEvent,
			) => void
			dragstart({ target: {} } as unknown as DragEvent)

			drop(view, dropEvent("uninitialized"))
			const result = transform(slice(h, paragraph("a", "id1")))

			expect(sliceIds(result.content)).toEqual([["paragraph", "id1"]])

			pluginView.destroy?.()

			expect(removeEventListener).toHaveBeenCalledWith("dragstart", dragstart)
		})
	})
})

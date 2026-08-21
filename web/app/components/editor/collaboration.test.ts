import type { HocuspocusProvider } from "@hocuspocus/provider"
import { Editor } from "@tiptap/core"
import CollaborationCaret from "@tiptap/extension-collaboration-caret"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, type Plugin } from "@tiptap/pm/state"
import { ySyncPluginKey } from "@tiptap/y-tiptap"
import { describe, it, vi } from "vitest"
import {
	extractHocuspocusProviderFromEditor,
	findEditingUsersByNodeUid,
	isEditingNodeInAwarenessOfType,
	isNodeBeingDraggedByOther,
	isNodeOrDescendantsBeingEditedByOther,
	isRangeBeingEditedByOther,
	otherUserDraggingUids,
	otherUserEditingNodes,
	otherUserEditingUids,
	RemoteDeleteSelectionGuard,
	setDraggingNodeInAwareness,
	setEditingNodeInAwareness,
} from "./collaboration"

// a minimal awareness fake: the helpers only read clientID, the state
// maps, and write through setLocalStateField
function makeProvider(
	states: Map<number, Record<string, unknown>>,
	localClientId = 1,
) {
	const setLocalStateField = vi.fn()

	const awareness = {
		clientID: localClientId,
		getStates: () => states,
		getLocalState: () => states.get(localClientId) ?? null,
		setLocalStateField,
	}

	return {
		provider: { awareness } as unknown as HocuspocusProvider,
		setLocalStateField,
	}
}

function makeAwarenessLessProvider() {
	return { awareness: null } as unknown as HocuspocusProvider
}

function editing(uid: string, type = "paragraph", user?: object) {
	return { editingNodeUid: { uid, type }, user }
}

// minimal schema for the document-walking helpers: only the uid
// attribute matters to them
const schema = new Schema({
	nodes: {
		doc: { content: "block*" },
		container: {
			group: "block",
			content: "block*",
			attrs: { uid: { default: null } },
		},
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		text: { group: "inline" },
	},
})

function pmParagraph(uid: string, text: string): PMNode {
	return schema.nodes.paragraph.create({ uid }, schema.text(text))
}

// doc positions: container [0, 8) wrapping paragraph "p1" [1, 7),
// top-level paragraph "p2" [8, 14)
function makeDoc(): PMNode {
	return schema.nodes.doc.create(null, [
		schema.nodes.container.create({ uid: "c1" }, pmParagraph("p1", "text")),
		pmParagraph("p2", "more"),
	])
}

describe("setDraggingNodeInAwareness", () => {
	it("broadcasts the dragged node uid", ({ expect }) => {
		const { provider, setLocalStateField } = makeProvider(new Map())

		setDraggingNodeInAwareness(provider, "n1")

		expect(setLocalStateField).toHaveBeenCalledExactlyOnceWith(
			"draggingNodeUid",
			"n1",
		)
	})

	it("clears the dragging state with null", ({ expect }) => {
		const { provider, setLocalStateField } = makeProvider(new Map())

		setDraggingNodeInAwareness(provider, null)

		expect(setLocalStateField).toHaveBeenCalledExactlyOnceWith(
			"draggingNodeUid",
			null,
		)
	})

	it("does nothing without an awareness instance", ({ expect }) => {
		expect(() => {
			setDraggingNodeInAwareness(makeAwarenessLessProvider(), "n1")
		}).not.toThrow()
	})
})

describe("otherUserDraggingUids", () => {
	it("collects the uids dragged by other users", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[1, { draggingNodeUid: "local-node" }],
				[2, { draggingNodeUid: "n1" }],
				[3, { draggingNodeUid: "n2" }],
				[4, {}],
			]),
		)

		expect(otherUserDraggingUids(provider)).toEqual(new Set(["n1", "n2"]))
	})

	it("returns an empty set for a null provider", ({ expect }) => {
		expect(otherUserDraggingUids(null)).toEqual(new Set())
	})
})

describe("isNodeBeingDraggedByOther", () => {
	it("reports whether another user drags the uid", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[1, { draggingNodeUid: "local-node" }],
				[2, { draggingNodeUid: "n1" }],
			]),
		)

		expect(isNodeBeingDraggedByOther(provider, "n1")).toBe(true)
		expect(isNodeBeingDraggedByOther(provider, "local-node")).toBe(false)
	})
})

describe("isEditingNodeInAwarenessOfType", () => {
	it("matches the type of the locally edited node", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([[1, editing("n1", "mermaidBlock")]]),
		)

		expect(isEditingNodeInAwarenessOfType(provider, "mermaidBlock")).toBe(true)
		expect(isEditingNodeInAwarenessOfType(provider, "paragraph")).toBe(false)
	})

	it("returns false when nothing is edited locally", ({ expect }) => {
		const { provider } = makeProvider(new Map([[1, {}]]))

		expect(isEditingNodeInAwarenessOfType(provider, "paragraph")).toBe(false)
	})

	it("returns false without an awareness instance", ({ expect }) => {
		expect(
			isEditingNodeInAwarenessOfType(makeAwarenessLessProvider(), "paragraph"),
		).toBe(false)
	})
})

describe("setEditingNodeInAwareness", () => {
	it("broadcasts the edited node info", ({ expect }) => {
		const { provider, setLocalStateField } = makeProvider(new Map())

		setEditingNodeInAwareness(provider, { uid: "n1", type: "paragraph" })

		expect(setLocalStateField).toHaveBeenCalledExactlyOnceWith(
			"editingNodeUid",
			{ uid: "n1", type: "paragraph" },
		)
	})

	it("does nothing without an awareness instance", ({ expect }) => {
		expect(() => {
			setEditingNodeInAwareness(makeAwarenessLessProvider(), null)
		}).not.toThrow()
	})
})

describe("otherUserEditingUids", () => {
	it("collects the uids edited by other users", ({ expect }) => {
		const { provider } = makeProvider(
			new Map<number, Record<string, unknown>>([
				[1, editing("local-node")],
				[2, editing("n1")],
				[3, {}],
			]),
		)

		expect(otherUserEditingUids(provider)).toEqual(new Set(["n1"]))
	})

	it("returns an empty set for a null provider", ({ expect }) => {
		expect(otherUserEditingUids(null)).toEqual(new Set())
	})
})

describe("otherUserEditingNodes", () => {
	it("collects the node infos edited by other users", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[1, editing("local-node")],
				[2, editing("n1", "mermaidBlock")],
				[3, editing("n2")],
			]),
		)

		expect(otherUserEditingNodes(provider)).toEqual([
			{ uid: "n1", type: "mermaidBlock" },
			{ uid: "n2", type: "paragraph" },
		])
	})

	it("returns an empty array for a null provider", ({ expect }) => {
		expect(otherUserEditingNodes(null)).toEqual([])
	})
})

describe("findEditingUsersByNodeUid", () => {
	it("lists the remote users editing the node", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[1, editing("n1", "paragraph", { name: "Local", color: "#000" })],
				[2, editing("n1", "paragraph", { name: "Ada", color: "#f00" })],
				[3, editing("n1", "paragraph", { name: "Grace" })],
				[4, editing("n1")],
				[5, editing("n2", "paragraph", { name: "Alan", color: "#00f" })],
			]),
		)

		// the local user, the user without a color, the user without a
		// user field, and the user on another node are all dropped
		expect(findEditingUsersByNodeUid(provider, "n1")).toEqual([
			{ name: "Ada", color: "#f00" },
		])
	})

	it("returns an empty array without an awareness instance", ({ expect }) => {
		expect(
			findEditingUsersByNodeUid(makeAwarenessLessProvider(), "n1"),
		).toEqual([])
	})
})

describe("isNodeOrDescendantsBeingEditedByOther", () => {
	it("detects the node itself being edited", ({ expect }) => {
		const { provider } = makeProvider(new Map([[2, editing("p2")]]))

		expect(isNodeOrDescendantsBeingEditedByOther(provider, makeDoc(), 8)).toBe(
			true,
		)
	})

	it("detects a descendant being edited", ({ expect }) => {
		const { provider } = makeProvider(new Map([[2, editing("p1")]]))

		expect(isNodeOrDescendantsBeingEditedByOther(provider, makeDoc(), 0)).toBe(
			true,
		)
	})

	it("returns false when neither the node nor its descendants are edited", ({
		expect,
	}) => {
		const { provider } = makeProvider(new Map([[2, editing("p2")]]))

		expect(isNodeOrDescendantsBeingEditedByOther(provider, makeDoc(), 0)).toBe(
			false,
		)
	})

	it("returns false when no other user is editing", ({ expect }) => {
		const { provider } = makeProvider(new Map([[1, editing("p1")]]))

		expect(isNodeOrDescendantsBeingEditedByOther(provider, makeDoc(), 0)).toBe(
			false,
		)
	})

	it("returns false when no node sits at the position", ({ expect }) => {
		const { provider } = makeProvider(new Map([[2, editing("p1")]]))
		const doc = makeDoc()

		expect(
			isNodeOrDescendantsBeingEditedByOther(provider, doc, doc.content.size),
		).toBe(false)
	})
})

describe("isRangeBeingEditedByOther", () => {
	it("detects an edited node inside the range", ({ expect }) => {
		const { provider } = makeProvider(new Map([[2, editing("p2")]]))

		expect(isRangeBeingEditedByOther(provider, makeDoc(), 8, 14)).toBe(true)
	})

	it("returns false when the range misses the edited node", ({ expect }) => {
		const { provider } = makeProvider(new Map([[2, editing("p2")]]))

		expect(isRangeBeingEditedByOther(provider, makeDoc(), 0, 8)).toBe(false)
	})

	it("returns false when no other user is editing", ({ expect }) => {
		const { provider } = makeProvider(new Map())

		expect(isRangeBeingEditedByOther(provider, makeDoc(), 0, 14)).toBe(false)
	})
})

describe("extractHocuspocusProviderFromEditor", () => {
	// the function only reads the extension list, so a stub editor
	// avoids booting a real editor around a live provider
	function fakeEditor(extensions: unknown[]): Editor {
		return { extensionManager: { extensions } } as unknown as Editor
	}

	it("returns the provider from the collaboration caret extension", ({
		expect,
	}) => {
		const { provider } = makeProvider(new Map())
		const editor = fakeEditor([CollaborationCaret.configure({ provider })])

		expect(extractHocuspocusProviderFromEditor(editor)).toBe(provider)
	})

	it("returns null when the caret extension has no provider", ({ expect }) => {
		const editor = fakeEditor([CollaborationCaret])

		expect(extractHocuspocusProviderFromEditor(editor)).toBeNull()
	})

	it("returns null without a collaboration caret extension", ({ expect }) => {
		expect(extractHocuspocusProviderFromEditor(fakeEditor([]))).toBeNull()
	})
})

describe("RemoteDeleteSelectionGuard", () => {
	// core extensions add unrelated plugins; disabling them leaves the
	// guard's plugin as the only appendTransaction source
	function makeGuardedState() {
		const editor = new Editor({
			element: null,
			enableCoreExtensions: false,
			extensions: [Document, Paragraph, Text, RemoteDeleteSelectionGuard],
			content: {
				type: "doc",
				content: [
					{
						type: "paragraph",
						content: [{ type: "text", text: "hello world" }],
					},
				],
			},
		})

		const plugins = editor.extensionManager.plugins
		const state = EditorState.create({ doc: editor.state.doc, plugins })
		const guardPlugin = plugins.find((plugin) => plugin.spec.appendTransaction)

		editor.destroy()

		if (!guardPlugin) {
			throw new Error("guard plugin not found")
		}

		return { state, guardPlugin }
	}

	function remoteDelete(state: EditorState) {
		return state.tr.delete(1, 6).setMeta(ySyncPluginKey, {
			isChangeOrigin: true,
		})
	}

	// an out-of-range selection cannot be produced through
	// applyTransaction (ProseMirror remaps it during apply), so the
	// recovery path is exercised by invoking the plugin directly with a
	// crafted state, the way a y-sync update can leave a stale selection
	function invokeGuard(
		guardPlugin: Plugin,
		state: EditorState,
		selection: { from: number; to: number },
	) {
		const brokenState = {
			selection,
			doc: state.doc,
			tr: state.tr,
		} as unknown as EditorState

		return guardPlugin.spec.appendTransaction?.(
			[remoteDelete(state)],
			state,
			brokenState,
		)
	}

	it("appends nothing for local changes", ({ expect }) => {
		const { state } = makeGuardedState()

		const result = state.applyTransaction(state.tr.delete(1, 6))

		expect(result.transactions).toHaveLength(1)
		expect(result.state.doc.textContent).toBe(" world")
	})

	it("leaves a still-valid selection alone after a remote change", ({
		expect,
	}) => {
		const { state } = makeGuardedState()

		const result = state.applyTransaction(remoteDelete(state))

		expect(result.transactions).toHaveLength(1)
		expect(result.state.selection.from).toBe(1)
	})

	it("moves an out-of-range selection to the nearest valid position", ({
		expect,
	}) => {
		const { state, guardPlugin } = makeGuardedState()

		// the document "hello world" spans positions 0-13
		const tr = invokeGuard(guardPlugin, state, { from: 99, to: 99 })

		expect(tr?.selection.from).toBe(12)
		expect(tr?.selection.empty).toBe(true)
	})

	it("re-anchors a selection whose head is out of range", ({ expect }) => {
		const { state, guardPlugin } = makeGuardedState()

		const tr = invokeGuard(guardPlugin, state, { from: 3, to: 99 })

		expect(tr?.selection.from).toBe(3)
		expect(tr?.selection.empty).toBe(true)
	})
})

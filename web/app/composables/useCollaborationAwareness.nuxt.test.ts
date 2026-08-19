import type { HocuspocusProvider } from "@hocuspocus/provider"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { Editor } from "@tiptap/core"
import { describe, it } from "vitest"
import { useCollaborationAwareness } from "./useCollaborationAwareness"

// a minimal awareness fake: the composable only reads clientID and
// getStates() and (un)subscribes to the change event
function makeProvider(states: Map<number, object>, localClientId = 1) {
	const listeners = new Set<() => void>()

	const awareness = {
		clientID: localClientId,
		getStates: () => states,
		on: (_event: string, handler: () => void) => {
			listeners.add(handler)
		},
		off: (_event: string, handler: () => void) => {
			listeners.delete(handler)
		},
	}

	return {
		provider: { awareness } as unknown as HocuspocusProvider,
		emitChange: () => {
			listeners.forEach((handler) => {
				handler()
			})
		},
		states,
	}
}

function editingState(uid: string, type = "paragraph", user?: object) {
	return { editingNodeUid: { uid, type }, user }
}

describe("useCollaborationAwareness", () => {
	it("passes a provider through", ({ expect }) => {
		const { provider } = makeProvider(new Map())

		const awareness = useCollaborationAwareness(provider)

		expect(awareness.provider.value).toBe(provider)
	})

	it("resolves no provider from an editor without collaboration", ({
		expect,
	}) => {
		const editor = new Editor({ extensions: [Document, Paragraph, Text] })

		const awareness = useCollaborationAwareness(editor)

		expect(awareness.provider.value).toBeNull()
		expect(awareness.editingUids.value).toEqual(new Set())

		editor.destroy()
	})

	it("reports empty state without a source", ({ expect }) => {
		const awareness = useCollaborationAwareness(null)

		expect(awareness.provider.value).toBeNull()
		expect(awareness.editingUids.value).toEqual(new Set())
		expect(awareness.editingNodes.value).toEqual([])
		expect(awareness.draggingUids.value).toEqual(new Set())
	})

	it("collects the node uids edited by other users", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[1, editingState("local-node")],
				[2, editingState("n1")],
				[3, editingState("n2")],
			]),
		)

		const awareness = useCollaborationAwareness(provider)

		expect(awareness.editingUids.value).toEqual(new Set(["n1", "n2"]))
		expect(awareness.editingNodes.value).toEqual([
			{ uid: "n1", type: "paragraph" },
			{ uid: "n2", type: "paragraph" },
		])
	})

	it("collects the node uids dragged by other users", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[1, { draggingNodeUid: "local-node" }],
				[2, { draggingNodeUid: "n1" }],
			]),
		)

		const awareness = useCollaborationAwareness(provider)

		expect(awareness.draggingUids.value).toEqual(new Set(["n1"]))
		expect(awareness.nodeBeingDraggedRef("n1").value).toBe(true)
		expect(awareness.nodeBeingDraggedRef("n2").value).toBe(false)
	})

	it("lists the users editing a specific node", ({ expect }) => {
		const { provider } = makeProvider(
			new Map([
				[2, editingState("n1", "paragraph", { name: "Ada", color: "#f00" })],
				[3, editingState("n1", "paragraph", { name: "Grace" })],
				[4, editingState("n2", "paragraph", { name: "Alan", color: "#00f" })],
			]),
		)

		const awareness = useCollaborationAwareness(provider)

		// the user without a color is dropped; the user on another node is
		// not listed
		expect(awareness.editingUsersRef("n1").value).toEqual([
			{ name: "Ada", color: "#f00" },
		])
		expect(awareness.nodeBeingEditedRef("n1").value).toBe(true)
		expect(awareness.nodeBeingEditedRef("n3").value).toBe(false)
	})

	it("updates reactively on awareness changes", ({ expect }) => {
		const { provider, emitChange, states } = makeProvider(new Map())
		const awareness = useCollaborationAwareness(provider)

		expect(awareness.editingUids.value).toEqual(new Set())

		states.set(2, editingState("n1"))
		emitChange()

		expect(awareness.awarenessVersion.value).toBe(1)
		expect(awareness.editingUids.value).toEqual(new Set(["n1"]))
	})
})

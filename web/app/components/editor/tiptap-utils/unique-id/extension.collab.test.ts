import { Editor, Extension } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { describe, it } from "vitest"
import { UniqueID } from "./extension"

function collabEditor() {
	const listeners = new Set<() => void>()
	const provider = {
		on: (_event: "synced", handler: () => void) => {
			listeners.add(handler)
		},
		off: (_event: "synced", handler: () => void) => {
			listeners.delete(handler)
		},
	}

	// the extension only reads `provider` from an extension named
	// "collaboration", so a stand-in is enough
	const Collaboration = Extension.create({
		name: "collaboration",
		addOptions() {
			return { provider }
		},
	})

	const editor = new Editor({
		extensions: [
			Document,
			Paragraph,
			Text,
			Collaboration,
			UniqueID.configure({
				types: ["paragraph"],
				generateID: () => "gen-1",
			}),
		],
		content: {
			type: "doc",
			content: [
				{
					type: "paragraph",
					attrs: { id: null },
					content: [{ type: "text", text: "a" }],
				},
			],
		},
	})

	// a headless editor never emits `create` (that happens on mount),
	// so trigger the extensions' onCreate hooks by hand
	editor.emit("create", { editor })

	return {
		editor,
		listeners,
		emitSynced: () => {
			;[...listeners].forEach((listener) => {
				listener()
			})
		},
	}
}

describe("UniqueID collaboration listener cleanup", () => {
	it("removes the synced listener when the editor is destroyed before the first sync", ({
		expect,
	}) => {
		const { editor, listeners } = collabEditor()

		expect(listeners.size).toBe(1)

		editor.destroy()

		expect(listeners.size).toBe(0)
	})

	it("removes the synced listener after the first sync and assigns ids", ({
		expect,
	}) => {
		const { editor, listeners, emitSynced } = collabEditor()

		expect(listeners.size).toBe(1)

		emitSynced()

		expect(listeners.size).toBe(0)
		expect(editor.state.doc.child(0).attrs.id).toBe("gen-1")

		editor.destroy()
	})
})

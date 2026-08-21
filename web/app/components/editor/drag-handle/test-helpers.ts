import type { JSONContent } from "@tiptap/core"
import type { Plugin } from "@tiptap/pm/state"
import { GapDecorations } from "./gap-decorations"

// addProseMirrorPlugins reads neither this.options nor this.editor, so
// a bare context stands in for the extension instance the editor
// normally binds it to
export function gapPlugin(): Plugin {
	const addProseMirrorPlugins = GapDecorations.config.addProseMirrorPlugins

	if (!addProseMirrorPlugins) {
		throw new Error("GapDecorations declares no prosemirror plugins")
	}

	const plugin = addProseMirrorPlugins.call({} as never)[0]

	if (!plugin) {
		throw new Error("GapDecorations produced no prosemirror plugin")
	}

	return plugin
}

export function bulletList(...items: JSONContent[]): JSONContent {
	return { type: "bulletList", content: items }
}

export function taskList(...items: JSONContent[]): JSONContent {
	return { type: "taskList", content: items }
}

import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"

// tiptap only wires extension plugins into the editor state once a view
// is mounted, so headless plugin tests pull the plugin out of the
// extension manager and attach it to a hand-built EditorState instead.
// Plugins are matched by their PluginKey name prefix, the same way
// tiptap's own unregisterPlugin resolves them.
export function findPluginByKey(editor: Editor, name: string): Plugin {
	const plugin = editor.extensionManager.plugins.find((candidate) =>
		(candidate as Plugin & { key: string }).key.startsWith(`${name}$`),
	)

	if (!plugin) {
		throw new Error(`plugin ${name} not found`)
	}

	return plugin
}

// compresses a document into one "type:childCount" string per
// top-level block, for readable assertions on structural changes
export function childCountShape(doc: PMNode): string[] {
	const blocks: string[] = []

	doc.forEach((node) => {
		blocks.push(`${node.type.name}:${node.childCount}`)
	})

	return blocks
}

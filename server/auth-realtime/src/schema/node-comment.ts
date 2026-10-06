import { Extension } from "@tiptap/core"

// the editor hangs block comments off this attribute. Without it here,
// every block this service writes would lose its comment.
export const NodeComment = Extension.create<{ types: string[] }>({
	name: "nodeComment",
	addOptions() {
		return { types: [] }
	},
	addGlobalAttributes() {
		return [
			{
				types: this.options.types,
				attributes: {
					nodeCommentId: { default: null },
				},
			},
		]
	},
})

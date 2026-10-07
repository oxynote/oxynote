import type { Attributes } from "@tiptap/core"
import Link from "@tiptap/extension-link"

export const LinkMark = Link.configure({
	openOnClick: false,
	HTMLAttributes: {
		class:
			"text-link underline underline-offset-2 hover:text-link/80 cursor-pointer",
	},
}).extend({
	inclusive: false,
	// a link keeps the classes it was created with as an attribute. Rendering
	// them would give a link saved earlier its old look back.
	addAttributes() {
		const attributes: Attributes = { ...this.parent?.() }

		return {
			...attributes,
			class: { ...attributes.class, rendered: false },
		}
	},
})

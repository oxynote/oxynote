import { mergeAttributes, Node } from "@tiptap/core"

export const ImageBlock = Node.create({
	name: "imageBlock",
	group: "block",
	atom: true,
	draggable: false,
	selectable: false,
	addAttributes() {
		return {
			src: {
				default: null,
			},
			alt: {
				default: null,
			},
			title: {
				default: null,
			},
			width: {
				default: null,
			},
		}
	},
	parseHTML() {
		return [{ tag: `img[data-type="image-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"img",
			mergeAttributes(HTMLAttributes, {
				"data-type": "image-block",
			}),
		]
	},
})

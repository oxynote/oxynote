import { Node, mergeAttributes } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import ImageBlockComponent from "./ImageBlock.vue"
import { IMAGE_BLOCK_NAME } from "../node-names"

export const ImageBlock = Node.create({
	name: IMAGE_BLOCK_NAME,
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
				parseHTML: (element) => {
					const width = element.getAttribute("width")
					return width ? Number.parseInt(width, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.width) {
						return {}
					}

					return { width: attrs.width as number }
				},
			},
			uploading: {
				default: false,
				rendered: false,
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
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(ImageBlockComponent)
	},
})

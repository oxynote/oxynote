import { Node, mergeAttributes } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import FileBlockComponent from "./FileBlock.vue"
import { FILE_BLOCK_NAME } from "../node-names"

export const FileBlock = Node.create({
	name: FILE_BLOCK_NAME,
	group: "block",
	atom: true,
	draggable: false,
	selectable: false,
	addAttributes() {
		return {
			src: {
				default: null,
				parseHTML: (element) => element.getAttribute("href"),
				renderHTML: (attrs) => {
					if (!attrs.src) {
						return {}
					}

					return { href: attrs.src as string }
				},
			},
			name: {
				default: null,
				parseHTML: (element) => element.getAttribute("data-name"),
				renderHTML: (attrs) => {
					if (!attrs.name) {
						return {}
					}

					return { "data-name": attrs.name as string }
				},
			},
			size: {
				default: null,
				parseHTML: (element) => {
					const size = element.getAttribute("data-size")
					return size ? Number.parseInt(size, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.size) {
						return {}
					}

					return { "data-size": attrs.size as number }
				},
			},
			contentType: {
				default: null,
				parseHTML: (element) => element.getAttribute("data-content-type"),
				renderHTML: (attrs) => {
					if (!attrs.contentType) {
						return {}
					}

					return { "data-content-type": attrs.contentType as string }
				},
			},
			uploading: {
				default: false,
				rendered: false,
			},
		}
	},
	parseHTML() {
		return [{ tag: `a[data-type="file-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"a",
			mergeAttributes(HTMLAttributes, {
				"data-type": "file-block",
			}),
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(FileBlockComponent)
	},
})

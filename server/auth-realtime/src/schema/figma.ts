import { mergeAttributes, Node } from "@tiptap/core"

export const FigmaBlock = Node.create({
	name: "figmaBlock",
	group: "block",
	atom: true,
	draggable: false,
	selectable: false,
	addAttributes() {
		return {
			src: {
				default: null,
				parseHTML: (element) =>
					element.getAttribute("data-src"),
				renderHTML: (attrs) => {
					if (!attrs.src) return {}
					return {
						"data-src": attrs.src as string,
					}
				},
			},
			width: {
				default: null,
				parseHTML: (element) => {
					const w =
						element.getAttribute(
							"data-width",
						)
					return w ? Number.parseInt(w, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.width) return {}
					return {
						"data-width":
							attrs.width as number,
					}
				},
			},
			height: {
				default: null,
				parseHTML: (element) => {
					const h =
						element.getAttribute(
							"data-height",
						)
					return h ? Number.parseInt(h, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.height) return {}
					return {
						"data-height":
							attrs.height as number,
					}
				},
			},
		}
	},
	parseHTML() {
		return [{ tag: `div[data-type="figma-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "figma-block",
			}),
		]
	},
})

import { Node, Extension, mergeAttributes } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import { Plugin } from "@tiptap/pm/state"
import FigmaBlockComponent from "./FigmaBlock.vue"
import { FIGMA_BLOCK_NAME } from "../node-names"

export const FIGMA_URL_RE =
	/^https?:\/\/(www\.)?figma\.com\/(file|design|board|proto)\/([A-Za-z0-9_-]+)/

export function isFigmaUrl(url: string): boolean {
	return FIGMA_URL_RE.test(url.trim())
}

export function convertToEmbedUrl(src: string, isDark: boolean): string {
	const url = new URL(src)
	const parts = url.pathname.split("/").filter(Boolean)
	const type = parts[0]
	const fileKey = parts[1]
	const embedType = type === "file" ? "design" : type

	const embedUrl = new URL(`https://embed.figma.com/${embedType}/${fileKey}`)
	embedUrl.searchParams.set("embed-host", "oxynote")
	embedUrl.searchParams.set("theme", isDark ? "dark" : "light")

	const nodeId = url.searchParams.get("node-id")
	if (nodeId) {
		embedUrl.searchParams.set("node-id", nodeId)
	}

	return embedUrl.toString()
}

export const FigmaBlock = Node.create({
	name: FIGMA_BLOCK_NAME,
	group: "block",
	atom: true,
	draggable: false,
	selectable: false,
	addAttributes() {
		return {
			src: {
				default: null,
				parseHTML: (element) => element.getAttribute("data-src"),
				renderHTML: (attrs) => {
					if (!attrs.src) return {}
					return { "data-src": attrs.src }
				},
			},
			width: {
				default: null,
				parseHTML: (element) => {
					const w = element.getAttribute("data-width")
					return w ? Number.parseInt(w, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.width) return {}
					return { "data-width": attrs.width }
				},
			},
			height: {
				default: null,
				parseHTML: (element) => {
					const h = element.getAttribute("data-height")
					return h ? Number.parseInt(h, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.height) return {}
					return { "data-height": attrs.height }
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
			mergeAttributes(HTMLAttributes, { "data-type": "figma-block" }),
		]
	},
	addNodeView() {
		return VueNodeViewRenderer(FigmaBlockComponent)
	},
})

export function createFigmaLinkHandler() {
	return Extension.create({
		name: "figmaLinkHandler",
		addProseMirrorPlugins() {
			return [
				new Plugin({
					props: {
						handlePaste(view, event) {
							const text = event.clipboardData?.getData("text/plain")?.trim()
							if (!text || !isFigmaUrl(text)) {
								return false
							}

							const { state } = view
							const { selection } = state

							// Only handle collapsed selections
							if (!selection.empty) {
								return false
							}

							const $pos = state.doc.resolve(selection.anchor)

							// Only insert at root level (depth <= 2)
							if ($pos.depth > 2) {
								return false
							}

							// Only replace an empty paragraph
							const parent = $pos.parent
							if (parent.content.size !== 0) {
								return false
							}

							const figmaNode = state.schema.nodes[FIGMA_BLOCK_NAME]
							if (!figmaNode) {
								return false
							}

							const paragraphStart = $pos.before()
							const paragraphEnd = $pos.after()

							const tr = state.tr.delete(paragraphStart, paragraphEnd)
							tr.insert(paragraphStart, figmaNode.create({ src: text }))
							view.dispatch(tr)

							return true
						},
					},
				}),
			]
		},
	})
}

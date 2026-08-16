import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "prosemirror-state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"
import type { Node as PMNode } from "@tiptap/pm/model"
import { CODE_BLOCK_TITLE_NAME } from "../node-names"

const patterns: { regex: RegExp; class: string; offset?: number }[] = [
	{
		// domain
		regex: /\b((?:[a-zA-Z0-9-]+\.)+[a-zA-Z]{2,})(?=[:/"]|$)/g,
		class: "opacity-50",
	},
	{
		// url path param: /:param
		regex: /\/:[A-Za-z0-9_-]+/g,
		class: "text-url-path-param",
		offset: 1,
	},
	{
		// url path param: /{param}
		regex: /\/\{[A-Za-z0-9_-]+\}/g,
		class: "text-url-path-param",
		offset: 1,
	},
	{ regex: /\bGET\b/g, class: "text-http-method-get" },
	{ regex: /\bPOST\b/g, class: "text-http-method-post" },
	{ regex: /\bPUT\b/g, class: "text-http-method-put" },
	{ regex: /\bPATCH\b/g, class: "text-http-method-patch" },
	{ regex: /\bDELETE\b/g, class: "text-http-method-delete" },
	{ regex: /\bOPTIONS\b/g, class: "text-http-method-options" },
	{ regex: /\bHEAD\b/g, class: "text-http-method-head" },
	{ regex: /\bTRACE\b/g, class: "text-http-method-trace" },
	{ regex: /\bCONNECT\b/g, class: "text-http-method-connect" },
]

export const KeywordColor = Extension.create({
	name: "keywordColor",
	addProseMirrorPlugins() {
		const buildDecorations = (doc: PMNode) => {
			const decos: Decoration[] = []

			doc.descendants((node, pos) => {
				if (node.type.name !== CODE_BLOCK_TITLE_NAME) {
					return
				}

				// collect all text nodes (even if split by marks)
				const textNodes: { text: string; from: number }[] = []
				node.descendants((child, childPos) => {
					if (child.isText && child.text) {
						// absolute position in the whole doc
						textNodes.push({ text: child.text, from: pos + 1 + childPos })
					}
				})

				if (textNodes.length === 0) {
					return
				}

				//  build a combined text string
				let combinedText = ""
				const boundaries: { end: number; from: number }[] = []
				let acc = 0

				for (const n of textNodes) {
					acc += n.text.length
					boundaries.push({ end: acc, from: n.from })
					combinedText += n.text
				}

				// helper to map regex index -> absolute document pos
				const indexToPos = (idx: number) => {
					const last = boundaries[boundaries.length - 1]
					if (!last) {
						return 0
					}

					if (idx < 0) {
						idx = 0
					} else if (idx > last.end) {
						idx = last.end
					}

					for (let i = 0; i < boundaries.length; i++) {
						const prevEnd = i === 0 ? 0 : (boundaries[i - 1]?.end ?? 0)
						const b = boundaries[i]
						if (b && idx <= b.end) {
							return b.from + (idx - prevEnd)
						}
					}

					const prevEnd =
						boundaries.length > 1
							? (boundaries[boundaries.length - 2]?.end ?? 0)
							: 0

					return last.from + (last.end - prevEnd)
				}

				// run regexes on combined text
				for (const { regex, class: className, offset = 0 } of patterns) {
					regex.lastIndex = 0
					let match

					while ((match = regex.exec(combinedText)) !== null) {
						const from = indexToPos(match.index + offset)
						const to = indexToPos(match.index + match[0].length)
						decos.push(
							Decoration.inline(
								from,
								to,
								{
									class: className,
								},
								{ inclusiveStart: false, inclusiveEnd: false },
							),
						)
					}
				}
			})

			return DecorationSet.create(doc, decos)
		}

		return [
			new Plugin({
				key: new PluginKey("keyword-color"),
				state: {
					init: (_, { doc }) => buildDecorations(doc),
					apply: (tr, old) => (tr.docChanged ? buildDecorations(tr.doc) : old),
				},
				props: {
					decorations(state) {
						return this.getState(state)
					},
				},
			}),
		]
	},
})

// Workaround function to fix selection issues after adding comment marks.
export function fixUserSelectionAroundKeywordColor() {
	requestAnimationFrame(() => {
		const sel = window.getSelection()
		if (sel) {
			sel.removeAllRanges()
		}
	})
}

import { mergeAttributes, Node } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import { Selection, TextSelection } from "prosemirror-state"
import MainBlock from "./MainBlock.vue"
import { mermaidHighlightPlugin } from "./mermaid-highlight"
import { MERMAID_BLOCK_NAME } from "../node-names"
import { COMMENT_MARK_NAME } from "../../mark-names"
import { deleteNode } from "../../tiptap-utils/node"

export const TAB_SIZE = 4

export const MermaidBlock = Node.create({
	name: MERMAID_BLOCK_NAME,
	group: "block",
	content: "text*",
	marks: COMMENT_MARK_NAME,
	code: true,
	defining: true,
	isolating: true,
	selectable: true,
	whitespace: "pre",
	parseHTML() {
		return [
			{
				tag: `pre[data-type="mermaid-block"]`,
				preserveWhitespace: "full",
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"pre",
			mergeAttributes(HTMLAttributes, {
				"data-type": "mermaid-block",
			}),
			["code", 0],
		]
	},
	addInputRules() {
		return []
	},
	addKeyboardShortcuts() {
		return {
			Tab: () => {
				const { state, view } = this.editor
				const { selection } = state

				if (selection.$from.parent.type.name !== this.name) {
					return false
				}

				view.dispatch(state.tr.insertText("\t"))

				return true
			},
			"Shift-Tab": () => {
				const { state, view } = this.editor
				const { selection } = state
				const { $from } = selection

				if ($from.parent.type.name !== this.name) {
					return false
				}

				const lineStart = state.doc.resolve($from.start())
				const textBefore = state.doc.textBetween(lineStart.pos, $from.pos, "\n")
				const lastNewline = textBefore.lastIndexOf("\n")
				const lineBegin =
					lastNewline === -1 ? lineStart.pos : lineStart.pos + lastNewline + 1
				const charAfter = state.doc.textBetween(lineBegin, lineBegin + 1)

				if (charAfter === "\t") {
					view.dispatch(state.tr.delete(lineBegin, lineBegin + 1))
					return true
				}

				// remove up to TAB_SIZE leading spaces
				const lineEnd = $from.end()
				const lineText = state.doc.textBetween(lineBegin, lineEnd)
				const leadingSpaces = /^ */.exec(lineText)?.[0].length ?? 0

				if (leadingSpaces > 0) {
					const removeCount = Math.min(leadingSpaces, TAB_SIZE)
					view.dispatch(state.tr.delete(lineBegin, lineBegin + removeCount))
					return true
				}

				return false
			},
			Backspace: () => {
				const { state } = this.editor
				const { empty, $anchor } = state.selection
				const isAtStart = $anchor.parentOffset === 0

				if (!empty || $anchor.parent.type.name !== this.name) {
					return false
				}

				if (!isAtStart) {
					return false
				}

				// Empty block: convert to paragraph
				if ($anchor.parent.content.size === 0) {
					const blockPos = $anchor.before()
					return deleteNode(this.editor, blockPos)
				}

				return false
			},
			ArrowDown: ({ editor }) => {
				const { state, view } = editor
				const { selection } = state

				if (
					!selection.empty ||
					selection.$anchor.parent.type.name !== this.name
				) {
					return false
				}

				const { $from, head } = selection

				let blockDepth = -1
				for (let depth = $from.depth; depth >= 0; depth--) {
					if ($from.node(depth).type.name === this.name) {
						blockDepth = depth
						break
					}
				}

				if (blockDepth === -1) {
					return false
				}

				if (head !== $from.end(blockDepth)) {
					return false
				}

				const posAfter = $from.after(blockDepth)
				const $posAfter = state.doc.resolve(posAfter)
				const nodeAfter = $posAfter.nodeAfter

				if (nodeAfter?.isTextblock) {
					const tr = state.tr.setSelection(
						TextSelection.create(state.doc, posAfter + 1),
					)
					view.dispatch(tr.scrollIntoView())
					return true
				}

				const sel = Selection.near(state.doc.resolve(posAfter), 1)

				if (sel.from !== selection.from || sel.to !== selection.to) {
					const tr = state.tr.setSelection(sel)
					view.dispatch(tr.scrollIntoView())
					return true
				}

				return false
			},
			"Shift-Enter": ({ editor }) => {
				const { state, view } = editor
				const { selection } = state
				const { $from } = selection

				if ($from.parent.type.name !== this.name) {
					return false
				}

				let blockDepth = -1
				for (let depth = $from.depth; depth >= 0; depth--) {
					if ($from.node(depth).type.name === this.name) {
						blockDepth = depth
						break
					}
				}

				if (blockDepth === -1) {
					return false
				}

				const posAfter = $from.after(blockDepth)
				const paragraphType = state.schema.nodes.paragraph
				if (!paragraphType) {
					return false
				}

				const tr = state.tr.insert(posAfter, paragraphType.create())
				tr.setSelection(TextSelection.create(tr.doc, posAfter + 1))
				view.dispatch(tr.scrollIntoView())

				return true
			},
		}
	},
	addProseMirrorPlugins() {
		return [mermaidHighlightPlugin(this.name)]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(MainBlock)
	},
})

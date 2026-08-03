import CodeBlockLowlight, {
	type CodeBlockLowlightOptions,
} from "@tiptap/extension-code-block-lowlight"
import {
	mergeAttributes,
	Node,
	type ChainedCommands,
	type ExtendedRegExpMatchArray,
	type Range,
} from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import Code from "./Code.vue"
import CodeTitle from "./CodeTitle.vue"
import { extendedCodeBlockLanguages } from "./languages"
import { createLowlight } from "lowlight"
import { Selection, TextSelection } from "prosemirror-state"
import { KeywordColor } from "./keyword"
import { deleteNode } from "../../tiptap-utils/node"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	TITLED_CODE_BLOCK_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	METRIC_BLOCK_NAME,
} from "../node-names"
import { COMMENT_MARK_NAME } from "../../mark-names"

export const lowlight = createLowlight(extendedCodeBlockLanguages)

export const CodeBlockTitle = Node.create({
	name: CODE_BLOCK_TITLE_NAME,
	group: CODE_BLOCK_TITLE_NAME, // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: "text*",
	marks: COMMENT_MARK_NAME,
	selectable: true,
	defining: true,
	isolating: true,
	parseHTML() {
		return [{ tag: `div[data-type="code-block-title"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "code-block-title",
			}),
			0,
		]
	},
	addExtensions() {
		return [KeywordColor]
	},
	addKeyboardShortcuts() {
		return {
			Backspace: () => {
				const { empty, $anchor } = this.editor.state.selection
				const isAtStart = $anchor.parentOffset === 0

				if (!empty || $anchor.parent.type.name !== this.name) {
					return false
				}

				if (isAtStart) {
					const grandGrandParent = $anchor.node($anchor.depth - 2)

					if (
						grandGrandParent.type.name === SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME
					) {
						const leftSideBlockCount = grandGrandParent.childCount
							? grandGrandParent.content.content.filter(
									(child) =>
										child.type.name === TITLED_CODE_BLOCK_NAME ||
										child.type.name === METRIC_BLOCK_NAME,
								).length
							: 0

						// In case this is the last code element, we wan't to
						// avoid doing anything as by default TipTap will
						// move the cursor outside of the SplitDocumentation
						// block.
						if (leftSideBlockCount === 1) {
							return true
						}
					}

					// Find the titledCodeBlock node position
					const titledCodeBlockDepth = $anchor.depth - 1
					const titledCodeBlockPos = $anchor.before(titledCodeBlockDepth)

					return deleteNode(this.editor, titledCodeBlockPos)
				}

				return false
			},
		}
	},
	addNodeView() {
		return VueNodeViewRenderer(CodeTitle)
	},
})

export interface CodeBlockOptions extends CodeBlockLowlightOptions {
	type: "document" | "comment"
}

export const CodeBlock = CodeBlockLowlight.extend<CodeBlockOptions>({
	name: CODE_BLOCK_NAME,
	content: "text*",
	marks: COMMENT_MARK_NAME,
	defining: true,
	isolating: false,
	selectable: true,
	whitespace: "pre",
	parseHTML() {
		return [
			{
				tag: `pre[data-type="code-block"]`,
				preserveWhitespace: "full",
			},
		]
	},
	renderHTML({ node, HTMLAttributes }) {
		return [
			"pre",
			mergeAttributes(HTMLAttributes, {
				"data-type": "code-block",
			}),
			[
				"code",
				{
					class: node.attrs.language
						? this.options.languageClassPrefix + node.attrs.language
						: null,
				},
				0,
			],
		]
	},
	addOptions(): CodeBlockOptions {
		return {
			...this.parent!(),
			defaultLanguage: null,
			enableTabIndentation: true,
			exitOnTripleEnter: false,
			lowlight: lowlight,
			type: "document",
		}
	},
	addInputRules() {
		return []
	},
	addKeyboardShortcuts() {
		return {
			...this.parent!(),
			Backspace: () => {
				const { state, view } = this.editor
				const { empty, $anchor } = state.selection
				const isAtStart = $anchor.parentOffset === 0

				if (!empty || $anchor.parent.type.name !== this.name) {
					return false
				}

				if (!isAtStart) {
					return false
				}

				const parentNode = $anchor.node($anchor.depth - 1)

				// If the parent node is TitledCodeBlock, prevent deletion.
				if (parentNode.type.name === TITLED_CODE_BLOCK_NAME) {
					return true
				}

				const paragraphType = state.schema.nodes.paragraph
				if (!paragraphType) {
					return false
				}

				const codeBlockPos = $anchor.before()
				const tr = state.tr.replaceWith(
					codeBlockPos,
					codeBlockPos + $anchor.parent.nodeSize,
					paragraphType.create(undefined, $anchor.parent.content),
				)

				tr.setSelection(TextSelection.create(tr.doc, codeBlockPos + 1))
				view.dispatch(tr.scrollIntoView())

				return true
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

				// find current code block depth
				let codeDepth = -1
				for (let depth = $from.depth; depth >= 0; depth--) {
					if ($from.node(depth).type.name === this.name) {
						codeDepth = depth
						break
					}
				}

				if (codeDepth === -1) {
					return false
				}

				// only when at the very end of the code block
				if (head !== $from.end(codeDepth)) {
					return false
				}

				const posAfter = $from.after(codeDepth)
				const $posAfter = state.doc.resolve(posAfter)
				const nodeAfter = $posAfter.nodeAfter

				// If next node is a textblock, move caret to its start
				if (nodeAfter && nodeAfter.isTextblock) {
					const tr = state.tr.setSelection(
						TextSelection.create(state.doc, posAfter + 1),
					)

					view.dispatch(tr.scrollIntoView())
					return true
				}

				// Otherwise, find the nearest valid selection *after* the code block.
				// This will land on a GapCursor (if enabled) or a NodeSelection/next text position.
				const sel = Selection.near(
					state.doc.resolve(posAfter),
					1 /* forward bias */,
				)

				// Make sure we actually moved somewhere different
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

				// Don't handle if inside TitledCodeBlock (used in SplitDocumentation)
				const parentNode = $from.node($from.depth - 1)
				if (parentNode.type.name === TITLED_CODE_BLOCK_NAME) {
					return false
				}

				// Find current code block depth
				let codeDepth = -1
				for (let depth = $from.depth; depth >= 0; depth--) {
					if ($from.node(depth).type.name === this.name) {
						codeDepth = depth
						break
					}
				}

				if (codeDepth === -1) {
					return false
				}

				const posAfter = $from.after(codeDepth)
				const paragraphType = state.schema.nodes.paragraph
				if (!paragraphType) {
					return false
				}

				// Insert a paragraph after the code block and move selection there
				const tr = state.tr.insert(posAfter, paragraphType.create())
				tr.setSelection(TextSelection.create(tr.doc, posAfter + 1))
				view.dispatch(tr.scrollIntoView())

				return true
			},
		}
	},
	addNodeView() {
		return VueNodeViewRenderer(Code)
	},
})

export const TitledCodeBlock = Node.create({
	name: TITLED_CODE_BLOCK_NAME,
	group: TITLED_CODE_BLOCK_NAME, // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `${CODE_BLOCK_TITLE_NAME} ${CODE_BLOCK_NAME}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="titled-code-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "titled-code-block",
			}),
			0,
		]
	},
})

export function setUpCodeBlockNode(
	range: Range,
	match: ExtendedRegExpMatchArray,
	chain: () => ChainedCommands,
) {
	chain()
		.deleteRange(range)
		.insertContent({
			type: CODE_BLOCK_NAME,
			attrs: {
				language: (match?.[1] || "").toLowerCase(),
			},
		})
		.run()
}

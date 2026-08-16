import { type Editor, Extension, InputRule } from "@tiptap/core"
import { setUpCalloutBlockNode } from "./blocks/callout"
import Paragraph from "@tiptap/extension-paragraph"
import {
	SplitDocumentation,
	SplitDocumentationLeftSide,
	setUpSplitDocumentationNode,
	SplitDocumentationRightSide,
} from "./blocks/split-documentation"
import { setUpCodeBlockNode } from "./blocks/code-block"
import { Selection, TextSelection, type Transaction } from "@tiptap/pm/state"
import { setUpMetricBlock } from "./blocks/metrics"
import { CODE_BLOCK_NAME, METRIC_BLOCK_NAME } from "./blocks/node-names"
import type { Node as PMNode } from "@tiptap/pm/model"

const ALLOWED_CALLOUT_CONTAINERS = [
	SplitDocumentation.name,
	SplitDocumentationLeftSide.name,
	Paragraph.name,
]

const ALLOWED_CODE_BLOCK_CONTAINERS = [
	SplitDocumentation.name,
	SplitDocumentationRightSide.name,
	Paragraph.name,
]

export const InputRules = Extension.create({
	name: "inputRules",
	addInputRules() {
		return [
			// Callout block.
			preventTiptapInputRuleInNode(
				new InputRule({
					find: /(^|\n)!!$/,
					handler: ({ range, commands }) => {
						setUpCalloutBlockNode(range, commands)
					},
				}),
				ALLOWED_CALLOUT_CONTAINERS,
			),
			// SplitDocumentation block.
			preventTiptapInputRuleInNode(
				new InputRule({
					find: /(^|\n)\|\|$/,
					handler: ({ state, range }) => {
						const { tr } = state
						setUpSplitDocumentationNode(state, tr, range.from)
					},
				}),
				[],
				true,
			),
			// CodeBlock block.
			preventTiptapInputRuleInNode(
				new InputRule({
					find: /(^|\n)```$/,
					handler: ({ range, match, chain }) => {
						setUpCodeBlockNode(range, match, chain)
					},
				}),
				ALLOWED_CODE_BLOCK_CONTAINERS,
			),
			// SplitDocumentation block.
			preventTiptapInputRuleInNode(
				new InputRule({
					find: /(^|\n)%%$/,
					handler: ({ state, range }) => {
						const { tr } = state
						setUpMetricBlock(state, tr, range.from)
					},
				}),
				[],
				true,
			),
		]
	},
})

export interface ParagraphDeletionHandlerOptions {
	onDeleted?: (args: { editor: Editor; tr: Transaction }) => void
}

export const ParagraphDeletionHandler =
	Extension.create<ParagraphDeletionHandlerOptions>({
		name: "ParagraphDeletionHandler",
		addOptions() {
			return {
				onDeleted: ({ tr }) => {
					if (tr.doc.firstChild) {
						const sel = TextSelection.create(tr.doc, 1)
						tr.setSelection(sel)
					}
				},
			}
		},
		addKeyboardShortcuts() {
			return {
				Backspace: () => {
					const { state, view } = this.editor
					const { selection } = state
					const cursor =
						selection instanceof TextSelection ? selection.$cursor : null

					if (!cursor) {
						return false
					}

					// Handle deletion at absolute position 1 (very top of the doc).
					if (cursor.pos === 1) {
						const firstNode = state.doc.firstChild
						if (!firstNode) {
							return false
						}

						// only when first block is an empty paragraph
						if (
							firstNode.type.name === Paragraph.name &&
							firstNode.content.size === 0
						) {
							const tr = state.tr.delete(0, firstNode.nodeSize)

							this.options.onDeleted?.({ editor: this.editor, tr })

							view.dispatch(tr)
							return true
						}
					}

					// If Backspace starts at the beginning of a paragraph and the previous
					// sibling ends in a metric/code block, remove this paragraph instead of
					// letting default ProseMirror behavior delete the previous block.
					if (
						cursor.parent.type.name === Paragraph.name &&
						cursor.parentOffset === 0
					) {
						const paragraphStart = cursor.before()
						const paragraphEnd = cursor.after()
						const $cut = state.doc.resolve(paragraphStart)
						const previousSibling = $cut.nodeBefore

						if (
							previousSibling &&
							(endsWithMetricBlock(previousSibling) ||
								endsWithCodeBlock(previousSibling))
						) {
							const paragraphContent = cursor.parent.content
							const paragraphText = cursor.parent.textBetween(
								0,
								cursor.parent.content.size,
								"\n",
								"\n",
							)
							const previousEndsWithCode = endsWithCodeBlock(previousSibling)
							const targetParagraph = findPreviousParagraph(
								state.doc,
								paragraphStart,
							)
							const tr = state.tr.delete(paragraphStart, paragraphEnd)

							// For code blocks, append paragraph text directly into the code block.
							if (previousEndsWithCode && paragraphText.length > 0) {
								const previousSiblingPos =
									paragraphStart - previousSibling.nodeSize
								const codeBlockEndPos = findLastCodeBlockContentEndPos(
									previousSibling,
									previousSiblingPos,
								)
								if (codeBlockEndPos !== null) {
									const insertPos = tr.mapping.map(codeBlockEndPos)
									tr.insertText(paragraphText, insertPos)
									tr.setSelection(
										TextSelection.create(
											tr.doc,
											insertPos + paragraphText.length,
										),
									)
									view.dispatch(tr.scrollIntoView())
									return true
								}
							}

							// If a previous paragraph exists, merge content into its end.
							if (targetParagraph && paragraphContent.size > 0) {
								const insertPos = tr.mapping.map(targetParagraph.end)
								tr.insert(insertPos, paragraphContent)
								tr.setSelection(
									Selection.near(
										tr.doc.resolve(insertPos + paragraphContent.size),
										-1,
									),
								)
							} else {
								const selectionPos = Math.max(0, paragraphStart - 1)
								const nextSelection = Selection.near(
									tr.doc.resolve(selectionPos),
									-1,
								)
								tr.setSelection(nextSelection)
							}

							view.dispatch(tr.scrollIntoView())
							return true
						}
					}

					return false
				},
			}
		},
	})

function endsWithMetricBlock(node: PMNode): boolean {
	let current: PMNode | null = node

	while (current) {
		if (current.type.name === METRIC_BLOCK_NAME) {
			return true
		}

		current = current.lastChild
	}

	return false
}

function endsWithCodeBlock(node: PMNode): boolean {
	let current: PMNode | null = node

	while (current) {
		if (current.type.name === CODE_BLOCK_NAME) {
			return true
		}

		current = current.lastChild
	}

	return false
}

function findLastCodeBlockContentEndPos(
	node: PMNode,
	nodePos: number,
): number | null {
	if (node.type.name === CODE_BLOCK_NAME) {
		return nodePos + node.nodeSize - 1
	}

	let found: number | null = null
	node.forEach((child, offset) => {
		const childPos = nodePos + 1 + offset
		const childFound = findLastCodeBlockContentEndPos(child, childPos)
		if (childFound !== null) {
			found = childFound
		}
	})

	return found
}

function findPreviousParagraph(
	doc: PMNode,
	beforePos: number,
): { end: number } | null {
	let lastParagraphEnd: number | null = null

	doc.nodesBetween(0, beforePos, (node, pos) => {
		if (node.type.name !== Paragraph.name) {
			return
		}

		if (pos >= beforePos) {
			return
		}

		lastParagraphEnd = pos + node.nodeSize - 1
	})

	// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- assigned inside the nodesBetween callback, which TS narrowing does not track
	if (lastParagraphEnd === null) {
		return null
	}

	return { end: lastParagraphEnd }
}

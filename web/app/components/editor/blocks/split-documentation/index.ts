import { type Editor, mergeAttributes, Node, type Range } from "@tiptap/core"
import type { Node as PMNode, ResolvedPos } from "@tiptap/pm/model"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import MainBlock from "./MainBlock.vue"
import Paragraph from "@tiptap/extension-paragraph"
import Heading from "@tiptap/extension-heading"
import { TaskList, BulletList, OrderedList } from "@tiptap/extension-list"
import RightSideBlock from "./RightSideBlock.vue"
import LeftSideBlock from "./LeftSideBlock.vue"
import {
	type EditorState,
	TextSelection,
	type Transaction,
} from "@tiptap/pm/state"
import { ParameterList, ParameterListItem } from "./parameter-list"
import { CalloutBlock } from "../callout"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	TITLED_CODE_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
} from "../node-names"
import { deleteNode } from "../../tiptap-utils/node"

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		splitDocumentation: {
			insertSplitDocumentation: (at: number) => ReturnType
			invertSplitDocumentation: (splitDocNodePos?: number) => ReturnType
			appendParameterListOnLeftSide: (leftSideNodePos: number) => ReturnType
			appendBlockOnRightSide: (
				rightSideNodePos: number,
				blockType: "code" | "metrics",
			) => ReturnType
			/* This method expects nodePos to be the position of a TitledCodeBlock or other hovered node */
			insertBlockOnRightSide: (
				nodePos: number,
				side: "above" | "below",
				blockType: "code" | "metrics",
			) => ReturnType
			/* This method expects nodePos to be the position of a ParameterList */
			insertParameterListOnLeftSide: (
				nodePos: number,
				side: "above" | "below",
			) => ReturnType
			/* This method expects nodePos to be the position of a ParameterListItem */
			insertParameterListItemOnLeftSide: (
				nodePos: number,
				side: "above" | "below",
			) => ReturnType
		}
	}
}

export const allowedLeftSideContent = [
	Paragraph.name,
	BulletList.name,
	OrderedList.name,
	TaskList.name,
	CalloutBlock.name,
]

export const extraLeftSideContent = [ParameterList.name]

export const SplitDocumentationLeftSide = Node.create({
	name: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	group: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `${Heading.name} (${allowedLeftSideContent.join(" | ")})+ (${extraLeftSideContent.join(" | ")})*`,
	isolating: false,
	defining: true,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="split-documentation-left-side"]` }]
	},
	addKeyboardShortcuts() {
		return {
			Backspace: ({ editor }) => {
				const { state } = editor
				const { selection } = state
				const { $from } = selection

				if (!isCursorInsideTiptapNode(state, [SplitDocumentation.name])) {
					return false
				}

				if ($from.parentOffset === 0) {
					if ($from.parent.type.name === Heading.name) {
						const d1 = $from.depth - 2

						// This selects the SplitDocumentation.
						const grandParent = $from.node(d1)
						if (grandParent.type.name !== SplitDocumentation.name) {
							return false
						}

						// This handles the deletion of the SplitDocumentation
						const posBefore = $from.before(d1)

						return deleteNode(editor, posBefore)
					}

					const parent = $from.node($from.depth - 1)
					if (parent.type.name !== SplitDocumentationLeftSide.name) {
						return false
					}

					// In case there is only one element from the
					// base nodes list, we want to avoid deletion.
					if (
						parent.children.length < 3 ||
						!allowedLeftSideContent.includes(
							parent.children[2]?.type.name || "",
						)
					) {
						return true
					}
				}

				return false
			},
		}
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-left-side",
			}),
			0,
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(LeftSideBlock)
	},
})

export const SplitDocumentationRightSide = Node.create({
	name: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	group: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `(${TITLED_CODE_BLOCK_NAME} | ${METRIC_BLOCK_NAME})+`,
	isolating: false,
	defining: true,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="split-documentation-right-side"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-right-side",
			}),
			0,
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(RightSideBlock)
	},
})

export const SplitDocumentationPlaceholderParents: {
	parent: boolean
	name: string
}[] = [
	{
		parent: true,
		name: SplitDocumentationLeftSide.name,
	},
	{
		parent: true,
		name: SplitDocumentationRightSide.name,
	},
	{
		parent: true,
		name: ParameterListItem.name,
	},
]

export const SplitDocumentation = Node.create({
	name: SPLIT_DOCUMENTATION_NAME,
	group: "block",
	content: `${SplitDocumentationLeftSide.name} ${SplitDocumentationRightSide.name}`,
	isolating: false,
	defining: true, // we want a new context here without extra formatting
	selectable: false,
	addAttributes() {
		return {
			inversed: {
				default: false,
				parseHTML: (element) =>
					element.getAttribute("data-inversed") === "true",
				renderHTML: (attrs) => {
					if (!attrs.inversed) {
						return {}
					}

					return { "data-inversed": "true" }
				},
			},
		}
	},
	addCommands() {
		return {
			insertSplitDocumentation:
				(at: number) =>
				({ tr, state, dispatch }) => {
					const res = setUpSplitDocumentationNode(state, tr, at)
					if (!res) {
						return false
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},
			appendParameterListOnLeftSide:
				(leftSideNodePos: number) =>
				({ state, tr, dispatch }) => {
					const { schema, doc } = state
					const leftNode = doc.nodeAt(leftSideNodePos)
					if (leftNode?.type.name !== SplitDocumentationLeftSide.name) {
						return false
					}

					const paramList = schema.nodes[ParameterList.name]?.createAndFill()
					if (!paramList) {
						return false
					}

					// insert just after the leftSide’s last element
					const insertPos = leftSideNodePos + leftNode.content.size + 1
					tr.insert(insertPos, paramList)

					const paramListPos = insertPos
					const textStart = paramListPos + 1

					tr.setSelection(
						TextSelection.near(tr.doc.resolve(textStart), 1),
					).scrollIntoView()

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},
			invertSplitDocumentation:
				(splitDocNodePos?: number) =>
				({ state, tr, dispatch }) => {
					const splitDocNodeAtPos =
						typeof splitDocNodePos === "number"
							? state.doc.nodeAt(splitDocNodePos)
							: null

					let splitDocPos: number | null = null
					let splitDocNode: PMNode | null = null

					if (splitDocNodeAtPos?.type.name === SplitDocumentation.name) {
						splitDocPos = splitDocNodePos ?? null
						splitDocNode = splitDocNodeAtPos
					} else {
						const { $from } = state.selection
						let splitDocDepth = -1
						for (let d = $from.depth; d >= 0; d--) {
							if ($from.node(d).type.name === SplitDocumentation.name) {
								splitDocDepth = d
								break
							}
						}

						if (splitDocDepth === -1) {
							return false
						}

						splitDocPos = $from.before(splitDocDepth)
						splitDocNode = $from.node(splitDocDepth)
					}

					if (splitDocPos == null) {
						return false
					}

					tr.setNodeMarkup(splitDocPos, undefined, {
						...splitDocNode.attrs,
						inversed: !splitDocNode.attrs.inversed,
					})

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},
			appendBlockOnRightSide:
				(rightSideNodePos: number, blockType: "code" | "metrics") =>
				({ state, tr, dispatch }) => {
					const { schema, doc } = state
					const rightNode = doc.nodeAt(rightSideNodePos)
					if (rightNode?.type.name !== SplitDocumentationRightSide.name) {
						return false
					}

					const titleNode = schema.nodes[CODE_BLOCK_TITLE_NAME]?.createAndFill()
					const extendedCode = schema.nodes[CODE_BLOCK_NAME]?.createAndFill()

					if (!titleNode || !extendedCode) {
						return false
					}

					let newBlock: PMNode | null | undefined = null

					if (blockType === "code") {
						newBlock = schema.nodes[TITLED_CODE_BLOCK_NAME]?.createAndFill(
							null,
							[titleNode, extendedCode],
						)
					} else {
						newBlock = schema.nodes[METRIC_BLOCK_NAME]?.createAndFill()
					}

					if (!newBlock) {
						return false
					}

					// insert just after the rightSide’s last element
					const insertPos = rightSideNodePos + rightNode.content.size + 1
					tr.insert(insertPos, newBlock)

					// metric blocks don’t have text content to select
					if (blockType === "code") {
						const newBlockPos = insertPos
						const textStart = newBlockPos + 1

						tr.setSelection(
							TextSelection.near(tr.doc.resolve(textStart), 1),
						).scrollIntoView()
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},

			/* This method expects nodePos to be the position of a ParameterList */
			insertParameterListOnLeftSide:
				(nodePos: number, side: "above" | "below") =>
				({ state, tr, dispatch }) => {
					const { schema, doc } = state

					// nodePos is the position of a ParameterList
					const node = doc.nodeAt(nodePos)
					if (node?.type.name !== ParameterList.name) {
						return false
					}

					// Resolve a position inside the node to find the parent LeftSide
					const $pos = doc.resolve(nodePos + 1)

					// Find the nearest ancestor SplitDocumentationLeftSide
					let leftSideDepth = -1
					for (let d = $pos.depth; d >= 0; d--) {
						if ($pos.node(d).type.name === SplitDocumentationLeftSide.name) {
							leftSideDepth = d
							break
						}
					}

					if (leftSideDepth === -1) {
						return false
					}

					const paramList = schema.nodes[ParameterList.name]?.createAndFill()
					if (!paramList) {
						return false
					}

					// Calculate the insert position based on side
					let insertPos: number
					if (side === "above") {
						// Insert before the current ParameterList
						insertPos = nodePos
					} else {
						// Insert after the current ParameterList
						insertPos = nodePos + node.nodeSize
					}

					tr.insert(insertPos, paramList)

					const textStart = insertPos + 1
					tr.setSelection(
						TextSelection.near(tr.doc.resolve(textStart), 1),
					).scrollIntoView()

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},

			/* This method expects nodePos to be the position of a ParameterListItem */
			insertParameterListItemOnLeftSide:
				(nodePos: number, side: "above" | "below") =>
				({ state, tr, dispatch }) => {
					const { schema, doc } = state

					// nodePos is the position of a ParameterListItem
					const node = doc.nodeAt(nodePos)
					if (node?.type.name !== ParameterListItem.name) {
						return false
					}

					// Resolve a position inside the node to find the parent ParameterList
					const $pos = doc.resolve(nodePos + 1)

					// Find the nearest ancestor ParameterList
					let paramListDepth = -1
					for (let d = $pos.depth; d >= 0; d--) {
						if ($pos.node(d).type.name === ParameterList.name) {
							paramListDepth = d
							break
						}
					}

					if (paramListDepth === -1) {
						return false
					}

					const paramListItem =
						schema.nodes[ParameterListItem.name]?.createAndFill()
					if (!paramListItem) {
						return false
					}

					// Calculate the insert position based on side
					let insertPos: number
					if (side === "above") {
						// Insert before the current ParameterListItem
						insertPos = nodePos
					} else {
						// Insert after the current ParameterListItem
						insertPos = nodePos + node.nodeSize
					}

					tr.insert(insertPos, paramListItem)

					const textStart = insertPos + 1
					tr.setSelection(
						TextSelection.near(tr.doc.resolve(textStart), 1),
					).scrollIntoView()

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},

			/* This method expects nodePos to be the position of a TitledCodeBlock or MetricBlock */
			insertBlockOnRightSide:
				(
					nodePos: number,
					side: "above" | "below",
					blockType: "code" | "metrics",
				) =>
				({ state, tr, dispatch }) => {
					const { schema, doc } = state

					// nodePos is the position of a TitledCodeBlock or MetricBlock
					const node = doc.nodeAt(nodePos)
					if (
						!node ||
						(node.type.name !== TITLED_CODE_BLOCK_NAME &&
							node.type.name !== METRIC_BLOCK_NAME)
					) {
						return false
					}

					// Resolve a position inside the node to find the parent RightSide
					// For atom nodes like MetricBlock, we resolve at nodePos itself
					const resolvePos = node.isAtom ? nodePos : nodePos + 1
					const $pos = doc.resolve(resolvePos)

					// Find the nearest ancestor SplitDocumentationRightSide
					let rightSideDepth = -1
					for (let d = $pos.depth; d >= 0; d--) {
						if ($pos.node(d).type.name === SplitDocumentationRightSide.name) {
							rightSideDepth = d
							break
						}
					}

					if (rightSideDepth === -1) {
						return false
					}

					const titleNode = schema.nodes[CODE_BLOCK_TITLE_NAME]?.createAndFill()
					const extendedCode = schema.nodes[CODE_BLOCK_NAME]?.createAndFill()

					if (!titleNode || !extendedCode) {
						return false
					}

					let newBlock: PMNode | null | undefined = null

					if (blockType === "code") {
						newBlock = schema.nodes[TITLED_CODE_BLOCK_NAME]?.createAndFill(
							null,
							[titleNode, extendedCode],
						)
					} else {
						newBlock = schema.nodes[METRIC_BLOCK_NAME]?.createAndFill()
					}

					if (!newBlock) {
						return false
					}

					// Calculate the insert position based on side
					let insertPos: number
					if (side === "above") {
						// Insert before the current TitledCodeBlock
						insertPos = nodePos
					} else {
						// Insert after the current TitledCodeBlock
						insertPos = nodePos + node.nodeSize
					}

					tr.insert(insertPos, newBlock)

					// metric blocks don’t have text content to select
					if (blockType === "code") {
						const textStart = insertPos + 1
						tr.setSelection(
							TextSelection.near(tr.doc.resolve(textStart), 1),
						).scrollIntoView()
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},
		}
	},
	addKeyboardShortcuts() {
		return {
			"Mod-e": ({ editor }) => {
				const { state } = editor
				const { selection } = state
				const $from = selection.$from

				// Find enclosing splitDocumentation node
				let splitDocDepth = -1
				for (let d = $from.depth; d >= 0; d--) {
					if ($from.node(d).type.name === this.name) {
						splitDocDepth = d
						break
					}
				}
				if (splitDocDepth === -1) {
					return false
				}

				const splitDocNode = $from.node(splitDocDepth)
				const splitDocPos = $from.before(splitDocDepth)

				// Figure out which child contains the cursor
				const index = $from.index(splitDocDepth) // child index inside splitDocumentation
				const child = splitDocNode.child(index)

				// Compute absolute pos of that child
				let offset = 0
				for (let i = 0; i < index; i++) {
					offset += splitDocNode.child(i).nodeSize
				}

				const childPos = splitDocPos + 1 + offset

				if (child.type.name === "splitDocumentationLeftSide") {
					return this.editor.commands.appendParameterListOnLeftSide(childPos)
				}

				if (child.type.name === "splitDocumentationRightSide") {
					return this.editor.commands.appendBlockOnRightSide(childPos, "code")
				}

				return false
			},
			"Mod-m": ({ editor }) => {
				const { state } = editor
				const { selection } = state
				const $from = selection.$from

				// Find enclosing splitDocumentation node
				let splitDocDepth = -1
				for (let d = $from.depth; d >= 0; d--) {
					if ($from.node(d).type.name === this.name) {
						splitDocDepth = d
						break
					}
				}
				if (splitDocDepth === -1) {
					return false
				}

				const splitDocNode = $from.node(splitDocDepth)
				const splitDocPos = $from.before(splitDocDepth)

				// Figure out which child contains the cursor
				const index = $from.index(splitDocDepth) // child index inside splitDocumentation
				const child = splitDocNode.child(index)

				// Compute absolute pos of that child
				let offset = 0
				for (let i = 0; i < index; i++) {
					offset += splitDocNode.child(i).nodeSize
				}

				const childPos = splitDocPos + 1 + offset

				if (child.type.name === "splitDocumentationRightSide") {
					return this.editor.commands.appendBlockOnRightSide(
						childPos,
						"metrics",
					)
				}

				return false
			},
			// when the cursor is inside the heading and the Enter key is
			// pressed, we don't want the cursor to create extra paragraphs,
			// especially if an empty one exists below
			Enter: ({ editor }) => {
				const { state } = editor
				const { $from } = state.selection
				const { schema } = state

				// only handle when inside a heading
				if ($from.parent.type.name !== Heading.name) {
					return false
				}

				// ensure the heading lives inside LeftSide
				const containerDepth = findAncestorDepth($from, [
					SplitDocumentationLeftSide.name,
				])
				if (containerDepth < 0) {
					return false
				}

				const container = $from.node(containerDepth)
				const headingDepth = containerDepth + 1 // heading is a direct child
				const headingIndex = $from.index(containerDepth) // index of heading in the container

				// position immediately after the heading, i.e. start of
				// the next sibling
				const posAfterHeading = $from.after(headingDepth)

				// if there is a next sibling inside the same container
				if (container.childCount > 1) {
					const nextNode = container.child(headingIndex + 1)

					if (
						nextNode.type.name === Paragraph.name &&
						nextNode.content.size === 0
					) {
						// next is an empty paragraph → move cursor there
						const tr = state.tr
							.setSelection(
								TextSelection.near(state.doc.resolve(posAfterHeading), 1),
							)
							.scrollIntoView()
						editor.view.dispatch(tr)

						return true
					}
				}

				const paragraphNode = schema.nodes[Paragraph.name]
				if (!paragraphNode) {
					return false
				}

				// next is not a paragraph (e.g. list) → insert a paragraph below the heading
				// or next is a non-empty paragraph → insert a new empty paragraph right below the heading
				const paragraph = paragraphNode.createAndFill()
				if (!paragraph) {
					return false
				}

				const tr = state.tr.insert(posAfterHeading, paragraph)
				tr.setSelection(
					TextSelection.near(tr.doc.resolve(posAfterHeading + 1), 1),
				)
				editor.view.dispatch(tr.scrollIntoView())

				return true
			},
		}
	},
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, { "data-type": "split-documentation" }),
			0,
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(MainBlock)
	},
})

export function setUpSplitDocumentationNode(
	state: EditorState,
	tr: Transaction,
	at: number,
): boolean {
	const { schema, doc } = state
	const pos = doc.resolve(at)

	// only trigger on a paragraph textblock that is
	// exactly "||"
	if (pos.parent.type !== schema.nodes[Paragraph.name]) {
		return false
	}

	const from = pos.before() // paragraph start
	const to = pos.after() // paragraph end

	const nodeType = schema.nodes[SplitDocumentation.name]
	if (!nodeType) {
		return false
	}

	const splitDoc = nodeType.createAndFill()
	if (!splitDoc) {
		return false
	}

	// single atomic TX: replace paragraph -> splitDocumentation
	tr.delete(from, to)
	tr.insert(from, splitDoc)

	// put the cursor just inside the new node
	tr.setSelection(TextSelection.near(tr.doc.resolve(from + 1)))
	tr.scrollIntoView()

	return true
}

export function insertSplitDocumentation(editor: Editor, range: Range) {
	editor
		.chain()
		.focus()
		.deleteRange(range)
		.insertSplitDocumentation(range.from)
		.setTextSelection(range.from)
		.run()
}

function findAncestorDepth(pos: ResolvedPos, names: string[]): number {
	for (let d = pos.depth; d >= 0; d--) {
		if (names.includes(pos.node(d).type.name)) {
			return d
		}
	}

	return -1
}

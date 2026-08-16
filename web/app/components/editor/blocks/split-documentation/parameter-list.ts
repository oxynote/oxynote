import { mergeAttributes, Node } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import ParameterListHeaderView from "./ParameterListHeader.vue"
import Paragraph from "@tiptap/extension-paragraph"
import { TextSelection } from "@tiptap/pm/state"
import { SplitDocumentation } from "."
import ParameterListItemHeaderTitleView from "./ParameterListItemHeaderTitle.vue"
import ParameterListItemHeaderTypeView from "./ParameterListItemHeaderType.vue"
import ParameterListItemHeaderView from "./ParameterListItemHeader.vue"
import { cn } from "~/lib/utils"
import { COMMENT_MARK_NAME } from "../../mark-names"
import {
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
} from "../node-names"
import { ParameterListSeparators } from "./parameter-list-separators"
import { deleteNode } from "../../tiptap-utils/node"

export const ParameterListItemHeaderType = Node.create({
	name: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
	group: "splitDocumentationParameterListItemHeaderType", // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `text*`,
	marks: COMMENT_MARK_NAME,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item-header-type"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-parameter-list-item-header-type",
			}),
			0,
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(ParameterListItemHeaderTypeView)
	},
})

export const ParameterListItemHeaderTitle = Node.create({
	name: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	group: "splitDocumentationParameterListItemHeaderTitle", // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `text*`,
	marks: COMMENT_MARK_NAME,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item-header-title"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-parameter-list-item-header-title",
			}),
			0,
		]
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

				if ($from.parent.type.name === this.name && $from.parentOffset === 0) {
					const d1 = $from.depth - 2
					const d2 = $from.depth - 3

					// This selects the ParameterListItem.
					const parent = $from.node(d1)

					if (parent.type.name !== ParameterListItem.name) {
						return false
					}

					// This selects the ParameterList.
					const parentParent = $from.node(d2)

					if (parentParent.type.name === ParameterList.name) {
						const sameTypeCount = parentParent.childCount
							? parentParent.content.content.filter(
									(child) => child.type.name === parent.type.name,
								).length
							: 0

						// In case this is the last element, we wan't to
						// avoid doing anything.
						if (sameTypeCount === 1) {
							return true
						}
					}

					// This handles the deletion of the ParameterListItem
					const posBefore = $from.before(d1)

					return deleteNode(editor, posBefore)
				}

				return false
			},
		}
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(ParameterListItemHeaderTitleView)
	},
})

export const ParameterListItemHeader = Node.create({
	name: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
	group: "splitDocumentationParameterListItemHeader", // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `${ParameterListItemHeaderTitle.name} ${ParameterListItemHeaderType.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item-header"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-parameter-list-item-header",
			}),
			0,
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(ParameterListItemHeaderView)
	},
})

export const ParameterListItem = Node.create({
	name: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	group: "splitDocumentationParameterListItem", // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `${ParameterListItemHeader.name} ${Paragraph.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="split-documentation-parameter-list-item"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-parameter-list-item",
				class: cn("[&.parameter-list-item]:prose-p:my-0 parameter-list-item"),
			}),
			0,
		]
	},
	addKeyboardShortcuts() {
		return {
			Enter: ({ editor }) => {
				const { state, view } = editor
				const { $from } = state.selection

				for (let d = $from.depth; d > 0; d--) {
					const node = $from.node(d)
					if (node.type.name === this.name) {
						const posAfter = $from.after(d)

						const newItem = this.type.createAndFill()
						if (!newItem) {
							return false
						}

						let tr = state.tr.insert(posAfter, newItem)

						const sel = TextSelection.near(tr.doc.resolve(posAfter + 1), 1)
						tr = tr.setSelection(sel).scrollIntoView()

						view.dispatch(tr)
						return true
					}
				}

				return false
			},
		}
	},
})

export const ParameterListHeader = Node.create({
	name: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	group: "splitDocumentationParameterListHeader", // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `text*`,
	marks: COMMENT_MARK_NAME,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{ tag: `div[data-type="split-documentation-parameter-list-header"]` },
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-parameter-list-header",
			}),
			0,
		]
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

				if ($from.parent.type.name === this.name && $from.parentOffset === 0) {
					const d1 = $from.depth - 1

					// This selects the ParameterList.
					const parent = $from.node(d1)

					if (parent.type.name !== ParameterList.name) {
						return false
					}

					// This handles the deletion of the ParameterList
					const posBefore = $from.before(d1)

					return deleteNode(editor, posBefore)
				}

				return false
			},
		}
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(ParameterListHeaderView)
	},
})

export const ParameterList = Node.create({
	name: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	group: "splitDocumentationParameterList", // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
	content: `${ParameterListHeader.name} ${ParameterListItem.name}+`,
	defining: true,
	isolating: true,
	selectable: false,
	addExtensions() {
		return [ParameterListSeparators]
	},
	parseHTML() {
		return [{ tag: `div[data-type="split-documentation-parameter-list"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation-parameter-list",
			}),
			0,
		]
	},
})

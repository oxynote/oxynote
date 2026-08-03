import {
	mergeAttributes,
	Node,
	type Range,
	type SingleCommands,
} from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import Paragraph from "@tiptap/extension-paragraph"
import { TaskList, BulletList, OrderedList } from "@tiptap/extension-list"
import MainBlock from "./MainBlock.vue"
import { deleteNode } from "../../tiptap-utils/node"
import { CALLOUT_BLOCK_NAME } from "../node-names"

const allowedContent = [
	Paragraph.name,
	BulletList.name,
	OrderedList.name,
	TaskList.name,
]

export const CalloutBlock = Node.create({
	name: CALLOUT_BLOCK_NAME,
	content: `(${allowedContent.join(" | ")})+`,
	defining: true,
	selectable: false,
	isolating: true,
	group: "block",
	parseHTML() {
		return [{ tag: `div[data-type="callout-block"]` }]
	},
	addAttributes() {
		return {
			icon: {
				default: "lucide:text",
				parseHTML: (element) => {
					return element.getAttribute("data-icon")
				},
				renderHTML: (attrs) => ({ "data-icon": attrs.icon }),
			},
			previousIcon: {
				default: null,
				parseHTML: (element) => {
					return element.getAttribute("data-previous-icon")
				},
				renderHTML: (attrs) => {
					if (attrs.previousIcon) {
						return { "data-previous-icon": attrs.previousIcon }
					}
					return {}
				},
			},
		}
	},
	addKeyboardShortcuts() {
		return {
			Backspace: ({ editor }) => {
				const { state } = editor
				const { selection } = state
				const { $from } = selection

				if ($from.parentOffset === 0) {
					const d1 = $from.depth - 1

					// This selects the callout block.
					const parent = $from.node(d1)

					if (parent.type.name !== CalloutBlock.name || parent.childCount > 1) {
						return false
					}

					// This handles the deletion of the CalloutBlock
					const posBefore = $from.before(d1)

					return deleteNode(editor, posBefore)
				}

				return false
			},
		}
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "callout-block",
			}),
			0,
		]
	},
	addNodeView() {
		return VueNodeViewRenderer(MainBlock)
	},
})

export function setUpCalloutBlockNode(range: Range, commands: SingleCommands) {
	commands.deleteRange(range)
	commands.insertContent({
		type: CalloutBlock.name,
		content: [
			{
				type: "paragraph",
			},
		],
	})
}

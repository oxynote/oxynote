import {
	mergeAttributes,
	Node,
	type Range,
	type SingleCommands,
} from "@tiptap/core"
import { Plugin } from "@tiptap/pm/state"
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
				renderHTML: (attrs) => ({ "data-icon": attrs.icon as string }),
			},
			previousIcon: {
				default: null,
				parseHTML: (element) => {
					return element.getAttribute("data-previous-icon")
				},
				renderHTML: (attrs) => {
					if (attrs.previousIcon) {
						return { "data-previous-icon": attrs.previousIcon as string }
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
	addProseMirrorPlugins() {
		return [
			new Plugin({
				props: {
					handleDOMEvents: {
						contextmenu: (_view, event) => {
							const target = event.target as HTMLElement | null
							const wrapper = target?.closest<HTMLElement>(
								"[data-node-view-wrapper]",
							)
							if (!wrapper) {
								return false
							}

							event.preventDefault()

							const rect = wrapper.getBoundingClientRect()
							wrapper.dispatchEvent(
								new MouseEvent("mousemove", {
									bubbles: true,
									clientX: rect.left + rect.width / 2,
									clientY: rect.top + rect.height / 2,
								}),
							)

							const handle = document.querySelector<HTMLElement>(
								"[data-drag-handle], .drag-handle",
							)
							const button = handle?.querySelector<HTMLElement>("button") ?? handle
							button?.click()

							return true
						},
					},
				},
			}),
		]
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
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
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

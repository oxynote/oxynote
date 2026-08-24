import { mergeAttributes, Node } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import { TaskList, BulletList, OrderedList } from "@tiptap/extension-list"

const allowedContent = [
	Paragraph.name,
	BulletList.name,
	OrderedList.name,
	TaskList.name,
]

export const CalloutBlock = Node.create({
	name: "calloutBlock",
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
				renderHTML: (attrs) => ({
					"data-icon": attrs.icon as string,
				}),
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
})

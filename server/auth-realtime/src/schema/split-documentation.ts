import { mergeAttributes, Node } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import Heading from "@tiptap/extension-heading"
import { TaskList, BulletList, OrderedList } from "@tiptap/extension-list"
import { ParameterList } from "./parameter-list.js"
import { CalloutBlock } from "./callout.js"
import { TitledCodeBlock } from "./code-block.js"
import { MetricBlock } from "./metric.js"

export const allowedLeftSideContent = [
	Paragraph.name,
	BulletList.name,
	OrderedList.name,
	TaskList.name,
	CalloutBlock.name,
]

export const extraLeftSideContent = [ParameterList.name]

export const SplitDocumentationLeftSide = Node.create({
	name: "splitDocumentationLeftSide",
	group: "block",
	isolating: false,
	defining: true,
	selectable: false,
	content: `${Heading.name} (${allowedLeftSideContent.join(" | ")})+ (${extraLeftSideContent.join(" | ")})*`,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-left-side"]`,
			},
		]
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
})

export const SplitDocumentationRightSide = Node.create({
	name: "splitDocumentationRightSide",
	group: "block",
	isolating: false,
	defining: true,
	selectable: false,
	content: `(${TitledCodeBlock.name} | ${MetricBlock.name})+`,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-right-side"]`,
			},
		]
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
})

export const SplitDocumentation = Node.create({
	name: "splitDocumentation",
	group: "block",
	content: `${SplitDocumentationLeftSide.name} ${SplitDocumentationRightSide.name}`,
	isolating: false,
	defining: true,
	selectable: false,
	addAttributes() {
		return {
			inversed: {
				default: false,
				parseHTML: (element) =>
					element.getAttribute(
						"data-inversed",
					) === "true",
				renderHTML: (attrs) => {
					if (!attrs.inversed) {
						return {}
					}

					return { "data-inversed": "true" }
				},
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
			mergeAttributes(HTMLAttributes, {
				"data-type": "split-documentation",
			}),
			0,
		]
	},
})

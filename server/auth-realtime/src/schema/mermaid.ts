import { mergeAttributes, Node } from "@tiptap/core"
import { CommentMark } from "./marks.js"

export const MermaidBlock = Node.create({
	name: "mermaidBlock",
	group: "block",
	content: "text*",
	marks: CommentMark.name,
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
})

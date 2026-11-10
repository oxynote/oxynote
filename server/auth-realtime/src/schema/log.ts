import { mergeAttributes, Node } from "@tiptap/core"

export const LogBlock = Node.create({
	name: "logBlock",
	group: "gridItem",
	atom: true,
	defining: true,
	selectable: false,
	isolating: true,
	addAttributes() {
		return {
			title: {},
			dataSourceId: {},
			queries: {},
			timeRange: {},
			refreshInterval: {},
			timestampFormat: {},
			order: {},
			width: {},
			height: {},
			widePosX: {},
			widePosY: {},
			stackPos: {},
		}
	},
	parseHTML() {
		return [{ tag: `div[data-type="log-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "log-block",
			}),
			0,
		]
	},
})

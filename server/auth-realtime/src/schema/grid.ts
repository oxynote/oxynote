import { mergeAttributes, Node } from "@tiptap/core"

export const BlockGrid = Node.create({
	name: "blockGrid",
	group: "rootOnlyBlock",
	// a grid may hold no blocks. The editor operation that empties a
	// grid deletes it.
	content: "gridItem*",
	isolating: true,
	draggable: false,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="block-grid"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "block-grid",
			}),
			0,
		]
	},
})

// the grid's former name. The editor turns it into a blockGrid on the
// first edit that touches it, but a protected branch is never edited, so
// documents keep it for good.
// NOTE: this is a legacy node type, and should be removed once all documents
// have been migrated to the new node type.
export const MetricGrid = Node.create({
	name: "metricGrid",
	group: "rootOnlyBlock",
	content: "gridItem*",
	isolating: true,
	draggable: false,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="metric-grid"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "metric-grid",
			}),
			0,
		]
	},
})

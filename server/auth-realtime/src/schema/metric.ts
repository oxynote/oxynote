import { mergeAttributes, Node } from "@tiptap/core"

export const MetricGrid = Node.create({
	name: "metricGrid",
	group: "block",
	content: "metricBlock*", // allow zero blocks; empty grids are auto-deleted by metricGridAutoManager
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

export const MetricBlock = Node.create({
	name: "metricBlock",
	group: "block",
	atom: true,
	defining: true,
	selectable: false,
	isolating: true,
	addAttributes() {
		return {
			config: {}, // NOTE: Old version.
			title: {},
			dataSourceId: {},
			visualizationType: {},
			queries: {},
			timeRange: {},
			refreshInterval: {},
			thresholds: {},
			baseThresholdColor: {},
			decimals: {},
			unitType: {},
			unitCustom: {},
			axisBoundsMin: {},
			axisBoundsMax: {},
			simulationPreset: {},
			width: {},
		}
	},
	parseHTML() {
		return [{ tag: `div[data-type="metric-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "metric-block",
			}),
			0,
		]
	},
})

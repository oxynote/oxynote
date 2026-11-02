import { mergeAttributes, Node } from "@tiptap/core"

export const MetricBlock = Node.create({
	name: "metricBlock",
	group: "gridItem",
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
			simulationActive: { default: false },
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

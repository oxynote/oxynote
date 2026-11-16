import { type Editor, Node, mergeAttributes, type Range } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import MainBlock from "./MainBlock.vue"
import { defaultMetricConfig } from "./utils"
import { GRID_ITEM_GROUP, METRIC_BLOCK_NAME } from "../node-names"
import {
	gridItemHeightAttribute,
	gridItemPlaceAttributes,
	gridItemWidthAttribute,
} from "../grid/utils"
import { insertGridItemCommand } from "../grid"

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		metricBlock: {
			insertMetricBlock: (at: number) => ReturnType
		}
	}
}

export const MetricBlock = Node.create({
	name: METRIC_BLOCK_NAME,
	group: GRID_ITEM_GROUP, // keeps it out of top-level content
	atom: true,
	defining: true,
	selectable: false,
	isolating: true,
	addAttributes() {
		const defaults = defaultMetricConfig()
		return {
			// legacy config attr (kept for backwards compat / migration)
			config: {
				default: null,
			},
			// individual config fields (split from MetricConfig for
			// per-field yjs merging and better performance)
			title: { default: defaults.title },
			dataSourceId: { default: defaults.dataSourceId },
			visualizationType: { default: defaults.visualizationType },
			queries: { default: defaults.queries },
			timeRange: { default: defaults.timeRange },
			refreshInterval: { default: defaults.refreshInterval },
			thresholds: { default: defaults.thresholds },
			baseThresholdColor: {
				default: defaults.baseThresholdColor,
			},
			decimals: { default: defaults.decimals },
			unitType: { default: defaults.unit.type },
			unitCustom: { default: defaults.unit.custom },
			axisBoundsMin: { default: defaults.axisBounds.min ?? null },
			axisBoundsMax: { default: defaults.axisBounds.max ?? null },
			simulationPreset: { default: defaults.simulationPreset },
			simulationActive: {
				default: defaults.simulationActive,
				rendered: false,
			},
			width: gridItemWidthAttribute(),
			height: gridItemHeightAttribute(),
			...gridItemPlaceAttributes(),
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
	addNodeView() {
		return VueNodeViewRenderer(MainBlock)
	},
	addCommands() {
		return {
			insertMetricBlock: insertGridItemCommand(METRIC_BLOCK_NAME),
		}
	},
})

export function insertMetricBlock(editor: Editor, range: Range) {
	editor.chain().focus().deleteRange(range).insertMetricBlock(range.from).run()
}

import { type Editor, Node, mergeAttributes, type Range } from "@tiptap/core"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import MainBlock from "./MainBlock.vue"
import { defaultLogConfig } from "./utils"
import { GRID_ITEM_GROUP, LOG_BLOCK_NAME } from "../node-names"
import {
	gridItemHeightAttribute,
	gridItemPlaceAttributes,
	gridItemWidthAttribute,
} from "../grid/utils"
import { insertGridItemCommand } from "../grid"

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		logBlock: {
			insertLogBlock: (at: number) => ReturnType
		}
	}
}

export const LogBlock = Node.create({
	name: LOG_BLOCK_NAME,
	group: GRID_ITEM_GROUP, // keeps it out of top-level content
	atom: true,
	defining: true,
	selectable: false,
	isolating: true,
	addAttributes() {
		const defaults = defaultLogConfig()

		return {
			title: { default: defaults.title },
			dataSourceId: { default: defaults.dataSourceId },
			queries: { default: defaults.queries },
			timeRange: { default: defaults.timeRange },
			refreshInterval: { default: defaults.refreshInterval },
			timestampFormat: { default: defaults.timestampFormat },
			order: { default: defaults.order },
			width: gridItemWidthAttribute(),
			height: gridItemHeightAttribute(),
			...gridItemPlaceAttributes(),
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
	addNodeView() {
		return VueNodeViewRenderer(MainBlock)
	},
	addCommands() {
		return {
			insertLogBlock: insertGridItemCommand(LOG_BLOCK_NAME),
		}
	},
})

export function insertLogBlock(editor: Editor, range: Range) {
	editor.chain().focus().deleteRange(range).insertLogBlock(range.from).run()
}

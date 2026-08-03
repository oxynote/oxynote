import { type Editor, Node, mergeAttributes, type Range } from "@tiptap/core"
import type { Node as PMNode, Schema } from "@tiptap/pm/model"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import Paragraph from "@tiptap/extension-paragraph"
import MainBlock from "./MainBlock.vue"
import GridView from "./GridView.vue"
import type { EditorState, Transaction } from "@tiptap/pm/state"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { defaultMetricConfig, MetricBlockWidth } from "./utils"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../node-names"

/**
 * Metadata key used to signal that empty MetricGrid cleanup has already been done
 * within the current transaction. This prevents the metricGridAutoManager plugin
 * from running duplicate cleanup in appendTransaction.
 */
export const METRIC_GRID_CLEANUP_DONE = "metricGridCleanupDone"

/**
 * Deletes all empty MetricGrid nodes from the document within the given transaction.
 * Also sets metadata to prevent duplicate cleanup by metricGridAutoManager.
 *
 * Use this when performing operations that may leave MetricGrids empty (e.g., drag-drop,
 * delete) to ensure cleanup happens in the same transaction and doesn't cause position
 * corruption from appendTransaction running in a separate transaction.
 *
 * @param tr - The transaction to apply deletions to
 * @param schema - The document schema
 * @returns The modified transaction
 */
export function deleteEmptyMetricGrids(
	tr: Transaction,
	schema: Schema,
): Transaction {
	const metricGridType = schema.nodes.metricGrid
	if (!metricGridType) {
		return tr
	}

	// Mark that we're handling cleanup explicitly
	tr.setMeta(METRIC_GRID_CLEANUP_DONE, true)

	// Collect empty grids to delete (positions in descending order)
	const emptyGrids: { pos: number; nodeSize: number }[] = []

	tr.doc.forEach((node, pos) => {
		if (node.type === metricGridType && node.childCount === 0) {
			emptyGrids.push({ pos, nodeSize: node.nodeSize })
		}
	})

	// Sort by position descending to preserve positions during deletions
	emptyGrids.sort((a, b) => b.pos - a.pos)

	// Delete empty grids
	for (const { pos, nodeSize } of emptyGrids) {
		tr.delete(pos, pos + nodeSize)
	}

	return tr
}

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		metricBlock: {
			insertMetricBlock: (at: number) => ReturnType
		}
	}
}

export const MetricGrid = Node.create({
	name: METRIC_GRID_NAME,
	group: "block",
	content: `${METRIC_BLOCK_NAME}*`, // allow zero blocks; empty grids are auto-deleted by metricGridAutoManager
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
	addNodeView() {
		return VueNodeViewRenderer(GridView)
	},
	addProseMirrorPlugins() {
		const metricGridName = this.name

		return [
			new Plugin({
				key: new PluginKey("metricGridAutoManager"),
				appendTransaction(transactions, _oldState, newState) {
					return null
					// Only process if document changed
					const docChanged = transactions.some((tr) => tr.docChanged)
					if (!docChanged) {
						return null
					}

					// Skip if cleanup was already done explicitly in the same transaction
					// (e.g., during drag-drop or delete operations)
					const alreadyCleaned = transactions.some((tr) =>
						tr.getMeta(METRIC_GRID_CLEANUP_DONE),
					)
					if (alreadyCleaned) {
						return null
					}

					const { schema, doc } = newState
					const metricGridType = schema.nodes[metricGridName]

					if (!metricGridType) {
						return null
					}

					const tr = newState.tr
					let modified = false

					// Collect empty grids to delete (positions in descending order)
					const emptyGrids: { pos: number; nodeSize: number }[] = []

					doc.forEach((node, pos) => {
						// Check for empty MetricGrids (can happen when all blocks are dragged out)
						if (node.type === metricGridType && node.childCount === 0) {
							emptyGrids.push({ pos, nodeSize: node.nodeSize })
						}
					})

					// Sort by position descending to preserve positions during deletions
					emptyGrids.sort((a, b) => b.pos - a.pos)

					// Delete empty grids
					for (const { pos, nodeSize } of emptyGrids) {
						tr.delete(pos, pos + nodeSize)
						modified = true
					}

					return modified ? tr : null
				},
			}),
		]
	},
})

export const MetricBlock = Node.create({
	name: METRIC_BLOCK_NAME,
	group: METRIC_BLOCK_NAME, // custom group to prevent it from being accepted elsewhere (e.g., top-level content)
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
			axisBoundsMin: { default: defaults.axisBounds?.min ?? null },
			axisBoundsMax: { default: defaults.axisBounds?.max ?? null },
			width: {
				default: MetricBlockWidth.Standard,
				parseHTML: (element) =>
					(element.getAttribute("data-width") as MetricBlockWidth) ||
					MetricBlockWidth.Standard,
				renderHTML: (attributes) => ({
					"data-width": attributes.width,
				}),
			},
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
			insertMetricBlock:
				(at: number) =>
				({ tr, state, dispatch }) => {
					const res = setUpMetricBlock(state, tr, at)
					if (!res) {
						return false
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},
		}
	},
})

export function setUpMetricBlock(
	state: EditorState,
	tr: Transaction,
	at: number,
): boolean {
	const { schema, doc } = state
	const pos = doc.resolve(at)

	// only trigger on a paragraph textblock
	if (pos.parent.type !== schema.nodes[Paragraph.name]) {
		return false
	}

	const from = pos.before() // paragraph start
	const to = pos.after() // paragraph end

	const metricBlockType = schema.nodes[METRIC_BLOCK_NAME]
	const metricGridType = schema.nodes[MetricGrid.name]

	if (!metricBlockType || !metricGridType) {
		return false
	}

	const metricBlock = metricBlockType.createAndFill()
	if (!metricBlock) {
		return false
	}

	// Check if we're at document root level (paragraph's parent is doc)
	// depth 1 = paragraph is direct child of doc
	const isAtRootLevel = pos.depth === 1

	let nodeToInsert: PMNode
	if (isAtRootLevel) {
		// Wrap MetricBlock in MetricGrid at root level
		const metricGrid = metricGridType.create(null, metricBlock)
		if (!metricGrid) {
			return false
		}

		nodeToInsert = metricGrid
	} else {
		// Insert MetricBlock directly when nested (e.g., inside
		// SplitDocumentation right side etc.)
		nodeToInsert = metricBlock
	}

	// single atomic TX: replace paragraph -> metrics (wrapped or not)
	tr.delete(from, to)
	tr.insert(from, nodeToInsert)

	return true
}

export function insertMetricBlock(editor: Editor, range: Range) {
	editor.chain().focus().deleteRange(range).insertMetricBlock(range.from).run()
}

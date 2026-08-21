import { type Editor, Node, mergeAttributes, type Range } from "@tiptap/core"
import type { Node as PMNode, Schema } from "@tiptap/pm/model"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import Paragraph from "@tiptap/extension-paragraph"
import MainBlock from "./MainBlock.vue"
import GridView from "./GridView.vue"
import type { EditorState, Transaction } from "@tiptap/pm/state"
import { defaultMetricConfig, MetricBlockWidth } from "./utils"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../node-names"

/**
 * Deletes all empty MetricGrid nodes from the document within the given transaction.
 *
 * This is the only cleanup mechanism for empty grids — there is no plugin
 * doing it automatically. Every operation that may leave a MetricGrid empty
 * (e.g., drag-drop, delete) must call this within the same transaction, so
 * the deletions cannot shift positions of a follow-up transaction.
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
	content: `${METRIC_BLOCK_NAME}*`, // allow zero blocks; emptying operations delete the grid via deleteEmptyMetricGrids
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
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(GridView)
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
			axisBoundsMin: { default: defaults.axisBounds.min ?? null },
			axisBoundsMax: { default: defaults.axisBounds.max ?? null },
			width: {
				default: MetricBlockWidth.Standard,
				parseHTML: (element) =>
					(element.getAttribute("data-width") as MetricBlockWidth | null) ||
					MetricBlockWidth.Standard,
				renderHTML: (attributes) => ({
					"data-width": attributes.width as MetricBlockWidth,
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
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(MainBlock)
	},
	addCommands() {
		return {
			insertMetricBlock:
				(at: number) =>
				({ tr, state, dispatch }) => {
					// a can() probe gets a throwaway transaction, so the shared
					// one is never left carrying the steps of a dry run
					if (!dispatch) {
						return setUpMetricBlock(state, state.tr, at)
					}

					const res = setUpMetricBlock(state, tr, at)
					if (!res) {
						return false
					}

					dispatch(tr)

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
		nodeToInsert = metricGridType.create(null, metricBlock)
	} else {
		// Insert MetricBlock directly when nested (e.g., inside
		// SplitDocumentation right side etc.)
		nodeToInsert = metricBlock
	}

	// insert before deleting: when the paragraph is the only child of its
	// parent, deleting it first empties the parent, prosemirror rejects
	// that step and the paragraph is left stranded beside the new node
	const insertedSize = nodeToInsert.nodeSize

	tr.insert(from, nodeToInsert)
	tr.delete(from + insertedSize, to + insertedSize)

	return true
}

export function insertMetricBlock(editor: Editor, range: Range) {
	editor.chain().focus().deleteRange(range).insertMetricBlock(range.from).run()
}

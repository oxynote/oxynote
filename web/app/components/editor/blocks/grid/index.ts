import {
	type Command,
	combineTransactionSteps,
	type Editor,
	getChangedRanges,
	mergeAttributes,
	Node,
} from "@tiptap/core"
import { isChangeOrigin } from "@tiptap/extension-collaboration"
import Paragraph from "@tiptap/extension-paragraph"
import type { Node as PMNode } from "@tiptap/pm/model"
import {
	type EditorState,
	Plugin,
	PluginKey,
	type Transaction,
} from "@tiptap/pm/state"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import GridView from "./GridView.vue"
import {
	BLOCK_GRID_NAME,
	GRID_ITEM_GROUP,
	LEGACY_METRIC_GRID_NAME,
	ROOT_ONLY_BLOCK_GROUP,
} from "../node-names"
import { BLOCK_GRID_NAMES } from "./utils"

const legacyGridMigrationKey = new PluginKey<boolean>("legacyGridMigration")

export const BlockGrid = Node.create({
	name: BLOCK_GRID_NAME,
	group: ROOT_ONLY_BLOCK_GROUP,
	// zero blocks keep emptying operations valid. They delete the grid
	// through deleteEmptyBlockGrids.
	content: `${GRID_ITEM_GROUP}*`,
	isolating: true,
	draggable: false,
	selectable: false,
	parseHTML() {
		return [
			{ tag: `div[data-type="block-grid"]` },
			// legacy: HTML copied before the rename
			{ tag: `div[data-type="metric-grid"]` },
		]
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
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(GridView)
	},
	addProseMirrorPlugins() {
		return [legacyGridMigration(this.editor)]
	},
})

// legacy: the grid's former name. It copies out as a block grid, so
// pasting one never creates the old type.
export const LegacyMetricGrid = BlockGrid.extend({
	name: LEGACY_METRIC_GRID_NAME,
	parseHTML() {
		return []
	},
	addProseMirrorPlugins() {
		return []
	},
})

// Deletes all empty grids from the document within the given transaction.
// This is the only cleanup mechanism for empty grids — there is no plugin
// doing it automatically. Every operation that may leave a grid empty
// (e.g., drag-drop, delete) must call this within the same transaction, so
// the deletions cannot shift positions of a follow-up transaction.
export function deleteEmptyBlockGrids(tr: Transaction): Transaction {
	const emptyGrids: { pos: number; nodeSize: number }[] = []

	tr.doc.forEach((node, pos) => {
		if (BLOCK_GRID_NAMES.has(node.type.name) && node.childCount === 0) {
			emptyGrids.push({ pos, nodeSize: node.nodeSize })
		}
	})

	// delete from the end so the earlier positions stay valid
	emptyGrids.sort((a, b) => b.pos - a.pos)

	for (const { pos, nodeSize } of emptyGrids) {
		tr.delete(pos, pos + nodeSize)
	}

	return tr
}

// setUpGridItem replaces the paragraph at the given position with an empty
// block of the given type. At the document root the block is wrapped in a
// new grid. A nested paragraph sits in a container that takes the block
// itself, such as the split documentation right side.
export function setUpGridItem(
	state: EditorState,
	tr: Transaction,
	at: number,
	blockName: string,
): boolean {
	const { schema, doc } = state
	const pos = doc.resolve(at)

	if (pos.parent.type !== schema.nodes[Paragraph.name]) {
		return false
	}

	const blockType = schema.nodes[blockName]
	const gridType = schema.nodes[BLOCK_GRID_NAME]

	if (!blockType || !gridType) {
		return false
	}

	const block = blockType.createAndFill()
	if (!block) {
		return false
	}

	const nodeToInsert = pos.depth === 1 ? gridType.create(null, block) : block
	const from = pos.before()
	const to = pos.after()

	// insert before deleting: when the paragraph is the only child of its
	// parent, deleting it first empties the parent, prosemirror rejects
	// that step and the paragraph is left stranded beside the new node
	const insertedSize = nodeToInsert.nodeSize

	tr.insert(from, nodeToInsert)
	tr.delete(from + insertedSize, to + insertedSize)

	return true
}

// insertGridItemCommand is the editor command that runs setUpGridItem
export function insertGridItemCommand(blockName: string) {
	return (at: number): Command =>
		({ tr, state, dispatch }) => {
			// a can() probe gets a throwaway transaction, so the shared
			// one is never left carrying the steps of a dry run
			if (!dispatch) {
				return setUpGridItem(state, state.tr, at, blockName)
			}

			if (!setUpGridItem(state, tr, at, blockName)) {
				return false
			}

			dispatch(tr)

			return true
		}
}

// legacyGridMigration turns every legacy grid that a local edit touches
// into a block grid, in the same transaction. A grid sits only at the
// document root, so only the root is checked. A remote edit is left alone.
// The client that made it converts the grid, so the grid is replaced once.
// Converting recreates the grid in the shared document, so two clients
// that edit one legacy grid at the same moment each create a copy.
function legacyGridMigration(editor: Editor) {
	return new Plugin<boolean>({
		key: legacyGridMigrationKey,
		// whether the document holds a root legacy grid. While it holds one,
		// every change recounts them. While it holds none, a local edit costs
		// nothing here, since only a remote change, such as the first sync, a
		// merge or an undo, can bring one in.
		state: {
			init: (_, state) => holdsLegacyGrid(state.doc),
			apply(tr, holds, oldState, newState) {
				if (!tr.docChanged) {
					return holds
				}

				if (holds) {
					return holdsLegacyGrid(newState.doc)
				}

				if (isChangeOrigin(tr)) {
					return holdsNewLegacyGrid(oldState.doc, newState.doc)
				}

				return false
			},
		},
		appendTransaction(transactions, oldState, newState) {
			const gridType = newState.schema.nodes[BLOCK_GRID_NAME]
			if (
				!legacyGridMigrationKey.getState(newState) ||
				!gridType ||
				!editor.isEditable ||
				!transactions.some((tr) => tr.docChanged) ||
				transactions.some((tr) => isChangeOrigin(tr))
			) {
				return null
			}

			const transform = combineTransactionSteps(oldState.doc, [...transactions])
			const legacyGridPositions = new Set<number>()

			for (const { newRange } of getChangedRanges(transform)) {
				newState.doc.nodesBetween(newRange.from, newRange.to, (node, pos) => {
					if (node.type.name === LEGACY_METRIC_GRID_NAME) {
						legacyGridPositions.add(pos)
					}

					// only the root's children are checked
					return false
				})
			}

			if (!legacyGridPositions.size) {
				return null
			}

			// a new type keeps the node's size, so every position stays valid
			const tr = newState.tr

			for (const pos of legacyGridPositions) {
				const node = newState.doc.nodeAt(pos)
				if (node) {
					tr.setNodeMarkup(pos, gridType, node.attrs)
				}
			}

			return tr
		},
	})
}

// holdsLegacyGrid reports whether the document holds a legacy grid at
// its root.
function holdsLegacyGrid(doc: PMNode): boolean {
	for (let i = 0; i < doc.childCount; i++) {
		if (doc.child(i).type.name === LEGACY_METRIC_GRID_NAME) {
			return true
		}
	}

	return false
}

// holdsNewLegacyGrid reports whether a remote change brought a legacy grid
// to the root. y-prosemirror rebuilds the whole document on a remote change
// but reuses the nodes that stay the same, so only new nodes are checked.
function holdsNewLegacyGrid(oldDoc: PMNode, newDoc: PMNode): boolean {
	const kept = new Set<PMNode>()

	oldDoc.forEach((child) => kept.add(child))

	for (let i = 0; i < newDoc.childCount; i++) {
		const child = newDoc.child(i)

		if (!kept.has(child) && child.type.name === LEGACY_METRIC_GRID_NAME) {
			return true
		}
	}

	return false
}

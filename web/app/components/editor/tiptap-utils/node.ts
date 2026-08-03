import type { Editor } from "@tiptap/core"
import { deleteEmptyMetricGrids } from "../blocks/metrics"

export function deleteNode(editor: Editor, nodePos: number): boolean {
	const { state, view } = editor
	let { tr } = state

	const node = state.doc.nodeAt(nodePos)
	if (!node) {
		return false
	}

	tr.delete(nodePos, nodePos + node.nodeSize)

	// Clean up empty MetricGrids in the same transaction to avoid
	// position corruption from appendTransaction
	tr = deleteEmptyMetricGrids(tr, state.schema)

	view.dispatch(tr)

	return true
}

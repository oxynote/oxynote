import type { Editor } from "@tiptap/core"
import { deleteEmptyBlockGrids } from "../blocks/grid"

export function deleteNode(editor: Editor, nodePos: number): boolean {
	const { state, view } = editor
	let { tr } = state

	const node = state.doc.nodeAt(nodePos)
	if (!node) {
		return false
	}

	tr.delete(nodePos, nodePos + node.nodeSize)

	// Clean up empty grids in the same transaction to avoid
	// position corruption from appendTransaction
	tr = deleteEmptyBlockGrids(tr)

	view.dispatch(tr)

	return true
}

import type { Editor, JSONContent } from "@tiptap/core"
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

// stripNode returns a copy of a node to use as a new block. The copy and
// every node inside it lose their uid, their comment and their comment
// marks.
export function stripNode(node: JSONContent): JSONContent {
	const nextNode: JSONContent = { ...node }

	if (nextNode.attrs && Object.hasOwn(nextNode.attrs, "uid")) {
		nextNode.attrs = { ...nextNode.attrs, uid: null }
	}

	if (nextNode.attrs && Object.hasOwn(nextNode.attrs, "nodeCommentId")) {
		nextNode.attrs = { ...nextNode.attrs, nodeCommentId: null }
	}

	if (nextNode.marks) {
		nextNode.marks = nextNode.marks.filter((mark) => mark.type !== "comment")
	}

	if (nextNode.content) {
		nextNode.content = nextNode.content.map(stripNode)
	}

	return nextNode
}

import type { Editor } from "@tiptap/core"
import type { Node as ProseMirrorNode } from "@tiptap/pm/model"
import type { Doc as YDoc } from "yjs"
import { deleteNode } from "../tiptap-utils/node"
import Collaboration from "@tiptap/extension-collaboration"

function getYDoc(editor: Editor): YDoc | null {
	const collaboration = editor.extensionManager.extensions.find(
		(e) => e.name === Collaboration.name,
	)

	const options = collaboration?.options as { document?: YDoc } | undefined

	return options?.document ?? null
}

function findNodeByUid(
	doc: ProseMirrorNode,
	uid: string,
): { node: ProseMirrorNode; pos: number } | null {
	let result: { node: ProseMirrorNode; pos: number } | null = null

	doc.descendants((node, pos) => {
		if (result) {
			return false
		}

		if (node.attrs.uid === uid) {
			result = { node, pos }
			return false
		}

		return true
	})

	return result
}

function stripNullAttrs(
	json: Record<string, unknown>,
): Record<string, unknown> {
	if (json.attrs) {
		const attrs = json.attrs as Record<string, unknown>
		const cleaned: Record<string, unknown> = {}

		for (const [key, value] of Object.entries(attrs)) {
			if (value !== null) {
				cleaned[key] = value
			}
		}

		json.attrs = Object.keys(cleaned).length > 0 ? cleaned : undefined
	}

	if (Array.isArray(json.content)) {
		json.content = json.content.map((child: Record<string, unknown>) =>
			stripNullAttrs(child),
		)
	}

	return json
}

function executeReadDocument(
	contentEditor: Editor,
	nameEditor: Editor,
): object {
	const ydoc = getYDoc(contentEditor)
	// toJSON is the typed alias of YText.toString, which the bundled yjs
	// declarations omit
	const icon = ydoc?.getText("icon").toJSON() ?? ""

	return {
		name: nameEditor.getText(),
		icon,
		editable: contentEditor.isEditable,
		content: stripNullAttrs(contentEditor.getJSON()),
	}
}

function executeInsertBlocks(
	contentEditor: Editor,
	args: InsertBlocksArgs,
): object {
	if (!contentEditor.isEditable) {
		return { error: "document is not editable" }
	}

	const found = findNodeByUid(contentEditor.state.doc, args.reference_uid)
	if (!found) {
		return { error: `block with uid "${args.reference_uid}" not found` }
	}

	const insertPos =
		args.position === InsertPosition.Before
			? found.pos
			: found.pos + found.node.nodeSize

	contentEditor.chain().insertContentAt(insertPos, args.blocks).run()
	return { success: true }
}

function executeReplaceBlockContent(
	contentEditor: Editor,
	args: ReplaceBlockContentArgs,
): object {
	if (!contentEditor.isEditable) {
		return { error: "document is not editable" }
	}

	const found = findNodeByUid(contentEditor.state.doc, args.uid)
	if (!found) {
		return { error: `block with uid "${args.uid}" not found` }
	}

	contentEditor
		.chain()
		.insertContentAt(
			{ from: found.pos + 1, to: found.pos + found.node.nodeSize - 1 },
			args.content,
		)
		.run()

	return { success: true }
}

function executeReplaceBlockAttributes(
	contentEditor: Editor,
	args: ReplaceBlockAttributesArgs,
): object {
	if (!contentEditor.isEditable) {
		return { error: "document is not editable" }
	}

	const found = findNodeByUid(contentEditor.state.doc, args.uid)
	if (!found) {
		return { error: `block with uid "${args.uid}" not found` }
	}

	const { tr } = contentEditor.state
	tr.setNodeMarkup(found.pos, undefined, {
		...found.node.attrs,
		...args.attributes,
	})
	contentEditor.view.dispatch(tr)

	return { success: true }
}

function executeUpdateDocumentName(
	nameEditor: Editor,
	args: UpdateDocumentNameArgs,
): object {
	if (!nameEditor.isEditable) {
		return { error: "document is not editable" }
	}

	nameEditor.commands.setContent(args.name)

	return { success: true }
}

function executeUpdateDocumentIcon(
	contentEditor: Editor,
	args: UpdateDocumentIconArgs,
): object {
	if (!contentEditor.isEditable) {
		return { error: "document is not editable" }
	}

	const ydoc = getYDoc(contentEditor)
	if (!ydoc) {
		return { error: "collaborative document not available" }
	}

	const iconFragment = ydoc.getText("icon")
	iconFragment.delete(0, iconFragment.length)
	iconFragment.insert(0, args.icon)

	return { success: true }
}

function executeDeleteBlocks(
	contentEditor: Editor,
	args: DeleteBlocksArgs,
): object {
	if (!contentEditor.isEditable) {
		return { error: "document is not editable" }
	}

	const positions: number[] = []
	for (const uid of args.uids) {
		const found = findNodeByUid(contentEditor.state.doc, uid)
		if (found) {
			positions.push(found.pos)
		}
	}

	// Sort descending so deletions don't shift earlier positions
	positions.sort((a, b) => b - a)

	for (const pos of positions) {
		deleteNode(contentEditor, pos)
	}

	return { success: true, deleted: positions.length }
}

function executeReadAvailableIcons(): object {
	return { icons: selectableIconList() }
}

export function executeTool(
	contentEditor: Editor,
	nameEditor: Editor,
	tool: AIToolName,
	args: ExecuteToolArgs,
): object {
	switch (tool) {
		case AIToolName.ReadDocument:
			return executeReadDocument(contentEditor, nameEditor)
		case AIToolName.InsertBlocks:
			return executeInsertBlocks(contentEditor, args as InsertBlocksArgs)
		case AIToolName.ReplaceBlockContent:
			return executeReplaceBlockContent(
				contentEditor,
				args as ReplaceBlockContentArgs,
			)
		case AIToolName.ReplaceBlockAttributes:
			return executeReplaceBlockAttributes(
				contentEditor,
				args as ReplaceBlockAttributesArgs,
			)
		case AIToolName.ReadAvailableIcons:
			return executeReadAvailableIcons()
		case AIToolName.UpdateDocumentName:
			return executeUpdateDocumentName(
				nameEditor,
				args as UpdateDocumentNameArgs,
			)
		case AIToolName.UpdateDocumentIcon:
			return executeUpdateDocumentIcon(
				contentEditor,
				args as UpdateDocumentIconArgs,
			)
		case AIToolName.DeleteBlocks:
			return executeDeleteBlocks(contentEditor, args as DeleteBlocksArgs)
		default:
			return { error: `unknown tool: ${String(tool)}` }
	}
}

import type { HocuspocusProvider } from "@hocuspocus/provider"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/core"
import { Extension } from "@tiptap/core"
import { Plugin, Selection, TextSelection } from "@tiptap/pm/state"
import { isChangeOrigin } from "@tiptap/extension-collaboration"
import { cn } from "~/lib/utils"
import CollaborationCaret from "@tiptap/extension-collaboration-caret"

export function renderCollaborationCaret(
	user: Record<string, any>,
): HTMLElement {
	const caret = document.createElement("span")
	caret.className = cn(
		"border-l border-r border-solid -mx-px pointer-events-none relative break-normal",
	)
	caret.setAttribute("style", `border-color: ${user.color}`)

	const label = document.createElement("div")
	label.className = cn(
		"z-5 absolute font-medium text-white text-xs rounded-sm -left-px -top-[1.1em] whitespace-nowrap px-[0.1rem] pointer-events-none select-none caret-transparent",
	)
	label.setAttribute("style", `background-color: ${user.color}`)
	label.insertBefore(document.createTextNode(user.name), null)
	caret.insertBefore(label, null)

	return caret
}

// drag awareness helpers for collaborative drag conflict resolution.
// these functions allow broadcasting which node a user is currently dragging
// and detecting if other users are dragging a specific node.

/**
 * Set the currently dragged node UID in awareness state.
 * Call with null to clear the dragging state.
 */
export function setDraggingNodeInAwareness(
	provider: HocuspocusProvider | null | undefined,
	uid: string | null,
): void {
	if (!provider?.awareness) {
		return
	}

	provider.awareness.setLocalStateField("draggingNodeUid", uid)
}

/**
 * Get the list of node UIDs that other users are currently dragging.
 */
export function otherUserDraggingUids(
	provider: HocuspocusProvider | null | undefined,
): Set<string> {
	if (!provider?.awareness) {
		return new Set()
	}

	const uids = new Set<string>()
	const localClientId = provider.awareness.clientID

	provider.awareness.getStates().forEach((state, clientId) => {
		if (clientId !== localClientId && state.draggingNodeUid) {
			uids.add(state.draggingNodeUid)
		}
	})

	return uids
}

/**
 * Check if a specific node UID is being dragged by another user.
 */
export function isNodeBeingDraggedByOther(
	provider: HocuspocusProvider | null | undefined,
	uid: string,
): boolean {
	return otherUserDraggingUids(provider).has(uid)
}

/**
 * Get the caret color of a remote user by their client ID.
 * Returns null if the user is not found or has no color set.
 */
export function findRemoteUserCaretColor(
	provider: HocuspocusProvider | null,
	clientId: number,
): string | null {
	if (!provider?.awareness) {
		return null
	}

	const state = provider.awareness.getStates().get(clientId)
	return state?.user?.color ?? null
}

// editing awareness helpers for collaborative edit conflict resolution.
// these functions allow broadcasting which node a user is currently editing
// and detecting if other users are editing a specific node or its descendants.

export interface EditingNodeInfo {
	uid: string
	type: string
}

export function isEditingNodeInAwarenessOfType(
	provider: HocuspocusProvider | null,
	type: string,
): boolean {
	if (!provider?.awareness) {
		return false
	}

	const localState = provider.awareness.getLocalState()
	return localState?.editingNodeUid?.type === type
}

/**
 * Set the currently edited node in awareness state.
 * Call with null to clear the editing state.
 */
export function setEditingNodeInAwareness(
	provider: HocuspocusProvider | null,
	info: EditingNodeInfo | null,
): void {
	if (!provider?.awareness) {
		return
	}

	provider.awareness.setLocalStateField("editingNodeUid", info)
}

// clears the editing awareness state only if the current local state
// matches the given info. This prevents accidentally clearing
// awareness that was set by a different node.
export function deleteEditingNodeInAwareness(
	provider: HocuspocusProvider | null,
	info: EditingNodeInfo,
): void {
	if (!provider?.awareness) {
		return
	}

	const localState = provider.awareness.getLocalState()
	const current = localState?.editingNodeUid

	if (current?.uid === info.uid && current?.type === info.type) {
		setEditingNodeInAwareness(provider, null)
	}
}

/**
 * Get the set of node UIDs that other users are currently editing.
 */
export function otherUserEditingUids(
	provider: HocuspocusProvider | null | undefined,
): Set<string> {
	if (!provider?.awareness) {
		return new Set()
	}

	const uids = new Set<string>()
	const localClientId = provider.awareness.clientID

	provider.awareness.getStates().forEach((state, clientId) => {
		if (clientId !== localClientId && state.editingNodeUid) {
			uids.add(state.editingNodeUid.uid)
		}
	})

	return uids
}

/**
 * Get the editing node info (uid and type) for all nodes being edited by other users.
 */
export function otherUserEditingNodes(
	provider: HocuspocusProvider | null,
): EditingNodeInfo[] {
	if (!provider?.awareness) {
		return []
	}

	const nodes: EditingNodeInfo[] = []
	const localClientId = provider.awareness.clientID

	provider.awareness.getStates().forEach((state, clientId) => {
		if (clientId !== localClientId && state.editingNodeUid) {
			nodes.push(state.editingNodeUid)
		}
	})

	return nodes
}

export interface EditingUser {
	name: string
	color: string
}

/**
 * Get all remote users editing a specific node by its UID.
 * Returns an array of users with their names and colors.
 * Returns an empty array if no other users are editing the node.
 */
export function findEditingUsersByNodeUid(
	provider: HocuspocusProvider | null,
	uid: string,
): EditingUser[] {
	if (!provider?.awareness) {
		return []
	}

	const users: EditingUser[] = []
	const localClientId = provider.awareness.clientID

	for (const [clientId, state] of provider.awareness.getStates()) {
		if (clientId !== localClientId && state.editingNodeUid?.uid === uid) {
			const user = state.user
			if (user?.name && user?.color) {
				users.push({ name: user.name, color: user.color })
			}
		}
	}

	return users
}

/**
 * Check if a node or any of its descendants are being edited by another user.
 * This checks the node at nodePos and recursively checks all child nodes.
 */
export function isNodeOrDescendantsBeingEditedByOther(
	provider: HocuspocusProvider | null | undefined,
	doc: PMNode,
	nodePos: number,
): boolean {
	const editingUids = otherUserEditingUids(provider)
	if (editingUids.size === 0) {
		return false
	}

	const node = doc.nodeAt(nodePos)
	if (!node) {
		return false
	}

	// check the node itself
	const nodeUid = node.attrs?.uid
	if (nodeUid && editingUids.has(nodeUid)) {
		return true
	}

	// check all descendants
	let found = false
	node.descendants((childNode) => {
		if (found) {
			return false // stop traversal
		}

		const childUid = childNode.attrs?.uid
		if (childUid && editingUids.has(childUid)) {
			found = true
			return false
		}

		return true
	})

	return found
}

/**
 * Check if any node in a range (or its descendants) is being edited by another user.
 */
export function isRangeBeingEditedByOther(
	provider: HocuspocusProvider | null,
	doc: PMNode,
	from: number,
	to: number,
): boolean {
	const editingUids = otherUserEditingUids(provider)
	if (editingUids.size === 0) {
		return false
	}

	let found = false
	doc.nodesBetween(from, to, (node) => {
		if (found) {
			return false
		}

		const nodeUid = node.attrs?.uid
		if (nodeUid && editingUids.has(nodeUid)) {
			found = true
			return false
		}

		return true
	})

	return found
}

/**
 * Extract the HocuspocusProvider from an editor instance via the CollaborationCaret extension.
 * Prefer passing the provider around directly where possible, this is a last
 * resort type of function.
 */
export function extractHocuspocusProviderFromEditor(
	editor: Editor,
): HocuspocusProvider | null {
	const ext = editor.extensionManager.extensions.find(
		(e) => e.name === CollaborationCaret.name,
	)

	return ext?.options?.provider ?? null
}

/**
 * Extension that safeguards the local selection when remote changes delete
 * the node containing the cursor. This prevents RangeError errors when
 * one user deletes a node while another is editing inside it.
 */
export const RemoteDeleteSelectionGuard = Extension.create({
	name: "remoteDeleteSelectionGuard",
	addProseMirrorPlugins() {
		return [
			new Plugin({
				appendTransaction(transactions, _oldState, newState) {
					// only handle remote changes
					const hasRemoteChange = transactions.some(
						(tr) => tr.docChanged && isChangeOrigin(tr),
					)
					if (!hasRemoteChange) {
						return null
					}

					// check if current selection position is still valid
					const { from, to } = newState.selection
					try {
						newState.doc.resolve(from)
						if (from !== to) {
							newState.doc.resolve(to)
						}

						return null
					} catch {
						// selection is invalid - find nearest valid position
						const safePos = Math.min(from, newState.doc.content.size)
						try {
							const $pos = newState.doc.resolve(safePos)
							const selection = Selection.near($pos)
							return newState.tr.setSelection(selection)
						} catch {
							// fallback to start of document
							return newState.tr.setSelection(
								TextSelection.create(newState.doc, 1),
							)
						}
					}
				},
			}),
		]
	},
})

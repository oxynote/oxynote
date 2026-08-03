import type { HocuspocusProvider } from "@hocuspocus/provider"
import { Editor } from "@tiptap/core"
import {
	extractHocuspocusProviderFromEditor,
	findEditingUsersByNodeUid,
	otherUserDraggingUids,
	otherUserEditingNodes,
	otherUserEditingUids,
	type EditingNodeInfo,
	type EditingUser,
} from "~/components/editor/collaboration"

/**
 * Composable for reactive access to collaboration awareness state.
 * Automatically updates when remote users' awareness state changes.
 */
export function useCollaborationAwareness(
	editorOrProvider: MaybeRefOrGetter<Editor | HocuspocusProvider | null>,
) {
	const awarenessVersion = ref(0)

	const provider = computed<HocuspocusProvider | null>(() => {
		const value = toValue(editorOrProvider)
		if (!value) return null

		if (value instanceof Editor) {
			return extractHocuspocusProviderFromEditor(value)
		}

		return value
	})

	watchImmediate(provider, (newProvider, _, onCleanup) => {
		if (!newProvider?.awareness) {
			return
		}

		const handler = () => {
			awarenessVersion.value++
		}

		newProvider.awareness.on("change", handler)
		onCleanup(() => newProvider.awareness?.off("change", handler))
	})

	/**
	 * Get all remote users editing a specific node by its UID.
	 */
	function editingUsersRef(
		uid: MaybeRefOrGetter<string>,
	): ComputedRef<EditingUser[]> {
		return computed(() => {
			// eslint-disable-next-line @typescript-eslint/no-unused-expressions
			awarenessVersion.value // reactive dependency
			return findEditingUsersByNodeUid(provider.value, toValue(uid))
		})
	}

	/**
	 * Get the set of node UIDs being edited by other users.
	 */
	const editingUids = computed(() => {
		// eslint-disable-next-line @typescript-eslint/no-unused-expressions
		awarenessVersion.value // reactive dependency
		return otherUserEditingUids(provider.value)
	})

	/**
	 * Get the editing node info (uid and type) for all nodes being edited by other users.
	 */
	const editingNodes = computed<EditingNodeInfo[]>(() => {
		// eslint-disable-next-line @typescript-eslint/no-unused-expressions
		awarenessVersion.value // reactive dependency
		return otherUserEditingNodes(provider.value)
	})

	/**
	 * Get the set of node UIDs being dragged by other users.
	 */
	const draggingUids = computed(() => {
		// eslint-disable-next-line @typescript-eslint/no-unused-expressions
		awarenessVersion.value // reactive dependency
		return otherUserDraggingUids(provider.value)
	})

	/**
	 * Check if a specific node is being edited by another user.
	 */
	function nodeBeingEditedRef(
		uid: MaybeRefOrGetter<string>,
	): ComputedRef<boolean> {
		return computed(() => {
			// eslint-disable-next-line @typescript-eslint/no-unused-expressions
			awarenessVersion.value // reactive dependency
			return otherUserEditingUids(provider.value).has(toValue(uid))
		})
	}

	/**
	 * Check if a specific node is being dragged by another user.
	 */
	function nodeBeingDraggedRef(
		uid: MaybeRefOrGetter<string>,
	): ComputedRef<boolean> {
		return computed(() => {
			// eslint-disable-next-line @typescript-eslint/no-unused-expressions
			awarenessVersion.value // reactive dependency
			return otherUserDraggingUids(provider.value).has(toValue(uid))
		})
	}

	return {
		provider,
		awarenessVersion,
		editingUids,
		editingNodes,
		draggingUids,
		editingUsersRef,
		nodeBeingEditedRef,
		nodeBeingDraggedRef,
	}
}

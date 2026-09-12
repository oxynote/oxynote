export default function () {
	// the reader's own read/edit choice. Whether the page can actually be
	// edited also depends on the branch, so consumers read isEditable
	const editablePreference = usePersistentState<boolean>({
		key: "editor-editable",
		defaultValue: true,
	})

	const editorStore = useEditorStore()

	const isEditable = computed(
		() => editablePreference.value && !editorStore.activeBranchProtected,
	)

	function toggleIsEditable() {
		editablePreference.value = !editablePreference.value
	}

	function setEditable(v: boolean) {
		editablePreference.value = v
	}

	function updateLock(v: boolean) {
		editorStore.updateLock(v)
	}

	return {
		isEditable,
		toggleIsEditable,
		setEditable,
		isBranchProtected: computed(() => editorStore.activeBranchProtected),
		isLocked: computed(() => editorStore.locked), // locks are used by drag handle menus
		updateLock,
		isEditableAndUnlocked: computed(
			() => isEditable.value && !editorStore.locked,
		),
	}
}

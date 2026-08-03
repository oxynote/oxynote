export default function () {
	const isEditable = usePersistentState<boolean>({
		key: "editor-editable",
		defaultValue: true,
	})

	const editorStore = useEditorStore()

	function toggleIsEditable() {
		isEditable.value = !isEditable.value
	}

	function setEditable(v: boolean) {
		isEditable.value = v
	}

	function updateLock(v: boolean) {
		editorStore.updateLock(v)
	}

	return {
		isEditable: readonly(isEditable),
		toggleIsEditable,
		setEditable,
		isLocked: computed(() => editorStore.locked), // locks are used by drag handle menus
		updateLock,
		isEditableAndUnlocked: computed(
			() => isEditable.value && !editorStore.locked,
		),
	}
}

import { showToastMessage } from "~/components/toast"

// useHookActions sends the requests a hook's menu makes. Each one closes
// the menu before it goes out, raises a toast when it fails, and resolves
// to whether it went through.
export function useHookActions(options: {
	type: DocumentHookType
	nodeId: () => string | null
	hook: () => DocumentHook | null | undefined
	close: () => void
}) {
	const { t } = useI18n({ useScope: "global" })
	const editorStore = useEditorStore()
	const documentHookAPI = useDocumentHookAPI()

	async function create(settings: DocumentHookSettings): Promise<boolean> {
		const docId = editorStore.activeDocumentId
		const branchId = editorStore.activeBranchId

		if (!docId || !branchId) {
			return false
		}

		options.close()

		try {
			await documentHookAPI.createDocumentHookByDocID.mutateAsync({
				docId: docId,
				req: {
					type: options.type,
					branchId: branchId,
					blockId: options.nodeId(),
					settings: settings,
				},
			})
		} catch {
			showToastMessage("error", t("editor.hooks.errors.create-failed"))
			return false
		}

		return true
	}

	// a renewal is an update that moves a reminder's date, and its toast
	// says so
	async function update(
		settings: DocumentHookSettings,
		renewal = false,
	): Promise<boolean> {
		const hook = options.hook()
		const docId = editorStore.activeDocumentId
		const branchId = editorStore.activeBranchId

		if (!hook || !docId || !branchId) {
			return false
		}

		options.close()

		try {
			await documentHookAPI.updateDocumentHookByDocID.mutateAsync({
				docId: docId,
				branchId: branchId,
				hookId: hook.id,
				req: { settings: settings },
			})
		} catch {
			showToastMessage(
				"error",
				renewal
					? t("editor.hooks.errors.renew-failed")
					: t("editor.hooks.errors.update-failed"),
			)
			return false
		}

		return true
	}

	async function remove(): Promise<boolean> {
		const hook = options.hook()
		const docId = editorStore.activeDocumentId
		const branchId = editorStore.activeBranchId

		if (!hook || !docId || !branchId) {
			return false
		}

		options.close()

		try {
			await documentHookAPI.deleteDocumentHookByDocID.mutateAsync({
				docId: docId,
				branchId: branchId,
				hookId: hook.id,
			})
		} catch {
			showToastMessage("error", t("editor.hooks.errors.delete-failed"))
			return false
		}

		return true
	}

	async function reset(): Promise<boolean> {
		const hook = options.hook()
		const docId = editorStore.activeDocumentId
		const branchId = editorStore.activeBranchId

		if (!hook || !docId || !branchId) {
			return false
		}

		options.close()

		try {
			await documentHookAPI.resetDocumentHookByDocID.mutateAsync({
				docId: docId,
				branchId: branchId,
				hookId: hook.id,
			})
		} catch {
			showToastMessage("error", t("editor.hooks.errors.reset-failed"))
			return false
		}

		return true
	}

	return { create, update, remove, reset }
}

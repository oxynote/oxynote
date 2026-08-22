import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import DocumentDeletionModal from "./DocumentDeletionModal.vue"
import { clearTeleportedOverlays, t, teleportedButton } from "./test-helpers"

function mountModal(
	target: { deleteDocument: () => Promise<void>; name: string } | null,
) {
	return mountSuspended(DocumentDeletionModal, {
		props: { modelValue: target },
	})
}

// the dialog body is teleported into <body>, so it is the only place the
// modal's own markup can be read from
function dialog() {
	return document.body.querySelector("[data-slot='dialog-content']")
}

// the dialog body is teleported into the shared <body> and the delete flow
// is driven by the global fake timers, so these tests cannot interleave
// t() reaches for the nuxt app context, so the label can only be
// resolved inside a test — not once at module scope
function confirmButton() {
	return teleportedButton(t("editor.document-deletion-modal.confirm-button"))
}

describe("<DocumentDeletionModal>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("stays closed while there is no deletion target", async ({ expect }) => {
		await mountModal(null)

		expect(dialog()).toBeNull()
	})

	it("names the document it is about to delete", async ({ expect }) => {
		await mountModal({ name: "Runbook", deleteDocument: vi.fn() })

		expect(dialog()?.textContent).toContain("Runbook")
	})

	it("deletes the document when the confirm button is pressed", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const deleteDocument = vi.fn().mockResolvedValue(undefined)
		const wrapper = await mountModal({ name: "Runbook", deleteDocument })

		confirmButton().click()
		await vi.advanceTimersByTimeAsync(300)

		expect(deleteDocument).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("shows a spinner and disables both buttons while deleting", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const deleteDocument = vi.fn().mockResolvedValue(undefined)
		await mountModal({ name: "Runbook", deleteDocument })

		confirmButton().click()
		await nextTick()

		expect(
			dialog()?.querySelector(".i-svg-spinners\\:blocks-shuffle-3"),
		).not.toBeNull()
		expect(confirmButton().disabled).toBe(true)
		expect(
			teleportedButton(t("editor.document-deletion-modal.cancel-button"))
				.disabled,
		).toBe(true)
	})

	it("closes without deleting when the cancel button is pressed", async ({
		expect,
	}) => {
		const deleteDocument = vi.fn()
		const wrapper = await mountModal({ name: "Runbook", deleteDocument })

		teleportedButton(t("editor.document-deletion-modal.cancel-button")).click()
		await nextTick()

		expect(deleteDocument).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("closes without deleting when the close button is pressed", async ({
		expect,
	}) => {
		const deleteDocument = vi.fn()
		const wrapper = await mountModal({ name: "Runbook", deleteDocument })

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(deleteDocument).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("keeps showing the document name while the dialog animates shut", async ({
		expect,
	}) => {
		const wrapper = await mountModal({
			name: "Runbook",
			deleteDocument: vi.fn(),
		})

		await wrapper.setProps({ modelValue: null })

		expect(document.body.textContent).toContain("Runbook")
	})
})

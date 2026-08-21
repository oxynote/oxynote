import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import DocumentDeletionModal from "./DocumentDeletionModal.vue"
import { clearTeleportedOverlays, teleportedButton } from "./test-helpers"

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

		teleportedButton("Delete").click()
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

		teleportedButton("Delete").click()
		await nextTick()

		expect(
			dialog()?.querySelector(".i-svg-spinners\\:blocks-shuffle-3"),
		).not.toBeNull()
		expect(teleportedButton("Delete").disabled).toBe(true)
		expect(teleportedButton("Cancel").disabled).toBe(true)
	})

	it("closes without deleting when the cancel button is pressed", async ({
		expect,
	}) => {
		const deleteDocument = vi.fn()
		const wrapper = await mountModal({ name: "Runbook", deleteDocument })

		teleportedButton("Cancel").click()
		await nextTick()

		expect(deleteDocument).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("closes without deleting when the close button is pressed", async ({
		expect,
	}) => {
		const deleteDocument = vi.fn()
		const wrapper = await mountModal({ name: "Runbook", deleteDocument })

		teleportedButton("Close").click()
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

import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import TagDeletionModal from "./TagDeletionModal.vue"
import { clearTeleportedOverlays, t, teleportedButton } from "./test-helpers"

function mountModal(
	target: { deleteTag: () => Promise<void>; name: string } | null,
) {
	return mountSuspended(TagDeletionModal, {
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
	return teleportedButton(t("sidebar.tag-deletion-modal.confirm-button"))
}

describe("<TagDeletionModal>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("stays closed while there is no deletion target", async ({ expect }) => {
		await mountModal(null)

		expect(dialog()).toBeNull()
	})

	it("names the tag it is about to delete", async ({ expect }) => {
		await mountModal({ name: "Production", deleteTag: vi.fn() })

		expect(dialog()?.textContent).toContain("Production")
	})

	it("deletes the tag when the confirm button is pressed", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const deleteTag = vi.fn().mockResolvedValue(undefined)
		const wrapper = await mountModal({ name: "Production", deleteTag })

		confirmButton().click()
		await vi.advanceTimersByTimeAsync(300)

		expect(deleteTag).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("shows a spinner and disables both buttons while deleting", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const deleteTag = vi.fn().mockResolvedValue(undefined)
		await mountModal({ name: "Production", deleteTag })

		confirmButton().click()
		await nextTick()

		expect(
			dialog()?.querySelector(".i-svg-spinners\\:blocks-shuffle-3"),
		).not.toBeNull()
		expect(confirmButton().disabled).toBe(true)
		expect(
			teleportedButton(t("sidebar.tag-deletion-modal.cancel-button")).disabled,
		).toBe(true)
	})

	it("closes without deleting when the cancel button is pressed", async ({
		expect,
	}) => {
		const deleteTag = vi.fn()
		const wrapper = await mountModal({ name: "Production", deleteTag })

		teleportedButton(t("sidebar.tag-deletion-modal.cancel-button")).click()
		await nextTick()

		expect(deleteTag).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("closes without deleting when the close button is pressed", async ({
		expect,
	}) => {
		const deleteTag = vi.fn()
		const wrapper = await mountModal({ name: "Production", deleteTag })

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(deleteTag).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("keeps showing the tag name while the dialog animates shut", async ({
		expect,
	}) => {
		const wrapper = await mountModal({
			name: "Production",
			deleteTag: vi.fn(),
		})

		await wrapper.setProps({ modelValue: null })

		expect(document.body.textContent).toContain("Production")
	})
})

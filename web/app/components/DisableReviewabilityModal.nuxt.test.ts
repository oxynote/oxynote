import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import DisableReviewabilityModal from "./DisableReviewabilityModal.vue"
import { clearTeleportedOverlays, t, teleportedButton } from "./test-helpers"

function mountModal(target: (() => Promise<void>) | null) {
	return mountSuspended(DisableReviewabilityModal, {
		props: { modelValue: target },
	})
}

// the dialog body is teleported into <body>, so it is the only place the
// modal's own markup can be read from
function dialog() {
	return document.body.querySelector("[data-slot='dialog-content']")
}

// the dialog body is teleported into the shared <body> and the disable flow
// is driven by the global fake timers, so these tests cannot interleave
// t() reaches for the nuxt app context, so the label can only be
// resolved inside a test — not once at module scope
function confirmButton() {
	return teleportedButton(
		t(
			"editor.navbar.document-options.review-workflow.disable-confirm-modal.confirm-button",
		),
	)
}

describe("<DisableReviewabilityModal>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("stays closed while there is no pending target", async ({ expect }) => {
		await mountModal(null)

		expect(dialog()).toBeNull()
	})

	it("disables reviewability when the confirm button is pressed", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const disableReviewability = vi.fn().mockResolvedValue(undefined)
		const wrapper = await mountModal(disableReviewability)

		confirmButton().click()
		await vi.advanceTimersByTimeAsync(300)

		expect(disableReviewability).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("shows a spinner and disables both buttons while disabling", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const disableReviewability = vi.fn().mockResolvedValue(undefined)
		await mountModal(disableReviewability)

		confirmButton().click()
		await nextTick()

		expect(
			dialog()?.querySelector(".i-svg-spinners\\:blocks-shuffle-3"),
		).not.toBeNull()
		expect(confirmButton().disabled).toBe(true)
		expect(
			teleportedButton(
				t(
					"editor.navbar.document-options.review-workflow.disable-confirm-modal.cancel-button",
				),
			).disabled,
		).toBe(true)
	})

	it("closes without disabling when the cancel button is pressed", async ({
		expect,
	}) => {
		const disableReviewability = vi.fn()
		const wrapper = await mountModal(disableReviewability)

		teleportedButton(
			t(
				"editor.navbar.document-options.review-workflow.disable-confirm-modal.cancel-button",
			),
		).click()
		await nextTick()

		expect(disableReviewability).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("closes without disabling when the close button is pressed", async ({
		expect,
	}) => {
		const disableReviewability = vi.fn()
		const wrapper = await mountModal(disableReviewability)

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(disableReviewability).toHaveBeenCalledTimes(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})
})

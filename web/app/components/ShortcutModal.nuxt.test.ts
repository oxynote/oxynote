import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import ShortcutModal from "./ShortcutModal.vue"
import { clearTeleportedOverlays, t, teleportedButton } from "./test-helpers"
import { SHORTCUT_GROUPS } from "~/utils/shortcuts"

function mountModal(open = true) {
	return mountSuspended(ShortcutModal, { props: { modelValue: open } })
}

function dialog() {
	return document.body.querySelector("[data-slot='dialog-content']")
}

// the dialog body is teleported into the shared <body>
describe("<ShortcutModal>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it("stays closed while the model is false", async ({ expect }) => {
		await mountModal(false)

		expect(dialog()).toBeNull()
	})

	it("titles the dialog once open", async ({ expect }) => {
		await mountModal()

		expect(dialog()?.textContent).toContain(t("shortcuts.modal.title"))
	})

	it("heads a section for every group, in order", async ({ expect }) => {
		await mountModal()

		const headings = Array.from(
			document.body.querySelectorAll("[data-slot='dialog-content'] h3"),
		).map((heading) => heading.textContent)

		expect(headings).toEqual(SHORTCUT_GROUPS.map((group) => t(group.i18nKey)))
	})

	it("names every grouped shortcut and prints its keys", async ({ expect }) => {
		await mountModal()

		const text = dialog()?.textContent ?? ""

		SHORTCUT_GROUPS.flatMap((group) => group.shortcuts).forEach((entry) => {
			expect(text).toContain(t(entry.i18nKey))
			expect(text).toContain(t(entry.descriptionI18nKey))
		})

		// the suite runs as a non-mac host. Only the keys themselves are
		// <kbd>; the connector between them is a plain span
		const keys = Array.from(
			document.body.querySelectorAll(
				"[data-slot='dialog-content'] kbd[data-slot='kbd-group'] kbd",
			),
		).map((key) => key.textContent)

		expect(keys).toEqual(
			SHORTCUT_GROUPS.flatMap((group) => group.shortcuts).flatMap((entry) =>
				entry.action.keyboardKey.other.split("+"),
			),
		)
	})

	it("separates the keys of a shortcut with the connector", async ({
		expect,
	}) => {
		await mountModal()

		const rendered = Array.from(
			document.body.querySelectorAll(
				"[data-slot='dialog-content'] kbd[data-slot='kbd-group']",
			),
		).map((group) =>
			Array.from(group.children).map((child) => child.textContent.trim()),
		)

		expect(rendered).toEqual(
			SHORTCUT_GROUPS.flatMap((group) => group.shortcuts).map((entry) =>
				entry.action.keyboardKey.other
					.split("+")
					.flatMap((key, index) =>
						index === 0 ? [key] : [t("shortcuts.connector"), key],
					),
			),
		)
	})

	it("closes when the close button is pressed", async ({ expect }) => {
		const wrapper = await mountModal()

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
	})
})

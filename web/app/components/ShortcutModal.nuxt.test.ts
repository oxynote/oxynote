import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import { allItems } from "./editor/slash/items"
import ShortcutModal from "./ShortcutModal.vue"
import { clearTeleportedOverlays, t, teleportedButton } from "./test-helpers"
import { SHORTCUT_GROUPS } from "~/utils/shortcuts"

function mountModal(open = true) {
	return mountSuspended(ShortcutModal, { props: { modelValue: open } })
}

function dialog() {
	return document.body.querySelector("[data-slot='dialog-content']")
}

function row(id: string): Element {
	const found = document.body.querySelector(`[data-shortcut-row="${id}"]`)
	if (!found) {
		throw new Error(`no shortcut row "${id}"`)
	}

	return found
}

// the keys a row prints, one <kbd> each; a sequence connector would be a
// plain span and is left out
function rowKeys(id: string): string[] {
	return Array.from(
		row(id).querySelectorAll("kbd[data-slot='kbd-group'] kbd"),
	).map((key) => key.textContent.trim())
}

function rowDescription(id: string): string {
	return (
		row(id).querySelector(".text-muted-foreground")?.textContent.trim() ?? ""
	)
}

const entryId = (key: string) => `shortcuts.modal.entries.${key}.label`

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

	it("heads a section for every group, then the slash commands", async ({
		expect,
	}) => {
		await mountModal()

		const headings = Array.from(
			document.body.querySelectorAll("[data-slot='dialog-content'] h3"),
		).map((heading) => heading.textContent)

		expect(headings).toEqual([
			...SHORTCUT_GROUPS.map((group) => t(group.i18nKey)),
			t("shortcuts.groups.slash-commands"),
		])
	})

	it("names and describes every grouped shortcut", async ({ expect }) => {
		await mountModal()

		SHORTCUT_GROUPS.flatMap((group) => group.shortcuts).forEach((entry) => {
			expect(row(entry.i18nKey).textContent).toContain(t(entry.i18nKey))
			expect(rowDescription(entry.i18nKey)).toBe(t(entry.descriptionI18nKey))
		})
	})

	// the suite runs as a non-mac host, and a chord's keys sit side by side
	it("prints an app shortcut and an editor binding as chords", async ({
		expect,
	}) => {
		await mountModal()

		expect(rowKeys(entryId("toggle-inbox"))).toEqual(["Ctrl", "Shift", "\\"])
		expect(rowKeys(entryId("bold"))).toEqual(["Ctrl", "B"])
		expect(rowKeys(entryId("outdent-list-item"))).toEqual(["Shift", "Tab"])
	})

	it("prints typed markdown as written", async ({ expect }) => {
		await mountModal()

		expect(rowKeys(entryId("md-bold"))).toEqual(["**text**"])
		expect(rowKeys(entryId("md-quote"))).toEqual([
			">",
			t("shortcuts.modal.space-key"),
		])
	})

	it("lists the markdown of every block the slash menu inserts", async ({
		expect,
	}) => {
		await mountModal()

		const withMarkdown = allItems.filter((item) => item.shortcut)

		expect(withMarkdown.length).toBeGreaterThan(0)
		withMarkdown.forEach((item) => {
			const id = `markdown-${item.titleI18nKey}`
			const pattern = item.shortcut ?? ""
			const keys = pattern.endsWith(" ")
				? [pattern.trimEnd(), t("shortcuts.modal.space-key")]
				: [pattern]

			expect(row(id).textContent).toContain(t(item.titleI18nKey))
			expect(rowKeys(id)).toEqual(keys)
			expect(rowDescription(id)).toBe(
				`${t(item.descriptionI18nKey)} ${t("shortcuts.modal.markdown-block-hint")}`,
			)
		})
	})

	it("lists every slash command by the text that opens it", async ({
		expect,
	}) => {
		await mountModal()

		const section = document.body.querySelector(
			"[data-shortcut-section='slash-commands']",
		)

		expect(section?.querySelectorAll("[data-shortcut-row]")).toHaveLength(
			allItems.length,
		)
		allItems.forEach((item) => {
			const id = `slash-${item.titleI18nKey}`

			expect(row(id).textContent).toContain(t(item.titleI18nKey))
			expect(rowKeys(id)).toEqual([`/${t(item.titleI18nKey).toLowerCase()}`])
			expect(rowDescription(id)).toBe(
				`${t(item.descriptionI18nKey)} ${t("shortcuts.modal.slash-hint")}`,
			)
		})
	})

	it("closes when the close button is pressed", async ({ expect }) => {
		const wrapper = await mountModal()

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
	})
})

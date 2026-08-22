import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import UnitSelect from "./UnitSelect.vue"
import { VisualizationCoreUnit, VisualizationDataUnit } from "../utils"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

function mountUnitSelect(props: Record<string, unknown> = {}) {
	return mountSuspended(UnitSelect, { props: props })
}

function triggerLabel(wrapper: VueWrapper): string {
	return wrapper.get("button").text()
}

async function openMenu(wrapper: VueWrapper) {
	await wrapper.get("button").trigger("pointerdown", { button: 0 })
	await wrapper.get("button").trigger("click")
	await nextTick()
}

// the editable flag is a shared cookie state, the editor store is
// app-wide, and the menu bodies are teleported into a shared <body>, so
// these tests cannot interleave
describe("<UnitSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("prompts for a unit while none is chosen", async ({ expect }) => {
		const wrapper = await mountUnitSelect()

		expect(triggerLabel(wrapper)).toBe(
			t("editor.metrics.config.unit-placeholder"),
		)
	})

	it("reports an unset unit as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountUnitSelect()

		expect(triggerLabel(wrapper)).toBe(
			t("editor.metrics.config.unit-empty-value-placeholder"),
		)
	})

	it("names the chosen unit through its category", async ({ expect }) => {
		const wrapper = await mountUnitSelect({
			modelValue: VisualizationDataUnit.Megabytes,
		})

		expect(triggerLabel(wrapper)).toBe(
			t("editor.metrics.config.unit-options.data.options.megabytes"),
		)
	})

	it("names a chosen core unit directly", async ({ expect }) => {
		const wrapper = await mountUnitSelect({
			modelValue: VisualizationCoreUnit.Custom,
		})

		expect(triggerLabel(wrapper)).toBe(
			t("editor.metrics.config.unit-options.custom"),
		)
	})

	it("falls back to the unit the diff replaced", async ({ expect }) => {
		const wrapper = await mountUnitSelect({
			modelValue: null,
			oldUnit: VisualizationDataUnit.Bytes,
		})

		expect(triggerLabel(wrapper)).toBe(
			t("editor.metrics.config.unit-options.data.options.bytes"),
		)
	})

	it("prefers the current unit over the one the diff replaced", async ({
		expect,
	}) => {
		const wrapper = await mountUnitSelect({
			modelValue: VisualizationDataUnit.Megabytes,
			oldUnit: VisualizationDataUnit.Bytes,
		})

		expect(triggerLabel(wrapper)).toBe(
			t("editor.metrics.config.unit-options.data.options.megabytes"),
		)
	})

	it.for([
		{ status: DiffStatus.Added, expected: "bg-diff-field-added" },
		{ status: DiffStatus.Removed, expected: "bg-diff-field-removed" },
	])("tints a $status unit field", async ({ status, expected }, { expect }) => {
		const wrapper = await mountUnitSelect({ diffStatus: status })

		expect(wrapper.get("button").classes()).toContain(expected)
	})

	it("leaves an unchanged unit field untinted", async ({ expect }) => {
		const wrapper = await mountUnitSelect({ diffStatus: DiffStatus.Unchanged })

		expect(wrapper.get("button").classes()).not.toContain("bg-diff-field-added")
		expect(wrapper.get("button").classes()).not.toContain(
			"bg-diff-field-removed",
		)
	})

	it("offers no menu in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountUnitSelect()

		expect(wrapper.get("button").attributes("disabled")).toBe("")
	})

	it("groups the units by category", async ({ expect }) => {
		const wrapper = await mountUnitSelect()

		await openMenu(wrapper)

		expect(
			menuItem(t("editor.metrics.config.unit-options.time.title")),
		).toBeDefined()
		expect(
			menuItem(t("editor.metrics.config.unit-options.data.title")),
		).toBeDefined()
		expect(
			menuItem(t("editor.metrics.config.unit-options.misc.title")),
		).toBeDefined()
	})

	it("picks a core unit straight off the menu", async ({ expect }) => {
		const wrapper = await mountUnitSelect()
		await openMenu(wrapper)

		menuItem(t("editor.metrics.config.unit-options.custom")).click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[VisualizationCoreUnit.Custom],
		])
	})
})

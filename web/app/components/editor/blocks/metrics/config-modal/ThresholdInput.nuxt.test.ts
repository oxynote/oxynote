import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import ThresholdInput from "./ThresholdInput.vue"
import { stubChartColorContext, stubThresholdPalette } from "../test-helpers"
import { at, clearTeleportedOverlays, t } from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

function mountThreshold(props: Record<string, unknown> = {}) {
	return mountSuspended(ThresholdInput, {
		props: {
			visualizationType: GenericQueryChartType.Line,
			color: "#ff0000",
			...props,
		},
	})
}

function inputs(wrapper: VueWrapper) {
	return wrapper.findAll("input")
}

function valueOf(wrapper: VueWrapper, index: number): string {
	return (at(inputs(wrapper), index).element as HTMLInputElement).value
}

// the editable flag is a shared cookie state, the editor store is
// app-wide and the popover body is teleported into a shared <body>, so
// these tests cannot interleave
describe("<ThresholdInput>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		stubChartColorContext()
		stubThresholdPalette()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("asks for a value and a label", async ({ expect }) => {
		const wrapper = await mountThreshold()

		expect(inputs(wrapper).map((i) => i.attributes("placeholder"))).toEqual([
			t("editor.metrics.config.threshold-value-placeholder"),
			t("editor.metrics.config.threshold-label-placeholder"),
		])
	})

	it("reports empty fields as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountThreshold()

		expect(inputs(wrapper).map((i) => i.attributes("placeholder"))).toEqual([
			t("editor.metrics.config.threshold-value-empty-placeholder"),
			t("editor.metrics.config.threshold-label-empty-placeholder"),
		])
	})

	it("shows the value and label it was given", async ({ expect }) => {
		const wrapper = await mountThreshold({ value: 42, label: "warn" })

		expect(valueOf(wrapper, 0)).toBe("42")
		expect(valueOf(wrapper, 1)).toBe("warn")
	})

	it("shows the threshold's colour on the swatch", async ({ expect }) => {
		const wrapper = await mountThreshold({ color: "#00ff00" })

		expect(wrapper.get("button div").attributes("style")).toBe(
			"background-color: #00ff00;",
		)
	})

	it("reports a value the reader typed", async ({ expect }) => {
		const wrapper = await mountThreshold()

		await at(inputs(wrapper), 0).setValue("13")

		expect(wrapper.emitted("update:value")).toEqual([[13]])
	})

	it("reports a label the reader typed", async ({ expect }) => {
		const wrapper = await mountThreshold()

		await at(inputs(wrapper), 1).setValue("critical")

		expect(wrapper.emitted("update:label")).toEqual([["critical"]])
	})

	it("reports when the value field is left", async ({ expect }) => {
		const wrapper = await mountThreshold()

		await at(inputs(wrapper), 0).trigger("blur")

		expect(wrapper.emitted("input-blur")).toHaveLength(1)
	})

	it("asks to be deleted", async ({ expect }) => {
		const wrapper = await mountThreshold()

		await at(wrapper.findAll("button"), 1).trigger("click")

		expect(wrapper.emitted("delete")).toHaveLength(1)
	})

	it("hides its delete button in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountThreshold()

		expect(wrapper.findAll("button")).toHaveLength(1)
	})

	it("drops the label field for a gauge", async ({ expect }) => {
		const wrapper = await mountThreshold({
			visualizationType: GenericQueryChartType.Gauge,
		})

		expect(inputs(wrapper)).toHaveLength(1)
		expect(at(inputs(wrapper), 0).attributes("placeholder")).toBe(
			t("editor.metrics.config.threshold-value-placeholder"),
		)
	})

	it("shows only a fixed label for the base threshold", async ({ expect }) => {
		const wrapper = await mountThreshold({
			visualizationType: GenericQueryChartType.Gauge,
			baseThreshold: { label: t("editor.metrics.config.base-threshold-label") },
		})

		expect(inputs(wrapper)).toHaveLength(1)
		expect(valueOf(wrapper, 0)).toBe(
			t("editor.metrics.config.base-threshold-label"),
		)
		expect(wrapper.emitted("update:label")).toEqual([
			[t("editor.metrics.config.base-threshold-label")],
		])
	})

	it("cannot be deleted when it is the base threshold", async ({ expect }) => {
		const wrapper = await mountThreshold({
			baseThreshold: { label: t("editor.metrics.config.base-threshold-label") },
		})

		expect(wrapper.findAll("button")).toHaveLength(1)
	})

	it.for([
		{ status: DiffStatus.Added, expected: "bg-diff-field-added" },
		{ status: DiffStatus.Removed, expected: "bg-diff-field-removed" },
	])(
		"tints a $status threshold row",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountThreshold({ diffStatus: status })

			expect(wrapper.classes()).toContain(expected)
		},
	)

	it("picks a colour from the palette popover", async ({ expect }) => {
		const palette = stubThresholdPalette()
		const wrapper = await mountThreshold()
		await at(wrapper.findAll("button"), 0).trigger("click")
		await nextTick()

		const swatches = Array.from(
			document.body.querySelectorAll<HTMLButtonElement>(
				"[data-slot='popover-content'] button",
			),
		)
		swatches[2]?.click()
		await nextTick()

		expect(wrapper.emitted("update:color")).toEqual([[at(palette, 2)]])
	})

	it("offers no colour popover in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountThreshold()

		await at(wrapper.findAll("button"), 0).trigger("click")
		await nextTick()

		expect(
			document.body.querySelectorAll("[data-slot='popover-content']"),
		).toHaveLength(0)
	})
})

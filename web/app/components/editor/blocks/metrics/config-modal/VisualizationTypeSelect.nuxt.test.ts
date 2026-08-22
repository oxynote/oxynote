import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import VisualizationTypeSelect from "./VisualizationTypeSelect.vue"
import { stubChartColorContext } from "../test-helpers"
import { defaultMetricConfig, type MetricConfig } from "../utils"
import { at, t } from "~/components/test-helpers"

const LINE = 0
const BAR = 1
const GAUGE = 2

function configWith(
	visualizationType: MetricConfig["visualizationType"],
): MetricConfig {
	return { ...defaultMetricConfig(), visualizationType: visualizationType }
}

function mountTypeSelect(props: Record<string, unknown> = {}) {
	return mountSuspended(VisualizationTypeSelect, { props: props })
}

function tiles(wrapper: VueWrapper) {
	return wrapper.findAll("div.flex-1")
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<VisualizationTypeSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		stubChartColorContext()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("offers the three visualization types", async ({ expect }) => {
		const wrapper = await mountTypeSelect({ modelValue: configWith(null) })

		expect(tiles(wrapper).map((t) => t.text())).toEqual([
			t("editor.metrics.config.type-options.line-chart.title"),
			t("editor.metrics.config.type-options.bar-chart.title"),
			t("editor.metrics.config.type-options.gauge-chart.title"),
		])
	})

	it("marks the chosen type", async ({ expect }) => {
		const wrapper = await mountTypeSelect({
			modelValue: configWith(GenericQueryChartType.Bar),
		})

		expect(at(tiles(wrapper), BAR).classes()).toContain("border-primary")
		expect(at(tiles(wrapper), LINE).classes()).not.toContain("border-primary")
	})

	it("marks nothing while no type is chosen", async ({ expect }) => {
		const wrapper = await mountTypeSelect({ modelValue: configWith(null) })

		expect(
			tiles(wrapper).filter((t) => t.classes().includes("border-primary")),
		).toHaveLength(0)
	})

	it.for([
		{ index: LINE, expected: GenericQueryChartType.Line },
		{ index: BAR, expected: GenericQueryChartType.Bar },
		{ index: GAUGE, expected: GenericQueryChartType.Gauge },
	])(
		"stores the $expected type when picked",
		async ({ index, expected }, { expect }) => {
			const config = configWith(null)
			const wrapper = await mountTypeSelect({ modelValue: config })

			await at(tiles(wrapper), index).trigger("click")

			expect(config.visualizationType).toBe(expected)
		},
	)

	it("changes nothing in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const config = configWith(null)
		const wrapper = await mountTypeSelect({ modelValue: config })

		await at(tiles(wrapper), BAR).trigger("click")

		expect(config.visualizationType).toBeNull()
	})

	it("changes nothing while a reviewable diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)
		const config = configWith(null)
		const wrapper = await mountTypeSelect({ modelValue: config })

		await at(tiles(wrapper), BAR).trigger("click")

		expect(config.visualizationType).toBeNull()
	})

	it("changes nothing when there is no config to edit", async ({ expect }) => {
		const wrapper = await mountTypeSelect({})

		await at(tiles(wrapper), BAR).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})

	it("marks the type a diff added and the one it replaced", async ({
		expect,
	}) => {
		const wrapper = await mountTypeSelect({
			modelValue: configWith(GenericQueryChartType.Gauge),
			oldConfig: configWith(GenericQueryChartType.Line),
			isModified: true,
		})

		expect(at(tiles(wrapper), GAUGE).classes()).toContain("bg-diff-field-added")
		expect(at(tiles(wrapper), LINE).classes()).toContain(
			"bg-diff-field-removed",
		)
	})

	it("leaves an unmodified type unmarked", async ({ expect }) => {
		const wrapper = await mountTypeSelect({
			modelValue: configWith(GenericQueryChartType.Gauge),
			oldConfig: configWith(GenericQueryChartType.Gauge),
		})

		expect(at(tiles(wrapper), GAUGE).classes()).not.toContain(
			"bg-diff-field-added",
		)
	})
})

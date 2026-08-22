import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import UnitSelectWithCustom from "./UnitSelectWithCustom.vue"
import UnitSelect from "./UnitSelect.vue"
import { stubChartColorContext } from "../test-helpers"
import {
	defaultMetricConfig,
	VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH,
	VisualizationCoreUnit,
	VisualizationDataUnit,
	type MetricConfig,
} from "../utils"
import { emitFrom, t } from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

function configWith(unit: MetricConfig["unit"]): MetricConfig {
	return { ...defaultMetricConfig(), unit: unit }
}

function mountUnit(props: Record<string, unknown> = {}) {
	return mountSuspended(UnitSelectWithCustom, { props: props })
}

function customInput(wrapper: VueWrapper) {
	return wrapper.find("input")
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<UnitSelectWithCustom>", { concurrent: false }, () => {
	beforeEach(() => {
		stubChartColorContext()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("offers the unit list for the config it edits", async ({ expect }) => {
		const wrapper = await mountUnit({
			modelValue: configWith({ type: VisualizationDataUnit.Bytes }),
		})

		expect(wrapper.get("button").text()).toBe(
			t("editor.metrics.config.unit-options.data.options.bytes"),
		)
	})

	it("hides the custom field for a non-custom unit", async ({ expect }) => {
		const wrapper = await mountUnit({
			modelValue: configWith({ type: VisualizationDataUnit.Bytes }),
		})

		expect(customInput(wrapper).exists()).toBe(false)
	})

	it("asks for the custom unit once custom is chosen", async ({ expect }) => {
		const wrapper = await mountUnit({
			modelValue: configWith({ type: VisualizationCoreUnit.Custom }),
		})

		expect(customInput(wrapper).attributes("placeholder")).toBe(
			t("editor.metrics.config.unit-custom-placeholder"),
		)
	})

	it("reports an unset custom unit as empty in read mode", async ({
		expect,
	}) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountUnit({
			modelValue: configWith({ type: VisualizationCoreUnit.Custom }),
		})

		expect(customInput(wrapper).attributes("placeholder")).toBe(
			t("editor.metrics.config.unit-custom-empty-value-placeholder"),
		)
	})

	it("shows the custom unit the config holds", async ({ expect }) => {
		const wrapper = await mountUnit({
			modelValue: configWith({
				type: VisualizationCoreUnit.Custom,
				custom: "req/s",
			}),
		})

		expect((customInput(wrapper).element as HTMLInputElement).value).toBe(
			"req/s",
		)
	})

	it("stores a unit type the reader picks", async ({ expect }) => {
		const config = configWith({ type: null })
		const wrapper = await mountUnit({ modelValue: config })

		emitFrom(
			wrapper,
			UnitSelect,
			"update:modelValue",
			VisualizationDataUnit.Bits,
		)
		await nextTick()

		expect(config.unit.type).toBe(VisualizationDataUnit.Bits)
	})

	it("stores the custom unit the reader typed", async ({ expect }) => {
		const config = configWith({ type: VisualizationCoreUnit.Custom })
		const wrapper = await mountUnit({ modelValue: config })

		await customInput(wrapper).setValue("  req/s  ")

		expect(config.unit.custom).toBe("req/s")
	})

	it("cuts an overlong custom unit down to size", async ({ expect }) => {
		const config = configWith({ type: VisualizationCoreUnit.Custom })
		const wrapper = await mountUnit({ modelValue: config })

		await customInput(wrapper).setValue("abcdefghijkl")

		expect(config.unit.custom).toHaveLength(
			VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH,
		)
	})

	it("shows the unit of the version a diff replaced", async ({ expect }) => {
		const wrapper = await mountUnit({
			oldConfig: configWith({
				type: VisualizationCoreUnit.Custom,
				custom: "old/s",
			}),
		})

		expect(wrapper.get("button").text()).toBe(
			t("editor.metrics.config.unit-options.custom"),
		)
		expect((customInput(wrapper).element as HTMLInputElement).value).toBe(
			"old/s",
		)
	})

	it.for([
		{ status: DiffStatus.Added, expected: "bg-diff-field-added" },
		{ status: DiffStatus.Removed, expected: "bg-diff-field-removed" },
	])(
		"tints a $status custom unit field",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountUnit({
				modelValue: configWith({ type: VisualizationCoreUnit.Custom }),
				diffStatus: status,
			})

			expect(customInput(wrapper).classes()).toContain(expected)
		},
	)

	it("shows nothing at all with neither a config nor an old one", async ({
		expect,
	}) => {
		const wrapper = await mountUnit({})

		expect(wrapper.get("button").text()).toBe(
			t("editor.metrics.config.unit-placeholder"),
		)
		expect(customInput(wrapper).exists()).toBe(false)
	})
})

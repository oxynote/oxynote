import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { DOMWrapper, VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import ConfigSidebar from "./ConfigSidebar.vue"
import ConfigField from "../ConfigField.vue"
import ThresholdInput from "./ThresholdInput.vue"
import DataSourceSelect from "./DataSourceSelect.vue"
import { stubChartColorContext, stubThresholdPalette } from "../test-helpers"
import {
	defaultMetricConfig,
	VisualizationDataUnit,
	type MetricConfig,
} from "../utils"
import {
	at,
	clearTeleportedOverlays,
	emitFrom,
	findButtonByText,
	t,
} from "~/components/test-helpers"
import {
	ANY_STRING,
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

function configWith(overrides: Partial<MetricConfig> = {}): MetricConfig {
	return { ...defaultMetricConfig(), ...overrides }
}

function mountSidebar(props: Record<string, unknown> = {}) {
	return mountSuspended(ConfigSidebar, {
		props: { modelValue: configWith(), ...props },
	})
}

// the fields are labelled through ConfigField's label slot, which is the
// only stable way to tell one input from the next
function fieldInputs(
	wrapper: VueWrapper,
	label: string,
): DOMWrapper<Element>[] {
	const field = wrapper
		.findAllComponents(ConfigField)
		.find((f) => f.text().startsWith(label))
	if (!field) {
		throw new Error(`no config field labelled "${label}"`)
	}

	return field.findAll("input")
}

function inputValue(input: DOMWrapper<Element>): string {
	return (input.element as HTMLInputElement).value
}

// the editable flag is a shared cookie state, the editor store and the
// query cache are app-wide, and the popovers are teleported into a shared
// <body>, so these tests cannot interleave
describe("<ConfigSidebar>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		stubChartColorContext()
		stubThresholdPalette()
		mockEndpoint("GET", "/api/data-sources", () => [])
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	afterEach(disposeMockEndpoints)

	it("labels every setting it offers", async ({ expect }) => {
		const wrapper = await mountSidebar()

		expect(wrapper.text()).toContain(t("editor.metrics.config.title-label"))
		expect(wrapper.text()).toContain(t("editor.metrics.config.unit-label"))
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.thresholds-label"),
		)
		expect(wrapper.text()).toContain(t("editor.metrics.config.decimals-label"))
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.bounds-min-label"),
		)
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.bounds-max-label"),
		)
	})

	it("shows the title the config carries", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({ title: "Requests" }),
		})

		expect(
			inputValue(
				at(fieldInputs(wrapper, t("editor.metrics.config.title-label")), 0),
			),
		).toBe("Requests")
	})

	it("stores a title the reader typed", async ({ expect }) => {
		const config = configWith()
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.title-label")),
			0,
		).setValue("Requests")

		expect(config.title).toBe("Requests")
	})

	it("prompts for a title while it is empty", async ({ expect }) => {
		const wrapper = await mountSidebar()

		expect(
			at(
				fieldInputs(wrapper, t("editor.metrics.config.title-label")),
				0,
			).attributes("placeholder"),
		).toBe(t("editor.metrics.config.title-placeholder"))
	})

	it("reports an empty title as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountSidebar()

		expect(
			at(
				fieldInputs(wrapper, t("editor.metrics.config.title-label")),
				0,
			).attributes("placeholder"),
		).toBe(t("editor.metrics.config.title-empty-value-placeholder"))
	})

	it("offers to add a first threshold", async ({ expect }) => {
		const config = configWith({
			visualizationType: GenericQueryChartType.Line,
		})
		const wrapper = await mountSidebar({ modelValue: config })

		await findButtonByText(
			wrapper,
			t("editor.metrics.config.add-threshold-button"),
		).trigger("click")

		expect(config.thresholds).toEqual([
			{ value: undefined, label: undefined, color: ANY_STRING },
		])
	})

	it("says there are no thresholds in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Line,
			}),
		})

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.no-thresholds-button"),
		)
	})

	it("lists the thresholds the config carries", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [
					{ value: 10, label: "warn", color: "#ff0000" },
					{ value: 20, label: "crit", color: "#00ff00" },
				],
			}),
		})

		expect(wrapper.findAllComponents(ThresholdInput)).toHaveLength(2)
	})

	it("stores a threshold value the reader typed", async ({ expect }) => {
		const config = configWith({
			visualizationType: GenericQueryChartType.Line,
			thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
		})
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.thresholds-label")),
			0,
		).setValue("42")

		expect(config.thresholds).toEqual([
			{ value: 42, label: "warn", color: "#ff0000" },
		])
	})

	it("stores a threshold label the reader typed", async ({ expect }) => {
		const config = configWith({
			visualizationType: GenericQueryChartType.Line,
			thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
		})
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.thresholds-label")),
			1,
		).setValue("critical")

		expect(config.thresholds).toEqual([
			{ value: 10, label: "critical", color: "#ff0000" },
		])
	})

	it("drops a threshold the reader deleted", async ({ expect }) => {
		const config = configWith({
			visualizationType: GenericQueryChartType.Line,
			thresholds: [
				{ value: 10, label: "warn", color: "#ff0000" },
				{ value: 20, label: "crit", color: "#00ff00" },
			],
		})
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			wrapper.findAllComponents(ThresholdInput)[0]?.findAll("button") ?? [],
			1,
		).trigger("click")

		expect(config.thresholds).toEqual([
			{ value: 20, label: "crit", color: "#00ff00" },
		])
	})

	it("gives a gauge a base threshold row of its own", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Gauge,
			}),
		})

		const rows = wrapper.findAllComponents(ThresholdInput)

		expect(rows).toHaveLength(1)
		expect(at(rows, 0).props("baseThreshold")).toEqual({
			label: t("editor.metrics.config.base-threshold-label"),
		})
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.add-threshold-button"),
		)
	})

	it("stores the decimal places the reader typed", async ({ expect }) => {
		const config = configWith()
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.decimals-label")),
			0,
		).setValue("3")

		expect(config.decimals).toBe(3)
	})

	it("stores the axis bounds the reader typed", async ({ expect }) => {
		const config = configWith()
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.bounds-min-label")),
			0,
		).setValue("-5")
		await at(
			fieldInputs(wrapper, t("editor.metrics.config.bounds-max-label")),
			0,
		).setValue("100")

		expect(config.axisBounds).toEqual({ min: -5, max: 100 })
	})

	it("keeps the lower bound below the lowest threshold", async ({ expect }) => {
		const config = configWith({
			visualizationType: GenericQueryChartType.Line,
			thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
		})
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.bounds-min-label")),
			0,
		).setValue("50")

		expect(config.axisBounds.min).toBe(10)
	})

	it("keeps the upper bound above the highest threshold", async ({
		expect,
	}) => {
		const config = configWith({
			visualizationType: GenericQueryChartType.Line,
			thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
		})
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.bounds-max-label")),
			0,
		).setValue("5")

		expect(config.axisBounds.max).toBe(10)
	})

	it("clears a bound the reader emptied", async ({ expect }) => {
		const config = configWith({ axisBounds: { min: 1, max: 2 } })
		const wrapper = await mountSidebar({ modelValue: config })

		await at(
			fieldInputs(wrapper, t("editor.metrics.config.bounds-min-label")),
			0,
		).setValue("")
		await at(
			fieldInputs(wrapper, t("editor.metrics.config.bounds-max-label")),
			0,
		).setValue("")

		expect(config.axisBounds).toEqual({ min: null, max: null })
	})

	it("shows the title a diff replaced beside the new one", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({ title: "After" }),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({ title: "Before" }),
		})

		const inputs = fieldInputs(wrapper, t("editor.metrics.config.title-label"))

		expect(inputs).toHaveLength(2)
		expect(inputValue(at(inputs, 0))).toBe("After")
		expect(at(inputs, 0).classes()).toContain("bg-diff-field-added")
		expect(inputValue(at(inputs, 1))).toBe("Before")
		expect(at(inputs, 1).classes()).toContain("bg-diff-field-removed")
	})

	it("shows the unit a diff replaced beside the new one", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				unit: { type: VisualizationDataUnit.Bytes, custom: null },
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				unit: { type: VisualizationDataUnit.Bits, custom: null },
			}),
		})

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.unit-options.data.options.bytes"),
		)
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.unit-options.data.options.bits"),
		)
	})

	it("shows the decimal places a diff replaced beside the new one", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({ decimals: 3 }),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({ decimals: 1 }),
		})

		const inputs = fieldInputs(
			wrapper,
			t("editor.metrics.config.decimals-label"),
		)

		expect(inputValue(at(inputs, 0))).toBe("3")
		expect(inputValue(at(inputs, 1))).toBe("1")
	})

	it("shows the bounds a diff replaced beside the new ones", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({ axisBounds: { min: 0, max: 10 } }),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({ axisBounds: { min: 1, max: 20 } }),
		})

		expect(
			inputValue(
				at(
					fieldInputs(wrapper, t("editor.metrics.config.bounds-min-label")),
					1,
				),
			),
		).toBe("1")
		expect(
			inputValue(
				at(
					fieldInputs(wrapper, t("editor.metrics.config.bounds-max-label")),
					1,
				),
			),
		).toBe("20")
	})

	it("marks each threshold a diff added, removed or left alone", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [
					{ value: 10, label: "warn", color: "#ff0000" },
					{ value: 30, label: "crit", color: "#00ff00" },
				],
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
			}),
		})

		const rows = wrapper.findAllComponents(ThresholdInput)

		expect(rows).toHaveLength(2)
		expect(at(rows, 0).props("diffStatus")).toBe(DiffStatus.Unchanged)
		expect(at(rows, 1).props("diffStatus")).toBe(DiffStatus.Added)
	})

	it("marks a threshold the diff changed as both added and removed", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [{ value: 30, label: "warn", color: "#ff0000" }],
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
			}),
		})

		const rows = wrapper.findAllComponents(ThresholdInput)

		expect(rows).toHaveLength(2)
		expect(at(rows, 0).props("diffStatus")).toBe(DiffStatus.Added)
		expect(at(rows, 1).props("diffStatus")).toBe(DiffStatus.Removed)
	})

	it("marks a threshold the diff dropped as removed", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [],
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				visualizationType: GenericQueryChartType.Line,
				thresholds: [{ value: 10, label: "warn", color: "#ff0000" }],
			}),
		})

		const rows = wrapper.findAllComponents(ThresholdInput)

		expect(rows).toHaveLength(1)
		expect(at(rows, 0).props("diffStatus")).toBe(DiffStatus.Removed)
	})

	it("shows the base gauge colour a diff replaced beside the new one", async ({
		expect,
	}) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				visualizationType: GenericQueryChartType.Gauge,
				baseThresholdColor: "#111111",
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				visualizationType: GenericQueryChartType.Gauge,
				baseThresholdColor: "#222222",
			}),
		})

		const rows = wrapper.findAllComponents(ThresholdInput)

		expect(at(rows, 0).props("color")).toBe("#111111")
		expect(at(rows, 1).props("color")).toBe("#222222")
	})

	it("asks the host to open the data source settings", async ({ expect }) => {
		const wrapper = await mountSidebar()

		emitFrom(wrapper, DataSourceSelect, "open-settings")
		await nextTick()

		expect(wrapper.emitted("open-settings")).toHaveLength(1)
	})
})

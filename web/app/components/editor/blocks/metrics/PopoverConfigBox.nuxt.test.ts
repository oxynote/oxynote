import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import PopoverConfigBox from "./PopoverConfigBox.vue"
import {
	defaultMetricConfig,
	RefreshInterval,
	TimeRangePreset,
	type MetricConfig,
} from "./utils"
import { makeEditor } from "../../test-helpers/node-view"
import { stubChartColorContext } from "./test-helpers"
import {
	at,
	emitFrom,
	emitFromNth,
	findButtonByText,
	t,
} from "~/components/test-helpers"
import { Select } from "~/components/shadcn/ui/select"

const BLOCK_UID = "metric-1"

function mountBox(
	options: {
		config?: MetricConfig
		getPos?: () => number | undefined
		setNodeSelection?: (pos: number) => void
	} = {},
) {
	const editor = makeEditor({
		commands: {
			setNodeSelection: options.setNodeSelection ?? (() => undefined),
		},
	})

	return mountSuspended(PopoverConfigBox, {
		props: {
			uid: BLOCK_UID,
			editor: editor.editor,
			getPos: options.getPos ?? (() => 3),
			modelValue: options.config ?? defaultMetricConfig(),
		},
	})
}

function selectedValues(wrapper: VueWrapper): string[] {
	return wrapper.findAll("[data-slot='select-value']").map((v) => v.text())
}

function triggerDisabled(
	wrapper: VueWrapper,
	index: number,
): string | undefined {
	return at(wrapper.findAll("[data-slot='select-trigger']"), index).attributes(
		"disable",
	)
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<PopoverConfigBox>", { concurrent: false }, () => {
	beforeEach(() => {
		stubChartColorContext()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().activateMetricBlockConfig(null)
	})

	it("labels the two settings it offers", async ({ expect }) => {
		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.time-range-label"),
		)
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.refresh-interval-label"),
		)
	})

	it("shows the time range and refresh interval the config carries", async ({
		expect,
	}) => {
		const wrapper = await mountBox({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			},
		})

		expect(selectedValues(wrapper)).toEqual([
			t("editor.metrics.config.time-range-options.last_24_hours"),
			t("editor.metrics.config.refresh-interval-options.30s"),
		])
	})

	it("prompts for a time range and interval when the config has none", async ({
		expect,
	}) => {
		const wrapper = await mountBox({
			config: {
				...defaultMetricConfig(),
				timeRange: null,
				refreshInterval: null,
			},
		})

		expect(selectedValues(wrapper)).toEqual([
			t("editor.metrics.config.time-range-placeholder"),
			t("editor.metrics.config.refresh-interval-placeholder"),
		])
	})

	it("stores a time range the reader picks", async ({ expect }) => {
		const config = defaultMetricConfig()
		const wrapper = await mountBox({ config: config })

		emitFrom(wrapper, Select, "update:modelValue", TimeRangePreset.Last7Days)
		await nextTick()

		expect(config.timeRange).toBe(TimeRangePreset.Last7Days)
	})

	it("stores a refresh interval the reader picks", async ({ expect }) => {
		const config = defaultMetricConfig()
		const wrapper = await mountBox({ config: config })

		emitFromNth(wrapper, Select, 1, "update:modelValue", RefreshInterval.H1)
		await nextTick()

		expect(config.refreshInterval).toBe(RefreshInterval.H1)
	})

	it("lets both settings be changed while editing", async ({ expect }) => {
		const wrapper = await mountBox()

		expect(triggerDisabled(wrapper, 0)).toBe("false")
		expect(triggerDisabled(wrapper, 1)).toBe("false")
	})

	it("blocks both settings in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountBox()

		expect(triggerDisabled(wrapper, 0)).toBe("true")
		expect(triggerDisabled(wrapper, 1)).toBe("true")
	})

	it("blocks both settings while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountBox()

		expect(triggerDisabled(wrapper, 0)).toBe("true")
		expect(triggerDisabled(wrapper, 1)).toBe("true")
	})

	it("offers to edit the metrics while editing", async ({ expect }) => {
		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.modal-trigger-button-normal"),
		)
	})

	it("offers only to view the metrics in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.modal-trigger-button-readonly"),
		)
	})

	it("opens the config modal for this block", async ({ expect }) => {
		const setNodeSelection = vi.fn()
		const wrapper = await mountBox({ setNodeSelection: setNodeSelection })

		await findButtonByText(
			wrapper,
			t("editor.metrics.config.modal-trigger-button-normal"),
		).trigger("click")

		expect(setNodeSelection).toHaveBeenCalledTimes(1)
		expect(setNodeSelection).toHaveBeenCalledWith(3)
		expect(useEditorStore().activeMetricBlockConfig).toBe(BLOCK_UID)
	})

	it("opens the config modal for a block with no resolvable position", async ({
		expect,
	}) => {
		const setNodeSelection = vi.fn()
		const wrapper = await mountBox({
			getPos: () => undefined,
			setNodeSelection: setNodeSelection,
		})

		await findButtonByText(
			wrapper,
			t("editor.metrics.config.modal-trigger-button-normal"),
		).trigger("click")

		expect(setNodeSelection).toHaveBeenCalledTimes(0)
		expect(useEditorStore().activeMetricBlockConfig).toBe(BLOCK_UID)
	})
})

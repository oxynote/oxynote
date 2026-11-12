import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import BaseModal from "./BaseModal.client.vue"
import ConfigSidebar from "./ConfigSidebar.vue"
import VisualizationContainer from "../VisualizationContainer.vue"
import { stubChartColorContext } from "../test-helpers"
import {
	defaultMetricConfig,
	MetricSimulationPreset,
	type MetricConfig,
} from "../utils"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"
import {
	emitFrom,
	mountUnderTooltipProvider,
	stubViewportMatches,
	t,
	teleportedButton,
} from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

// echarts measures text on a canvas, which happy-dom does not implement
vi.mock("../visualizations/LineChart.vue", () => ({
	default: { name: "LineChart", render: () => null },
}))
vi.mock("../visualizations/BarChart.vue", () => ({
	default: { name: "BarChart", render: () => null },
}))
vi.mock("../visualizations/GaugeChart.vue", () => ({
	default: { name: "GaugeChart", render: () => null },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

let uidCounter = 0

function nextUid(): string {
	uidCounter++

	return `metric-modal-${uidCounter}`
}

// the modal reads everything it shows out of the editor store, which is
// where the metric block publishes its config
function activateBlock(
	options: {
		config?: MetricConfig
		diffStatus?: DiffStatus
		oldConfig?: MetricConfig | null
	} = {},
): string {
	const uid = nextUid()
	const store = useEditorStore()

	store.updateActiveDocumentId(DOCUMENT_ID)
	store.updateActiveBranchId(BRANCH_ID)
	store.setMetricBlockConfig(uid, options.config ?? defaultMetricConfig())

	if (options.diffStatus) {
		store.setMetricBlockDiffInfo(
			uid,
			options.diffStatus,
			options.oldConfig ?? null,
		)
	}

	store.activateMetricBlockConfig(uid)

	return uid
}

function mountModal() {
	return mountUnderTooltipProvider(BaseModal, {})
}

function dialogText(): string {
	return document.body.querySelector("[role='dialog']")?.textContent ?? ""
}

function dialogOpen(): boolean {
	return document.body.querySelector("[role='dialog']") !== null
}

// the editor store, the editable flag, the viewport stub and the
// teleported dialog body are all shared, so these tests cannot interleave
describe("<MetricConfigBaseModal>", { concurrent: false }, () => {
	// the dialog body is teleported into <body> and vue keeps patching it
	// for as long as the modal is mounted, so each test's modal is taken
	// down properly rather than ripped out of the dom
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearQueryCache()
		stubChartColorContext()
		stubViewportMatches(true)
		mockEndpoint("GET", "/api/data-sources", () => [])
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().activateMetricBlockConfig(null)
	})

	afterEach(disposeMockEndpoints)

	it("stays shut while no block is being configured", async ({ expect }) => {
		await mountModal()

		expect(dialogOpen()).toBe(false)
	})

	it("opens for the block being configured", async ({ expect }) => {
		activateBlock()

		await mountModal()

		expect(dialogOpen()).toBe(true)
		expect(dialogText()).toContain(
			t("editor.metrics.config.modal-title-normal"),
		)
	})

	it("titles itself as read-only in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		activateBlock()

		await mountModal()

		expect(dialogText()).toContain(
			t("editor.metrics.config.modal-title-readonly"),
		)
	})

	it.for([
		{
			status: DiffStatus.Added,
			expectedKey: "editor.metrics.config.modal-title-diff-added",
		},
		{
			status: DiffStatus.Removed,
			expectedKey: "editor.metrics.config.modal-title-diff-removed",
		},
		{
			status: DiffStatus.Modified,
			expectedKey: "editor.metrics.config.modal-title-diff-modified",
		},
	])(
		"titles itself for a $status block in a diff",
		async ({ status, expectedKey }, { expect }) => {
			activateBlock({ diffStatus: status })

			await mountModal()

			expect(dialogText()).toContain(t(expectedKey))
		},
	)

	it("shows the block's time range and refresh interval", async ({
		expect,
	}) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			},
		})

		await mountModal()

		expect(dialogText()).toContain(
			t("general.time-range-options.last_24_hours"),
		)
		expect(dialogText()).toContain(
			t("general.refresh-interval-options-short.30s"),
		)
	})

	it("names the built-in variables the block uses", async ({ expect }) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: METRIC_TIME_RANGE_REFERENCE,
				refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
			},
		})

		await mountModal()

		const timeRange = t("editor.config-modal-utils.variable-time-range")
		const refreshInterval = t(
			"editor.config-modal-utils.variable-refresh-interval",
		)

		// each button names the shared setting, not what it resolves to
		expect(teleportedButton(timeRange).textContent.trim()).toBe(timeRange)
		expect(teleportedButton(refreshInterval).textContent.trim()).toBe(
			refreshInterval,
		)
	})

	it.for([
		{
			name: "offers the global time first",
			trigger: () => t("general.time-range-options.last_24_hours"),
			label: () => t("editor.config-modal-utils.variable-time-range"),
		},
		{
			name: "offers the global refresh first",
			trigger: () => t("general.refresh-interval-options-short.30s"),
			label: () => t("editor.config-modal-utils.variable-refresh-interval"),
		},
	])("$name", async ({ trigger, label }, { expect }) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			},
		})
		await mountModal()

		await openSetting(trigger())

		const options = Array.from(
			document.body.querySelectorAll("[role='option']"),
		).map((o) => o.textContent.trim())

		if (trigger() === t("general.time-range-options.last_24_hours")) {
			expect(teleportedButton(label())).toBeDefined()
		} else {
			expect(options[0]).toBe(label())
		}
	})

	it("stores the reference once the global time is picked", async ({
		expect,
	}) => {
		const uid = activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
			},
		})
		await mountModal()
		await openSetting(t("general.time-range-options.last_24_hours"))

		teleportedButton(t("editor.config-modal-utils.variable-time-range")).click()
		// the item selects only after two ticks of its own
		await flushPromises()

		const store = useEditorStore()

		expect(
			store.metricBlockConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid]?.timeRange,
		).toBe(METRIC_TIME_RANGE_REFERENCE)
	})

	it("shows the time range a diff replaced beside the new one", async ({
		expect,
	}) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
			},
			diffStatus: DiffStatus.Modified,
			oldConfig: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last1Hour,
			},
		})

		await mountModal()

		expect(dialogText()).toContain(
			t("general.time-range-options.last_24_hours"),
		)
		expect(dialogText()).toContain(t("general.time-range-options.last_1_hour"))
	})

	it.for([
		{
			name: "moves the chart's title down while it shows an old time range",
			input: TimeRangePreset.Last1Hour,
			expected: 6.5,
		},
		{
			name: "keeps the chart's title in place while the time range is unchanged",
			input: TimeRangePreset.Last24Hours,
			expected: 0,
		},
	])("$name", async ({ input, expected }, { expect }) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
			},
			diffStatus: DiffStatus.Modified,
			oldConfig: { ...defaultMetricConfig(), timeRange: input },
		})

		const wrapper = await mountModal()

		expect(
			wrapper.findComponent(VisualizationContainer).props("titleOffsetTop"),
		).toBe(expected)
		expect(
			teleportedButton(t("general.time-range-options.last_24_hours"))
				.closest(".absolute")
				?.classList.contains("md:h-11"),
		).toBe(expected > 0)
	})

	it("shows the refresh interval a diff replaced beside the new one", async ({
		expect,
	}) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				refreshInterval: RefreshInterval.S30,
			},
			diffStatus: DiffStatus.Modified,
			oldConfig: {
				...defaultMetricConfig(),
				refreshInterval: RefreshInterval.H1,
			},
		})

		await mountModal()

		expect(dialogText()).toContain(
			t("general.refresh-interval-options-short.30s"),
		)
		expect(dialogText()).toContain(
			t("general.refresh-interval-options-short.1h"),
		)
	})

	it("shows the settings only once on a wide screen", async ({ expect }) => {
		activateBlock()

		const wrapper = await mountModal()

		expect(
			document.body.querySelectorAll("[role='dialog'] .w-70"),
		).toHaveLength(1)
		expect(wrapper.findAllComponents(ConfigSidebar)).toHaveLength(1)
	})

	it("folds the settings into the body on a narrow screen", async ({
		expect,
	}) => {
		stubViewportMatches(false)
		activateBlock()

		const wrapper = await mountModal()

		expect(
			document.body.querySelectorAll("[role='dialog'] .w-70"),
		).toHaveLength(0)
		expect(wrapper.findAllComponents(ConfigSidebar)).toHaveLength(1)
	})

	it("closes when the reader dismisses it", async ({ expect }) => {
		activateBlock()
		await mountModal()

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(useEditorStore().activeMetricBlockConfig).toBeNull()
	})

	it("starts a simulation the preview asks for", async ({ expect }) => {
		const config = defaultMetricConfig()
		const uid = activateBlock({ config: config })
		const wrapper = await mountModal()

		emitFrom(
			wrapper,
			VisualizationContainer,
			"simulate",
			MetricSimulationPreset.HTTPLatency,
		)
		await nextTick()

		expect(
			useEditorStore().metricBlockConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid]
				?.simulationPreset,
		).toBe(MetricSimulationPreset.HTTPLatency)
	})

	it("shows the simulation icon while the preview simulates", async ({
		expect,
	}) => {
		activateBlock()
		const wrapper = await mountModal()

		emitFrom(wrapper, VisualizationContainer, "simulation-visible", true)
		await nextTick()

		expect(simulationIconShown()).toBe(true)
	})

	it("hides the simulation icon while the preview draws real data", async ({
		expect,
	}) => {
		activateBlock()
		const wrapper = await mountModal()

		emitFrom(wrapper, VisualizationContainer, "simulation-visible", false)
		await nextTick()

		expect(simulationIconShown()).toBe(false)
	})

	it("stands aside when the reader asks for the data source settings", async ({
		expect,
	}) => {
		activateBlock()
		const wrapper = await mountModal()

		emitFrom(wrapper, ConfigSidebar, "open-settings")
		await nextTick()

		expect(useEditorStore().activeMetricBlockConfig).toBeNull()
		expect(
			wrapper.findComponent(BaseModal).emitted("open-settings"),
		).toHaveLength(1)
	})
})

// the modal teleports to the body, out of the wrapper's reach
function simulationIconShown(): boolean {
	return (
		document.body.querySelector('[class~="i-mingcute:test-tube-line"]') !== null
	)
}

async function openSetting(label: string) {
	const button = teleportedButton(label)
	if (button.getAttribute("aria-haspopup") === "dialog") {
		button.click()
	} else {
		button.dispatchEvent(
			new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
		)
	}
	await nextTick()
}

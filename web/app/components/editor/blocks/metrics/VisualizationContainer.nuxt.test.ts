import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import VisualizationContainer from "./VisualizationContainer.vue"
import { stubChartColorContext } from "./test-helpers"
import {
	defaultMetricConfig,
	TimeRangePreset,
	type MetricConfig,
} from "./utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	findButtonByText,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

const DATA_SOURCE_ID = makeXid("ds")
const QUERY_URL = `/api/data-sources/${DATA_SOURCE_ID}/query`

// the chart components draw through echarts onto a canvas happy-dom does
// not implement; the container's contract with them is which one it picks
// and what it hands over
const chartStubs = {
	LineChart: true,
	BarChart: true,
	GaugeChart: true,
}

function seriesResult(): unknown {
	return {
		status: GenericQueryResultStatus.Ok,
		data: [
			{
				labels: { instance: "web-1" },
				metrics: [
					[1700000000, 1],
					[1700000060, 2],
				],
			},
		],
	}
}

function metricConfig(overrides: Partial<MetricConfig> = {}): MetricConfig {
	return {
		...defaultMetricConfig(),
		dataSourceId: DATA_SOURCE_ID,
		visualizationType: GenericQueryChartType.Line,
		timeRange: TimeRangePreset.Last5Minutes,
		queries: [{ name: "Query 1", query: "up", legendFormat: "" }],
		...overrides,
	}
}

let uidCounter = 0

// the store tracks each block's next refresh by uid and keeps it for the
// whole document, so a shared uid would make later blocks look up to date
function nextUid(): string {
	uidCounter++

	return `metric-${uidCounter}`
}

function mountContainer(
	config: MetricConfig,
	props: Record<string, unknown> = {},
) {
	return mountSuspended(VisualizationContainer, {
		props: { config: config, uid: nextUid(), ...props },
		global: { stubs: chartStubs },
	})
}

function chartNames(wrapper: VueWrapper): string[] {
	return ["LineChart", "BarChart", "GaugeChart"].filter((name) =>
		wrapper.findComponent({ name: name }).exists(),
	)
}

// the editor store, the query cache and the editable flag are shared
// app-wide, so these tests cannot interleave
describe("<VisualizationContainer>", { concurrent: false }, () => {
	beforeEach(() => {
		stubChartColorContext()
		clearQueryCache()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().updateActiveDocumentId(makeXid("doc"))
		useEditorStore().updateActiveBranchId(makeXid("branch"))
		useEditorStore().activateMetricBlockConfig(null)
	})

	afterEach(disposeMockEndpoints)

	it("asks for a visualization type before anything else", async ({
		expect,
	}) => {
		const wrapper = await mountContainer(
			metricConfig({ visualizationType: null }),
		)

		expect(wrapper.text()).toContain(
			t("editor.metrics.status.type-not-selected.title"),
		)
		expect(wrapper.text()).toContain(
			t("editor.metrics.status.type-not-selected.normal-action-button"),
		)
	})

	it("offers only to view the metrics in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountContainer(
			metricConfig({ visualizationType: null }),
		)

		expect(wrapper.text()).toContain(
			t("editor.metrics.status.type-not-selected.readonly-action-button"),
		)
	})

	it("hides the action button when the host asks it to", async ({ expect }) => {
		const wrapper = await mountContainer(
			metricConfig({ visualizationType: null }),
			{ hideEmptyActionButton: true },
		)

		expect(wrapper.findAll("button")).toHaveLength(0)
	})

	it("shows a bare placeholder in a simplified host", async ({ expect }) => {
		const wrapper = await mountContainer(
			metricConfig({ visualizationType: null }),
			{ simplifiedEmpty: true },
		)

		expect(wrapper.text()).toBe(
			t("editor.metrics.status.simplified-config-in-progress.title"),
		)
	})

	it("asks for a data source once a type is chosen", async ({ expect }) => {
		const wrapper = await mountContainer(
			metricConfig({ dataSourceId: null, queries: [] }),
		)

		expect(wrapper.text()).toContain(
			t("editor.metrics.status.data-source-not-selected.title"),
		)
	})

	it("shows a bare placeholder for a missing data source in a simplified host", async ({
		expect,
	}) => {
		const wrapper = await mountContainer(
			metricConfig({ dataSourceId: null, queries: [] }),
			{ simplifiedEmpty: true },
		)

		expect(wrapper.text()).toBe(
			t("editor.metrics.status.simplified-config-in-progress.title"),
		)
	})

	it("reports that the queries returned nothing", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => ({
			status: GenericQueryResultStatus.NoData,
			data: [],
		}))

		const wrapper = await mountContainer(metricConfig())

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.status.no-data-loaded.title"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain(
			"The current configuration returned no data",
		)
	})

	it("draws a line chart from the returned series", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => seriesResult())

		const wrapper = await mountContainer(metricConfig({ title: "Requests" }))

		await vi.waitFor(() => {
			expect(chartNames(wrapper)).toEqual(["LineChart"])
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.findComponent({ name: "LineChart" }).props("title")).toBe(
			"Requests",
		)
	})

	it("draws a bar chart when that is the chosen type", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => seriesResult())

		const wrapper = await mountContainer(
			metricConfig({ visualizationType: GenericQueryChartType.Bar }),
		)

		await vi.waitFor(() => {
			expect(chartNames(wrapper)).toEqual(["BarChart"])
		}, WAIT_FOR_OPTIONS)
	})

	it("draws a gauge when that is the chosen type", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => seriesResult())

		const wrapper = await mountContainer(
			metricConfig({ visualizationType: GenericQueryChartType.Gauge }),
		)

		await vi.waitFor(() => {
			expect(chartNames(wrapper)).toEqual(["GaugeChart"])
		}, WAIT_FOR_OPTIONS)
	})

	it("passes only the enabled thresholds to the chart", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => seriesResult())

		const wrapper = await mountContainer(
			metricConfig({
				thresholds: [
					{ value: 10, label: "warn", color: "#ff0000" },
					{ label: "disabled", color: "#00ff00" },
				],
			}),
		)

		await vi.waitFor(() => {
			expect(chartNames(wrapper)).toEqual(["LineChart"])
		}, WAIT_FOR_OPTIONS)
		expect(
			wrapper.findComponent({ name: "LineChart" }).props("thresholds"),
		).toEqual([{ value: 10, label: "warn", color: "#ff0000" }])
	})

	it("reports the error a single failing query returned", async ({
		expect,
	}) => {
		mockEndpoint("GET", QUERY_URL, (_call, event) => {
			setResponseStatus(event, 400)

			return { code: "query.error", message: "bad selector" }
		})

		const wrapper = await mountContainer(metricConfig())

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.status.query-error.title"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).not.toContain("Query 1:")
	})

	it("numbers the failing query when there are several", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, (call, event) => {
			if (call.query.q === "down") {
				setResponseStatus(event, 400)

				return { code: "query.error", message: "bad selector" }
			}

			return seriesResult()
		})

		const wrapper = await mountContainer(
			metricConfig({
				queries: [
					{ name: "Query 1", query: "up", legendFormat: "" },
					{ name: "Query 2", query: "down", legendFormat: "" },
				],
			}),
		)

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.status.query-error.title"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain("Query 2:")
	})

	it("reports data the chosen chart cannot show", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => ({
			status: GenericQueryResultStatus.ChartAndDataMismatch,
			data: [],
		}))

		const wrapper = await mountContainer(metricConfig())

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.status.invalid-data.title"),
			)
		}, WAIT_FOR_OPTIONS)
	})

	it("opens the config modal from the action button", async ({ expect }) => {
		const uid = nextUid()
		const wrapper = await mountContainer(
			metricConfig({ visualizationType: null }),
			{ uid: uid },
		)

		await findButtonByText(
			wrapper,
			t("editor.metrics.status.type-not-selected.normal-action-button"),
		).trigger("click")

		expect(useEditorStore().activeMetricBlockConfig).toBe(uid)
	})

	it("reports its loading state around a refresh", async ({ expect }) => {
		mockEndpoint("GET", QUERY_URL, () => seriesResult())

		const wrapper = await mountContainer(metricConfig())

		await vi.waitFor(() => {
			expect(wrapper.emitted("loading")).toEqual([[true], [false]])
		}, WAIT_FOR_OPTIONS)
	})

	it("skips the refresh while refreshing is disabled", async ({ expect }) => {
		const calls = mockEndpoint("GET", QUERY_URL, () => seriesResult())

		const wrapper = await mountContainer(metricConfig(), {
			disableRefresh: true,
		})

		expect(wrapper.emitted("loading")).toBeUndefined()
		expect(calls.length).toBeLessThanOrEqual(1)
	})

	it("skips the refresh for a block that is not due yet", async ({
		expect,
	}) => {
		mockEndpoint("GET", QUERY_URL, () => seriesResult())
		const uid = nextUid()
		useEditorStore().setMetricBlockNextRefreshTimestamp(uid, 60_000)

		const wrapper = await mountContainer(metricConfig(), { uid: uid })

		expect(wrapper.emitted("loading")).toBeUndefined()
	})
})

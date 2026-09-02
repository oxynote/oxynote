import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import VisualizationContainer from "./VisualizationContainer.vue"
import { stubChartColorContext } from "./test-helpers"
import {
	defaultMetricConfig,
	MetricSimulationPreset,
	RefreshInterval,
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
	at,
	findButtonByText,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

const DATA_SOURCE_ID = makeXid("ds")
const QUERY_URL = `/api/data-sources/${DATA_SOURCE_ID}/query`
const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

function simulationCheckURL(uid: string): string {
	return `/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/blocks/${uid}/run`
}

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
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
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

	describe("simulation", () => {
		it("offers to simulate a configured block that returned nothing", async ({
			expect,
		}) => {
			mockEndpoint("GET", QUERY_URL, () => ({
				status: GenericQueryResultStatus.NoData,
				data: [],
			}))

			const wrapper = await mountContainer(metricConfig())

			await vi.waitFor(() => {
				expect(wrapper.text()).toContain(
					t("editor.metrics.status.no-data-loaded.simulate-action-button"),
				)
			}, WAIT_FOR_OPTIONS)
		})

		it.for([
			{
				name: "the block has no data source",
				config: { dataSourceId: null },
			},
			{
				name: "no query has been written yet",
				config: {
					queries: [{ name: "Query 1", query: " ", legendFormat: "" }],
				},
			},
		])(
			"hides the simulate button when $name",
			async ({ config }, { expect }) => {
				mockEndpoint("GET", QUERY_URL, () => ({
					status: GenericQueryResultStatus.NoData,
					data: [],
				}))

				const wrapper = await mountContainer(metricConfig(config))

				await vi.waitFor(() => {
					expect(wrapper.text()).toContain("No Data")
				}, WAIT_FOR_OPTIONS)
				expect(wrapper.text()).not.toContain(
					t("editor.metrics.status.no-data-loaded.simulate-action-button"),
				)
			},
		)

		it("hides the simulate button from a reader", async ({ expect }) => {
			useEditorMeta().setEditable(false)
			mockEndpoint("GET", QUERY_URL, () => ({
				status: GenericQueryResultStatus.NoData,
				data: [],
			}))

			const wrapper = await mountContainer(metricConfig())

			await vi.waitFor(() => {
				expect(wrapper.text()).toContain("No Data")
			}, WAIT_FOR_OPTIONS)
			expect(wrapper.text()).not.toContain(
				t("editor.metrics.status.no-data-loaded.simulate-action-button"),
			)
		})

		it("asks for the default preset when the button is pressed", async ({
			expect,
		}) => {
			mockEndpoint("GET", QUERY_URL, () => ({
				status: GenericQueryResultStatus.NoData,
				data: [],
			}))

			const wrapper = await mountContainer(metricConfig())

			await vi.waitFor(() => {
				expect(wrapper.text()).toContain(
					t("editor.metrics.status.no-data-loaded.simulate-action-button"),
				)
			}, WAIT_FOR_OPTIONS)
			await findButtonByText(
				wrapper,
				t("editor.metrics.status.no-data-loaded.simulate-action-button"),
			).trigger("click")

			expect(wrapper.emitted("simulate")).toEqual([
				[MetricSimulationPreset.CPUUsage],
			])
		})

		it("draws generated data without querying the data source", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", QUERY_URL, () => seriesResult())

			const wrapper = await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.HTTPLatency }),
			)

			expect(chartNames(wrapper)).toEqual(["LineChart"])
			expect(
				wrapper.findComponent({ name: "LineChart" }).props("seriesData"),
			).toHaveLength(3)
			expect(calls).toHaveLength(0)
		})

		it("offers to simulate from the config modal's simplified state", async ({
			expect,
		}) => {
			mockEndpoint("GET", QUERY_URL, () => ({
				status: GenericQueryResultStatus.NoData,
				data: [],
			}))

			const wrapper = await mountContainer(metricConfig(), {
				hideEmptyActionButton: true,
				simplifiedEmpty: true,
			})

			await vi.waitFor(() => {
				expect(wrapper.text()).toContain(
					t("editor.metrics.status.no-data-loaded.simulate-action-button"),
				)
			}, WAIT_FOR_OPTIONS)
			await findButtonByText(
				wrapper,
				t("editor.metrics.status.no-data-loaded.simulate-action-button"),
			).trigger("click")

			expect(wrapper.emitted("simulate")).toEqual([
				[MetricSimulationPreset.CPUUsage],
			])
		})

		it("queries normally when the stored preset is not one it knows", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", QUERY_URL, () => seriesResult())

			const wrapper = await mountContainer(
				metricConfig({
					simulationPreset: "pie_of_the_day" as MetricSimulationPreset,
				}),
			)

			await vi.waitFor(() => {
				expect(calls.length).toBeGreaterThan(0)
			}, WAIT_FOR_OPTIONS)
			expect(wrapper.text()).not.toContain(t("editor.metrics.simulation.label"))
		})

		// a simulation stands in for a metric that has not arrived, not
		// for a source nobody can reach
		it.for([
			{ name: "unreachable", status: DataSourceStatus.Unreachable },
			{ name: "unauthorized", status: DataSourceStatus.Unauthorized },
		])(
			"draws no simulation over a $name data source",
			async ({ status }, { expect }) => {
				mockEndpoint("GET", "/api/data-sources", () => [
					{ id: DATA_SOURCE_ID, name: "Test", status: status },
				])
				mockEndpoint("GET", QUERY_URL, () => seriesResult())

				const wrapper = await mountContainer(
					metricConfig({
						simulationPreset: MetricSimulationPreset.CPUUsage,
					}),
				)

				await vi.waitFor(() => {
					expect(wrapper.text()).not.toContain(
						t("editor.metrics.simulation.label"),
					)
				}, WAIT_FOR_OPTIONS)
			},
		)

		it("draws the simulation while the source answers", async ({ expect }) => {
			mockEndpoint("GET", "/api/data-sources", () => [
				{ id: DATA_SOURCE_ID, name: "Test", status: DataSourceStatus.Success },
			])
			mockEndpoint("GET", QUERY_URL, () => seriesResult())

			const wrapper = await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.CPUUsage }),
			)

			expect(wrapper.text()).toContain(t("editor.metrics.simulation.label"))
		})

		it("draws the window the block is configured for", async ({ expect }) => {
			mockEndpoint("GET", QUERY_URL, () => seriesResult())

			const wrapper = await mountContainer(
				metricConfig({
					timeRange: TimeRangePreset.Last15Minutes,
					simulationPreset: MetricSimulationPreset.CPUUsage,
				}),
			)

			const series = wrapper
				.findComponent({ name: "LineChart" })
				.props("seriesData") as { data: [Date, number][] }[]
			const points = at(series, 0).data
			const first = at(points, 0)[0]
			const last = at(points, points.length - 1)[0]

			expect(last.getTime() - first.getTime()).toBeGreaterThan(13 * 60 * 1000)
			expect(last.getTime() - first.getTime()).toBeLessThanOrEqual(
				15 * 60 * 1000,
			)
			// the window ends at the present, which is what makes it slide
			expect(Date.now() - last.getTime()).toBeLessThan(60 * 1000)
		})

		it.for([
			{ type: GenericQueryChartType.Line, chart: "LineChart" },
			{ type: GenericQueryChartType.Bar, chart: "BarChart" },
			{ type: GenericQueryChartType.Gauge, chart: "GaugeChart" },
		])(
			"simulates through a $chart too",
			async ({ type, chart }, { expect }) => {
				mockEndpoint("GET", QUERY_URL, () => seriesResult())

				const wrapper = await mountContainer(
					metricConfig({
						visualizationType: type,
						simulationPreset: MetricSimulationPreset.CPUUsage,
					}),
				)

				expect(chartNames(wrapper)).toEqual([chart])
			},
		)

		it("marks a simulated block for every reader", async ({ expect }) => {
			useEditorMeta().setEditable(false)
			mockEndpoint("GET", QUERY_URL, () => seriesResult())

			const wrapper = await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.CPUUsage }),
			)

			expect(wrapper.text()).toContain(t("editor.metrics.simulation.label"))
		})

		it("leaves an unsimulated block unmarked", async ({ expect }) => {
			mockEndpoint("GET", QUERY_URL, () => seriesResult())

			const wrapper = await mountContainer(metricConfig())

			expect(wrapper.text()).not.toContain(t("editor.metrics.simulation.label"))
		})

		it("asks core whether the real data has arrived", async ({ expect }) => {
			const uid = nextUid()
			const checks = mockEndpoint("POST", simulationCheckURL(uid), () => ({
				cleared: false,
			}))
			mockEndpoint("GET", QUERY_URL, () => seriesResult())

			await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.CPUUsage }),
				{ uid: uid },
			)

			await vi.waitFor(() => {
				expect(checks.length).toBeGreaterThan(0)
			}, WAIT_FOR_OPTIONS)
		})

		// the check runs on its own cadence: a chart that redraws once a
		// day would otherwise keep simulating for a day after its metric
		// went live
		it("asks even when the block redraws once a day", async ({ expect }) => {
			const uid = nextUid()
			const checks = mockEndpoint("POST", simulationCheckURL(uid), () => ({
				cleared: false,
			}))
			mockEndpoint("GET", QUERY_URL, () => seriesResult())

			await mountContainer(
				metricConfig({
					refreshInterval: RefreshInterval.D1,
					simulationPreset: MetricSimulationPreset.CPUUsage,
				}),
				{ uid: uid },
			)

			await vi.waitFor(() => {
				expect(checks.length).toBeGreaterThan(0)
			}, WAIT_FOR_OPTIONS)
		})

		// the block mounts before the route has published the ids, which
		// is what a page load looks like
		it("waits for the branch id rather than skipping the check", async ({
			expect,
		}) => {
			const uid = nextUid()
			const checks = mockEndpoint("POST", simulationCheckURL(uid), () => ({
				cleared: false,
			}))
			mockEndpoint("GET", QUERY_URL, () => seriesResult())
			useEditorStore().updateActiveBranchId(null)

			await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.CPUUsage }),
				{ uid: uid },
			)

			expect(checks).toHaveLength(0)

			useEditorStore().updateActiveBranchId(BRANCH_ID)

			await vi.waitFor(() => {
				expect(checks.length).toBeGreaterThan(0)
			}, WAIT_FOR_OPTIONS)
		})

		it("does not ask while refreshing is disabled", async ({ expect }) => {
			const uid = nextUid()
			const checks = mockEndpoint("POST", simulationCheckURL(uid), () => ({
				cleared: false,
			}))

			await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.CPUUsage }),
				{ uid: uid, disableRefresh: true },
			)

			expect(checks).toHaveLength(0)
		})

		// the query starts cold when the simulation is lifted, and the
		// empty state would otherwise flash at data that is on its way
		it("keeps drawing the generated series until the first result lands", async ({
			expect,
		}) => {
			let release!: () => void
			const answered = new Promise<void>((resolve) => {
				release = resolve
			})
			mockEndpoint("GET", QUERY_URL, async () => {
				await answered

				return seriesResult()
			})
			const config = metricConfig({
				simulationPreset: MetricSimulationPreset.HTTPLatency,
			})

			const wrapper = await mountContainer(config)

			await wrapper.setProps({
				config: { ...config, simulationPreset: null },
			})
			await nextTick()

			expect(wrapper.text()).not.toContain(
				t("editor.metrics.status.no-data-loaded.title"),
			)
			expect(wrapper.text()).toContain(t("editor.metrics.simulation.label"))
			expect(
				wrapper.findComponent({ name: "LineChart" }).props("seriesData"),
			).toHaveLength(3)

			release()

			await vi.waitFor(() => {
				expect(
					wrapper.findComponent({ name: "LineChart" }).props("seriesData"),
				).toHaveLength(1)
			}, WAIT_FOR_OPTIONS)
			expect(wrapper.text()).not.toContain(t("editor.metrics.simulation.label"))
		})

		// a block whose queries cannot run is never answered, and holding
		// on that would draw generated data for good — with the sidebar
		// section gone, there would be nothing left to turn it off with
		it("stops drawing once cleared when the queries cannot run", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", QUERY_URL, () => seriesResult())
			const config = metricConfig({
				queries: [
					{ name: "Query 1", query: "up", legendFormat: "" },
					{ name: "Query 2", query: "", legendFormat: "" },
				],
				simulationPreset: MetricSimulationPreset.CPUUsage,
			})

			const wrapper = await mountContainer(config)

			await wrapper.setProps({
				config: { ...config, simulationPreset: null },
			})
			await nextTick()

			expect(wrapper.text()).not.toContain(t("editor.metrics.simulation.label"))
			expect(calls).toHaveLength(0)
		})

		// the cache is keyed by the block's time range and not by the
		// window it resolves to, so the answer waiting after a clear was
		// taken while the metric was still missing
		it("refetches on a clear rather than trusting the cached answer", async ({
			expect,
		}) => {
			let answered = false
			const calls = mockEndpoint("GET", QUERY_URL, () => {
				if (!answered) {
					answered = true

					return { status: GenericQueryResultStatus.NoData, data: [] }
				}

				return seriesResult()
			})
			const config = metricConfig()

			// the block asks once while its metric is still missing,
			// which is the answer that gets cached
			const wrapper = await mountContainer(config)

			await vi.waitFor(() => {
				expect(calls.length).toBeGreaterThan(0)
			}, WAIT_FOR_OPTIONS)

			const asked = calls.length

			await wrapper.setProps({
				config: {
					...config,
					simulationPreset: MetricSimulationPreset.CPUUsage,
				},
			})
			await nextTick()

			await wrapper.setProps({
				config: { ...config, simulationPreset: null },
			})

			await vi.waitFor(() => {
				expect(chartNames(wrapper)).toEqual(["LineChart"])
			}, WAIT_FOR_OPTIONS)
			expect(calls.length).toBeGreaterThan(asked)
		})

		it("swaps to real data when the backend clears the simulation", async ({
			expect,
		}) => {
			const uid = nextUid()
			mockEndpoint("POST", simulationCheckURL(uid), () => ({ cleared: true }))
			const calls = mockEndpoint("GET", QUERY_URL, () => seriesResult())
			const config = metricConfig({
				simulationPreset: MetricSimulationPreset.CPUUsage,
			})

			const wrapper = await mountContainer(config, { uid: uid })

			expect(wrapper.text()).toContain(t("editor.metrics.simulation.label"))

			// core removes the attribute on the live document, which reaches
			// the editor as a node attribute change
			await wrapper.setProps({
				config: { ...config, simulationPreset: null },
			})

			await vi.waitFor(() => {
				expect(calls.length).toBeGreaterThan(0)
			}, WAIT_FOR_OPTIONS)
			expect(wrapper.text()).not.toContain(t("editor.metrics.simulation.label"))
		})

		it("keeps drawing when the check fails", async ({ expect }) => {
			const uid = nextUid()
			mockEndpoint("POST", simulationCheckURL(uid), (_call, event) => {
				setResponseStatus(event, 500)

				return { error: "boom" }
			})

			const wrapper = await mountContainer(
				metricConfig({ simulationPreset: MetricSimulationPreset.CPUUsage }),
				{ uid: uid },
			)

			await vi.waitFor(() => {
				expect(chartNames(wrapper)).toEqual(["LineChart"])
			}, WAIT_FOR_OPTIONS)
		})
	})
})

import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	buildConfigFromNodeAttrs,
	defaultMetricConfig,
	MetricSimulationPreset,
	RefreshInterval,
	refreshIntervalToMs,
	resolveTimeRange,
	TimeRangePreset,
	VisualizationDataUnit,
	VisualizationTimeUnit,
} from "./utils"

// chartStyles resolves CSS variables through a canvas, which needs a real
// DOM — the default threshold color is all defaultMetricConfig reads
vi.mock("~/assets/css", () => ({
	chartStyles: () => ({ selectableColors: { default: "#8a3ffc" } }),
}))

// a wednesday in the middle of a month, with sub-second precision so
// boundary presets prove they snap to exact instants. Local-time
// components keep the expectations timezone-independent.
const NOW = new Date(2026, 7, 19, 15, 30, 45, 123)

describe("defaultMetricConfig", () => {
	it("returns a config with one empty query and the default threshold color", ({
		expect,
	}) => {
		expect(defaultMetricConfig()).toEqual({
			title: "",
			dataSourceId: null,
			visualizationType: null,
			queries: [{ name: "Query 1", query: "", legendFormat: "" }],
			timeRange: TimeRangePreset.Last5Minutes,
			refreshInterval: RefreshInterval.M5,
			thresholds: null,
			baseThresholdColor: "#8a3ffc",
			decimals: null,
			unit: { type: null, custom: null },
			axisBounds: { min: null, max: null },
			simulationPreset: null,
		})
	})

	it("returns a fresh object graph on every call", ({ expect }) => {
		const first = defaultMetricConfig()
		const second = defaultMetricConfig()

		expect(first).not.toBe(second)
		expect(first.queries).not.toBe(second.queries)
		expect(first.unit).not.toBe(second.unit)
		expect(first.axisBounds).not.toBe(second.axisBounds)
	})
})

describe("buildConfigFromNodeAttrs", () => {
	it("maps a legacy config blob, translating the old type field", ({
		expect,
	}) => {
		const config = buildConfigFromNodeAttrs({
			config: {
				title: "CPU",
				dataSourceId: "ds-1",
				type: GenericQueryChartType.Line,
				queries: [{ name: "Query 1", query: "up", legendFormat: "" }],
				timeRange: TimeRangePreset.Last1Hour,
				refreshInterval: RefreshInterval.M1,
				thresholds: [{ value: 10, label: "warn", color: "#f00" }],
				baseThresholdColor: "#00f",
				decimals: 2,
				unit: { type: VisualizationTimeUnit.Seconds, custom: null },
				axisBounds: { min: 0, max: 100 },
			},
		})

		expect(config).toEqual({
			title: "CPU",
			dataSourceId: "ds-1",
			visualizationType: GenericQueryChartType.Line,
			queries: [{ name: "Query 1", query: "up", legendFormat: "" }],
			timeRange: TimeRangePreset.Last1Hour,
			refreshInterval: RefreshInterval.M1,
			thresholds: [{ value: 10, label: "warn", color: "#f00" }],
			baseThresholdColor: "#00f",
			decimals: 2,
			unit: { type: VisualizationTimeUnit.Seconds, custom: null },
			axisBounds: { min: 0, max: 100 },
			simulationPreset: null,
		})
	})

	it("falls back to visualizationType when the legacy blob has no type field", ({
		expect,
	}) => {
		const config = buildConfigFromNodeAttrs({
			config: { visualizationType: GenericQueryChartType.Gauge },
		})

		expect(config.visualizationType).toBe(GenericQueryChartType.Gauge)
	})

	it("fills defaults for fields the legacy blob is missing", ({ expect }) => {
		expect(buildConfigFromNodeAttrs({ config: {} })).toEqual({
			title: "",
			dataSourceId: null,
			visualizationType: null,
			queries: null,
			timeRange: null,
			refreshInterval: null,
			thresholds: null,
			baseThresholdColor: "",
			decimals: null,
			unit: { type: null, custom: null },
			axisBounds: { min: null, max: null },
			simulationPreset: null,
		})
	})

	it("assembles the config from flat node attributes", ({ expect }) => {
		const config = buildConfigFromNodeAttrs({
			title: "Memory",
			dataSourceId: "ds-2",
			visualizationType: GenericQueryChartType.Bar,
			queries: [{ name: "Query 1", query: "mem", legendFormat: "{{job}}" }],
			timeRange: TimeRangePreset.Last30Days,
			refreshInterval: RefreshInterval.H1,
			thresholds: [{ value: 5 }],
			baseThresholdColor: "#0f0",
			decimals: 0,
			unitType: VisualizationDataUnit.Megabytes,
			unitCustom: "reqs",
			axisBoundsMin: -1,
			axisBoundsMax: 1,
		})

		expect(config).toEqual({
			title: "Memory",
			dataSourceId: "ds-2",
			visualizationType: GenericQueryChartType.Bar,
			queries: [{ name: "Query 1", query: "mem", legendFormat: "{{job}}" }],
			timeRange: TimeRangePreset.Last30Days,
			refreshInterval: RefreshInterval.H1,
			thresholds: [{ value: 5 }],
			baseThresholdColor: "#0f0",
			decimals: 0,
			unit: { type: VisualizationDataUnit.Megabytes, custom: "reqs" },
			axisBounds: { min: -1, max: 1 },
			simulationPreset: null,
		})
	})

	it("reads the simulation preset from the flat attribute", ({ expect }) => {
		expect(
			buildConfigFromNodeAttrs({
				simulationPreset: MetricSimulationPreset.HTTPLatency,
			}).simulationPreset,
		).toBe(MetricSimulationPreset.HTTPLatency)
	})

	it("fills defaults when there are no attributes at all", ({ expect }) => {
		expect(buildConfigFromNodeAttrs({})).toEqual({
			title: "",
			dataSourceId: null,
			visualizationType: null,
			queries: null,
			timeRange: null,
			refreshInterval: null,
			thresholds: null,
			baseThresholdColor: "",
			decimals: null,
			unit: { type: null, custom: null },
			axisBounds: { min: null, max: null },
			simulationPreset: null,
		})
	})
})

describe("resolveTimeRange", () => {
	it.for([
		{
			preset: TimeRangePreset.Last5Minutes,
			from: new Date(2026, 7, 19, 15, 25, 45, 123),
		},
		{
			preset: TimeRangePreset.Last15Minutes,
			from: new Date(2026, 7, 19, 15, 15, 45, 123),
		},
		{
			preset: TimeRangePreset.Last30Minutes,
			from: new Date(2026, 7, 19, 15, 0, 45, 123),
		},
		{
			preset: TimeRangePreset.Last1Hour,
			from: new Date(2026, 7, 19, 14, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last3Hours,
			from: new Date(2026, 7, 19, 12, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last6Hours,
			from: new Date(2026, 7, 19, 9, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last12Hours,
			from: new Date(2026, 7, 19, 3, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last24Hours,
			from: new Date(2026, 7, 18, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last2Days,
			from: new Date(2026, 7, 17, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last7Days,
			from: new Date(2026, 7, 12, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last30Days,
			from: new Date(2026, 6, 20, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last90Days,
			from: new Date(2026, 4, 21, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last6Months,
			from: new Date(2026, 1, 19, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last1Year,
			from: new Date(2025, 7, 19, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last2Years,
			from: new Date(2024, 7, 19, 15, 30, 45, 123),
		},
		{
			preset: TimeRangePreset.Last5Years,
			from: new Date(2021, 7, 19, 15, 30, 45, 123),
		},
	])(
		"resolves $preset relative to the reference date",
		({ preset, from }, { expect }) => {
			expect(resolveTimeRange(preset, NOW)).toEqual({ from, to: NOW })
		},
	)

	it.for([
		{
			preset: TimeRangePreset.Today,
			from: new Date(2026, 7, 19, 0, 0, 0, 0),
			to: new Date(2026, 7, 19, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.TodaySoFar,
			from: new Date(2026, 7, 19, 0, 0, 0, 0),
			to: NOW,
		},
		{
			preset: TimeRangePreset.Yesterday,
			from: new Date(2026, 7, 18, 0, 0, 0, 0),
			to: new Date(2026, 7, 18, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.ThisWeek,
			from: new Date(2026, 7, 17, 0, 0, 0, 0),
			to: new Date(2026, 7, 23, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.ThisWeekSoFar,
			from: new Date(2026, 7, 17, 0, 0, 0, 0),
			to: NOW,
		},
		{
			preset: TimeRangePreset.ThisMonth,
			from: new Date(2026, 7, 1, 0, 0, 0, 0),
			to: new Date(2026, 7, 31, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.ThisMonthSoFar,
			from: new Date(2026, 7, 1, 0, 0, 0, 0),
			to: NOW,
		},
		{
			preset: TimeRangePreset.ThisYear,
			from: new Date(2026, 0, 1, 0, 0, 0, 0),
			to: new Date(2026, 11, 31, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.ThisYearSoFar,
			from: new Date(2026, 0, 1, 0, 0, 0, 0),
			to: NOW,
		},
		{
			preset: TimeRangePreset.PreviousWeek,
			from: new Date(2026, 7, 10, 0, 0, 0, 0),
			to: new Date(2026, 7, 16, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.PreviousMonth,
			from: new Date(2026, 6, 1, 0, 0, 0, 0),
			to: new Date(2026, 6, 31, 23, 59, 59, 999),
		},
		{
			preset: TimeRangePreset.PreviousYear,
			from: new Date(2025, 0, 1, 0, 0, 0, 0),
			to: new Date(2025, 11, 31, 23, 59, 59, 999),
		},
	])(
		"aligns $preset to calendar boundaries",
		({ preset, from, to }, { expect }) => {
			expect(resolveTimeRange(preset, NOW)).toEqual({ from, to })
		},
	)

	// weeks start on monday, so a sunday reference date is the last day
	// of its week, and a january reference date pushes every "previous"
	// preset across the year boundary
	it.for([
		{
			name: "keeps this_week anchored to monday when now is a sunday",
			now: new Date(2026, 7, 23, 12, 0, 0, 0),
			preset: TimeRangePreset.ThisWeek,
			from: new Date(2026, 7, 17, 0, 0, 0, 0),
			to: new Date(2026, 7, 23, 23, 59, 59, 999),
		},
		{
			name: "starts this_week at midnight when now is a monday",
			now: new Date(2026, 7, 17, 8, 0, 0, 0),
			preset: TimeRangePreset.ThisWeek,
			from: new Date(2026, 7, 17, 0, 0, 0, 0),
			to: new Date(2026, 7, 23, 23, 59, 59, 999),
		},
		{
			name: "resolves previous_week across the year boundary",
			now: new Date(2026, 0, 2, 10, 0, 0, 0),
			preset: TimeRangePreset.PreviousWeek,
			from: new Date(2025, 11, 22, 0, 0, 0, 0),
			to: new Date(2025, 11, 28, 23, 59, 59, 999),
		},
		{
			name: "resolves previous_month in january to december of last year",
			now: new Date(2026, 0, 2, 10, 0, 0, 0),
			preset: TimeRangePreset.PreviousMonth,
			from: new Date(2025, 11, 1, 0, 0, 0, 0),
			to: new Date(2025, 11, 31, 23, 59, 59, 999),
		},
		{
			name: "spans the whole previous month even when it is shorter",
			now: new Date(2026, 2, 15, 10, 0, 0, 0),
			preset: TimeRangePreset.PreviousMonth,
			from: new Date(2026, 1, 1, 0, 0, 0, 0),
			to: new Date(2026, 1, 28, 23, 59, 59, 999),
		},
	])("$name", ({ now, preset, from, to }, { expect }) => {
		expect(resolveTimeRange(preset, now)).toEqual({ from, to })
	})

	// fake timers pin the wall clock — shared mutable state — so this
	// block cannot interleave with other tests
	describe("when now is omitted", { concurrent: false }, () => {
		beforeEach(() => {
			vi.useFakeTimers()
			vi.setSystemTime(NOW)
		})

		afterEach(() => {
			vi.useRealTimers()
		})

		it("reads the current time", ({ expect }) => {
			expect(resolveTimeRange(TimeRangePreset.Last5Minutes)).toEqual({
				from: new Date(2026, 7, 19, 15, 25, 45, 123),
				to: NOW,
			})
		})
	})
})

describe("refreshIntervalToMs", () => {
	it.for([
		{ interval: RefreshInterval.S5, expected: 5_000 },
		{ interval: RefreshInterval.S10, expected: 10_000 },
		{ interval: RefreshInterval.S30, expected: 30_000 },
		{ interval: RefreshInterval.M1, expected: 60_000 },
		{ interval: RefreshInterval.M5, expected: 300_000 },
		{ interval: RefreshInterval.M15, expected: 900_000 },
		{ interval: RefreshInterval.M30, expected: 1_800_000 },
		{ interval: RefreshInterval.H1, expected: 3_600_000 },
		{ interval: RefreshInterval.H2, expected: 7_200_000 },
		{ interval: RefreshInterval.D1, expected: 86_400_000 },
	])(
		"converts $interval to $expected milliseconds",
		({ interval, expected }, { expect }) => {
			expect(refreshIntervalToMs(interval)).toBe(expected)
		},
	)
})

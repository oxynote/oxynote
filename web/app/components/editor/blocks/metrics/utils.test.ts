import { describe, it } from "vitest"
import {
	buildConfigFromNodeAttrs,
	defaultMetricConfig,
	MetricSimulationPreset,
	requestErrorMessage,
	VisualizationDataUnit,
	VisualizationTimeUnit,
} from "./utils"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

describe("defaultMetricConfig", () => {
	it("returns a config that follows the built-in time range and refresh variables", ({
		expect,
	}) => {
		expect(defaultMetricConfig()).toEqual({
			title: "",
			dataSourceId: null,
			visualizationType: GenericQueryChartType.Line,
			queries: [{ name: "Query 1", query: "", legendFormat: "" }],
			timeRange: METRIC_TIME_RANGE_REFERENCE,
			refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
			thresholds: null,
			baseThresholdColor: "green",
			decimals: null,
			unit: { type: null, custom: null },
			axisBounds: { min: null, max: null },
			simulationPreset: null,
			simulationActive: false,
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
			simulationActive: false,
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
			visualizationType: GenericQueryChartType.Line,
			queries: null,
			timeRange: null,
			refreshInterval: null,
			thresholds: null,
			baseThresholdColor: "",
			decimals: null,
			unit: { type: null, custom: null },
			axisBounds: { min: null, max: null },
			simulationPreset: null,
			simulationActive: false,
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
			simulationActive: false,
		})
	})

	it("reads the simulation preset from the flat attribute", ({ expect }) => {
		expect(
			buildConfigFromNodeAttrs({
				simulationPreset: MetricSimulationPreset.HTTPLatency,
			}).simulationPreset,
		).toBe(MetricSimulationPreset.HTTPLatency)
	})

	it.for([
		{ name: "a set flag", input: true, expected: true },
		{ name: "a cleared flag", input: false, expected: false },
		{
			name: "a block saved before the flag existed",
			input: undefined,
			expected: false,
		},
		{ name: "a flag that is not a boolean", input: "true", expected: false },
	])(
		"reads $name as simulating: $expected",
		({ input, expected }, { expect }) => {
			expect(
				buildConfigFromNodeAttrs({
					simulationPreset: MetricSimulationPreset.HTTPLatency,
					simulationActive: input,
				}).simulationActive,
			).toBe(expected)
		},
	)

	it("fills defaults when there are no attributes at all", ({ expect }) => {
		expect(buildConfigFromNodeAttrs({})).toEqual({
			title: "",
			dataSourceId: null,
			visualizationType: GenericQueryChartType.Line,
			queries: null,
			timeRange: null,
			refreshInterval: null,
			thresholds: null,
			baseThresholdColor: "",
			decimals: null,
			unit: { type: null, custom: null },
			axisBounds: { min: null, max: null },
			simulationPreset: null,
			simulationActive: false,
		})
	})
})

describe("requestErrorMessage", () => {
	it.for([
		{
			name: "returns the message the server sent with the error",
			input: { data: { code: "internal", message: "connection refused" } },
			expected: "connection refused",
		},
		{
			name: "returns null for an error the server sent no message with",
			input: { data: { code: "internal" } },
			expected: null,
		},
		{
			name: "returns null for an empty message",
			input: { data: { message: "" } },
			expected: null,
		},
		{
			name: "returns null for an error without a response body",
			input: { data: "" },
			expected: null,
		},
		{
			name: "returns null for something that is not an error object",
			input: "network down",
			expected: null,
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(requestErrorMessage(input)).toBe(expected)
	})
})

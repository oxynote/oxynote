import { describe, it } from "vitest"
import {
	buildLogConfigFromNodeAttrs,
	defaultLogConfig,
	LogTimestampFormat,
} from "./utils"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

describe("defaultLogConfig", () => {
	it("returns a config with one empty query, the last 5 minutes refreshed every 5 minutes, readable times and the newest entries first", ({
		expect,
	}) => {
		expect(defaultLogConfig()).toEqual({
			title: "",
			dataSourceId: null,
			queries: [{ name: "Query 1", query: "" }],
			timeRange: TimeRangePreset.Last5Minutes,
			refreshInterval: RefreshInterval.M5,
			timestampFormat: LogTimestampFormat.ReadableTime,
			order: LogOrder.Desc,
		})
	})

	it("returns a fresh object graph on every call", ({ expect }) => {
		const first = defaultLogConfig()
		const second = defaultLogConfig()

		expect(first).not.toBe(second)
		expect(first.queries).not.toBe(second.queries)
	})
})

describe("buildLogConfigFromNodeAttrs", () => {
	it("reads every field from the attributes", ({ expect }) => {
		const config = buildLogConfigFromNodeAttrs({
			title: "API errors",
			dataSourceId: "ds-1",
			queries: [{ name: "Query 1", query: '{app="api"}' }],
			timeRange: METRIC_TIME_RANGE_REFERENCE,
			refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
			timestampFormat: LogTimestampFormat.NumericNano,
			order: LogOrder.Asc,
			width: "wide",
		})

		expect(config).toEqual({
			title: "API errors",
			dataSourceId: "ds-1",
			queries: [{ name: "Query 1", query: '{app="api"}' }],
			timeRange: METRIC_TIME_RANGE_REFERENCE,
			refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
			timestampFormat: LogTimestampFormat.NumericNano,
			order: LogOrder.Asc,
		})
	})

	it.for([
		{
			name: "falls back to the defaults when the attributes are missing",
			input: {},
		},
		{
			name: "falls back to the defaults when the attributes are null",
			input: {
				title: null,
				dataSourceId: null,
				queries: null,
				timeRange: null,
				refreshInterval: null,
				timestampFormat: null,
				order: null,
			},
		},
	])("$name", ({ input }, { expect }) => {
		expect(buildLogConfigFromNodeAttrs(input)).toEqual({
			title: "",
			dataSourceId: null,
			queries: null,
			timeRange: null,
			refreshInterval: null,
			timestampFormat: LogTimestampFormat.ReadableTime,
			order: LogOrder.Desc,
		})
	})

	it("reads a timestamp format the block no longer offers as the default", ({
		expect,
	}) => {
		const config = buildLogConfigFromNodeAttrs({ timestampFormat: "standard" })

		expect(config.timestampFormat).toBe(LogTimestampFormat.ReadableTime)
	})
})

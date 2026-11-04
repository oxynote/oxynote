import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import type { RefreshIntervalSetting, TimeRangeSetting } from "~/utils/variable"

export enum LogTimestampFormat {
	None = "none",
	Readable = "readable",
	ReadableMilli = "readable_milli",
	ReadableNano = "readable_nano",
	ReadableTime = "readable_time",
	ReadableTimeMilli = "readable_time_milli",
	ReadableTimeNano = "readable_time_nano",
	Numeric = "numeric",
	NumericMilli = "numeric_milli",
	NumericNano = "numeric_nano",
	NumericTime = "numeric_time",
	NumericTimeMilli = "numeric_time_milli",
	NumericTimeNano = "numeric_time_nano",
}

export interface LogQuery {
	name: string
	query: string
}

export interface LogConfig {
	title: string
	dataSourceId: string | null
	queries: LogQuery[] | null
	timeRange: TimeRangeSetting | null
	refreshInterval: RefreshIntervalSetting | null
	timestampFormat: LogTimestampFormat
	order: LogOrder
}

export function defaultLogConfig(): LogConfig {
	// a new field needs a node attribute in index.ts too
	return {
		title: "",
		dataSourceId: null,
		// this runs outside a reactive context, so the name is not translated
		queries: [{ name: "Query 1", query: "" }],
		timeRange: TimeRangePreset.Last5Minutes,
		refreshInterval: RefreshInterval.M5,
		timestampFormat: LogTimestampFormat.ReadableTime,
		order: LogOrder.Desc,
	}
}

// buildLogConfigFromNodeAttrs reads a config from node attributes, such as
// those of the original node in a diff
export function buildLogConfigFromNodeAttrs(
	attrs: Record<string, unknown>,
): LogConfig {
	const defaults = defaultLogConfig()

	return {
		title: (attrs.title as string | null | undefined) ?? defaults.title,
		dataSourceId: (attrs.dataSourceId as string | null | undefined) ?? null,
		queries: (attrs.queries as LogQuery[] | null | undefined) ?? null,
		timeRange: (attrs.timeRange as TimeRangeSetting | null | undefined) ?? null,
		refreshInterval:
			(attrs.refreshInterval as RefreshIntervalSetting | null | undefined) ??
			null,
		// a format the block no longer offers reads as the default
		timestampFormat: isTimestampFormat(attrs.timestampFormat)
			? attrs.timestampFormat
			: defaults.timestampFormat,
		order: (attrs.order as LogOrder | null | undefined) ?? defaults.order,
	}
}

function isTimestampFormat(value: unknown): value is LogTimestampFormat {
	return Object.values<unknown>(LogTimestampFormat).includes(value)
}

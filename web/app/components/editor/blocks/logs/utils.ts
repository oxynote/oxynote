import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import type { RefreshIntervalSetting, TimeRangeSetting } from "~/utils/variable"

export enum LogTimestampFormat {
	None = "none",
	Standard = "standard",
	Detailed = "detailed",
	DetailedMilli = "detailed_milli",
	DetailedNano = "detailed_nano",
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
		timestampFormat: LogTimestampFormat.Standard,
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
		timestampFormat:
			(attrs.timestampFormat as LogTimestampFormat | null | undefined) ??
			defaults.timestampFormat,
		order: (attrs.order as LogOrder | null | undefined) ?? defaults.order,
	}
}

import { DEFAULT_PALETTE_COLOR } from "~/utils/color"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import type { RefreshIntervalSetting, TimeRangeSetting } from "~/utils/variable"

export const CONFIG_DEBOUNCE_MS = 500
export const NODE_ATTR_UPDATE_REFRESH_DISABLE_MS = CONFIG_DEBOUNCE_MS + 1000 // slightly longer than config debounce to avoid content refreshes on drag-drop

export const VISUALIZATION_MAX_DECIMALS = 16
export const VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH = 8

export enum VisualizationCoreUnit {
	Custom = "custom",
}

export enum VisualizationTimeUnit {
	Nanoseconds = "nanoseconds",
	Microseconds = "microseconds",
	Milliseconds = "milliseconds",
	Seconds = "seconds",
	Minutes = "minutes",
	Hours = "hours",
	Days = "days",
}

export enum VisualizationDataUnit {
	Bytes = "bytes",
	Kilobytes = "kilobytes",
	Megabytes = "megabytes",
	Gigabytes = "gigabytes",
	Terabytes = "terabytes",
	Bits = "bits",
	Kilobits = "kilobits",
	Megabits = "megabits",
	Gigabits = "gigabits",
	Terabits = "terabits",
}

export enum VisualizationMiscUnit {
	Percent0to100 = "percent0to100",
	Percent0to1 = "percent0to1",
}

export type VisualizationUnit =
	| VisualizationCoreUnit
	| VisualizationTimeUnit
	| VisualizationDataUnit
	| VisualizationMiscUnit

// the generated series a block draws while the metric it documents has no
// real data to answer with
export enum MetricSimulationPreset {
	CPUUsage = "cpu_usage",
	MemoryUsage = "memory_usage",
	DiskUsage = "disk_usage",
	HTTPRequests = "http_requests",
	HTTPLatency = "http_latency",
	ErrorRate = "error_rate",
}

export interface MetricConfig {
	title: string
	dataSourceId: string | null
	visualizationType: GenericQueryChartType
	queries:
		| {
				name: string
				query: string
				legendFormat: string
		  }[]
		| null
	timeRange: TimeRangeSetting | null
	refreshInterval: RefreshIntervalSetting | null
	thresholds?:
		| {
				value?: number // undefined means that the threshold is disabled
				label?: string
				color?: string
		  }[]
		| null
	baseThresholdColor: string // used by gauge
	decimals?: number | null // number of decimal places to show; null means "auto"
	unit: {
		type?: VisualizationUnit | null
		custom?: string | null
	}
	axisBounds: {
		min?: number | null
		max?: number | null
	}
	// the generated series the block draws while simulationActive is set.
	// It stays after the simulation ends, so a block that simulates again
	// reuses it. Null on a block nobody has simulated
	simulationPreset: MetricSimulationPreset | null
	// true while the block renders its preset instead of querying its data
	// source. The server switches it off once real data answers
	simulationActive: boolean
}

export function defaultMetricConfig(): MetricConfig {
	// NOTE: new fields need to be added to index.ts's addAttributes default
	// as well for proper Yjs merging and backwards compatibility with existing
	// blocks
	return {
		title: "",
		dataSourceId: null,
		visualizationType: GenericQueryChartType.Line,
		// NOTE: this doesn't use i18n because this isn't a reactive context
		queries: [{ name: "Query 1", query: "", legendFormat: "" }],
		timeRange: TimeRangePreset.Last5Minutes,
		refreshInterval: RefreshInterval.M5,
		thresholds: null,
		baseThresholdColor: DEFAULT_PALETTE_COLOR,
		decimals: null,
		unit: {
			type: null,
			custom: null,
		},
		axisBounds: {
			min: null,
			max: null,
		},
		simulationPreset: null,
		simulationActive: false,
	}
}

/**
 * reconstruct a MetricConfig from flat node attributes (or a legacy config blob).
 * used to build the "old" config from the oldNode JSON in diff mode.
 */
export function buildConfigFromNodeAttrs(
	attrs: Record<string, any>,
): MetricConfig {
	// legacy blobs may miss fields entirely and may still use the old "type"
	// field name, so Partial reflects the actual runtime shape.
	const legacy = attrs.config as
		(Partial<MetricConfig> & { type?: GenericQueryChartType | null }) | null
	if (legacy) {
		return {
			title: legacy.title ?? "",
			dataSourceId: legacy.dataSourceId ?? null,
			visualizationType:
				legacy.type ?? legacy.visualizationType ?? GenericQueryChartType.Line,
			queries: legacy.queries ?? null,
			timeRange: legacy.timeRange ?? null,
			refreshInterval: legacy.refreshInterval ?? null,
			thresholds: legacy.thresholds ?? null,
			baseThresholdColor: legacy.baseThresholdColor ?? "",
			decimals: legacy.decimals ?? null,
			unit: {
				type: legacy.unit?.type ?? null,
				custom: legacy.unit?.custom ?? null,
			},
			axisBounds: {
				min: legacy.axisBounds?.min ?? null,
				max: legacy.axisBounds?.max ?? null,
			},
			// the legacy blob predates simulation, so it never carries one
			simulationPreset: null,
			simulationActive: false,
		}
	}

	return {
		title: (attrs.title as string | undefined) ?? "",
		dataSourceId: (attrs.dataSourceId as string | null | undefined) ?? null,
		visualizationType:
			(attrs.visualizationType as GenericQueryChartType | null | undefined) ??
			GenericQueryChartType.Line,
		queries: (attrs.queries as MetricConfig["queries"] | undefined) ?? null,
		timeRange: (attrs.timeRange as TimeRangeSetting | null | undefined) ?? null,
		refreshInterval:
			(attrs.refreshInterval as RefreshIntervalSetting | null | undefined) ??
			null,
		thresholds: (attrs.thresholds as MetricConfig["thresholds"]) ?? null,
		baseThresholdColor: (attrs.baseThresholdColor as string | undefined) ?? "",
		decimals: (attrs.decimals as number | null | undefined) ?? null,
		unit: {
			type: (attrs.unitType as VisualizationUnit | null | undefined) ?? null,
			custom: (attrs.unitCustom as string | null | undefined) ?? null,
		},
		axisBounds: {
			min: (attrs.axisBoundsMin as number | null | undefined) ?? null,
			max: (attrs.axisBoundsMax as number | null | undefined) ?? null,
		},
		simulationPreset:
			(attrs.simulationPreset as MetricSimulationPreset | null | undefined) ??
			null,
		simulationActive: attrs.simulationActive === true,
	}
}

// the message the server sent along with a failed request, or null when it
// sent none
export function requestErrorMessage(err: unknown): string | null {
	if (
		!err ||
		typeof err !== "object" ||
		!("data" in err) ||
		!err.data ||
		typeof err.data !== "object" ||
		!("message" in err.data) ||
		typeof err.data.message !== "string" ||
		!err.data.message
	) {
		return null
	}

	return err.data.message
}

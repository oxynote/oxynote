import {
	subMinutes,
	subHours,
	subDays,
	subMonths,
	subYears,
	startOfDay,
	endOfDay,
	startOfWeek,
	endOfWeek,
	startOfMonth,
	endOfMonth,
	startOfYear,
	endOfYear,
} from "date-fns"
import { DEFAULT_PALETTE_COLOR } from "~/utils/color"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import type {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

export const CONFIG_DEBOUNCE_MS = 500
export const NODE_ATTR_UPDATE_REFRESH_DISABLE_MS = CONFIG_DEBOUNCE_MS + 1000 // slightly longer than config debounce to avoid content refreshes on drag-drop

export enum MetricBlockWidth {
	Compact = "compact",
	Standard = "standard",
	Wide = "wide",
}

// a chart either takes a fixed setting or follows the built-in variable
export type MetricTimeRange =
	TimeRangePreset | typeof METRIC_TIME_RANGE_REFERENCE
export type MetricRefreshInterval =
	RefreshInterval | typeof METRIC_REFRESH_INTERVAL_REFERENCE

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
	timeRange: MetricTimeRange | null
	refreshInterval: MetricRefreshInterval | null
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
		timeRange: (attrs.timeRange as MetricTimeRange | null | undefined) ?? null,
		refreshInterval:
			(attrs.refreshInterval as MetricRefreshInterval | null | undefined) ??
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

const WEEK_OPTIONS = { weekStartsOn: 1 as const }

export function resolveTimeRange(
	preset: TimeRangePreset,
	now: Date = new Date(),
): { from: Date; to: Date } {
	switch (preset) {
		// ────────────────
		// Quick / relative
		// ────────────────
		case TimeRangePreset.Last5Minutes:
			return { from: subMinutes(now, 5), to: now }
		case TimeRangePreset.Last15Minutes:
			return { from: subMinutes(now, 15), to: now }
		case TimeRangePreset.Last30Minutes:
			return { from: subMinutes(now, 30), to: now }
		case TimeRangePreset.Last1Hour:
			return { from: subHours(now, 1), to: now }
		case TimeRangePreset.Last3Hours:
			return { from: subHours(now, 3), to: now }
		case TimeRangePreset.Last6Hours:
			return { from: subHours(now, 6), to: now }
		case TimeRangePreset.Last12Hours:
			return { from: subHours(now, 12), to: now }
		case TimeRangePreset.Last24Hours:
			return { from: subHours(now, 24), to: now }
		case TimeRangePreset.Last2Days:
			return { from: subDays(now, 2), to: now }
		case TimeRangePreset.Last7Days:
			return { from: subDays(now, 7), to: now }
		case TimeRangePreset.Last30Days:
			return { from: subDays(now, 30), to: now }
		case TimeRangePreset.Last90Days:
			return { from: subDays(now, 90), to: now }
		case TimeRangePreset.Last6Months:
			return { from: subMonths(now, 6), to: now }
		case TimeRangePreset.Last1Year:
			return { from: subYears(now, 1), to: now }
		case TimeRangePreset.Last2Years:
			return { from: subYears(now, 2), to: now }
		case TimeRangePreset.Last5Years:
			return { from: subYears(now, 5), to: now }

		// ────────────────
		// Calendar-aligned ("This")
		// ────────────────
		case TimeRangePreset.Today:
			return { from: startOfDay(now), to: endOfDay(now) }
		case TimeRangePreset.TodaySoFar:
			return { from: startOfDay(now), to: now }
		case TimeRangePreset.Yesterday: {
			const yesterday = subDays(now, 1)
			return {
				from: startOfDay(yesterday),
				to: endOfDay(yesterday),
			}
		}
		case TimeRangePreset.ThisWeek:
			return {
				from: startOfWeek(now, WEEK_OPTIONS),
				to: endOfWeek(now, WEEK_OPTIONS),
			}
		case TimeRangePreset.ThisWeekSoFar:
			return {
				from: startOfWeek(now, WEEK_OPTIONS),
				to: now,
			}
		case TimeRangePreset.ThisMonth:
			return {
				from: startOfMonth(now),
				to: endOfMonth(now),
			}
		case TimeRangePreset.ThisMonthSoFar:
			return {
				from: startOfMonth(now),
				to: now,
			}
		case TimeRangePreset.ThisYear:
			return {
				from: startOfYear(now),
				to: endOfYear(now),
			}
		case TimeRangePreset.ThisYearSoFar:
			return {
				from: startOfYear(now),
				to: now,
			}

		// ────────────────
		// Previous periods
		// ────────────────
		case TimeRangePreset.PreviousWeek: {
			const prev = subDays(startOfWeek(now, WEEK_OPTIONS), 1)
			return {
				from: startOfWeek(prev, WEEK_OPTIONS),
				to: endOfWeek(prev, WEEK_OPTIONS),
			}
		}
		case TimeRangePreset.PreviousMonth: {
			const prev = subMonths(startOfMonth(now), 1)
			return {
				from: startOfMonth(prev),
				to: endOfMonth(prev),
			}
		}
		case TimeRangePreset.PreviousYear: {
			const prev = subYears(startOfYear(now), 1)
			return {
				from: startOfYear(prev),
				to: endOfYear(prev),
			}
		}
	}
}

export function refreshIntervalToMs(interval: RefreshInterval): number {
	switch (interval) {
		case RefreshInterval.S5:
			return 5_000
		case RefreshInterval.S10:
			return 10_000
		case RefreshInterval.S30:
			return 30_000

		case RefreshInterval.M1:
			return 60_000
		case RefreshInterval.M5:
			return 5 * 60_000
		case RefreshInterval.M15:
			return 15 * 60_000
		case RefreshInterval.M30:
			return 30 * 60_000

		case RefreshInterval.H1:
			return 60 * 60_000
		case RefreshInterval.H2:
			return 2 * 60 * 60_000

		case RefreshInterval.D1:
			return 24 * 60 * 60_000
	}
}

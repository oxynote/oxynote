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
// the time ranges a metric chart or a time range variable can show,
// grouped as the pickers list them
export enum TimeRangePreset {
	// Quick / relative
	Last5Minutes = "last_5_minutes",
	Last15Minutes = "last_15_minutes",
	Last30Minutes = "last_30_minutes",
	Last1Hour = "last_1_hour",
	Last3Hours = "last_3_hours",
	Last6Hours = "last_6_hours",
	Last12Hours = "last_12_hours",
	Last24Hours = "last_24_hours",
	Last2Days = "last_2_days",
	Last7Days = "last_7_days",
	Last30Days = "last_30_days",
	Last90Days = "last_90_days",
	Last6Months = "last_6_months",
	Last1Year = "last_1_year",
	Last2Years = "last_2_years",
	Last5Years = "last_5_years",

	// Calendar-aligned ("This")
	Today = "today",
	Yesterday = "yesterday",
	TodaySoFar = "today_so_far",
	ThisWeek = "this_week",
	ThisWeekSoFar = "this_week_so_far",
	ThisMonth = "this_month",
	ThisMonthSoFar = "this_month_so_far",
	ThisYear = "this_year",
	ThisYearSoFar = "this_year_so_far",

	// Previous periods
	PreviousWeek = "previous_week",
	PreviousMonth = "previous_month",
	PreviousYear = "previous_year",
}

// absolute windows stay fixed when a chart refreshes or a document reloads
export type CustomTimeRange = `custom:${string}/${string}`
export type TimeRange = TimeRangePreset | CustomTimeRange

// the intervals a metric chart refreshes at, shortest first
export enum RefreshInterval {
	S5 = "5s",
	S10 = "10s",
	S30 = "30s",
	M1 = "1m",
	M5 = "5m",
	M15 = "15m",
	M30 = "30m",
	H1 = "1h",
	H2 = "2h",
	D1 = "1d",
}

const WEEK_OPTIONS = { weekStartsOn: 1 as const }

export function resolveTimeRange(
	preset: TimeRange,
	now: Date = new Date(),
): { from: Date; to: Date } {
	const custom = parseCustomTimeRange(preset)
	if (custom) {
		return custom
	}

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
		default:
			throw new RangeError("Invalid time range")
	}
}

export function parseCustomTimeRange(
	value: string,
): { from: Date; to: Date } | null {
	if (!value.startsWith("custom:")) {
		return null
	}

	const [start, end, extra] = value.slice("custom:".length).split("/")
	if (!start || !end || extra !== undefined) {
		return null
	}

	const from = new Date(start)
	const to = new Date(end)
	if (
		!Number.isFinite(from.getTime()) ||
		!Number.isFinite(to.getTime()) ||
		from >= to ||
		from.toISOString() !== start ||
		to.toISOString() !== end
	) {
		return null
	}

	return { from, to }
}

export function serializeCustomTimeRange(
	from: Date,
	to: Date,
): CustomTimeRange {
	const value: CustomTimeRange = `custom:${from.toISOString()}/${to.toISOString()}`
	if (!parseCustomTimeRange(value)) {
		throw new RangeError("Invalid time range")
	}

	return value
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

import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	RefreshInterval,
	refreshIntervalToMs,
	resolveTimeRange,
	TimeRangePreset,
	parseCustomTimeRange,
	serializeCustomTimeRange,
} from "./time-range"

// a wednesday in the middle of a month, with sub-second precision so
// boundary presets prove they snap to exact instants. Local-time
// components keep the expectations timezone-independent.
const NOW = new Date(2026, 7, 19, 15, 30, 45, 123)
const CUSTOM_FROM = new Date("2026-03-28T22:30:01.123Z")
const CUSTOM_TO = new Date("2026-03-29T04:45:02.456Z")
const CUSTOM_RANGE = "custom:2026-03-28T22:30:01.123Z/2026-03-29T04:45:02.456Z"

describe("resolveTimeRange", () => {
	it("keeps an absolute window fixed when the clock moves", ({ expect }) => {
		expect(resolveTimeRange(CUSTOM_RANGE, NOW)).toEqual({
			from: CUSTOM_FROM,
			to: CUSTOM_TO,
		})
		expect(resolveTimeRange(CUSTOM_RANGE, new Date(2030, 0, 1))).toEqual({
			from: CUSTOM_FROM,
			to: CUSTOM_TO,
		})
	})

	it("rejects an invalid custom window", ({ expect }) => {
		expect(() => resolveTimeRange("custom:invalid/invalid")).toThrow(RangeError)
	})

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

describe("parseCustomTimeRange", () => {
	it("restores both UTC instants with millisecond precision", ({ expect }) => {
		expect(parseCustomTimeRange(CUSTOM_RANGE)).toEqual({
			from: CUSTOM_FROM,
			to: CUSTOM_TO,
		})
	})

	it.for([
		TimeRangePreset.Last1Hour,
		"custom:",
		"custom:/2026-03-29T04:45:02.456Z",
		"custom:2026-03-28T22:30:01.123Z/",
		`${CUSTOM_RANGE}/extra`,
		"custom:not-a-date/2026-03-29T04:45:02.456Z",
		"custom:2026-03-28T22:30:01.123Z/not-a-date",
		"custom:2026-03-29T04:45:02.456Z/2026-03-28T22:30:01.123Z",
		"custom:2026-03-28T22:30:01.123Z/2026-03-28T22:30:01.123Z",
		"custom:2026-03-28/2026-03-29",
		"custom:2026-02-30T00:00:00.000Z/2026-03-29T04:45:02.456Z",
	])("rejects %s", (input, { expect }) => {
		expect(parseCustomTimeRange(input)).toBeNull()
	})
})

describe("serializeCustomTimeRange", () => {
	it("serializes absolute instants for document storage and query keys", ({
		expect,
	}) => {
		expect(serializeCustomTimeRange(CUSTOM_FROM, CUSTOM_TO)).toBe(CUSTOM_RANGE)
	})

	it.for([
		{ name: "inverted", from: CUSTOM_TO, to: CUSTOM_FROM },
		{ name: "empty", from: CUSTOM_FROM, to: CUSTOM_FROM },
		{ name: "invalid", from: new Date(NaN), to: CUSTOM_TO },
	])("rejects an $name window", ({ from, to }, { expect }) => {
		expect(() => serializeCustomTimeRange(from, to)).toThrow(RangeError)
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

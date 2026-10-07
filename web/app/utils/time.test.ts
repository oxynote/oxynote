import { enUS } from "date-fns/locale"
import { describe, it, vi } from "vitest"
import {
	addDurationToDate,
	convertDateFnsLocale,
	dateToCalendarDate,
	delay,
	parseDurationString,
	relativeTimeLabel,
	roundDateToNearest5Seconds,
} from "./time"

// noon, so a moment from this morning and one from late last night fall on
// different calendar days
const NOW = new Date(2026, 2, 14, 12, 0, 0)

describe("convertDateFnsLocale", () => {
	it.for([
		{ name: "resolves a known language code", input: "en", expected: enUS },
		{ name: "ignores the region part", input: "en-US", expected: enUS },
		{
			name: "falls back to english for unknown locales",
			input: "fr",
			expected: enUS,
		},
		{
			name: "falls back to english for an empty locale",
			input: "",
			expected: enUS,
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(convertDateFnsLocale(input)).toBe(expected)
	})
})

describe("delay", () => {
	it("resolves after the given number of milliseconds", async ({ expect }) => {
		vi.useFakeTimers()

		try {
			let resolved = false
			const pending = delay(1000).then(() => {
				resolved = true
			})

			await vi.advanceTimersByTimeAsync(999)
			expect(resolved).toBe(false)

			await vi.advanceTimersByTimeAsync(1)
			await pending
			expect(resolved).toBe(true)
		} finally {
			vi.useRealTimers()
		}
	})
})

describe("parseDurationString", () => {
	it.for([
		{ input: "1s", expected: 1000 },
		{ input: "500ms", expected: 500 },
		{ input: "2h", expected: 7200000 },
		{ input: "1m30s", expected: 90000 },
		{ input: "1h2m3s", expected: 3723000 },
		{ input: "100ns", expected: 0.0001 },
		{ input: "10µs", expected: 0.01 },
		{ input: "10us", expected: 0.01 },
		{ input: "", expected: 0 },
		{ input: "xyz", expected: 0 },
	])("parses $input into $expected ms", ({ input, expected }, { expect }) => {
		expect(parseDurationString(input)).toBeCloseTo(expected, 6)
	})
})

describe("addDurationToDate", () => {
	it("shifts the date by the parsed duration", ({ expect }) => {
		const base = new Date("2024-06-15T12:00:00Z")

		expect(addDurationToDate(base, "1m30s")).toEqual(
			new Date("2024-06-15T12:01:30Z"),
		)
	})

	it("returns an equal date for an empty duration", ({ expect }) => {
		const base = new Date("2024-06-15T12:00:00Z")

		expect(addDurationToDate(base, "")).toEqual(base)
	})
})

describe("dateToCalendarDate", () => {
	it("builds a calendar date from a Date's local components", ({ expect }) => {
		const result = dateToCalendarDate(new Date(2024, 5, 15, 12))

		expect(result.year).toBe(2024)
		expect(result.month).toBe(6)
		expect(result.day).toBe(15)
	})

	it("maps January to month 1", ({ expect }) => {
		const result = dateToCalendarDate(new Date(2024, 0, 3, 12))

		expect(result.year).toBe(2024)
		expect(result.month).toBe(1)
		expect(result.day).toBe(3)
	})

	it("parses a string input before converting", ({ expect }) => {
		const fromString = dateToCalendarDate("2024-06-15T12:00:00")
		const fromDate = dateToCalendarDate(new Date("2024-06-15T12:00:00"))

		expect(fromString).toEqual(fromDate)
	})
})

describe("roundDateToNearest5Seconds", () => {
	it.for([
		{
			name: "keeps an exact boundary unchanged",
			input: "2024-06-15T12:00:05.000Z",
			expected: "2024-06-15T12:00:05Z",
		},
		{
			name: "rounds a boundary with milliseconds up",
			input: "2024-06-15T12:00:05.001Z",
			expected: "2024-06-15T12:00:10Z",
		},
		{
			name: "rounds mid-interval seconds up",
			input: "2024-06-15T12:00:03.000Z",
			expected: "2024-06-15T12:00:05Z",
		},
		{
			name: "rolls over into the next minute",
			input: "2024-06-15T12:00:58.500Z",
			expected: "2024-06-15T12:01:00Z",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(roundDateToNearest5Seconds(input)).toBe(expected)
	})

	it("accepts a Date input", ({ expect }) => {
		expect(
			roundDateToNearest5Seconds(new Date("2024-06-15T12:00:03.000Z")),
		).toBe("2024-06-15T12:00:05Z")
	})
})

describe("relativeTimeLabel", () => {
	it.for([
		{
			name: "says now within the first minute",
			input: new Date(2026, 2, 14, 11, 59, 30),
			expected: "general.relative-time.now",
		},
		{
			name: "counts minutes within the first hour",
			input: new Date(2026, 2, 14, 11, 55, 0),
			expected: 'general.relative-time.minutes:{"count":5}',
		},
		{
			name: "counts hours within the first day",
			input: new Date(2026, 2, 14, 10, 0, 0),
			expected: 'general.relative-time.hours:{"count":2}',
		},
		{
			name: "counts hours for last night",
			input: new Date(2026, 2, 13, 23, 0, 0),
			expected: 'general.relative-time.hours:{"count":13}',
		},
		{
			name: "counts one day once yesterday is a full day back",
			input: new Date(2026, 2, 13, 9, 0, 0),
			expected: 'general.relative-time.days:{"count":1}',
		},
		{
			name: "shows the date before yesterday",
			input: new Date(2026, 2, 10, 9, 0, 0),
			expected: "month-day-short:2026-03-10",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(relativeTimeLabel(input, NOW, fakeT, fakeD)).toBe(expected)
	})

	it("accepts a moment written as a string", ({ expect }) => {
		expect(
			relativeTimeLabel(
				new Date(2026, 2, 14, 10, 0, 0).toISOString(),
				NOW,
				fakeT,
				fakeD,
			),
		).toBe('general.relative-time.hours:{"count":2}')
	})
})

// answers with the key and its params, so a test reads which message the
// helper picked and what it filled in
function fakeT(key: string, params?: Record<string, unknown>) {
	return params ? `${key}:${JSON.stringify(params)}` : key
}

function fakeD(value: Date, format: string) {
	const month = String(value.getMonth() + 1).padStart(2, "0")
	const day = String(value.getDate()).padStart(2, "0")

	return `${format}:${String(value.getFullYear())}-${month}-${day}`
}

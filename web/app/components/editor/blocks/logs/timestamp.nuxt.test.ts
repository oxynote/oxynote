import { describe, it } from "vitest"
import { formatLogTime } from "./timestamp"
import { LogTimestampFormat } from "./utils"

// built from local fields, so every format reads the same clock in any time
// zone
const LOCAL_TIME = new Date(2026, 0, 2, 3, 4, 5).toISOString()

describe("formatLogTime", () => {
	it("returns null when the format shows no time", ({ expect }) => {
		expect(
			formatLogTime(LOCAL_TIME, LogTimestampFormat.None, appDate()),
		).toBeNull()
	})

	it.for([
		{
			name: "writes the readable format in the locale's style",
			input: LogTimestampFormat.Readable,
			expected: { date: "Jan 2, 2026", time: "03:04:05 AM" },
		},
		{
			name: "writes the readable format with milliseconds",
			input: LogTimestampFormat.ReadableMilli,
			expected: { date: "Jan 2, 2026", time: "03:04:05.123 AM" },
		},
		{
			name: "writes the readable format with every nanosecond",
			input: LogTimestampFormat.ReadableNano,
			expected: { date: "Jan 2, 2026", time: "03:04:05.123456789 AM" },
		},
		{
			name: "writes only the time in the locale's style",
			input: LogTimestampFormat.ReadableTime,
			expected: { date: "", time: "03:04:05 AM" },
		},
		{
			name: "writes only the time with milliseconds",
			input: LogTimestampFormat.ReadableTimeMilli,
			expected: { date: "", time: "03:04:05.123 AM" },
		},
		{
			name: "writes only the time with every nanosecond",
			input: LogTimestampFormat.ReadableTimeNano,
			expected: { date: "", time: "03:04:05.123456789 AM" },
		},
	])("$name", ({ input, expected }, { expect }) => {
		const time = LOCAL_TIME.replace(".000Z", ".123456789Z")

		expect(plainSpaces(formatLogTime(time, input, appDate()))).toEqual(expected)
	})

	it.for([
		{
			name: "drops the fraction for the numeric format",
			input: { fraction: ".123456789", format: LogTimestampFormat.Numeric },
			expected: "03:04:05",
		},
		{
			name: "pads a short fraction to milliseconds",
			input: { fraction: ".5", format: LogTimestampFormat.NumericMilli },
			expected: "03:04:05.500",
		},
		{
			name: "cuts a long fraction to milliseconds",
			input: {
				fraction: ".123456789",
				format: LogTimestampFormat.NumericMilli,
			},
			expected: "03:04:05.123",
		},
		{
			name: "adds a zero fraction to a time without one",
			input: { fraction: "", format: LogTimestampFormat.NumericMilli },
			expected: "03:04:05.000",
		},
		{
			name: "keeps every nanosecond",
			input: {
				fraction: ".123456789",
				format: LogTimestampFormat.NumericNano,
			},
			expected: "03:04:05.123456789",
		},
		{
			name: "pads a short fraction to nanoseconds",
			input: { fraction: ".5", format: LogTimestampFormat.NumericNano },
			expected: "03:04:05.500000000",
		},
	])("$name", ({ input, expected }, { expect }) => {
		const time = LOCAL_TIME.replace(".000Z", `${input.fraction}Z`)

		expect(formatLogTime(time, input.format, appDate())).toEqual({
			date: "2026-01-02",
			time: expected,
		})
	})

	it.for([
		{ input: LogTimestampFormat.NumericTime, expected: "03:04:05" },
		{ input: LogTimestampFormat.NumericTimeMilli, expected: "03:04:05.123" },
		{
			input: LogTimestampFormat.NumericTimeNano,
			expected: "03:04:05.123456789",
		},
	])(
		"writes only the time in digits for $input",
		({ input, expected }, { expect }) => {
			const time = LOCAL_TIME.replace(".000Z", ".123456789Z")

			expect(formatLogTime(time, input, appDate())).toEqual({
				date: "",
				time: expected,
			})
		},
	)

	it("moves a time with an offset into the local zone", ({ expect }) => {
		const utc = new Date(LOCAL_TIME)
		const shifted = new Date(utc.getTime() + 2 * 60 * 60 * 1000)
		const time = `${shifted.toISOString().slice(0, 19)}.123+02:00`

		expect(
			formatLogTime(time, LogTimestampFormat.NumericMilli, appDate()),
		).toEqual({
			date: "2026-01-02",
			time: "03:04:05.123",
		})
	})

	it("reads the milliseconds of a time without a zone", ({ expect }) => {
		expect(
			formatLogTime(
				"2026-01-02T03:04:05.678",
				LogTimestampFormat.NumericNano,
				appDate(),
			),
		).toEqual({ date: "2026-01-02", time: "03:04:05.678000000" })
	})

	it("shows an unparsable time as it is", ({ expect }) => {
		expect(
			formatLogTime("yesterday", LogTimestampFormat.Readable, appDate()),
		).toEqual({ date: "", time: "yesterday" })
	})
})

// newer ICU versions put a narrow no-break space before AM and PM, and
// older ones a plain space
function plainSpaces(
	result: { date: string; time: string } | null,
): { date: string; time: string } | null {
	return result && { ...result, time: result.time.replace(/\s/gu, " ") }
}

// the app's own d(), with the named formats the rows use
function appDate() {
	return useNuxtApp().$i18n.d
}

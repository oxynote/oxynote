import { describe, it, vi } from "vitest"
import { formatLogTime } from "./timestamp"
import { LogTimestampFormat } from "./utils"

// built from local fields, so every format reads the same clock in any time
// zone
const LOCAL_TIME = new Date(2026, 0, 2, 3, 4, 5).toISOString()

describe("formatLogTime", () => {
	it("returns null when the format shows no time", ({ expect }) => {
		const d = vi.fn(fakeFormat)

		expect(formatLogTime(LOCAL_TIME, LogTimestampFormat.None, d)).toBeNull()
		expect.soft(d).not.toHaveBeenCalled()
	})

	it("writes the standard format through the locale's formats", ({
		expect,
	}) => {
		const d = vi.fn(fakeFormat)

		expect(formatLogTime(LOCAL_TIME, LogTimestampFormat.Standard, d)).toEqual({
			date: `short ${LOCAL_TIME}`,
			time: `time-with-seconds ${LOCAL_TIME}`,
		})
		expect.soft(d).toHaveBeenCalledTimes(2)
	})

	it.for([
		{
			name: "drops the fraction for the detailed format",
			input: { fraction: ".123456789", format: LogTimestampFormat.Detailed },
			expected: "03:04:05",
		},
		{
			name: "pads a short fraction to milliseconds",
			input: { fraction: ".5", format: LogTimestampFormat.DetailedMilli },
			expected: "03:04:05.500",
		},
		{
			name: "cuts a long fraction to milliseconds",
			input: {
				fraction: ".123456789",
				format: LogTimestampFormat.DetailedMilli,
			},
			expected: "03:04:05.123",
		},
		{
			name: "adds a zero fraction to a time without one",
			input: { fraction: "", format: LogTimestampFormat.DetailedMilli },
			expected: "03:04:05.000",
		},
		{
			name: "keeps every nanosecond",
			input: {
				fraction: ".123456789",
				format: LogTimestampFormat.DetailedNano,
			},
			expected: "03:04:05.123456789",
		},
		{
			name: "pads a short fraction to nanoseconds",
			input: { fraction: ".5", format: LogTimestampFormat.DetailedNano },
			expected: "03:04:05.500000000",
		},
	])("$name", ({ input, expected }, { expect }) => {
		const time = LOCAL_TIME.replace(".000Z", `${input.fraction}Z`)

		expect(formatLogTime(time, input.format, fakeFormat)).toEqual({
			date: "2026-01-02",
			time: expected,
		})
	})

	it("moves a time with an offset into the local zone", ({ expect }) => {
		const utc = new Date(LOCAL_TIME)
		const shifted = new Date(utc.getTime() + 2 * 60 * 60 * 1000)
		const time = `${shifted.toISOString().slice(0, 19)}.123+02:00`

		expect(
			formatLogTime(time, LogTimestampFormat.DetailedMilli, fakeFormat),
		).toEqual({ date: "2026-01-02", time: "03:04:05.123" })
	})

	it("reads the milliseconds of a time without a zone", ({ expect }) => {
		expect(
			formatLogTime(
				"2026-01-02T03:04:05.678",
				LogTimestampFormat.DetailedNano,
				fakeFormat,
			),
		).toEqual({ date: "2026-01-02", time: "03:04:05.678000000" })
	})

	it("shows an unparsable time as it is", ({ expect }) => {
		expect(
			formatLogTime("yesterday", LogTimestampFormat.Standard, fakeFormat),
		).toEqual({ date: "", time: "yesterday" })
	})
})

function fakeFormat(value: Date, key: string) {
	return `${key} ${value.toISOString()}`
}

import { format } from "date-fns"
import { LogTimestampFormat } from "./utils"

// a line's time split into the time up to its seconds, its fraction of a
// second and its zone
const TIME_PARTS = /^(.+\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/

// the digits each detailed format keeps of the fraction of a second
const FRACTION_DIGITS: Partial<Record<LogTimestampFormat, number>> = {
	[LogTimestampFormat.Detailed]: 0,
	[LogTimestampFormat.DetailedMilli]: 3,
	[LogTimestampFormat.DetailedNano]: 9,
}

// formatLogTime writes a line's time in the block's format and the reader's
// time zone. It splits the time into a date part and a time part that the
// row colours apart, and is null when the format shows no time. The
// standard format follows the locale. A detailed one reads like
// "2026-10-29 13:23:43.123" and keeps the fraction exact down to the
// nanosecond, which a Date cannot hold.
export function formatLogTime(
	time: string,
	timestampFormat: LogTimestampFormat,
	d: (value: Date, key: string) => string,
): { date: string; time: string } | null {
	if (timestampFormat === LogTimestampFormat.None) {
		return null
	}

	const parts = TIME_PARTS.exec(time)
	const [, seconds = "", fraction, zone = ""] = parts ?? []
	const date = new Date(parts ? `${seconds}${zone}` : time)
	if (Number.isNaN(date.getTime())) {
		return { date: "", time: time }
	}

	const digits = FRACTION_DIGITS[timestampFormat]
	if (digits === undefined) {
		return { date: d(date, "short"), time: d(date, "time-with-seconds") }
	}

	const allDigits = fraction ?? String(date.getMilliseconds()).padStart(3, "0")
	const kept = digits
		? `.${allDigits.padEnd(digits, "0").slice(0, digits)}`
		: ""

	return {
		date: format(date, "yyyy-MM-dd"),
		time: `${format(date, "HH:mm:ss")}${kept}`,
	}
}

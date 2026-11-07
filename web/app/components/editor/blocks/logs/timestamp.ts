import { format } from "date-fns"
import { LogTimestampFormat } from "./utils"

// a line's time split into the time up to its seconds, its fraction of a
// second and its zone
const TIME_PARTS = /^(.+\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/

// how each format writes a time: in the locale's style or all in digits,
// with or without its date. digits is how much of the fraction of a
// second it keeps.
const FORMATS: Record<
	Exclude<LogTimestampFormat, LogTimestampFormat.None>,
	{ readable: boolean; withDate: boolean; digits: 0 | 3 | 9 }
> = {
	[LogTimestampFormat.Readable]: { readable: true, withDate: true, digits: 0 },
	[LogTimestampFormat.ReadableMilli]: {
		readable: true,
		withDate: true,
		digits: 3,
	},
	[LogTimestampFormat.ReadableNano]: {
		readable: true,
		withDate: true,
		digits: 9,
	},
	[LogTimestampFormat.ReadableTime]: {
		readable: true,
		withDate: false,
		digits: 0,
	},
	[LogTimestampFormat.ReadableTimeMilli]: {
		readable: true,
		withDate: false,
		digits: 3,
	},
	[LogTimestampFormat.ReadableTimeNano]: {
		readable: true,
		withDate: false,
		digits: 9,
	},
	[LogTimestampFormat.Numeric]: { readable: false, withDate: true, digits: 0 },
	[LogTimestampFormat.NumericMilli]: {
		readable: false,
		withDate: true,
		digits: 3,
	},
	[LogTimestampFormat.NumericNano]: {
		readable: false,
		withDate: true,
		digits: 9,
	},
	[LogTimestampFormat.NumericTime]: {
		readable: false,
		withDate: false,
		digits: 0,
	},
	[LogTimestampFormat.NumericTimeMilli]: {
		readable: false,
		withDate: false,
		digits: 3,
	},
	[LogTimestampFormat.NumericTimeNano]: {
		readable: false,
		withDate: false,
		digits: 9,
	},
}

// formatLogTime writes a line's time in the block's format and the reader's
// time zone. It splits the time into a date part and a time part that the
// row colours apart, and is null when the format shows no time. The
// readable formats follow the locale, so English shows AM and PM. A
// numeric one reads like "2026-10-29 13:23:43.123". Every format
// keeps the fraction exact down to the nanosecond, which a Date cannot
// hold.
export function formatLogTime(
	time: string,
	timestampFormat: LogTimestampFormat,
	d: ReturnType<typeof useI18n>["d"],
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

	const { readable, withDate, digits } = FORMATS[timestampFormat]
	const kept = (fraction ?? String(date.getMilliseconds()).padStart(3, "0"))
		.padEnd(digits, "0")
		.slice(0, digits)

	if (!readable) {
		return {
			date: withDate ? format(date, "yyyy-MM-dd") : "",
			time: `${format(date, "HH:mm:ss")}${digits ? `.${kept}` : ""}`,
		}
	}

	// the locale formats milliseconds with its own decimal mark, and the
	// fraction it writes is swapped for the exact one
	const clock = d(date, {
		key: digits ? "log-time-fraction" : "log-time",
		part: true,
	})
		.map((part) => (part.type === "fractionalSecond" ? kept : part.value))
		.join("")

	return {
		date: withDate ? d(date, "short") : "",
		time: clock,
	}
}

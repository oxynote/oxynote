import { CalendarDate } from "@internationalized/date"
import {
	differenceInCalendarDays,
	differenceInHours,
	differenceInMinutes,
} from "date-fns"
import { enUS, type Locale } from "date-fns/locale"

const dateFnsLocales: Record<string, Locale> = {
	en: enUS,
}

export function convertDateFnsLocale(locale: string) {
	return dateFnsLocales[locale.split("-")[0] ?? ""] ?? enUS
}

export function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms))
}

const durationStringRegex = /(\d+)(ns|µs|us|ms|s|m|h)/g

// parseDurationString converts go's duration into ms.
export function parseDurationString(dur: string): number {
	let totalMs = 0
	let match

	const conversionFactors: Record<string, number> = {
		ns: 1e-6, // nanoseconds to milliseconds
		µs: 1e-3, // microseconds to milliseconds
		us: 1e-3, // microseconds to milliseconds (alternative representation)
		ms: 1, // milliseconds to milliseconds
		s: 1000, // seconds to milliseconds
		m: 60000, // minutes to milliseconds
		h: 3600000, // hours to milliseconds
	}

	while ((match = durationStringRegex.exec(dur)) !== null) {
		const [, value, unit] = match
		if (value === undefined || unit === undefined) {
			continue
		}

		totalMs += parseFloat(value) * (conversionFactors[unit] ?? 0)
	}

	return totalMs
}

export function addDurationToDate(date: Date, dur: string): Date {
	const msToAdd = parseDurationString(dur)
	return new Date(date.getTime() + msToAdd)
}

export function dateToCalendarDate(date: Date | string): CalendarDate {
	if (typeof date === "string") {
		date = new Date(date)
	}

	// getMonth is 0-based while CalendarDate months are 1-based
	return new CalendarDate(
		date.getFullYear(),
		date.getMonth() + 1,
		date.getDate(),
	)
}

/**
 * Rounds a Date up to the nearest 5-second interval and returns an ISO string
 * with milliseconds stripped (precision to seconds only).
 */
export function roundDateToNearest5Seconds(date: Date | string): string {
	const input = typeof date === "string" ? new Date(date) : date
	const seconds = input.getSeconds()
	const milliseconds = input.getMilliseconds()

	// Calculate how many seconds to add to reach the next 5-second boundary
	const remainder = seconds % 5
	const secondsToAdd = remainder === 0 && milliseconds === 0 ? 0 : 5 - remainder

	const rounded = new Date(input.getTime())
	rounded.setMilliseconds(0)
	rounded.setSeconds(seconds + secondsToAdd)

	// Return ISO string without milliseconds (strip .000Z and add Z)
	return rounded.toISOString().replace(/\.\d{3}Z$/, "Z")
}

// relativeTimeLabel says in a few characters how long ago a moment was:
// now, then minutes, hours and one day. Anything before yesterday, by the
// calendar, shows its date instead.
export function relativeTimeLabel(
	date: Date | string,
	now: Date,
	t: (key: string, params?: Record<string, unknown>) => string,
	d: (value: Date, format: string) => string,
): string {
	const then = new Date(date)
	if (differenceInCalendarDays(now, then) > 1) {
		return d(then, "month-day-short")
	}

	const minutes = differenceInMinutes(now, then)
	if (minutes < 1) {
		return t("general.relative-time.now")
	}

	if (minutes < 60) {
		return t("general.relative-time.minutes", { count: minutes })
	}

	const hours = differenceInHours(now, then)
	if (hours < 24) {
		return t("general.relative-time.hours", { count: hours })
	}

	return t("general.relative-time.days", { count: 1 })
}

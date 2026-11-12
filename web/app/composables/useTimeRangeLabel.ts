import {
	fromDate,
	getLocalTimeZone,
	toCalendarDateTime,
} from "@internationalized/date"
import { toValue, type MaybeRefOrGetter } from "vue"
import { parseCustomTimeRange } from "~/utils/time-range"
import { METRIC_TIME_RANGE_REFERENCE } from "~/utils/variable"

export function useTimeRangeLabel(
	timeZone?: MaybeRefOrGetter<string>,
	precise = false,
) {
	const { t, d, locale } = useI18n({ useScope: "global" })

	return (value: unknown): string => {
		if (typeof value !== "string" || !value) {
			return ""
		}

		if (value === METRIC_TIME_RANGE_REFERENCE) {
			return t("editor.config-modal-utils.variable-time-range")
		}

		const range = parseCustomTimeRange(value)
		if (range) {
			const zone = toValue(timeZone) ?? getLocalTimeZone()
			if (precise) {
				const options = {
					key: "short-with-full-time",
					timeZone: zone,
					fractionalSecondDigits: 3 as const,
					timeZoneName: "longOffset" as const,
				}

				return t("general.custom-time-range.label", {
					from: d(range.from, options),
					to: d(range.to, options),
				})
			}

			const start = fromDate(range.from, zone)
			const end = fromDate(range.to, zone)
			const sameDay =
				start.year === end.year &&
				start.month === end.month &&
				start.day === end.day
			const dateOptions = {
				month: "short",
				day: "numeric",
				timeZone: zone,
				...(start.year !== end.year ? { year: "numeric" as const } : {}),
			} as const
			const time = (date: Date) => {
				const local = fromDate(date, zone)
				const wallTime = toCalendarDateTime(local)
				const repeated =
					wallTime.toDate(zone, "earlier").getTime() !==
					wallTime.toDate(zone, "later").getTime()
				const clock = `${local.hour}:${String(local.minute).padStart(2, "0")}`
				if (!repeated) {
					return clock
				}

				const offset = new Intl.DateTimeFormat(locale.value, {
					timeZone: zone,
					timeZoneName: "longOffset",
				})
					.formatToParts(date)
					.find((part) => part.type === "timeZoneName")?.value

				return offset ? `${clock} ${offset}` : clock
			}
			const from = `${d(range.from, dateOptions)} ${time(range.from)}`
			const endTime = time(range.to)
			const to = sameDay ? endTime : `${d(range.to, dateOptions)} ${endTime}`

			return t("general.custom-time-range.label", { from, to })
		}

		return t(`general.time-range-options.${value}`)
	}
}

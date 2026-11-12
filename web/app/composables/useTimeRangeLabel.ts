import { fromDate, getLocalTimeZone } from "@internationalized/date"
import { toValue, type MaybeRefOrGetter } from "vue"
import { parseCustomTimeRange } from "~/utils/time-range"
import { METRIC_TIME_RANGE_REFERENCE } from "~/utils/variable"

export function useTimeRangeLabel(timeZone?: MaybeRefOrGetter<string>) {
	const { t, d } = useI18n({ useScope: "global" })

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
			const from = `${d(range.from, dateOptions)} ${start.hour}:${String(start.minute).padStart(2, "0")}`
			const endTime = `${end.hour}:${String(end.minute).padStart(2, "0")}`
			const to = sameDay ? endTime : `${d(range.to, dateOptions)} ${endTime}`

			return t("general.custom-time-range.label", { from, to })
		}

		return t(`general.time-range-options.${value}`)
	}
}

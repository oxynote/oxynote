import { format, isSameDay, isSameYear } from "date-fns"
import { parseCustomTimeRange } from "~/utils/time-range"
import { METRIC_TIME_RANGE_REFERENCE } from "~/utils/variable"

export function useTimeRangeLabel() {
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
			const dateOptions = {
				month: "short",
				day: "numeric",
				...(!isSameYear(range.from, range.to) ? { year: "numeric" } : {}),
			} as const
			const from = `${d(range.from, dateOptions)} ${format(range.from, "H:mm")}`
			const to = isSameDay(range.from, range.to)
				? format(range.to, "H:mm")
				: `${d(range.to, dateOptions)} ${format(range.to, "H:mm")}`

			return t("general.custom-time-range.label", { from, to })
		}

		return t(`general.time-range-options.${value}`)
	}
}

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
			return t("general.custom-time-range.label", {
				from: d(range.from, {
					key: "short-with-full-time",
					fractionalSecondDigits: 3,
				}),
				to: d(range.to, {
					key: "short-with-full-time",
					fractionalSecondDigits: 3,
				}),
			})
		}

		return t(`general.time-range-options.${value}`)
	}
}

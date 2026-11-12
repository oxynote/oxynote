import { afterEach, describe, it } from "vitest"
import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount } from "@vue/test-utils"
import { t } from "~/components/test-helpers"
import { serializeCustomTimeRange } from "~/utils/time-range"
import { useTimeRangeLabel } from "./useTimeRangeLabel"

// mounted harnesses share the Nuxt app
describe("useTimeRangeLabel", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)
	it.for([
		{
			from: "2026-11-01T05:30:00Z",
			to: "2026-11-01T07:30:00Z",
			startOffset: "GMT-04:00",
			endOffset: "",
		},
		{
			from: "2026-11-01T06:30:00Z",
			to: "2026-11-01T07:30:00Z",
			startOffset: "GMT-05:00",
			endOffset: "",
		},
		{
			from: "2026-11-01T04:30:00Z",
			to: "2026-11-01T05:30:00Z",
			startOffset: "",
			endOffset: "GMT-04:00",
		},
		{
			from: "2026-11-01T04:30:00Z",
			to: "2026-11-01T06:30:00Z",
			startOffset: "",
			endOffset: "GMT-05:00",
		},
	])(
		"distinguishes repeated-hour bounds $from to $to",
		async ({ from, to, startOffset, endOffset }, { expect }) => {
			const zone = "America/New_York"
			const start = new Date(from)
			const end = new Date(to)
			const wrapper = await mountSuspended(
				defineComponent({
					setup() {
						const label = useTimeRangeLabel(zone)
						return () => h("div", label(serializeCustomTimeRange(start, end)))
					},
				}),
			)
			const { d } = useNuxtApp().$i18n
			const clock = (date: Date) =>
				new Intl.DateTimeFormat("en", {
					timeZone: zone,
					hour: "numeric",
					minute: "2-digit",
					hourCycle: "h23",
				})
					.format(date)
					.replace(/^0/, "")
			expect(wrapper.text()).toBe(
				t("general.custom-time-range.label", {
					from: `${d(start, { month: "short", day: "numeric", timeZone: zone })} ${clock(start)}${startOffset ? ` ${startOffset}` : ""}`,
					to: `${clock(end)}${endOffset ? ` ${endOffset}` : ""}`,
				}),
			)
		},
	)
})

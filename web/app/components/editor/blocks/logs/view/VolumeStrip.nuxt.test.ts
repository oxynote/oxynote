import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import VolumeStrip from "./VolumeStrip.vue"
import type { VolumeBucket } from "../volume"
import { t } from "~/components/test-helpers"
import { stubThemeColorContext } from "~/components/editor/test-helpers/theme"

// echarts draws onto a canvas happy-dom does not implement. The stand-in
// keeps the option and renders the tooltip slot for the params a test sets.
const tooltip = vi.hoisted((): { params: unknown[] | null } => ({
	params: null,
}))
vi.mock("vue-echarts", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		default: defineComponent({
			name: "VChart",
			props: { option: { type: Object, required: true } },
			setup:
				(_, { slots }) =>
				() =>
					h("div", tooltip.params ? slots.tooltip?.(tooltip.params) : []),
		}),
	}
})

// built from local fields, so the axis reads the same clock in any time
// zone
const WINDOW = {
	from: new Date(2026, 0, 1, 0, 0),
	to: new Date(2026, 0, 1, 3, 0),
}

interface Series {
	id: string
	name: string
	stack: string
	itemStyle: { color: string }
	barMinHeight?: number
	data: (number | null)[]
}

// the tooltip params are module state, and chartStyles paints colours
// onto the stubbed canvas, so these tests cannot interleave
describe("<VolumeStrip>", { concurrent: false }, () => {
	beforeEach(() => {
		tooltip.params = null
		stubThemeColorContext()
	})

	it("names the strip as an image of the line volume", async ({ expect }) => {
		const wrapper = await mountStrip([])

		expect(wrapper.get("[role='img']").attributes("aria-label")).toBe(
			t("editor.logs.view.volume-label"),
		)
	})

	it("stacks one series for each level with lines, most severe at the bottom", async ({
		expect,
	}) => {
		const wrapper = await mountStrip([
			bucket(0, { info: 2, error: 1 }),
			bucket(1, { error: 3, unknown: 4 }),
		])

		expect(
			chartSeries(wrapper).map(({ id, name, stack, data }) => ({
				id,
				name,
				stack,
				data,
			})),
		).toEqual([
			{
				id: "error",
				name: t("editor.logs.level-names.error"),
				stack: "volume",
				data: [1, 3],
			},
			{
				id: "info",
				name: t("editor.logs.level-names.info"),
				stack: "volume",
				data: [2, 0],
			},
			{
				id: "unknown",
				name: t("editor.logs.level-names.unknown"),
				stack: "volume",
				data: [0, 4],
			},
			{ id: "empty", name: undefined, stack: "volume", data: [null, null] },
		])
	})

	it("marks each interval without lines with a dash on the baseline", async ({
		expect,
	}) => {
		const wrapper = await mountStrip([
			bucket(0),
			bucket(1, { info: 2 }),
			bucket(2),
		])

		const empty = chartSeries(wrapper).at(-1)
		expect(empty?.id).toBe("empty")
		expect(empty?.stack).toBe("volume")
		expect(empty?.barMinHeight).toBe(2)
		expect(empty?.data).toEqual([0, null, 0])
	})

	it("says an interval under the pointer has no lines", async ({ expect }) => {
		tooltip.params = [
			tooltipParam(LogLevel.Info, 0, 0),
			{ ...tooltipParam(LogLevel.Info, 0, 0), seriesId: "empty" },
		]

		const wrapper = await mountStrip([bucket(0), bucket(1, { info: 2 })])

		expect(wrapper.get(".px-2.pt-1").text()).toBe(
			useNuxtApp().$i18n.d(WINDOW.from, "short-with-full-time"),
		)
		expect(wrapper.text()).toContain(t("editor.logs.view.volume-empty"))
		expect(wrapper.findAll(".flex-col > .flex-col > .flex")).toHaveLength(0)
	})

	it("lists the count of each level under the pointer with the bar's time", async ({
		expect,
	}) => {
		tooltip.params = [
			tooltipParam(LogLevel.Error, 1, 3),
			tooltipParam(LogLevel.Unknown, 1, 4),
		]

		const wrapper = await mountStrip([
			bucket(0, { error: 1 }),
			bucket(1, { error: 3, unknown: 4 }),
		])

		const lines = wrapper
			.findAll(".flex-col > .flex-col > .flex")
			.map((row) => row.text())
		expect(wrapper.get(".px-2.pt-1").text()).toBe(
			useNuxtApp().$i18n.d(
				new Date(WINDOW.from.getTime() + 60_000),
				"short-with-full-time",
			),
		)
		expect(lines).toEqual([
			`${t("editor.logs.level-names.error")}3`,
			`${t("editor.logs.level-names.unknown")}4`,
		])
	})

	it.for([
		{
			name: "names times on the axis of a short window",
			input: WINDOW,
			expected: ["00:00", "01:00", "02:00", "03:00"],
		},
		{
			name: "names times on the axis of a two-day window",
			input: {
				from: new Date(2026, 0, 1, 0, 0),
				to: new Date(2026, 0, 3, 0, 0),
			},
			expected: ["00:00", "16:00", "08:00", "00:00"],
		},
		{
			name: "names days on the axis of a longer window",
			input: {
				from: new Date(2026, 0, 1, 0, 0),
				to: new Date(2026, 0, 7, 0, 0),
			},
			expected: ["Jan 1", "Jan 3", "Jan 5", "Jan 7"],
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountStrip([], input)

		expect(
			wrapper.findAll(".justify-between > span").map((s) => s.text()),
		).toEqual(expected)
	})
})

function mountStrip(
	buckets: VolumeBucket[],
	window: { from: Date; to: Date } = WINDOW,
) {
	return mountSuspended(VolumeStrip, { props: { buckets, window } })
}

function bucket(
	index: number,
	counts: Partial<Record<LogLevel, number>> = {},
): VolumeBucket {
	return {
		start: WINDOW.from.getTime() + index * 60_000,
		counts: {
			critical: 0,
			error: 0,
			warning: 0,
			info: 0,
			debug: 0,
			trace: 0,
			unknown: 0,
			...counts,
		},
	}
}

function tooltipParam(level: LogLevel, dataIndex: number, value: number) {
	return {
		seriesId: level,
		seriesName: t(`editor.logs.level-names.${level}`),
		marker: "",
		value,
		dataIndex,
	}
}

function chartSeries(wrapper: VueWrapper): Series[] {
	const option = wrapper.findComponent({ name: "VChart" }).props("option") as {
		series: Series[]
	}

	return option.series
}

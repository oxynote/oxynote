import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it } from "vitest"
import MetricDiffDetails from "./MetricDiffDetails.vue"
import { makeNode } from "../../test-helpers/node-view"
import { METRIC_BLOCK_NAME } from "../node-names"
import { VisualizationTimeUnit } from "./utils"
import {
	RefreshInterval,
	TimeRangePreset,
	serializeCustomTimeRange,
} from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { settleMutations, t } from "~/components/test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"

// the data source list lives in the app-wide query cache
describe("<MetricDiffDetails>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
	})

	afterEach(() => {
		disposeMockEndpoints()
	})

	it("labels each field of an added query by the query's number", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{ queries: [query("up", "")] },
			{ queries: [query("up", ""), query("rate(x[5m])", "{{job}}")] },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.metrics.config.query-name-default-format", { index: 2 }),
				null,
				"rate(x[5m])",
			],
			[t("editor.metrics.diff.legend", { index: 2 }), null, "{{job}}"],
		])
		expect(wrapper.findAll("dd").every((dd) => dd.classes("font-mono"))).toBe(
			true,
		)
	})

	it("shows an empty legend as auto", async ({ expect }) => {
		const wrapper = await mountDetails(
			{ queries: [query("up", "{{job}}")] },
			{ queries: [query("up", "")] },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.metrics.diff.legend", { index: 1 }),
				"{{job}}",
				t("editor.diff-change-marker.auto"),
			],
		])
	})

	it.for([
		{ name: "decimal places", input: "decimals", label: "decimals-label" },
		{ name: "min value", input: "axisBoundsMin", label: "bounds-min-label" },
		{ name: "max value", input: "axisBoundsMax", label: "bounds-max-label" },
	])(
		"shows auto as the old $name when it was not set",
		async ({ input, label }, { expect }) => {
			const wrapper = await mountDetails({}, { [input]: 1 })

			expect(rows(wrapper)).toEqual([
				[
					t(`editor.metrics.config.${label}`),
					t("editor.diff-change-marker.auto"),
					"1",
				],
			])
		},
	)

	it("shows a setting that was not set before as empty", async ({ expect }) => {
		const wrapper = await mountDetails(
			{},
			{ unitType: VisualizationTimeUnit.Seconds },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.metrics.config.unit-label"),
				t("editor.diff-change-marker.empty"),
				t("editor.metrics.config.unit-options.time.options.seconds"),
			],
		])
	})

	it("shows the settings by the names the config modal uses", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{
				timeRange: TimeRangePreset.Last15Minutes,
				refreshInterval: RefreshInterval.M1,
				thresholds: [{ value: 1 }],
			},
			{
				timeRange: TimeRangePreset.Last5Minutes,
				refreshInterval: RefreshInterval.M5,
				thresholds: [{ value: 1 }, { value: 2 }],
			},
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.config-modal-utils.time-range-label"),
				t("general.time-range-options.last_15_minutes"),
				t("general.time-range-options.last_5_minutes"),
			],
			[
				t("editor.config-modal-utils.refresh-interval-label"),
				t("general.refresh-interval-options.1m"),
				t("general.refresh-interval-options.5m"),
			],
			[t("editor.metrics.config.thresholds-label"), "1", "2"],
		])
	})

	it.for([
		{
			name: "different years",
			oldStart: "2025-11-11T05:38:00.000Z",
			oldEnd: "2025-11-11T05:42:00.000Z",
			start: "2026-11-11T05:38:00.000Z",
			end: "2026-11-11T05:42:00.000Z",
		},
		{
			name: "different milliseconds",
			oldStart: "2026-11-11T05:38:00.123Z",
			oldEnd: "2026-11-11T05:42:00.000Z",
			start: "2026-11-11T05:38:00.124Z",
			end: "2026-11-11T05:42:00.000Z",
		},
	])(
		"distinguishes ranges with $name in a review",
		async ({ oldStart, oldEnd, start, end }, { expect }) => {
			const wrapper = await mountDetails(
				{
					timeRange: serializeCustomTimeRange(
						new Date(oldStart),
						new Date(oldEnd),
					),
				},
				{ timeRange: serializeCustomTimeRange(new Date(start), new Date(end)) },
			)
			const result = rows(wrapper)[0]
			expect(result?.[1]).not.toBe(result?.[2])
		},
	)

	it("names the built-in variable a setting follows", async ({ expect }) => {
		const wrapper = await mountDetails(
			{
				timeRange: METRIC_TIME_RANGE_REFERENCE,
				refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
			},
			{
				timeRange: TimeRangePreset.Last5Minutes,
				refreshInterval: RefreshInterval.M5,
			},
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.config-modal-utils.time-range-label"),
				t("editor.config-modal-utils.variable-time-range"),
				t("general.time-range-options.last_5_minutes"),
			],
			[
				t("editor.config-modal-utils.refresh-interval-label"),
				t("editor.config-modal-utils.variable-refresh-interval"),
				t("general.refresh-interval-options.5m"),
			],
		])
	})

	it("shows the exact dates of a custom range in the review diff", async ({
		expect,
	}) => {
		const from = new Date("2026-03-28T09:30:15.123Z")
		const to = new Date("2026-03-29T18:45:30.456Z")
		const wrapper = await mountDetails(
			{ timeRange: TimeRangePreset.Last1Hour },
			{ timeRange: serializeCustomTimeRange(from, to) },
		)
		const { d } = useNuxtApp().$i18n

		expect(rows(wrapper)).toEqual([
			[
				t("editor.config-modal-utils.time-range-label"),
				t("general.time-range-options.last_1_hour"),
				t("general.custom-time-range.label", {
					from: d(from, {
						key: "short-with-full-time",
						fractionalSecondDigits: 3,
						timeZoneName: "longOffset",
					}),
					to: d(to, {
						key: "short-with-full-time",
						fractionalSecondDigits: 3,
						timeZoneName: "longOffset",
					}),
				}),
			],
		])
	})

	it("names a data source instead of showing its id", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => [
			{ id: "ds-1", name: "Demo", type: "prometheus", url: "demo://x" },
		])

		const wrapper = await mountDetails(
			{ dataSourceId: "ds-gone" },
			{ dataSourceId: "ds-1" },
		)
		await settleMutations()

		expect(rows(wrapper)).toEqual([
			[
				t("editor.config-modal-utils.data-source-label"),
				t("editor.config-modal-utils.data-source-label-deleted"),
				"Demo",
			],
		])
	})

	it("shows an attribute it has no label for by its name", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{ config: { title: "CPU" } },
			{ config: { title: "RAM" } },
		)

		expect(rows(wrapper)).toEqual([
			["config", '{"title":"CPU"}', '{"title":"RAM"}'],
		])
	})
})

function query(expr: string, legendFormat: string) {
	return { name: "Query", query: expr, legendFormat: legendFormat }
}

// each row as its label, its old value and its new value, null for a side
// the row does not show
function rows(wrapper: Awaited<ReturnType<typeof mountDetails>>) {
	const labels = wrapper.findAll("dt")

	return wrapper
		.findAll("dd")
		.map((dd, index) => [
			labels[index]?.text(),
			dd.find(".line-through").exists() ? dd.get(".line-through").text() : null,
			dd.find(".bg-diff-text-added").exists()
				? dd.get(".bg-diff-text-added").text()
				: null,
		])
}

function mountDetails(
	oldAttrs: Record<string, unknown>,
	newAttrs: Record<string, unknown>,
) {
	return mountSuspended(MetricDiffDetails, {
		props: {
			node: makeNode(
				{
					...newAttrs,
					diffStatus: DiffStatus.Modified,
					oldNode: { attrs: oldAttrs },
				},
				{ typeName: METRIC_BLOCK_NAME },
			),
		},
	})
}

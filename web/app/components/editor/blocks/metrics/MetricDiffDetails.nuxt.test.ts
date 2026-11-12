import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it } from "vitest"
import MetricDiffDetails from "./MetricDiffDetails.vue"
import { makeNode } from "../../test-helpers/node-view"
import { METRIC_BLOCK_NAME } from "../node-names"
import { GridItemWidth } from "@oxynote/common/grid"
import { VisualizationTimeUnit } from "./utils"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
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

	it("shows the width and height by the names the block menu uses", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{ width: GridItemWidth.Standard },
			{ widePosX: 0, widePosY: 1, width: 24, height: 2 },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.diff-change-marker.grid-item.width"),
				t("editor.grid-item.width-options.standard"),
				t("editor.grid-item.width-options.wide"),
			],
			[
				t("editor.diff-change-marker.grid-item.height"),
				t("editor.grid-item.height-options.standard"),
				t("editor.grid-item.height-options.tall"),
			],
		])
	})

	it("lists where the block shows in both layouts", async ({ expect }) => {
		const wrapper = await mountDetails(
			{ gridPlace: { row: 0, column: 12 }, stackPlace: 1 },
			{ gridPlace: { row: 1, column: 0 }, stackPlace: 3 },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.diff-change-marker.grid-item.wide-layout"),
				t("editor.diff-change-marker.grid-item.wide-place", {
					row: 1,
					column: 13,
				}),
				t("editor.diff-change-marker.grid-item.wide-place", {
					row: 2,
					column: 1,
				}),
			],
			[
				t("editor.diff-change-marker.grid-item.stacked-layout"),
				t("editor.diff-change-marker.grid-item.stacked-place.two", {
					place: 2,
				}),
				t("editor.diff-change-marker.grid-item.stacked-place.other", {
					place: 4,
				}),
			],
		])
	})

	it("shows a width the block menu has no name for by its columns", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{ width: 10, height: 2 },
			{ width: 8, height: 1 },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.diff-change-marker.grid-item.width"),
				t("editor.grid-item.width-options.columns", { count: 10 }, 10),
				t("editor.grid-item.width-options.compact"),
			],
			[
				t("editor.diff-change-marker.grid-item.height"),
				t("editor.grid-item.height-options.tall"),
				t("editor.grid-item.height-options.standard"),
			],
		])
	})

	it("shows no change for a block that only gained its place and a numeric size", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{ width: GridItemWidth.Standard },
			{
				widePosX: 12,
				widePosY: 3,
				width: 12,
				height: 1,
			},
		)

		expect(rows(wrapper)).toEqual([])
	})

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

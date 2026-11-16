import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it } from "vitest"
import LogDiffDetails from "./LogDiffDetails.vue"
import { makeNode } from "../../test-helpers/node-view"
import { LOG_BLOCK_NAME } from "../node-names"
import { GridItemWidth } from "@oxynote/common/grid"
import { LogTimestampFormat } from "./utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { settleMutations, t } from "~/components/test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

// the data source list lives in the app-wide query cache
describe("<LogDiffDetails>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
	})

	afterEach(() => {
		disposeMockEndpoints()
	})

	it("labels a changed query by the query's number", async ({ expect }) => {
		const wrapper = await mountDetails(
			{ queries: [query('{app="api"}'), query('{app="web"}')] },
			{ queries: [query('{app="api"}'), query('{app="web"} |= "error"')] },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.logs.config.query-name-default-format", { index: 2 }),
				'{app="web"}',
				'{app="web"} |= "error"',
			],
		])
		expect(wrapper.get("dd").classes()).toContain("font-mono")
	})

	it("shows an added query on its new side only", async ({ expect }) => {
		const wrapper = await mountDetails(
			{ queries: [query('{app="api"}')] },
			{ queries: [query('{app="api"}'), query('{app="web"}')] },
		)

		expect(rows(wrapper)).toEqual([
			[
				t("editor.logs.config.query-name-default-format", { index: 2 }),
				null,
				'{app="web"}',
			],
		])
	})

	it("shows a setting that was not set before as empty", async ({ expect }) => {
		const wrapper = await mountDetails({}, { title: "API errors" })

		expect(rows(wrapper)).toEqual([
			[
				t("editor.logs.config.title-label"),
				t("editor.diff-change-marker.empty"),
				"API errors",
			],
		])
		expect(wrapper.get("dd").classes()).not.toContain("font-mono")
	})

	it("shows the settings by the names the config modal uses", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			{
				title: "Errors",
				timeRange: TimeRangePreset.Last15Minutes,
				refreshInterval: RefreshInterval.M1,
				timestampFormat: LogTimestampFormat.Readable,
				order: LogOrder.Desc,
				width: GridItemWidth.Standard,
			},
			{
				title: "API errors",
				timeRange: TimeRangePreset.Last5Minutes,
				refreshInterval: RefreshInterval.M5,
				timestampFormat: LogTimestampFormat.NumericNano,
				order: LogOrder.Asc,
				width: GridItemWidth.Wide,
			},
		)

		expect(rows(wrapper)).toEqual([
			[t("editor.logs.config.title-label"), "Errors", "API errors"],
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
			[
				t("editor.logs.config.timestamp-format-label"),
				t("editor.logs.config.timestamp-format-options.readable.title"),
				t("editor.logs.config.timestamp-format-options.numeric_nano.title"),
			],
			[
				t("editor.logs.config.order-label"),
				t("editor.logs.config.order-options.desc"),
				t("editor.logs.config.order-options.asc"),
			],
			[
				t("editor.diff-change-marker.grid-item.width"),
				t("editor.grid-item.width-options.standard"),
				t("editor.grid-item.width-options.wide"),
			],
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
		const wrapper = await mountDetails({ legacy: "a" }, { legacy: "b" })

		expect(rows(wrapper)).toEqual([["legacy", "a", "b"]])
	})
})

function query(expr: string) {
	return { name: "Query", query: expr }
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
	return mountSuspended(LogDiffDetails, {
		props: {
			node: makeNode(
				{
					...newAttrs,
					diffStatus: DiffStatus.Modified,
					oldNode: { attrs: oldAttrs },
				},
				{ typeName: LOG_BLOCK_NAME },
			),
		},
	})
}

import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import BottomConfig from "./BottomConfig.vue"
import QueryEditor from "./query/QueryEditor.vue"
import LegendEditor from "./query/LegendEditor.vue"
import { stubChartColorContext } from "../test-helpers"
import { defaultMetricConfig, type MetricConfig } from "../utils"
import {
	at,
	clearTeleportedOverlays,
	emitFrom,
	emitFromNth,
	findButtonByText,
	t,
} from "~/components/test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

const DATA_SOURCE_ID = makeXid("ds")

const DATA_SOURCES = [
	{
		id: DATA_SOURCE_ID,
		name: "Prod",
		url: "https://db.test",
		type: DataSourceType.PostgreSQL,
		status: DataSourceStatus.Success,
	},
]

function query(
	name: string,
	q: string,
	legendFormat = "",
): {
	name: string
	query: string
	legendFormat: string
} {
	return { name: name, query: q, legendFormat: legendFormat }
}

function configWith(overrides: Partial<MetricConfig> = {}): MetricConfig {
	return {
		...defaultMetricConfig(),
		dataSourceId: DATA_SOURCE_ID,
		...overrides,
	}
}

function mountBottom(props: Record<string, unknown> = {}) {
	return mountSuspended(BottomConfig, {
		props: { modelValue: configWith(), ...props },
	})
}

function queryEditors(wrapper: VueWrapper) {
	return wrapper.findAllComponents(QueryEditor)
}

function legendEditors(wrapper: VueWrapper) {
	return wrapper.findAllComponents(LegendEditor)
}

// the editable flag is a shared cookie state, the editor store and the
// query cache are app-wide, and the help popover is teleported into a
// shared <body>, so these tests cannot interleave
describe("<BottomConfig>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		stubChartColorContext()
		mockEndpoint("GET", "/api/data-sources", () => DATA_SOURCES)
		// seeding the cache too puts the data source's type in place before
		// the first render, so the help popover knows which language to
		// explain right away
		seedQueryData(["data-sources", "list"], DATA_SOURCES)
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	afterEach(disposeMockEndpoints)

	it("shows the single query without naming it", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: [query("Query 1", "up")] }),
		})

		expect(queryEditors(wrapper)).toHaveLength(1)
		expect(legendEditors(wrapper)).toHaveLength(1)
		expect(wrapper.text()).not.toContain("Query 1")
		expect(wrapper.text()).toContain(t("editor.metrics.config.query-label"))
	})

	it("names each query once there are several", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Query 1", "up"), query("Query 2", "down")],
			}),
		})

		expect(queryEditors(wrapper)).toHaveLength(2)
		expect(wrapper.text()).toContain("Query 1")
		expect(wrapper.text()).toContain("Query 2")
	})

	it("stores a query the reader typed", async ({ expect }) => {
		const config = configWith({ queries: [query("Query 1", "up")] })
		const wrapper = await mountBottom({ modelValue: config })

		emitFrom(wrapper, QueryEditor, "update:modelValue", "rate(up[5m])")
		await nextTick()

		expect(config.queries).toEqual([query("Query 1", "rate(up[5m])")])
	})

	it("stores a legend format the reader typed", async ({ expect }) => {
		const config = configWith({ queries: [query("Query 1", "up")] })
		const wrapper = await mountBottom({ modelValue: config })

		emitFrom(wrapper, LegendEditor, "update:modelValue", "{{instance}}")
		await nextTick()

		expect(config.queries).toEqual([query("Query 1", "up", "{{instance}}")])
	})

	it("stores the second query the reader typed", async ({ expect }) => {
		const config = configWith({
			queries: [query("Query 1", "up"), query("Query 2", "down")],
		})
		const wrapper = await mountBottom({ modelValue: config })

		emitFromNth(wrapper, QueryEditor, 1, "update:modelValue", "sum(down)")
		await nextTick()

		expect(config.queries).toEqual([
			query("Query 1", "up"),
			query("Query 2", "sum(down)"),
		])
	})

	it("adds a numbered query", async ({ expect }) => {
		const config = configWith({ queries: [query("Query 1", "up")] })
		const wrapper = await mountBottom({ modelValue: config })

		await findButtonByText(
			wrapper,
			t("editor.metrics.config.add-query-button"),
		).trigger("click")

		expect(config.queries).toEqual([
			query("Query 1", "up"),
			query("Query 2", ""),
		])
	})

	it("adds the first query to a config that has none", async ({ expect }) => {
		const config = configWith({ queries: null })
		const wrapper = await mountBottom({ modelValue: config })

		await findButtonByText(
			wrapper,
			t("editor.metrics.config.add-query-button"),
		).trigger("click")

		expect(config.queries).toEqual([query("Query 1", "")])
	})

	it("renumbers the remaining queries after a deletion", async ({ expect }) => {
		const config = configWith({
			queries: [
				query("Query 1", "a"),
				query("Query 2", "b"),
				query("Query 3", "c"),
			],
		})
		const wrapper = await mountBottom({ modelValue: config })

		await at(wrapper.findAll("button"), 0).trigger("click")

		expect(config.queries).toEqual([
			query("Query 1", "b"),
			query("Query 2", "c"),
		])
	})

	it("refuses to delete the last remaining query", async ({ expect }) => {
		const config = configWith({
			queries: [query("Query 1", "a"), query("Query 2", "b")],
		})
		const wrapper = await mountBottom({ modelValue: config })
		await at(wrapper.findAll("button"), 0).trigger("click")

		await at(wrapper.findAll("button"), 0).trigger("click")

		expect(config.queries).toEqual([query("Query 1", "b")])
	})

	it("offers no editing controls in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Query 1", "a"), query("Query 2", "b")],
			}),
		})

		expect(wrapper.text()).not.toContain(
			t("editor.metrics.config.add-query-button"),
		)
		expect(
			wrapper.findAll("button").filter((b) => b.text() === ""),
		).toHaveLength(0)
	})

	it("explains the query language of the chosen data source", async ({
		expect,
	}) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: [query("Query 1", "up")] }),
		})

		const help = findButtonByText(
			wrapper,
			t("editor.metrics.config.query-explanation-button-label"),
		)
		await help.trigger("pointerdown", { button: 0 })
		await help.trigger("click")
		await nextTick()

		expect(document.body.textContent).toContain(
			t(
				"editor.metrics.config.query-explanations.postgresql.main-placeholders.postgresql-queries",
			),
		)
	})

	it("shows the query a diff replaced beside the new one", async ({
		expect,
	}) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: [query("Query 1", "after")] }),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({ queries: [query("Query 1", "before")] }),
		})

		const editors = queryEditors(wrapper)

		expect(editors).toHaveLength(2)
		expect(at(editors, 0).props("diffStatus")).toBe(DiffStatus.Added)
		expect(at(editors, 1).props("modelValue")).toBe("before")
		expect(at(editors, 1).props("diffStatus")).toBe(DiffStatus.Removed)
	})

	it("shows the legend format a diff replaced beside the new one", async ({
		expect,
	}) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: [query("Query 1", "up", "after")] }),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				queries: [query("Query 1", "up", "before")],
			}),
		})

		const editors = legendEditors(wrapper)

		expect(editors).toHaveLength(2)
		expect(at(editors, 1).props("modelValue")).toBe("before")
		expect(at(editors, 1).props("diffStatus")).toBe(DiffStatus.Removed)
	})

	it("marks a query the diff added", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Query 1", "a"), query("Query 2", "b")],
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({ queries: [query("Query 1", "a")] }),
		})

		const editors = queryEditors(wrapper)

		expect(editors).toHaveLength(2)
		expect(at(editors, 0).props("diffStatus")).toBeNull()
		expect(at(editors, 1).props("diffStatus")).toBe(DiffStatus.Added)
	})

	it("marks a query the diff dropped", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: [query("Query 1", "a")] }),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				queries: [query("Query 1", "a"), query("Query 2", "b")],
			}),
		})

		const editors = queryEditors(wrapper)

		expect(editors).toHaveLength(2)
		expect(at(editors, 1).props("modelValue")).toBe("b")
		expect(at(editors, 1).props("diffStatus")).toBe(DiffStatus.Removed)
	})

	it("leaves an unchanged query unmarked in a diff", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Query 1", "a"), query("Query 2", "b")],
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				queries: [query("Query 1", "a"), query("Query 2", "changed")],
			}),
		})

		const editors = queryEditors(wrapper)

		expect(at(editors, 0).props("diffStatus")).toBeNull()
		expect(at(editors, 1).props("diffStatus")).toBe(DiffStatus.Added)
		expect(at(editors, 2).props("diffStatus")).toBe(DiffStatus.Removed)
	})
})

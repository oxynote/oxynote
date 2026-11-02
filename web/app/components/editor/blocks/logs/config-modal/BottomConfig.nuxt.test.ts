import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import BottomConfig from "./BottomConfig.vue"
import QueryEditor from "../../config-modal-utils/query/QueryEditor.vue"
import { defaultLogConfig, type LogConfig, type LogQuery } from "../utils"
import {
	at,
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
} from "~/composables/api/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { TimeRangePreset } from "~/utils/time-range"

const DATA_SOURCE_ID = makeXid("ds")

// the editable flag is a shared cookie state and the editor store and the
// query cache are app-wide, so these tests cannot interleave
describe("<BottomConfig>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		mockEndpoint("GET", "/api/data-sources", () => [])
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	afterEach(disposeMockEndpoints)

	it("shows the single query without naming it", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: [query("Errors", "level:error")] }),
		})

		expect(queryEditors(wrapper)).toHaveLength(1)
		expect(wrapper.text()).toContain(t("editor.logs.config.query-label"))
		expect(wrapper.text()).not.toContain("Errors")
		expect(deleteButtons(wrapper)).toHaveLength(0)
		expect(wrapper.text()).toContain(t("editor.logs.config.add-query-button"))
	})

	it("names each query once there are several", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Errors", "level:error"), query("Slow", "slow")],
			}),
		})

		expect(queryEditors(wrapper)).toHaveLength(2)
		expect(wrapper.text()).toContain("Errors")
		expect(wrapper.text()).toContain("Slow")
		expect(deleteButtons(wrapper)).toHaveLength(2)
	})

	it("hands each query editor its query and the data source", async ({
		expect,
	}) => {
		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Errors", "level:error"), query("Slow", "slow")],
			}),
		})

		const editors = queryEditors(wrapper)

		expect(editors).toHaveLength(2)
		expect(at(editors, 0).props()).toEqual({
			modelValue: "level:error",
			dataSourceId: DATA_SOURCE_ID,
			timeRange: TimeRangePreset.Last1Hour,
			diffStatus: null,
		})
		expect(at(editors, 1).props()).toEqual({
			modelValue: "slow",
			dataSourceId: DATA_SOURCE_ID,
			timeRange: TimeRangePreset.Last1Hour,
			diffStatus: null,
		})
	})

	it("shows no query while the config has none", async ({ expect }) => {
		const wrapper = await mountBottom({
			modelValue: configWith({ queries: null }),
		})

		expect(queryEditors(wrapper)).toHaveLength(0)
		expect(wrapper.text()).toContain(t("editor.logs.config.add-query-button"))
	})

	it("stores a query the reader typed", async ({ expect }) => {
		const config = configWith({ queries: [query("Errors", "level:error")] })
		const wrapper = await mountBottom({ modelValue: config })

		emitFrom(wrapper, QueryEditor, "update:modelValue", "level:warn")
		await nextTick()

		expect(config.queries).toEqual([query("Errors", "level:warn")])
	})

	it("stores the second query the reader typed", async ({ expect }) => {
		const config = configWith({
			queries: [query("Errors", "level:error"), query("Slow", "slow")],
		})
		const wrapper = await mountBottom({ modelValue: config })

		emitFromNth(wrapper, QueryEditor, 1, "update:modelValue", "very slow")
		await nextTick()

		expect(config.queries).toEqual([
			query("Errors", "level:error"),
			query("Slow", "very slow"),
		])
	})

	it("adds a numbered query", async ({ expect }) => {
		const config = configWith({ queries: [query("Errors", "level:error")] })
		const wrapper = await mountBottom({ modelValue: config })

		await findButtonByText(
			wrapper,
			t("editor.logs.config.add-query-button"),
		).trigger("click")

		expect(config.queries).toEqual([
			query("Errors", "level:error"),
			query(defaultName(2), ""),
		])
	})

	it("adds the first query to a config that has none", async ({ expect }) => {
		const config = configWith({ queries: null })
		const wrapper = await mountBottom({ modelValue: config })

		await findButtonByText(
			wrapper,
			t("editor.logs.config.add-query-button"),
		).trigger("click")

		expect(config.queries).toEqual([query(defaultName(1), "")])
	})

	it("renumbers the remaining queries after a deletion", async ({ expect }) => {
		const config = configWith({
			queries: [query("Errors", "a"), query("Slow", "b"), query("Audit", "c")],
		})
		const wrapper = await mountBottom({ modelValue: config })

		await at(deleteButtons(wrapper), 0).trigger("click")

		expect(config.queries).toEqual([
			query(defaultName(1), "b"),
			query(defaultName(2), "c"),
		])
	})

	// a plain config does not redraw the list, so the second press still
	// finds a delete button while only one query is left
	it("refuses to delete the last remaining query", async ({ expect }) => {
		const config = configWith({
			queries: [query("Errors", "a"), query("Slow", "b")],
		})
		const wrapper = await mountBottom({ modelValue: config })
		await at(deleteButtons(wrapper), 0).trigger("click")

		await at(deleteButtons(wrapper), 0).trigger("click")

		expect(config.queries).toEqual([query(defaultName(1), "b")])
	})

	it("offers no editing controls in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Errors", "a"), query("Slow", "b")],
			}),
		})

		expect(wrapper.text()).toContain("Errors")
		expect(wrapper.text()).not.toContain(
			t("editor.logs.config.add-query-button"),
		)
		expect(deleteButtons(wrapper)).toHaveLength(0)
	})

	it("offers no editing controls while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountBottom({
			modelValue: configWith({
				queries: [query("Errors", "a"), query("Slow", "b")],
			}),
		})

		expect(wrapper.text()).toContain("Errors")
		expect(wrapper.text()).not.toContain(
			t("editor.logs.config.add-query-button"),
		)
		expect(deleteButtons(wrapper)).toHaveLength(0)
	})

	describe("in a diff", () => {
		it("shows the query a diff replaced after the new one", async ({
			expect,
		}) => {
			const wrapper = await mountBottom({
				modelValue: configWith({ queries: [query("Errors", "after")] }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({ queries: [query("Errors", "before")] }),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "after", diffStatus: DiffStatus.Added },
				{ query: "before", diffStatus: DiffStatus.Removed },
			])
			// both sides of the one query count, so each is named
			expect(wrapper.text()).toContain("Errors")
		})

		it("marks a query the diff added", async ({ expect }) => {
			const wrapper = await mountBottom({
				modelValue: configWith({
					queries: [query("Errors", "a"), query("Slow", "b")],
				}),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({ queries: [query("Errors", "a")] }),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "a", diffStatus: null },
				{ query: "b", diffStatus: DiffStatus.Added },
			])
		})

		it("marks a query the diff dropped", async ({ expect }) => {
			const wrapper = await mountBottom({
				modelValue: configWith({ queries: [query("Errors", "a")] }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({
					queries: [query("Errors", "a"), query("Slow", "b")],
				}),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "a", diffStatus: null },
				{ query: "b", diffStatus: DiffStatus.Removed },
			])
		})

		it("leaves an unchanged query unmarked beside a changed one", async ({
			expect,
		}) => {
			const wrapper = await mountBottom({
				modelValue: configWith({
					queries: [query("Errors", "a"), query("Slow", "b")],
				}),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({
					queries: [query("Errors", "a"), query("Slow", "changed")],
				}),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "a", diffStatus: null },
				{ query: "b", diffStatus: DiffStatus.Added },
				{ query: "changed", diffStatus: DiffStatus.Removed },
			])
		})

		it("leaves a query unmarked when only its name changed", async ({
			expect,
		}) => {
			const wrapper = await mountBottom({
				modelValue: configWith({ queries: [query("Errors", "a")] }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({ queries: [query("Failures", "a")] }),
			})

			expect(editorDiffs(wrapper)).toEqual([{ query: "a", diffStatus: null }])
		})

		it("marks every query of a block that gained its queries", async ({
			expect,
		}) => {
			const wrapper = await mountBottom({
				modelValue: configWith({
					queries: [query("Errors", "a"), query("Slow", "b")],
				}),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({ queries: null }),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "a", diffStatus: DiffStatus.Added },
				{ query: "b", diffStatus: DiffStatus.Added },
			])
		})

		it("marks every query of a block that lost its queries", async ({
			expect,
		}) => {
			const wrapper = await mountBottom({
				modelValue: configWith({ queries: null }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({
					queries: [query("Errors", "a"), query("Slow", "b")],
				}),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "a", diffStatus: DiffStatus.Removed },
				{ query: "b", diffStatus: DiffStatus.Removed },
			])
		})

		it.for([
			{
				name: "lists the queries plainly for a block the diff added",
				input: () => ({
					diffStatus: DiffStatus.Added,
					oldConfig: configWith({ queries: [query("Errors", "before")] }),
				}),
			},
			{
				name: "lists the queries plainly for a modification without the old config",
				input: () => ({ diffStatus: DiffStatus.Modified, oldConfig: null }),
			},
			{
				name: "lists the queries plainly when a modification kept them",
				input: () => ({
					diffStatus: DiffStatus.Modified,
					oldConfig: configWith({
						title: "Before",
						queries: [query("Errors", "after")],
					}),
				}),
			},
		])("$name", async ({ input }, { expect }) => {
			const wrapper = await mountBottom({
				modelValue: configWith({ queries: [query("Errors", "after")] }),
				...input(),
			})

			expect(editorDiffs(wrapper)).toEqual([
				{ query: "after", diffStatus: null },
			])
		})
	})
})

function query(name: string, q: string): LogQuery {
	return { name: name, query: q }
}

function configWith(overrides: Partial<LogConfig> = {}): LogConfig {
	return {
		...defaultLogConfig(),
		dataSourceId: DATA_SOURCE_ID,
		...overrides,
	}
}

function defaultName(index: number): string {
	return t("editor.logs.config.query-name-default-format", { index: index })
}

function mountBottom(props: Record<string, unknown> = {}) {
	return mountSuspended(BottomConfig, {
		props: { modelValue: configWith(), ...props },
	})
}

function queryEditors(wrapper: VueWrapper) {
	return wrapper.findAllComponents(QueryEditor)
}

function deleteButtons(wrapper: VueWrapper) {
	const label = t("editor.logs.config.delete-query-button")

	return wrapper.findAll("button").filter((b) => b.text() === label)
}

function editorDiffs(wrapper: VueWrapper) {
	return queryEditors(wrapper).map((editor) => ({
		query: editor.props("modelValue") as string,
		diffStatus: editor.props("diffStatus") as DiffStatus | null,
	}))
}

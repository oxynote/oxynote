import { createRequire } from "node:module"
import { describe, it, vi } from "vitest"
import { ref } from "vue"
import { EditorState, type Extension } from "@codemirror/state"
import {
	CompletionContext,
	type CompletionSource,
} from "@codemirror/autocomplete"
import type { EditorView } from "codemirror"
import { TimeRangePreset } from "../../utils"
import {
	installAutoImportGlobals,
	stubPrometheusAPI,
	stubSQLAPI,
	type PrometheusStubData,
	type SQLStubData,
} from "./test-helpers"
import {
	singleLineExtension,
	trimEditorWhitespace,
	useLegendEditorExtension,
	useQueryEditorExtension,
} from "./editor"

// the promql extensions live in the CJS copies of the codemirror
// packages (see promql.test.ts), so running them needs state and
// completion classes from those same copies. The sql extensions live in
// the regular ESM copies.
const cjsRequire = createRequire(import.meta.url)
const { EditorState: CjsEditorState } = cjsRequire(
	"@codemirror/state",
) as typeof import("@codemirror/state")
const { CompletionContext: CjsCompletionContext } = cjsRequire(
	"@codemirror/autocomplete",
) as typeof import("@codemirror/autocomplete")

installAutoImportGlobals()

const prometheusAPIMock = vi.hoisted(() => vi.fn())
const sqlAPIMock = vi.hoisted(() => vi.fn())
const dataSourceAPIMock = vi.hoisted(() => vi.fn())

vi.mock("~/composables/api/usePrometheusDataSourceAPI", () => ({
	default: prometheusAPIMock,
}))

vi.mock("~/composables/api/useSQLDataSourceAPI", () => ({
	default: sqlAPIMock,
}))

vi.mock("~/composables/api/useDataSourceAPI", () => ({
	default: dataSourceAPIMock,
}))

const t = (key: string) => key

const DATA_SOURCES = [
	{ id: "prom-1", type: DataSourceType.Prometheus },
	{ id: "pg-1", type: DataSourceType.PostgreSQL },
]

interface StubOptions {
	prometheusData?: PrometheusStubData
	sqlData?: SQLStubData
}

function stubAPIs(options: StubOptions) {
	const prom = stubPrometheusAPI(prometheusAPIMock, options.prometheusData)
	const sql = stubSQLAPI(sqlAPIMock, options.sqlData)

	dataSourceAPIMock.mockReturnValue({
		fetchDataSources: { state: ref({ data: DATA_SOURCES }) },
	})

	return { prom, sql }
}

function makeEditor(
	options: StubOptions & {
		dataSourceId?: string | null
		editingEnabled?: boolean
	} = {},
) {
	const stubs = stubAPIs(options)
	const editor = useQueryEditorExtension(
		t,
		() => options.dataSourceId ?? null,
		() => TimeRangePreset.Last1Hour,
		() => options.editingEnabled ?? true,
	)

	return { ...stubs, ...editor }
}

function makeLegendEditor(
	options: StubOptions & { dataSourceId?: string | null } = {},
) {
	const stubs = stubAPIs(options)
	const legend = useLegendEditorExtension(
		() => "up",
		() => options.dataSourceId ?? null,
		() => TimeRangePreset.Last1Hour,
	)

	return { ...stubs, ...legend }
}

// merges the labels of every completion source active at pos, the same
// way codemirror's autocompletion queries all of them
async function completionLabels(
	stateCtor: typeof EditorState,
	contextCtor: typeof CompletionContext,
	extensions: Extension,
	doc: string,
	pos: number,
) {
	const state = stateCtor.create({ doc, extensions })
	const sources = state.languageDataAt<CompletionSource>("autocomplete", pos)
	const ctx = new contextCtor(state, pos, false)
	const labels: string[] = []

	for (const source of sources) {
		const res = await source(ctx)

		if (res) {
			labels.push(...res.options.map((o) => o.label))
		}
	}

	return labels
}

describe("singleLineExtension", () => {
	it("keeps single-line edits untouched", ({ expect }) => {
		const state = EditorState.create({
			doc: "hello",
			extensions: singleLineExtension(),
		})

		const tr = state.update({ changes: { from: 5, insert: " world" } })

		expect(tr.state.doc.toString()).toBe("hello world")
	})

	it("joins the lines of a multi-line insert that grows the document", ({
		expect,
	}) => {
		const state = EditorState.create({ extensions: singleLineExtension() })

		const tr = state.update({ changes: { from: 0, insert: "a\nb\nc" } })

		expect(tr.state.doc.toString()).toBe("a b c")
	})

	it("joins the lines of a multi-line insert that shrinks the document", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: "0123456789",
			extensions: singleLineExtension(),
		})

		const tr = state.update({ changes: { from: 0, to: 10, insert: "a\nb" } })

		expect(tr.state.doc.toString()).toBe("a b")
	})

	it("joins the lines of a multi-line insert into existing content", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: "up",
			extensions: singleLineExtension(),
		})

		const tr = state.update({ changes: { from: 2, insert: "\nrate(x[5m])" } })

		expect(tr.state.doc.toString()).toBe("up rate(x[5m])")
	})
})

describe("trimEditorWhitespace", () => {
	function makeView(doc: string) {
		const dispatch = vi.fn()
		const view = {
			state: EditorState.create({ doc }),
			dispatch,
		} as unknown as EditorView

		return { view, dispatch }
	}

	it("replaces the document with its trimmed content", ({ expect }) => {
		const { view, dispatch } = makeView("  select 1  ")

		trimEditorWhitespace(view)

		expect(dispatch).toHaveBeenCalledExactlyOnceWith({
			changes: { from: 0, to: 12, insert: "select 1" },
		})
	})

	it("dispatches nothing when there is no surrounding whitespace", ({
		expect,
	}) => {
		const { view, dispatch } = makeView("select 1")

		trimEditorWhitespace(view)

		expect(dispatch).not.toHaveBeenCalled()
	})
})

describe("useQueryEditorExtension", () => {
	it("wires promql completions for a prometheus data source", async ({
		expect,
	}) => {
		const { extensions } = makeEditor({ dataSourceId: "prom-1" })

		expect(extensions.value).toBeDefined()

		const labels = await completionLabels(
			CjsEditorState,
			CjsCompletionContext,
			extensions.value as Extension,
			"rate(up[",
			8,
		)

		expect(labels).toContain("$__interval")
	})

	it("wires sql completions for a postgresql data source", async ({
		expect,
	}) => {
		const { extensions } = makeEditor({ dataSourceId: "pg-1" })

		expect(extensions.value).toBeDefined()

		const labels = await completionLabels(
			EditorState,
			CompletionContext,
			extensions.value as Extension,
			"$",
			1,
		)

		expect(labels).toContain("$__timeFilter(dateColumn)")
	})

	it.for([
		{ name: "provides no extensions without a data source", id: null },
		{ name: "provides no extensions for an unknown data source", id: "gone" },
	])("$name", ({ id }, { expect }) => {
		const { extensions } = makeEditor({ dataSourceId: id })

		expect(extensions.value).toBeUndefined()
	})

	it.for([
		{
			name: "announces read-only mode while editing is disabled",
			dataSourceId: "prom-1",
			editingEnabled: false,
			expected: "editor.metrics.config.query-placeholder.empty",
		},
		{
			name: "prompts with the prometheus placeholder",
			dataSourceId: "prom-1",
			editingEnabled: true,
			expected: "editor.metrics.config.query-placeholder.prometheus",
		},
		{
			name: "prompts with the postgresql placeholder",
			dataSourceId: "pg-1",
			editingEnabled: true,
			expected: "editor.metrics.config.query-placeholder.postgresql",
		},
		{
			name: "prompts with the default placeholder without a data source",
			dataSourceId: null,
			editingEnabled: true,
			expected: "editor.metrics.config.query-placeholder.default",
		},
	])("$name", ({ dataSourceId, editingEnabled, expected }, { expect }) => {
		const { placeholder } = makeEditor({ dataSourceId, editingEnabled })

		expect(placeholder.value).toBe(expected)
	})
})

describe("useLegendEditorExtension", () => {
	it("routes label names through the prometheus fetcher", async ({
		expect,
	}) => {
		const { prom, sql, fetchAllLabelNames } = makeLegendEditor({
			dataSourceId: "prom-1",
			prometheusData: { labels: { result: ["job", "__name__"] } },
		})

		await expect(fetchAllLabelNames()).resolves.toEqual(["job"])
		expect(prom.api.labels.refresh).toHaveBeenCalledTimes(1)
		expect(sql.api.labels.refresh).toHaveBeenCalledTimes(0)
	})

	it("routes example values through the prometheus fetcher", async ({
		expect,
	}) => {
		const { prom, sql, fetchExampleLabelValues } = makeLegendEditor({
			dataSourceId: "prom-1",
			prometheusData: { series: { result: [{ job: "api" }] } },
		})

		await expect(fetchExampleLabelValues()).resolves.toEqual({ job: "api" })
		expect(prom.api.series.refresh).toHaveBeenCalledTimes(1)
		expect(sql.api.labels.refresh).toHaveBeenCalledTimes(0)
	})

	it("routes label names through the sql fetcher", async ({ expect }) => {
		const { prom, sql, fetchAllLabelNames } = makeLegendEditor({
			dataSourceId: "pg-1",
			sqlData: { labels: { labels: { hostname: "web-1" } } },
		})

		await expect(fetchAllLabelNames()).resolves.toEqual(["hostname"])
		expect(sql.api.labels.refresh).toHaveBeenCalledTimes(1)
		expect(prom.api.labels.refresh).toHaveBeenCalledTimes(0)
	})

	it("routes example values through the sql fetcher", async ({ expect }) => {
		const { prom, sql, fetchExampleLabelValues } = makeLegendEditor({
			dataSourceId: "pg-1",
			sqlData: { labels: { labels: { hostname: "web-1" } } },
		})

		await expect(fetchExampleLabelValues()).resolves.toEqual({
			hostname: "web-1",
		})
		expect(sql.api.labels.refresh).toHaveBeenCalledTimes(1)
		expect(prom.api.series.refresh).toHaveBeenCalledTimes(0)
	})

	it("returns no label names without a data source", async ({ expect }) => {
		const { prom, sql, fetchAllLabelNames } = makeLegendEditor()

		await expect(fetchAllLabelNames()).resolves.toEqual([])
		expect(prom.api.labels.refresh).toHaveBeenCalledTimes(0)
		expect(sql.api.labels.refresh).toHaveBeenCalledTimes(0)
	})

	it("returns no example values without a data source", async ({ expect }) => {
		const { prom, sql, fetchExampleLabelValues } = makeLegendEditor()

		await expect(fetchExampleLabelValues()).resolves.toEqual({})
		expect(prom.api.series.refresh).toHaveBeenCalledTimes(0)
		expect(sql.api.labels.refresh).toHaveBeenCalledTimes(0)
	})
})

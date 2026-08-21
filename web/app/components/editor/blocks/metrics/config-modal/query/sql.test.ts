import { describe, it, vi } from "vitest"
import { toValue } from "vue"
import { EditorState, type Extension } from "@codemirror/state"
import {
	CompletionContext,
	type Completion,
	type CompletionSource,
} from "@codemirror/autocomplete"
import { TimeRangePreset } from "../../utils"
import {
	installAutoImportGlobals,
	stubSQLAPI,
	type SQLStubData,
} from "./test-helpers"
import { useSQLLegendExtension, useSQLQueryExtension } from "./sql"

installAutoImportGlobals()

const sqlAPIMock = vi.hoisted(() => vi.fn())

vi.mock("~/composables/api/useSQLDataSourceAPI", () => ({
	default: sqlAPIMock,
}))

const t = (key: string) => key

const METADATA: SQLMetadataResult = {
	tables: {
		"public.users": {
			columns: [{ name: "id" }, { name: "email" }],
		},
		"public.orders": {
			columns: [{ name: "id" }, { name: "total" }],
		},
	},
	defaultSchema: "public",
}

function makeComplete(
	dataSourceType: DataSourceType | null,
	data?: SQLStubData,
) {
	const stub = stubSQLAPI(sqlAPIMock, data)
	const { extensions } = useSQLQueryExtension(
		t,
		() => "ds-1",
		() => dataSourceType,
		() => TimeRangePreset.Last1Hour,
		() => true,
	)

	// merges the options of every completion source active at pos, the
	// same way codemirror's autocompletion queries all of them
	async function complete(doc: string, pos: number, explicit = false) {
		const state = EditorState.create({
			doc,
			extensions: extensions.value as Extension,
		})
		const sources = state.languageDataAt<CompletionSource>("autocomplete", pos)
		const ctx = new CompletionContext(state, pos, explicit)
		const options: Completion[] = []

		for (const source of sources) {
			const res = await source(ctx)

			if (res) {
				options.push(...res.options)
			}
		}

		return options
	}

	return { ...stub, complete }
}

function makeLegend(
	options: {
		query?: string | null
		timeRange?: TimeRangePreset | null
		data?: SQLStubData
	} = {},
) {
	const stub = stubSQLAPI(sqlAPIMock, options.data)
	const legend = useSQLLegendExtension(
		() => ("query" in options ? options.query : "SELECT 1"),
		() => "ds-1",
		() =>
			"timeRange" in options ? options.timeRange : TimeRangePreset.Last1Hour,
		() => true,
	)

	return { ...stub, ...legend }
}

describe("useSQLQueryExtension", () => {
	it.for([
		{
			dialect: DataSourceType.PostgreSQL,
			timeMacroInfo: 'EXTRACT(EPOCH FROM dateColumn) AS "time"',
		},
		{
			dialect: DataSourceType.MySQL,
			timeMacroInfo: "UNIX_TIMESTAMP(dateColumn) AS `time`",
		},
		{
			dialect: DataSourceType.MariaDB,
			timeMacroInfo: "UNIX_TIMESTAMP(dateColumn) AS `time`",
		},
	])(
		"offers $dialect macros when typing a dollar",
		async ({ dialect, timeMacroInfo }, { expect }) => {
			const { complete } = makeComplete(dialect)

			const options = await complete("$", 1)
			const labels = options.map((o) => o.label)

			expect(labels).toContain("$__from")
			expect(labels).toContain("$__to")
			expect(labels).toContain("$__interval")
			expect(labels).toContain("$__timeFilter(dateColumn)")
			expect(labels).toContain(
				"$__unixEpochGroupAlias(dateColumn,'5m'[, fill])",
			)

			const timeMacro = options.find((o) => o.label === "$__time(dateColumn)")

			expect(timeMacro?.info).toBe(timeMacroInfo)
			expect(timeMacro?.detail).toBe("macro")
		},
	)

	it("translates the variable descriptions", async ({ expect }) => {
		const { complete } = makeComplete(DataSourceType.PostgreSQL)

		const options = await complete("$", 1)
		const byLabel = new Map(options.map((o) => [o.label, o]))

		expect(byLabel.get("$__from")?.info).toBe(
			"editor.metrics.config.query-info.sql.from",
		)
		expect(byLabel.get("$__to")?.info).toBe(
			"editor.metrics.config.query-info.sql.to",
		)
		expect(byLabel.get("$__interval")?.info).toBe(
			"editor.metrics.config.query-info.sql.interval",
		)
	})

	it("offers no dollar macros for an unknown data source type", async ({
		expect,
	}) => {
		const { complete } = makeComplete(null)

		const options = await complete("$", 1)

		expect(options.filter((o) => o.label.startsWith("$__"))).toEqual([])
	})

	it("excludes write keywords from keyword completions", async ({ expect }) => {
		const { complete } = makeComplete(DataSourceType.PostgreSQL)

		const options = await complete("SEL", 3)
		const keywords = options
			.filter((o) => o.type === "keyword")
			.map((o) => o.label)

		expect(keywords).toContain("SELECT")
		expect(keywords).toContain("WHERE")
		expect(keywords).not.toContain("INSERT")
		expect(keywords).not.toContain("UPDATE")
		expect(keywords).not.toContain("DELETE")
		expect(keywords).not.toContain("DROP")
		expect(keywords).not.toContain("TRUNCATE")
	})

	it("completes table names from the fetched metadata", async ({ expect }) => {
		const { complete } = makeComplete(DataSourceType.PostgreSQL, {
			metadata: METADATA,
		})

		const options = await complete("SELECT * FROM ", 14, true)
		const labels = options.map((o) => o.label)

		expect(labels).toContain("users")
		expect(labels).toContain("orders")
	})

	it("completes column names for a qualified table", async ({ expect }) => {
		const { complete } = makeComplete(DataSourceType.PostgreSQL, {
			metadata: METADATA,
		})

		const options = await complete("SELECT users.", 13, true)
		const labels = options.map((o) => o.label)

		expect(labels).toContain("id")
		expect(labels).toContain("email")
		expect(labels).not.toContain("total")
	})

	it("offers no schema completions before metadata arrives", async ({
		expect,
	}) => {
		const { complete } = makeComplete(DataSourceType.PostgreSQL)

		const options = await complete("SELECT * FROM ", 14, true)

		expect(options.map((o) => o.label)).not.toContain("users")
	})
})

describe("useSQLLegendExtension", () => {
	it("requests labels for the debounced query and time range", ({ expect }) => {
		const { captured } = makeLegend({ query: "SELECT hostname FROM hosts" })

		const params = toValue(captured.labelsParams)

		expect(params?.q).toBe("SELECT hostname FROM hosts")
		expect(params?.timeRangeKey).toBe(TimeRangePreset.Last1Hour)
		expect(params?.from).toBeInstanceOf(Date)
		expect(params?.to).toBeInstanceOf(Date)
	})

	it.for([
		{ name: "requests nothing while the query is empty", query: "" },
		{
			name: "requests nothing without a time range",
			query: "SELECT 1",
			timeRange: null,
		},
	])("$name", ({ query, timeRange }, { expect }) => {
		const { captured } = makeLegend({ query, timeRange })

		expect(toValue(captured.labelsParams)).toBeNull()
	})

	describe("fetchAllLabelNames", () => {
		it("returns the label names of the query result", async ({ expect }) => {
			const { api, fetchAllLabelNames } = makeLegend({
				data: { labels: { labels: { hostname: "web-1", region: "eu" } } },
			})

			await expect(fetchAllLabelNames()).resolves.toEqual([
				"hostname",
				"region",
			])
			expect(api.labels.refresh).toHaveBeenCalledTimes(1)
		})

		it("returns no names when the labels query has no data", async ({
			expect,
		}) => {
			const { fetchAllLabelNames } = makeLegend({ data: { labels: null } })

			await expect(fetchAllLabelNames()).resolves.toEqual([])
		})
	})

	describe("fetchExampleLabelValues", () => {
		it("returns the labels of the query result", async ({ expect }) => {
			const { api, fetchExampleLabelValues } = makeLegend({
				data: { labels: { labels: { hostname: "web-1", region: "eu" } } },
			})

			await expect(fetchExampleLabelValues()).resolves.toEqual({
				hostname: "web-1",
				region: "eu",
			})
			expect(api.labels.refresh).toHaveBeenCalledTimes(1)
		})

		it("returns no values when the labels query has no data", async ({
			expect,
		}) => {
			const { fetchExampleLabelValues } = makeLegend({ data: { labels: null } })

			await expect(fetchExampleLabelValues()).resolves.toEqual({})
		})
	})
})

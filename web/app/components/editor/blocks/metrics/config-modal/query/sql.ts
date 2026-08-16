import {
	SQLDialect,
	PostgreSQL,
	StandardSQL,
	MySQL,
	MariaSQL,
	schemaCompletionSource,
	keywordCompletionSource,
} from "@codemirror/lang-sql"
import { EditorView } from "@codemirror/view"
import { resolveTimeRange, type TimeRangePreset } from "../../utils"
import type {
	Completion,
	CompletionContext,
	CompletionSource,
} from "@codemirror/autocomplete"
import type { Extension } from "@codemirror/state"
import { snippetCompletion } from "@codemirror/autocomplete"
import { LanguageSupport } from "@codemirror/language"

const QUERY_PROCESSING_DEBOUNCE_MS = 700

function postgreSQLCompletions(t: (key: string) => string): Completion[] {
	// NOTE: i18n here is static
	// TODO: add support for ${...} style variables, e.g. ${__from}
	return [
		{
			label: "$__from",
			type: "variable",
			info: t("editor.metrics.config.query-info.sql.from"),
		},
		{
			label: "$__to",
			type: "variable",
			info: t("editor.metrics.config.query-info.sql.to"),
		},
		{
			label: "$__interval",
			type: "variable",
			info: t("editor.metrics.config.query-info.sql.interval"),
		},
		// macros
		snippetCompletion("$__time(${1:dateColumn})", {
			label: "$__time(dateColumn)",
			type: "function",
			detail: "macro",
			info: 'EXTRACT(EPOCH FROM dateColumn) AS "time"',
		}),
		snippetCompletion("$__timeEpoch(${1:dateColumn})", {
			label: "$__timeEpoch(dateColumn)",
			type: "function",
			detail: "macro",
			info: 'EXTRACT(EPOCH FROM dateColumn) AS "time"',
		}),
		snippetCompletion("$__timeFilter(${1:dateColumn})", {
			label: "$__timeFilter(dateColumn)",
			type: "function",
			detail: "macro",
			info: "dateColumn BETWEEN 'from'::TIMESTAMPTZ AND 'to'::TIMESTAMPTZ",
		}),
		{
			label: "$__timeFrom",
			type: "function",
			detail: "macro",
			info: "'from'::TIMESTAMPTZ",
			apply: "$__timeFrom()",
		},
		{
			label: "$__timeTo",
			type: "function",
			detail: "macro",
			info: "'to'::TIMESTAMPTZ",
			apply: "$__timeTo()",
		},
		snippetCompletion("$__timeGroup(${1:dateColumn}, '${2:5m}')", {
			label: "$__timeGroup(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: "FLOOR(EXTRACT(EPOCH FROM dateColumn)/300)*300",
		}),
		snippetCompletion("$__timeGroupAlias(${1:dateColumn}, '${2:5m}')", {
			label: "$__timeGroupAlias(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: `FLOOR(EXTRACT(EPOCH FROM dateColumn)/300)*300 AS "time"`,
		}),
		snippetCompletion("$__unixEpochFilter(${1:dateColumn})", {
			label: "$__unixEpochFilter(dateColumn)",
			type: "function",
			detail: "macro",
			info: "dateColumn >= unixFrom AND dateColumn <= unixTo",
		}),
		{
			label: "$__unixEpochFrom",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-from"),
			apply: "$__unixEpochFrom()",
		},
		{
			label: "$__unixEpochTo",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-to"),
			apply: "$__unixEpochTo()",
		},
		snippetCompletion("$__unixEpochNanoFilter(${1:dateColumn})", {
			label: "$__unixEpochNanoFilter(dateColumn)",
			type: "function",
			detail: "macro",
			info: "dateColumn >= nanoFrom AND dateColumn <= nanoTo",
		}),
		{
			label: "$__unixEpochNanoFrom",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-nano-from"),
			apply: "$__unixEpochNanoFrom()",
		},
		{
			label: "$__unixEpochNanoTo",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-nano-to"),
			apply: "$__unixEpochNanoTo()",
		},
		snippetCompletion("$__unixEpochGroup(${1:dateColumn}, '${2:5m}')", {
			label: "$__unixEpochGroup(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: "FLOOR(dateColumn/300)*300",
		}),
		snippetCompletion("$__unixEpochGroupAlias(${1:dateColumn}, '${2:5m}')", {
			label: "$__unixEpochGroupAlias(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: `FLOOR(dateColumn/300)*300 AS "time"`,
		}),
	].map((c, i) => ({
		...c,
		boost: 99 - i, // preserves array order
	}))
}

// these can be used for mariadb as well since the macros are compatible with
// MySQL.
function mySQLCompletions(t: (key: string) => string): Completion[] {
	// NOTE: i18n here is static
	// TODO: add support for ${...} style variables, e.g. ${__from}
	return [
		{
			label: "$__from",
			type: "variable",
			info: t("editor.metrics.config.query-info.sql.from"),
		},
		{
			label: "$__to",
			type: "variable",
			info: t("editor.metrics.config.query-info.sql.to"),
		},
		{
			label: "$__interval",
			type: "variable",
			info: t("editor.metrics.config.query-info.sql.interval"),
		},
		// macros
		snippetCompletion("$__time(${1:dateColumn})", {
			label: "$__time(dateColumn)",
			type: "function",
			detail: "macro",
			info: "UNIX_TIMESTAMP(dateColumn) AS `time`",
		}),
		snippetCompletion("$__timeEpoch(${1:dateColumn})", {
			label: "$__timeEpoch(dateColumn)",
			type: "function",
			detail: "macro",
			info: "UNIX_TIMESTAMP(dateColumn) AS `time`",
		}),
		snippetCompletion("$__timeFilter(${1:dateColumn})", {
			label: "$__timeFilter(dateColumn)",
			type: "function",
			detail: "macro",
			info: "dateColumn BETWEEN 'from' AND 'to'",
		}),
		{
			label: "$__timeFrom",
			type: "function",
			detail: "macro",
			info: "'from'",
			apply: "$__timeFrom()",
		},
		{
			label: "$__timeTo",
			type: "function",
			detail: "macro",
			info: "'to'",
			apply: "$__timeTo()",
		},
		snippetCompletion("$__timeGroup(${1:dateColumn}, '${2:5m}')", {
			label: "$__timeGroup(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: "FLOOR(UNIX_TIMESTAMP(dateColumn)/300)*300",
		}),
		snippetCompletion("$__timeGroupAlias(${1:dateColumn}, '${2:5m}')", {
			label: "$__timeGroupAlias(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: "FLOOR(UNIX_TIMESTAMP(dateColumn)/300)*300 AS `time`",
		}),
		snippetCompletion("$__unixEpochFilter(${1:dateColumn})", {
			label: "$__unixEpochFilter(dateColumn)",
			type: "function",
			detail: "macro",
			info: "dateColumn >= unixFrom AND dateColumn <= unixTo",
		}),
		{
			label: "$__unixEpochFrom",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-from"),
			apply: "$__unixEpochFrom()",
		},
		{
			label: "$__unixEpochTo",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-to"),
			apply: "$__unixEpochTo()",
		},
		snippetCompletion("$__unixEpochNanoFilter(${1:dateColumn})", {
			label: "$__unixEpochNanoFilter(dateColumn)",
			type: "function",
			detail: "macro",
			info: "dateColumn >= nanoFrom AND dateColumn <= nanoTo",
		}),
		{
			label: "$__unixEpochNanoFrom",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-nano-from"),
			apply: "$__unixEpochNanoFrom()",
		},
		{
			label: "$__unixEpochNanoTo",
			type: "function",
			detail: "macro",
			info: t("editor.metrics.config.query-info.sql.unix-epoch-nano-to"),
			apply: "$__unixEpochNanoTo()",
		},
		snippetCompletion("$__unixEpochGroup(${1:dateColumn}, '${2:5m}')", {
			label: "$__unixEpochGroup(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: "FLOOR(dateColumn/300)*300",
		}),
		snippetCompletion("$__unixEpochGroupAlias(${1:dateColumn}, '${2:5m}')", {
			label: "$__unixEpochGroupAlias(dateColumn,'5m'[, fill])",
			type: "function",
			detail: "macro",
			info: "FLOOR(dateColumn/300)*300 AS `time`",
		}),
	].map((c, i) => ({
		...c,
		boost: 99 - i, // preserves array order
	}))
}

function completeDollar(context: CompletionContext, completions: Completion[]) {
	// this supports ${...} and $... style completions
	const match =
		context.matchBefore(/\$[_A-Za-z][\w]*$/) ??
		context.matchBefore(/\$$/) ??
		context.matchBefore(/\$\{[_A-Za-z][\w:]*$/) ??
		context.matchBefore(/\$\{$/)

	if (!match && !context.explicit) {
		return null
	}

	return {
		from: match ? match.from : context.pos,
		options: completions,
		validFor: /^(\$[\w]*|\$\{[\w:]*)$/,
	}
}

function extendDialect(base: SQLDialect) {
	const current = base.spec.specialVar ?? "?"
	const specialVar = current.includes("$") ? current : `${current}$`

	return SQLDialect.define({
		...base.spec,
		specialVar,
		// we need to disable this to prevent the parser from choking on $ in
		// queries, e.g. for PostgreSQL functions like $__timeFilter() would
		// be split into $__ and timeFilter(), which would produce weird
		// syntax highlighting
		doubleDollarQuotedStrings: false,
	})
}

// keywords to exclude from autocomplete (DML/DDL write operations)
const WRITE_KEYWORDS = new Set([
	"INSERT",
	"UPDATE",
	"DELETE",
	"DROP",
	"CREATE",
	"ALTER",
	"TRUNCATE",
	"REPLACE",
	"MERGE",
	"GRANT",
	"REVOKE",
	"COMMIT",
	"ROLLBACK",
	"SAVEPOINT",
	"BEGIN",
	"LOCK",
	"UNLOCK",
	"RENAME",
	"CALL",
	"EXECUTE",
	"SET",
])

function withoutWriteKeywords(source: CompletionSource): CompletionSource {
	return (context: CompletionContext) => {
		const result = source(context)
		if (!result || result instanceof Promise) {
			return result
		}

		return {
			...result,
			options: result.options.filter(
				(c) => c.type !== "keyword" || !WRITE_KEYWORDS.has(c.label),
			),
		}
	}
}

export function useSQLQueryExtension(
	t: (key: string) => string,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	dataSourceType: MaybeRefOrGetter<DataSourceType | null | undefined>,
	_timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
	enable: MaybeRefOrGetter<boolean>,
) {
	const { useSQLMetadata } = useSQLDataSourceAPI()
	const fetchSQLMetadata = useSQLMetadata(dataSourceId, enable)

	// refresh metadata when the editor gains focus (staleTime in the query
	// prevents redundant requests)
	const refreshOnFocus = EditorView.updateListener.of((update) => {
		if (update.focusChanged && update.view.hasFocus) {
			void fetchSQLMetadata.refresh()
		}
	})

	// tracks the first table from FROM/JOIN in the editor so we can pass it
	// as defaultTable for unqualified column completions
	const currentTable = ref<string | undefined>()
	const trackTable = EditorView.updateListener.of((update) => {
		if (!update.docChanged) {
			return
		}

		const doc = update.state.doc.toString()
		const match = /\b(?:from|join)\s+([\w.]+)/i.exec(doc)

		currentTable.value = match?.[1] ?? undefined
	})

	return {
		extensions: computed<Extension[]>(() => {
			let dialect = StandardSQL
			let autocompletions: (
				t: (key: string) => string,
			) => Completion[] = () => []

			switch (toValue(dataSourceType)) {
				case DataSourceType.PostgreSQL:
					dialect = PostgreSQL
					autocompletions = (localT) => postgreSQLCompletions(localT)

					break
				case DataSourceType.MySQL:
					dialect = MySQL
					autocompletions = (localT) => mySQLCompletions(localT)

					break
				case DataSourceType.MariaDB:
					dialect = MariaSQL
					autocompletions = (localT) => mySQLCompletions(localT)

					break
			}

			dialect = extendDialect(dialect)

			const metadata = fetchSQLMetadata.data.value
			const schema: Record<string, string[]> = {}
			if (metadata) {
				for (const [table, { columns }] of Object.entries(metadata.tables)) {
					schema[table] = columns.map((c) => c.name)
				}
			}

			return [
				new LanguageSupport(dialect.language, [
					dialect.language.data.of({
						autocomplete: schemaCompletionSource({
							dialect,
							schema,
							defaultSchema: metadata?.defaultSchema,
							defaultTable: currentTable.value,
						}),
					}),
					dialect.language.data.of({
						autocomplete: withoutWriteKeywords(
							keywordCompletionSource(dialect, true),
						),
					}),
					dialect.language.data.of({
						autocomplete: (context: CompletionContext) =>
							completeDollar(context, autocompletions(t)),
					}),
				]),
				refreshOnFocus,
				trackTable,
			]
		}),
	}
}

export function useSQLLegendExtension(
	query: MaybeRefOrGetter<string | null | undefined>,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
	enable: MaybeRefOrGetter<boolean>,
) {
	const debouncedQuery = refDebounced(
		toRef(query),
		QUERY_PROCESSING_DEBOUNCE_MS,
	)

	const { useSQLLabels } = useSQLDataSourceAPI()
	const fetchSQLLabels = useSQLLabels(
		dataSourceId,
		() => {
			const timeRangeVal = toValue(timeRange)
			if (!timeRangeVal || !debouncedQuery.value) {
				return null
			}

			const resolvedTimeRange = resolveTimeRange(timeRangeVal)

			return {
				from: resolvedTimeRange.from,
				to: resolvedTimeRange.to,
				timeRangeKey: timeRangeVal,
				q: debouncedQuery.value,
			}
		},
		enable,
	)

	async function fetchAllLabelNames(): Promise<string[]> {
		const res = await fetchSQLLabels.refresh()

		return Object.keys(res.data?.labels ?? {})
	}

	async function fetchExampleLabelValues(): Promise<Record<string, string>> {
		const res = await fetchSQLLabels.refresh()

		return res.data?.labels ?? {}
	}

	return {
		fetchAllLabelNames,
		fetchExampleLabelValues,
	}
}

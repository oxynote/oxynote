import { describe, it, vi } from "vitest"
import {
	EditorState,
	type Extension,
	type TransactionSpec,
} from "@codemirror/state"
import type { EditorView } from "@codemirror/view"
import {
	CompletionContext,
	type Completion,
	type CompletionSource,
} from "@codemirror/autocomplete"
import { syntaxTree } from "@codemirror/language"
import { highlightTree, tagHighlighter, tags } from "@lezer/highlight"
import {
	filterFunctions,
	mathFunctions,
	pipeNames,
	runningStatsFunctions,
	statsFunctions,
} from "@oxynote/lezer-logsql"
import queryLanguagesLocale from "../../../../../../i18n/locales/en/query-languages.json"
import { TimeRangePreset } from "~/utils/time-range"
import {
	installAutoImportGlobals,
	interpolatingT,
	stubLogAPI,
	variableTree,
	type LogStubData,
} from "./test-helpers"
import { useLogsQLQueryExtension } from "./logsql"

installAutoImportGlobals()

const logAPIMock = vi.hoisted(() => vi.fn())

vi.mock("~/composables/api/useLogDataSourceAPI", () => ({
	default: logAPIMock,
}))

const DATA: LogStubData = {
	fields: {
		fields: [
			{ name: "app", kind: LogFieldKind.Label },
			{ name: "level", kind: LogFieldKind.Field },
			{ name: "odd name", kind: LogFieldKind.Field },
			{ name: "by", kind: LogFieldKind.Field },
			{ name: "trace-id", kind: LogFieldKind.Field },
		],
	},
	fieldValues: {
		values: [
			{ value: "api", count: 3 },
			{ value: 'a.b"c', count: 1 },
		],
	},
}

const VARIABLES = ["$env", "$window"]
const INFO_KEY = "query-languages.victorialogs"

describe("useLogsQLQueryExtension", () => {
	describe("in a stream filter", () => {
		it("offers only the stream fields at a matcher's start", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete("{¦}")

			expect(labels).toEqual(["app", ...VARIABLES])
			expect(captured.fieldsParams?.value?.q).toBeUndefined()
		})

		it("offers the values of a field, narrowed by the other matchers", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete('{env="prod", app="¦", x=~"y"}')

			expect(labels).toEqual(["api", 'a.b"c', ...VARIABLES])
			expect(captured.fieldValuesParams?.value).toMatchObject({
				field: "app",
				q: '{env="prod", x=~"y"}',
			})
		})

		it("does not narrow the values across an or", async ({ expect }) => {
			const { complete, captured, api } = makeComplete()

			const { labels } = await complete('{env="prod" or app="¦"}')

			expect(labels).toContain("api")
			expect(api.fieldValues.refresh).toHaveBeenCalledTimes(1)
			expect(api.fields.refresh).not.toHaveBeenCalled()
			expect(captured.fieldValuesParams?.value?.field).toBe("app")
			expect(captured.fieldValuesParams?.value?.q).toBeUndefined()
		})

		it("reads the values of a quoted field by its unquoted name", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			await complete('{"a\\"b"="¦"}')

			expect(captured.fieldValuesParams?.value?.field).toBe('a"b')
		})

		it.for([
			{ name: "an empty list", doc: "{app in (¦)}" },
			{ name: "a list after a value", doc: "{app in (api, ¦)}" },
			{ name: "a list still open", doc: "{app in (api, ¦" },
			{ name: "a not_in list", doc: "{app not_in (¦)}" },
			{ name: "a quoted list item", doc: '{app in ("api", "¦")}' },
		])("offers the values in $name", async ({ doc }, { expect }) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete(doc)

			expect(labels).toContain("api")
			expect(captured.fieldValuesParams?.value?.field).toBe("app")
		})

		it("narrows the values of a list by the other matchers", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			await complete('{env="prod", app in (¦)}')

			expect(captured.fieldValuesParams?.value?.q).toBe('{env="prod"}')
		})

		it.for([
			{ name: "a new alternative", doc: '{app=~"api|¦"}', expected: 11 },
			{ name: "a typed alternative", doc: '{app=~"api|ga¦"}', expected: 11 },
			{ name: "a field regex", doc: 'level:~"err|¦"', expected: 12 },
			{ name: "an escaped pipe", doc: '{app=~"a\\\\|b¦"}', expected: 7 },
			{ name: "a plain string", doc: '{app="a|¦"}', expected: 6 },
			{
				name: "a pipe after an escaped backslash",
				doc: '{app=~"a\\\\\\\\|¦"}',
				expected: 13,
			},
			{ name: "a backtick escaped pipe", doc: "{app=~`a\\|b¦`}", expected: 7 },
			{
				name: "a backtick pipe after an escaped backslash",
				doc: "{app=~`a\\\\|¦`}",
				expected: 11,
			},
		])(
			"completes from the right place in $name",
			async ({ doc, expected }, { expect }) => {
				const { complete } = makeComplete()

				const { from } = await complete(doc)

				expect(from).toBe(expected)
			},
		)

		it.for([
			{
				name: "escapes a value for a double-quoted string",
				doc: '{app="¦"}',
				expected: 'a.b\\"c',
			},
			{
				name: "keeps a double quote in a single-quoted string",
				doc: "{app='¦'}",
				expected: 'a.b"c',
			},
			{
				name: "keeps a value as it is in backticks",
				doc: "{app=`¦`}",
				expected: 'a.b"c',
			},
			{
				name: "escapes a value for a regex matcher",
				doc: '{app=~"¦"}',
				expected: 'a\\\\.b\\"c',
			},
			{
				name: "quotes a value typed straight after the operator",
				doc: "{app=¦}",
				expected: '"a.b\\"c"',
			},
		])("$name", async ({ doc, expected }, { expect }) => {
			const { complete } = makeComplete()

			const { options } = await complete(doc)

			expect(options.find((o) => o.label === 'a.b"c')?.apply).toBe(expected)
		})

		it.for([
			{
				name: "the rest of a value",
				doc: '{app="ap¦ple"}',
				expected: '{app="api"}',
			},
			{
				name: "the rest of a value in a field filter",
				doc: 'level:="ap¦ple" error',
				expected: 'level:="api" error',
			},
			{
				name: "only one alternative of a regex",
				doc: '{app=~"web|ap¦ple|db"}',
				expected: '{app=~"web|api|db"}',
			},
			{
				name: "an alternative that holds an escaped pipe",
				doc: "{app=~`web|ap¦p\\|le|db`}",
				expected: "{app=~`web|api|db`}",
			},
			{
				name: "only one alternative inside a regex group",
				doc: '{app=~"web|(foo|ap¦ple)|db"}',
				expected: '{app=~"web|(foo|api)|db"}',
			},
			{
				name: "only the alternative after a regex flag group",
				doc: '{app=~"(?i)ap¦ple"}',
				expected: '{app=~"(?i)api"}',
			},
		])(
			"replaces $name when a value is accepted",
			async ({ doc, expected }, { expect }) => {
				const { complete } = makeComplete()

				const { options, state, from } = await complete(doc)
				const option = options.find((o) => o.label === "api")

				expect(accept(state, option, from)).toBe(expected)
			},
		)

		it("keeps the text after a string that is still open", async ({
			expect,
		}) => {
			const { complete } = makeComplete()

			const { options } = await complete('{app="ap¦ple}')

			expect(options.find((o) => o.label === "api")?.apply).toBe("api")
		})
	})

	describe("in a filter", () => {
		it("offers the fields and filter functions only when asked", async ({
			expect,
		}) => {
			const { complete } = makeComplete()

			const typed = await complete("err¦")
			const asked = await complete("¦", true)

			expect(typed.labels).toEqual([])
			expect(asked.labels).toEqual(
				expect.arrayContaining(["seq", "app", "level", "_time", ...VARIABLES]),
			)
		})

		it.for([
			{ name: "a plain field", label: "level", expected: "level:" },
			{ name: "a builtin field", label: "_time", expected: "_time:" },
			{ name: "an odd field name", label: "odd name", expected: '"odd name":' },
			{ name: "a reserved field name", label: "by", expected: '"by":' },
		])(
			"applies $name with its colon",
			async ({ label, expected }, { expect }) => {
				const { complete } = makeComplete()

				const { options } = await complete("¦", true)

				expect(options.find((o) => o.label === label)?.apply).toBe(expected)
			},
		)

		it("offers the values of a field and the filter functions after its colon", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete("level:¦")

			expect(labels).toEqual(expect.arrayContaining(["api", "in", "seq"]))
			expect(captured.fieldValuesParams?.value).toMatchObject({
				field: "level",
				q: undefined,
			})
		})

		it("narrows the values by the stream filter", async ({ expect }) => {
			const { complete, captured } = makeComplete()

			await complete('{app="api"} level:¦')

			expect(captured.fieldValuesParams?.value?.q).toBe('{app="api"}')
		})

		it.for([
			{ name: "an exact match", doc: "level:=¦" },
			{ name: "a list of values", doc: "level:in(a, ¦)" },
			{ name: "a list after an uppercase function", doc: "level:IN(a, ¦)" },
			{ name: "a group", doc: "level:(a or ¦)" },
		])("offers the values in $name", async ({ doc }, { expect }) => {
			const { complete } = makeComplete()

			const { labels } = await complete(doc)

			expect(labels).toContain("api")
		})

		it.for([
			{ name: "an exact match", doc: "level:=¦" },
			{ name: "a regex match", doc: "level:~¦" },
		])(
			"offers no filter functions after $name",
			async ({ doc }, { expect }) => {
				const { complete } = makeComplete()

				const { labels } = await complete(doc)

				expect(labels).not.toContain("seq")
			},
		)

		it.for([
			{ name: "a double-quoted name", doc: '"a\\"b":¦', expected: 'a"b' },
			{
				name: "a name quoted for its space",
				doc: '"odd name":¦',
				expected: "odd name",
			},
			{ name: "a single-quoted name", doc: "'it\\'s':¦", expected: "it's" },
			{ name: "a name in backticks", doc: "`a\\b`:¦", expected: "a\\b" },
			{ name: "a bare name", doc: "trace-id:¦", expected: "trace-id" },
		])(
			"reads the values of $name by the field it names",
			async ({ doc, expected }, { expect }) => {
				const { complete, captured } = makeComplete()

				await complete(doc)

				expect(captured.fieldValuesParams?.value?.field).toBe(expected)
			},
		)

		it("offers no values in a function that takes no field value", async ({
			expect,
		}) => {
			const { complete, api } = makeComplete()

			await complete("level:len_range(¦)")

			expect(api.fieldValues.refresh).not.toHaveBeenCalled()
		})
	})

	describe("in a pipeline", () => {
		it("offers the pipes, the fields and the variables after a pipe", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete('{app="api"} error | ¦')

			expect(labels).toEqual(
				expect.arrayContaining(["stats", "sort", "app", "level", ...VARIABLES]),
			)
			expect(labels).not.toContain("median")
			expect(captured.fieldsParams?.value?.q).toBe('{app="api"} error')
		})

		it.for([
			{
				name: "the stats functions in a stats pipe",
				doc: "* | stats ¦",
				contains: "median",
				missing: "abs",
			},
			{
				name: "the next stats function after a comma",
				doc: "* | stats count() x, ¦",
				contains: "quantile",
				missing: "abs",
			},
			{
				name: "the running functions in a running_stats pipe",
				doc: "* | running_stats ¦",
				contains: "first",
				missing: "median",
			},
			{
				name: "the running functions in a total_stats pipe",
				doc: "* | total_stats ¦",
				contains: "last",
				missing: "median",
			},
			{
				name: "the math functions and fields in a math pipe",
				doc: "* | math a + ¦",
				contains: "abs",
				missing: "median",
			},
			{
				name: "the fields in a fields pipe",
				doc: "* | fields a, ¦",
				contains: "level",
				missing: "abs",
			},
			{
				name: "the fields in a by clause",
				doc: "* | stats by (¦",
				contains: "level",
				missing: "median",
			},
		])("offers $name", async ({ doc, contains, missing }, { expect }) => {
			const { complete } = makeComplete()

			const { labels } = await complete(doc)

			expect(labels).toContain(contains)
			expect(labels).not.toContain(missing)
		})

		it("reads the fields through the pipes before the cursor", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			await complete("error | unpack_json | fields ¦")

			expect(captured.fieldsParams?.value?.q).toBe("error | unpack_json")
		})

		it("reads the fields of a subquery from the subquery", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			await complete("a | join by (x) (b | fields ¦")

			expect(captured.fieldsParams?.value?.q).toBe("b")
		})

		it.for([
			{ name: "after as", doc: "* | stats count() as ¦" },
			{ name: "right after a stats function", doc: "* | stats count() ¦" },
		])(
			"offers no fields for a result name $name",
			async ({ doc }, { expect }) => {
				const { complete, api } = makeComplete()

				await complete(doc)

				expect(api.fields.refresh).not.toHaveBeenCalled()
			},
		)

		it.for([
			{
				name: "quotes a field with a hyphen in a math pipe",
				doc: "* | math ¦",
				expected: '"trace-id"',
			},
			{
				name: "quotes a field with a hyphen in a math function",
				doc: "* | math round(¦",
				expected: '"trace-id"',
			},
			{
				name: "keeps a field with a hyphen bare in a fields pipe",
				doc: "* | fields ¦",
				expected: "trace-id",
			},
		])("$name", async ({ doc, expected }, { expect }) => {
			const { complete } = makeComplete()

			const { options } = await complete(doc)

			expect(options.find((o) => o.label === "trace-id")?.apply).toBe(expected)
		})

		it("inserts a function with its parentheses", async ({ expect }) => {
			const { complete } = makeComplete()

			const { options } = await complete("* | stats ¦")
			const apply = options.find((o) => o.label === "median")?.apply

			expect(typeof apply).toBe("function")
		})
	})

	describe("variables", () => {
		it("offers only the variables once a $ is typed", async ({ expect }) => {
			const { complete, api } = makeComplete()

			const { labels, from } = await complete("error | $¦")

			expect(labels).toEqual(VARIABLES)
			expect(from).toBe(8)
			expect(api.fields.refresh).not.toHaveBeenCalled()
		})

		it("offers no variables where the grammar refuses one", async ({
			expect,
		}) => {
			const { complete } = makeComplete()

			const { labels } = await complete("* | stats $¦")

			expect(labels).toEqual([])
		})

		it("offers the variables in a string", async ({ expect }) => {
			const { complete } = makeComplete()

			const { labels } = await complete('* | extract "$¦"')

			expect(labels).toEqual(VARIABLES)
		})

		it("sends the values of the variables a lookup references", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			await complete('{app="$env"} | ¦')

			expect(captured.fieldsParams?.value?.variableValues).toEqual({
				env: "prod",
			})
		})

		it("describes a variable by its type and the value it resolves to", async ({
			expect,
		}) => {
			const { complete } = makeComplete()

			const { options } = await complete("* | $¦")

			expect(options[0]).toMatchObject({
				label: "$env",
				detail: "general.variable-types.list",
				info: "editor.config-modal-utils.query-info.variable prod",
			})
		})
	})

	it.for([
		{ name: "in a comment", doc: "error # lev¦", explicit: true },
		{ name: "in the middle of a word", doc: "* | sta¦ts", explicit: true },
	])("offers nothing $name", async ({ doc, explicit }, { expect }) => {
		const { complete } = makeComplete()

		const { labels, from } = await complete(doc, explicit)

		expect(labels).toEqual([])
		expect(from).toBeNull()
	})

	it.for([
		{ name: "a pipe", doc: "* | ¦", label: "stats", key: "pipes.stats" },
		{ name: "a pipe alias", doc: "* | ¦", label: "rm", key: "pipes.delete" },
		{
			name: "a stats function",
			doc: "* | stats ¦",
			label: "count_uniq",
			key: "stats.count-uniq",
		},
		{
			name: "a running function",
			doc: "* | running_stats ¦",
			label: "count",
			key: "running-stats.count",
		},
		{
			name: "a total function",
			doc: "* | total_stats ¦",
			label: "count",
			key: "total-stats.count",
		},
		{
			name: "a math function",
			doc: "* | math ¦",
			label: "abs",
			key: "math.abs",
		},
		{ name: "a filter function", doc: "x:¦", label: "seq", key: "filters.seq" },
		{
			name: "a builtin field",
			doc: "* | ¦",
			label: "_time",
			key: "fields.time",
		},
		{ name: "a stream field", doc: "{¦}", label: "app", key: "label" },
		{ name: "a field", doc: "* | ¦", label: "level", key: "field" },
	])("describes $name", async ({ doc, label, key }, { expect }) => {
		const { complete } = makeComplete()

		const { options } = await complete(doc, true)

		expect(options.find((o) => o.label === label)?.info).toBe(
			`${INFO_KEY}.${key}`,
		)
	})

	it.for([
		{ name: "pipes", doc: "* | ¦", names: pipeNames },
		{ name: "filter functions", doc: "x:¦", names: filterFunctions },
		{ name: "stats functions", doc: "* | stats ¦", names: statsFunctions },
		{
			name: "running functions",
			doc: "* | running_stats ¦",
			names: runningStatsFunctions,
		},
		{
			name: "total functions",
			doc: "* | total_stats ¦",
			names: runningStatsFunctions,
		},
		{ name: "math functions", doc: "* | math ¦", names: mathFunctions },
	])(
		"has a description for all the $name",
		async ({ doc, names }, { expect }) => {
			const { complete } = makeComplete()

			const { options } = await complete(doc)
			const described = options.filter(
				(o) => names.includes(o.label) && localeText(o.info) !== undefined,
			)

			expect(described.map((o) => o.label).sort()).toEqual([...names].sort())
		},
	)

	it("highlights the query with the grammar's tags", ({ expect }) => {
		const { extensions } = makeComplete()
		const state = EditorState.create({
			doc: "* | stats count()",
			extensions: extensions.value as Extension,
		})
		const classes: string[] = []
		const highlighter = tagHighlighter([
			{ tag: tags.keyword, class: "keyword" },
			{ tag: tags.function(tags.variableName), class: "function" },
		])

		highlightTree(syntaxTree(state), highlighter, (_from, _to, cls) => {
			classes.push(cls)
		})

		expect(classes).toEqual(["keyword", "function"])
	})

	it("comments a line with a hash", ({ expect }) => {
		const { extensions } = makeComplete()
		const state = EditorState.create({
			doc: "error",
			extensions: extensions.value as Extension,
		})

		expect(state.languageDataAt("commentTokens", 0)).toEqual([{ line: "#" }])
	})
})

// the English text of a description key
function localeText(key: Completion["info"]) {
	let node: unknown = queryLanguagesLocale

	for (const part of String(key).split(".")) {
		node = (node as Record<string, unknown>)[part]
	}

	return typeof node === "string" ? node : undefined
}

// accept picks a completion the way the editor does, and gives the doc
// that results. It gives an empty doc when the completion is plain text.
function accept(
	state: EditorState,
	option: Completion | undefined,
	from: number | null,
) {
	let doc = ""
	const view = {
		state: state,
		dispatch: (spec: TransactionSpec) => {
			doc = state.update(spec).state.doc.toString()
		},
	}

	if (typeof option?.apply === "function" && from !== null) {
		option.apply(
			view as unknown as EditorView,
			option,
			from,
			state.selection.main.head,
		)
	}

	return doc
}

function makeComplete(variables: VariableTreeResponse = variableTree()) {
	const stub = stubLogAPI(logAPIMock, DATA)
	const { extensions } = useLogsQLQueryExtension(
		interpolatingT,
		() => "vl-1",
		() => TimeRangePreset.Last1Hour,
		() => true,
		() => variables,
	)

	// the cursor goes where the doc holds a ¦, which is removed
	async function complete(marked: string, explicit = false) {
		const pos = marked.indexOf("¦")
		const state = EditorState.create({
			doc: marked.replace("¦", ""),
			selection: { anchor: pos },
			extensions: extensions.value as Extension,
		})
		const sources = state.languageDataAt<CompletionSource>("autocomplete", pos)
		const ctx = new CompletionContext(state, pos, explicit)
		const options: Completion[] = []
		let from: number | null = null

		for (const source of sources) {
			const res = await source(ctx)

			if (res) {
				options.push(...res.options)
				from = res.from
			}
		}

		return { options, labels: options.map((o) => o.label), from, state }
	}

	return { ...stub, extensions, complete }
}

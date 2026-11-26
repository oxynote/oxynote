import { describe, it, vi } from "vitest"
import { EditorState, type Extension } from "@codemirror/state"
import type { EditorView } from "@codemirror/view"
import {
	CompletionContext,
	type Completion,
	type CompletionSource,
} from "@codemirror/autocomplete"
import { TimeRangePreset } from "~/utils/time-range"
import {
	installAutoImportGlobals,
	interpolatingT,
	stubLogAPI,
	variableTree,
	type LogStubData,
} from "./test-helpers"
import { useLogQLQueryExtension } from "./logql"

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
		],
	},
	fieldValues: {
		values: [
			{ value: "api", count: 3 },
			{ value: 'a.b"c', count: 1 },
		],
	},
}

const STAGES = ["json", "logfmt", "regexp", "line_format", "drop", "keep"]

function makeComplete(variables: VariableTreeResponse = variableTree()) {
	const stub = stubLogAPI(logAPIMock, DATA)
	const { extensions } = useLogQLQueryExtension(
		interpolatingT,
		() => "loki-1",
		() => TimeRangePreset.Last1Hour,
		() => true,
		() => variables,
	)

	// the cursor goes where the doc holds a ¦, which is removed
	async function complete(marked: string, explicit = false) {
		const pos = marked.indexOf("¦")
		const doc = marked.replace("¦", "")
		const state = EditorState.create({
			doc,
			extensions: extensions.value as Extension,
		})
		const sources = state.languageDataAt<CompletionSource>("autocomplete", pos)
		const ctx = new CompletionContext(state, pos, explicit)
		const options: Completion[] = []
		let from: number | null = null
		let to: number | null = null

		for (const source of sources) {
			const res = await source(ctx)

			if (res) {
				options.push(...res.options)
				from = res.from
				to = res.to ?? null
			}
		}

		return { options, labels: options.map((o) => o.label), from, to }
	}

	return { ...stub, complete }
}

function applyCompletion(
	option: Completion,
	doc: string,
	from: number,
	pos: number,
) {
	const state = EditorState.create({ doc, selection: { anchor: pos } })
	let result = state

	if (typeof option.apply !== "function") {
		throw new Error("expected an apply function")
	}

	option.apply(
		{
			state,
			dispatch: (spec: Parameters<EditorState["update"]>[0]) => {
				result = state.update(spec).state
			},
		} as unknown as EditorView,
		option,
		from,
		pos,
	)

	return result.doc.toString()
}

describe("useLogQLQueryExtension", () => {
	describe("in a selector", () => {
		it("offers the stream labels and the variables at a matcher's start", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete("{¦}")

			expect(labels).toEqual(["app", "level", "$env", "$window"])
			expect(captured.fieldsParams?.value?.q).toBeUndefined()
		})

		it("leaves the variables out while a name is typed", async ({ expect }) => {
			const { complete } = makeComplete()

			const { labels, from } = await complete('{env="x", ap¦}')

			expect(labels).toEqual(["app", "level"])
			expect(from).toBe(10)
		})

		it("offers the values of a label, narrowed by the other matchers", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete('{env="prod", app="¦", x=~"y"}')

			expect(labels).toEqual(["api", 'a.b"c', "$env", "$window"])
			expect(captured.fieldValuesParams?.value).toMatchObject({
				field: "app",
				q: '{env="prod", x=~"y"}',
			})
		})

		it.for([
			{
				name: "escapes a value for its string",
				doc: '{app="¦"}',
				expected: 'a.b\\"c',
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

		it("starts value completions after the last regex alternative", async ({
			expect,
		}) => {
			const { complete } = makeComplete()
			const marked = '{app=~"api|¦"}'

			const { labels, from } = await complete(marked)

			expect(labels).toEqual(["api", 'a.b"c', "$env", "$window"])
			expect(from).toBe(marked.indexOf("|") + 1)
		})

		it("filters on the alternative around the cursor", async ({ expect }) => {
			const { complete } = makeComplete()
			const marked = '{app=~"api|ngi¦nx|web"}'
			const doc = marked.replace("¦", "")
			const pos = marked.indexOf("¦")

			const { from, to } = await complete(marked)
			const first = doc.indexOf("|")

			expect(from).toBe(first + 1)
			expect(to).toBe(pos)
		})

		it("replaces only the alternative when a value is picked", async ({
			expect,
		}) => {
			const { complete } = makeComplete()
			const marked = '{app=~"api|ngi¦nx|web"}'
			const doc = marked.replace("¦", "")
			const pos = marked.indexOf("¦")

			const { options, from } = await complete(marked)
			const option = options.find((o) => o.label === "api")

			expect(option).toBeDefined()
			expect(from).not.toBeNull()

			if (!option || from === null) {
				throw new Error("expected the api completion")
			}

			const applied = applyCompletion(option, doc, from, pos)

			expect(applied).toBe('{app=~"api|api|web"}')
		})

		it("replaces the alternative at the positions current when it applies", async ({
			expect,
		}) => {
			const { complete } = makeComplete()
			const marked = '{app=~"api|ngi¦nx|web"}'
			const doc = marked.replace("¦", "")
			const pos = marked.indexOf("¦")

			const { options, from } = await complete(marked)
			const option = options.find((o) => o.label === "api")

			expect(option).toBeDefined()
			expect(from).not.toBeNull()

			if (!option || from === null) {
				throw new Error("expected the api completion")
			}

			const prefix = "foo"
			const applied = applyCompletion(
				option,
				prefix + doc,
				from + prefix.length,
				pos + prefix.length,
			)

			expect(applied).toBe(`${prefix}{app=~"api|api|web"}`)
		})

		it("replaces the whole regex when an escape changes what a | means", async ({
			expect,
		}) => {
			const { complete } = makeComplete()
			const marked = '{app=~"api\\|ngi¦nx"}'
			const doc = marked.replace("¦", "")
			const pos = marked.indexOf("¦")

			const { options, from, to } = await complete(marked)
			const option = options.find((o) => o.label === "api")

			expect(from).toBe(doc.indexOf('"') + 1)
			expect(to).toBe(pos)
			expect(option).toBeDefined()

			if (!option || from === null) {
				throw new Error("expected the api completion")
			}

			const applied = applyCompletion(option, doc, from, pos)

			expect(applied).toBe('{app=~"api"}')
		})

		it.for([
			{ name: "an escaped pipe", doc: '{app=~"api\\|v1¦"}' },
			{ name: "a group", doc: '{app=~"(api|v1)¦"}' },
			{ name: "a character class", doc: '{app=~"[a|b]¦"}' },
		])("does not split on $name", async ({ doc }, { expect }) => {
			const { complete } = makeComplete()

			const { from } = await complete(doc)

			expect(from).toBe(doc.indexOf('"') + 1)
		})

		it("offers no variables where the grammar needs a string", async ({
			expect,
		}) => {
			const { complete } = makeComplete()

			const { labels } = await complete("{app=$¦}")

			expect(labels).toEqual([])
		})
	})

	describe("in a pipeline", () => {
		it("offers the stages, the fields and the variables after a pipe", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete('{app="api"} | ¦')

			expect(labels).toEqual(expect.arrayContaining(STAGES))
			expect(labels).toEqual(
				expect.arrayContaining(["app", "level", "$env", "$window"]),
			)
			expect(captured.fieldsParams?.value?.q).toBe('{app="api"}')
		})

		it("offers only the variables once a $ is typed", async ({ expect }) => {
			const { complete } = makeComplete()

			const { labels } = await complete('{app="api"} | json | $¦')

			expect(labels).toEqual(["$env", "$window"])
		})

		it("offers the values of a field, read through the stages before it", async ({
			expect,
		}) => {
			const { complete, captured } = makeComplete()

			const { labels } = await complete('{app="api"} | json | level="¦"')

			expect(labels).toEqual(["api", 'a.b"c', "$env", "$window"])
			expect(captured.fieldValuesParams?.value).toMatchObject({
				field: "level",
				q: '{app="api"} | json',
			})
		})

		it("starts value completions after a regex alternative in a label filter", async ({
			expect,
		}) => {
			const { complete } = makeComplete()
			const marked = '{app="api"} | json | level=~"info|¦"'

			const { labels, from } = await complete(marked)

			expect(labels).toEqual(["api", 'a.b"c', "$env", "$window"])
			expect(from).toBe(marked.lastIndexOf("|") + 1)
		})

		it.for([
			{ name: "after drop", doc: '{app="api"} | drop ¦' },
			{ name: "in a list of kept names", doc: '{app="api"} | keep a, ¦' },
			{
				name: "after and in a label filter",
				doc: '{app="api"} | json | level="x" and ¦',
			},
		])("offers the field names $name", async ({ doc }, { expect }) => {
			const { complete } = makeComplete()

			const { labels } = await complete(doc)

			expect(labels).toEqual(expect.arrayContaining(["app", "level"]))
			expect(labels).not.toContain("json")
		})

		it.for([
			{ name: "a line filter", doc: '{app="api"} |= "¦"' },
			{ name: "a line format", doc: '{app="api"} | line_format "¦"' },
		])("offers only the variables in $name", async ({ doc }, { expect }) => {
			const { complete } = makeComplete()

			expect((await complete(doc)).labels).toEqual([])
			expect((await complete(doc, true)).labels).toEqual(["$env", "$window"])
		})

		it("offers the line filters after a selector when asked", async ({
			expect,
		}) => {
			const { complete } = makeComplete()

			expect((await complete('{app="api"} ¦')).labels).toEqual([])
			expect((await complete('{app="api"} ¦', true)).labels).toEqual([
				"|=",
				"!=",
				"|~",
				"!~",
			])
		})
	})

	it.for([
		{ name: "a stage", doc: '{app="api"} | ¦', label: "json", key: "json" },
		{
			name: "a line filter",
			doc: '{app="api"} ¦',
			label: "!~",
			key: "not-matches",
		},
		{ name: "a stream label", doc: "{¦}", label: "app", key: "label" },
		{ name: "a field", doc: "{¦}", label: "level", key: "field" },
	])("describes $name", async ({ doc, label, key }, { expect }) => {
		const { complete } = makeComplete()

		const { options } = await complete(doc, true)

		expect(options.find((o) => o.label === label)?.info).toBe(
			`query-languages.loki.${key}`,
		)
	})

	it("sends the values of the variables a lookup references", async ({
		expect,
	}) => {
		const { complete, captured } = makeComplete()

		await complete('{env="$env"} | ¦')

		expect(captured.fieldsParams?.value?.variableValues).toEqual({
			env: "prod",
		})
	})

	it("describes a variable by its type and the value it resolves to", async ({
		expect,
	}) => {
		const { complete } = makeComplete()

		const { options } = await complete('{app="api"} | $¦')

		expect(options[0]).toMatchObject({
			label: "$env",
			detail: "general.variable-types.list",
			info: "editor.config-modal-utils.query-info.variable prod",
		})
	})
})

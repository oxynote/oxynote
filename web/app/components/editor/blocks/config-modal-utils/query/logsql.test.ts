import { describe, it } from "vitest"
import { EditorState, type Extension } from "@codemirror/state"
import {
	CompletionContext,
	type CompletionSource,
} from "@codemirror/autocomplete"
import {
	installAutoImportGlobals,
	interpolatingT,
	variableTree,
} from "./test-helpers"
import { useLogsQLQueryExtension } from "./logsql"

installAutoImportGlobals()

describe("useLogsQLQueryExtension", () => {
	it("completes a typed variable key from its $ on", async ({ expect }) => {
		const { labels, from } = await complete("error $en¦")

		expect(labels).toEqual(["$env", "$window"])
		expect(from).toBe(6)
	})

	it("offers the variables at the cursor when asked for", async ({
		expect,
	}) => {
		const { labels, from } = await complete("error ¦", true)

		expect(labels).toEqual(["$env", "$window"])
		expect(from).toBe(6)
	})

	it.for([
		{ name: "without being asked", doc: "error ¦", explicit: false },
		{ name: "in the middle of a word", doc: "err¦or", explicit: true },
	])("offers nothing $name", async ({ doc, explicit }, { expect }) => {
		const { labels, from } = await complete(doc, explicit)

		expect(labels).toEqual([])
		expect(from).toBeNull()
	})

	it("comments a line with a hash", ({ expect }) => {
		const { extensions } = useLogsQLQueryExtension(interpolatingT, variableTree)
		const state = EditorState.create({
			doc: "error",
			extensions: extensions.value as Extension,
		})

		expect(state.languageDataAt("commentTokens", 0)).toEqual([{ line: "#" }])
	})
})

// the cursor goes where the doc holds a ¦, which is removed
async function complete(marked: string, explicit = false) {
	const { extensions } = useLogsQLQueryExtension(interpolatingT, variableTree)
	const pos = marked.indexOf("¦")
	const state = EditorState.create({
		doc: marked.replace("¦", ""),
		extensions: extensions.value as Extension,
	})
	const sources = state.languageDataAt<CompletionSource>("autocomplete", pos)
	const ctx = new CompletionContext(state, pos, explicit)
	const labels: string[] = []
	let from: number | null = null

	for (const source of sources) {
		const res = await source(ctx)

		if (res) {
			labels.push(...res.options.map((o) => o.label))
			from = res.from
		}
	}

	return { labels, from }
}

import type { Node as PMNode } from "@tiptap/pm/model"
import { Schema } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { EditorState } from "@tiptap/pm/state"
import type { DecorationSet } from "@tiptap/pm/view"
import { describe, it } from "vitest"
import { SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME } from "../node-names"
import { ParameterListSeparators } from "./parameter-list-separators"
import {
	docNode,
	paramItem,
	paramList,
	paramListSchema,
	splitDoc,
} from "./test-helpers"

// the extension's plugin factory reads nothing from the extension
// context, so it can be invoked without a live editor
function makePlugin(): Plugin {
	const addProseMirrorPlugins = ParameterListSeparators.config
		.addProseMirrorPlugins as unknown as (this: unknown) => Plugin[]

	const plugin = addProseMirrorPlugins.call(undefined)[0]
	if (!plugin) {
		throw new Error("ParameterListSeparators produced no plugin")
	}

	return plugin
}

function makeState(doc: PMNode): { state: EditorState; plugin: Plugin } {
	const plugin = makePlugin()
	const state = EditorState.create({ doc, plugins: [plugin] })

	return { state, plugin }
}

function decorations(state: EditorState, plugin: Plugin): DecorationSet | null {
	return plugin.getState(state) as DecorationSet | null
}

function widgetPositions(set: DecorationSet | null): number[] {
	return (set?.find() ?? []).map((deco) => deco.from)
}

function widgetKeys(set: DecorationSet | null): string[] {
	return (set?.find() ?? []).map((deco) => (deco.spec as { key: string }).key)
}

// the start position of every parameter list child except the first —
// exactly where a separator belongs
function separatorTargets(doc: PMNode): number[] {
	const targets: number[] = []

	doc.descendants((node, pos) => {
		if (node.type.name !== SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME) {
			return
		}

		let index = 0

		node.forEach((_child, offset) => {
			if (index > 0) {
				targets.push(pos + 1 + offset)
			}

			index++
		})
	})

	return targets
}

describe("ParameterListSeparators", () => {
	it("stores no decorations for a document without parameter lists", ({
		expect,
	}) => {
		const { state, plugin } = makeState(
			docNode({
				type: "paragraph",
				content: [{ type: "text", text: "plain" }],
			}),
		)

		expect(decorations(state, plugin)).toBeNull()
	})

	it("stores no decorations when the schema has no parameter list node", ({
		expect,
	}) => {
		const bareSchema = new Schema({
			nodes: {
				doc: { content: "block+" },
				paragraph: { group: "block", content: "inline*" },
				text: { group: "inline" },
			},
		})
		const plugin = makePlugin()
		const state = EditorState.create({
			schema: bareSchema,
			plugins: [plugin],
		})

		expect(decorations(state, plugin)).toBeNull()
	})

	it("places a separator before every list child except the first", ({
		expect,
	}) => {
		const doc = docNode(
			splitDoc(
				paramList(
					"Params",
					paramItem("id", "string", "the id"),
					paramItem("name", "string", "the name"),
				),
			),
		)
		const { state, plugin } = makeState(doc)

		const positions = widgetPositions(decorations(state, plugin))

		expect(positions).toHaveLength(2)
		expect(positions).toEqual(separatorTargets(doc))
	})

	it("decorates every parameter list in the document", ({ expect }) => {
		const doc = docNode(
			splitDoc(
				paramList("First", paramItem("a", "string", "a")),
				paramList("Second", paramItem("b", "string", "b")),
			),
		)
		const { state, plugin } = makeState(doc)

		const positions = widgetPositions(decorations(state, plugin))

		expect(positions).toHaveLength(2)
		expect(positions).toEqual(separatorTargets(doc))
	})

	it("rebuilds decorations with fresh widget keys when the document changes", ({
		expect,
	}) => {
		const doc = docNode(
			splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
		)
		const { state, plugin } = makeState(doc)
		const before = decorations(state, plugin)

		// append a second item at the end of the list, which needs one
		// more separator
		let listEnd = -1
		doc.descendants((node, pos) => {
			if (node.type.name === SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME) {
				listEnd = pos + node.nodeSize - 1
			}
		})
		const next = state.apply(
			state.tr.insert(
				listEnd,
				paramListSchema.nodeFromJSON(paramItem("name", "string", "n")),
			),
		)
		const after = decorations(next, plugin)

		expect(widgetPositions(after)).toHaveLength(2)
		expect(widgetPositions(after)).toEqual(separatorTargets(next.doc))

		// keys never repeat across rebuilds, so the view cannot reuse a
		// stale separator element
		const beforeKeys = widgetKeys(before)
		for (const key of widgetKeys(after)) {
			expect(beforeKeys).not.toContain(key)
		}
	})

	it("keeps the decoration set when the document is unchanged", ({
		expect,
	}) => {
		const { state, plugin } = makeState(
			docNode(
				splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
			),
		)

		const next = state.apply(state.tr)

		expect(decorations(next, plugin)).toBe(decorations(state, plugin))
	})
})

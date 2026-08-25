import type { CommandProps, Editor } from "@tiptap/core"
import { until } from "@vueuse/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, type Plugin } from "@tiptap/pm/state"
import type { DecorationSet } from "@tiptap/pm/view"
import { describe, it, vi } from "vitest"
import { shallowRef } from "vue"
import {
	GapDecorations,
	refreshGapDecorationsInBackground,
} from "./gap-decorations"
import {
	METRIC_BLOCK_NAME,
	METRIC_GRID_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	TITLED_CODE_BLOCK_NAME,
} from "../blocks/node-names"
import { gapPlugin } from "./test-helpers"
import { attrBlockBuilder, docBuilder } from "../test-helpers"

// mirrors the real content expressions of the block extensions the gap
// config knows about, so the collected gaps match production shapes.
// `plainBlock` is the one synthetic type: nothing in the real schema
// produces a container whose last child has no gap config
const schema = new Schema({
	nodes: {
		doc: { content: "block*" },
		text: { group: "inline" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		heading: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		plainBlock: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		bulletList: {
			group: "block",
			content: "listItem+",
			attrs: { uid: { default: null } },
		},
		orderedList: {
			group: "block",
			content: "listItem+",
			attrs: { uid: { default: null } },
		},
		listItem: {
			content: "paragraph (bulletList | orderedList)?",
			attrs: { uid: { default: null } },
		},
		taskList: {
			group: "block",
			content: "taskItem+",
			attrs: { uid: { default: null } },
		},
		taskItem: {
			content: "paragraph taskList?",
			attrs: { uid: { default: null } },
		},
		[TITLED_CODE_BLOCK_NAME]: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		[METRIC_GRID_NAME]: {
			group: "block",
			content: `${METRIC_BLOCK_NAME}*`,
			attrs: { uid: { default: null } },
		},
		[METRIC_BLOCK_NAME]: {
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_NAME]: {
			group: "block",
			content: `${SPLIT_DOCUMENTATION_LEFT_SIDE_NAME} ${SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME}`,
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_LEFT_SIDE_NAME]: {
			content: `heading (paragraph | bulletList | orderedList | taskList)+ ${SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME}*`,
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME]: {
			content: `(${TITLED_CODE_BLOCK_NAME} | ${METRIC_BLOCK_NAME})+`,
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME]: {
			content: `${SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME} ${SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME}+`,
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME]: {
			content: "text*",
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME]: {
			content: `${SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME} paragraph`,
			attrs: { uid: { default: null } },
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME]: {
			content: "text*",
			attrs: { uid: { default: null } },
		},
	},
})

const docOf = docBuilder(schema)
const block = attrBlockBuilder(schema)

const para = (uid: string | null, text = "ab") =>
	block("paragraph", { uid }, text)

const heading = (uid: string | null, text = "ab") =>
	block("heading", { uid }, text)

const plainBlock = (uid: string | null) => block("plainBlock", { uid }, "ab")

const titledCodeBlock = (uid: string | null) =>
	block(TITLED_CODE_BLOCK_NAME, { uid }, "ab")

const bulletList = (uid: string | null, ...items: PMNode[]) =>
	block("bulletList", { uid }, ...items)

const listItem = (uid: string | null, ...content: PMNode[]) =>
	block("listItem", { uid }, ...content)

const taskList = (uid: string | null, ...items: PMNode[]) =>
	block("taskList", { uid }, ...items)

const taskItem = (uid: string | null, ...content: PMNode[]) =>
	block("taskItem", { uid }, ...content)

const metricGrid = (uid: string | null, ...blocks: PMNode[]) =>
	block(METRIC_GRID_NAME, { uid }, ...blocks)

const metricBlock = (uid: string | null) =>
	block(METRIC_BLOCK_NAME, { uid }, "ab")

const splitDoc = (uid: string | null, left: PMNode, right: PMNode) =>
	block(SPLIT_DOCUMENTATION_NAME, { uid }, left, right)

const leftSide = (uid: string | null, ...content: PMNode[]) =>
	block(SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, { uid }, ...content)

const rightSide = (uid: string | null, ...content: PMNode[]) =>
	block(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, { uid }, ...content)

const paramList = (uid: string | null, ...items: PMNode[]) =>
	block(
		SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
		{ uid },
		block(SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME, { uid: "plh" }),
		...items,
	)

const paramItem = (uid: string | null) =>
	block(
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
		{ uid },
		block(SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME, null),
		para(null),
	)

interface GapState {
	decorations: DecorationSet
	gapsByKey: Map<string, number>
}

function makeState(doc: PMNode): { plugin: Plugin; state: EditorState } {
	const plugin = gapPlugin()

	return { plugin, state: EditorState.create({ doc, plugins: [plugin] }) }
}

function gapState(plugin: Plugin, state: EditorState): GapState {
	const value = plugin.getState(state) as GapState | undefined

	if (!value) {
		throw new Error("gap decoration plugin state is missing")
	}

	return value
}

// the collected gap map as sorted [key, position] rows
function gapRows(plugin: Plugin, state: EditorState): [string, number][] {
	return [...gapState(plugin, state).gapsByKey].sort(
		(a, b) => a[1] - b[1] || a[0].localeCompare(b[0]),
	)
}

// the same shape read back through the plugin's decoration prop, which
// is what prosemirror actually renders — it diverges from gapRows
// whenever an update fails to re-place a moved decoration
function decoRows(plugin: Plugin, state: EditorState): [string, number][] {
	const decorations = plugin.props.decorations?.call(plugin, state) as
		DecorationSet | undefined

	if (!decorations) {
		return []
	}

	return decorations
		.find()
		.map((deco): [string, number] => [
			(deco.spec as { key: string }).key,
			deco.from,
		])
		.sort((a, b) => a[1] - b[1] || a[0].localeCompare(b[0]))
}

describe("gap collection", () => {
	it("collects no gaps for an empty document", ({ expect }) => {
		const { plugin, state } = makeState(docOf())

		expect(gapRows(plugin, state)).toEqual([])
		expect(decoRows(plugin, state)).toEqual([])
	})

	it("collects a leading and a trailing gap for a single block", ({
		expect,
	}) => {
		const { plugin, state } = makeState(docOf(para("p1")))

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:p1", 0],
			["doc:after:p1", 4],
		])
	})

	it("collects one gap before every sibling plus a trailing gap", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(para("p1"), heading("h1"), para("p2")),
		)

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:p1", 0],
			["doc:before:h1", 4],
			["doc:before:p2", 8],
			["doc:after:p2", 12],
		])
	})

	it("falls back to child offsets when blocks carry no uid", ({ expect }) => {
		const { plugin, state } = makeState(docOf(para(null), para(null)))

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:idx-0", 0],
			["doc:before:idx-4", 4],
			["doc:after:idx-4", 8],
		])
	})

	it("skips blocks without a gap config but keeps surrounding gaps", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(para("p1"), plainBlock("x1"), para("p2")),
		)

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:p1", 0],
			["doc:before:p2", 8],
			["doc:after:p2", 12],
		])
	})

	it("omits the trailing gap when the last block has no gap config", ({
		expect,
	}) => {
		const { plugin, state } = makeState(docOf(para("p1"), plainBlock("x1")))

		expect(gapRows(plugin, state)).toEqual([["doc:before:p1", 0]])
	})

	it("collects gaps inside a list container keyed by the list uid", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(
				bulletList(
					"bl",
					listItem("li1", para("a")),
					listItem("li2", para("b")),
				),
			),
		)

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:bl", 0],
			["bl:before:li1", 1],
			["bl:before:li2", 7],
			["bl:after:li2", 13],
			["doc:after:bl", 14],
		])
	})

	it("keys container gaps by node type and position when the uid is absent", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(bulletList(null, listItem("li1", para("a")))),
		)

		expect(gapRows(plugin, state).map(([key]) => key)).toEqual([
			"doc:before:idx-0",
			"type-bulletList-pos-0:before:li1",
			"type-bulletList-pos-0:after:li1",
			"doc:after:idx-0",
		])
	})

	it("marks nested lists with rising indent levels", ({ expect }) => {
		const { plugin, state } = makeState(
			docOf(
				bulletList(
					"l0",
					listItem(
						"li0",
						para("a"),
						bulletList(
							"l1",
							listItem(
								"li1",
								para("b"),
								bulletList("l2", listItem("li2", para("c"))),
							),
						),
					),
				),
			),
		)

		// every list level contributes its own container gaps; the indent
		// level itself only shows up in the rendered widget attributes
		expect(gapRows(plugin, state).map(([key]) => key)).toEqual([
			"doc:before:l0",
			"l0:before:li0",
			"l1:before:li1",
			"l2:before:li2",
			"l2:after:li2",
			"l1:after:li1",
			"l0:after:li0",
			"doc:after:l0",
		])
	})

	it("collects vertical gaps for metric blocks inside a metric grid", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(metricGrid("mg", metricBlock("m1"), metricBlock("m2"))),
		)

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:mg", 0],
			["mg:before:m1", 1],
			["mg:before:m2", 5],
			["mg:after:m2", 9],
			["doc:after:mg", 10],
		])
	})

	it("collects no inner gaps for an empty metric grid", ({ expect }) => {
		const { plugin, state } = makeState(docOf(metricGrid("mg")))

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:mg", 0],
			["doc:after:mg", 2],
		])
	})

	it("collects gaps for both split documentation sides", ({ expect }) => {
		const { plugin, state } = makeState(
			docOf(
				splitDoc(
					"sd",
					leftSide("ls", heading("h1"), para("p1")),
					rightSide("rs", titledCodeBlock("tc1"), metricBlock("mb1")),
				),
			),
		)

		expect(gapRows(plugin, state).map(([key]) => key)).toEqual([
			"doc:before:sd",
			"ls:before:h1",
			"ls:before:p1",
			"ls:after:p1",
			"rs:before:tc1",
			"rs:before:mb1",
			"rs:after:mb1",
			"doc:after:sd",
		])
	})

	it("skips the header of a parameter list and gaps only its items", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(
				splitDoc(
					"sd",
					leftSide(
						"ls",
						heading("h1"),
						para("p1"),
						paramList("pl", paramItem("pi1"), paramItem("pi2")),
					),
					rightSide("rs", titledCodeBlock("tc1")),
				),
			),
		)

		expect(gapRows(plugin, state).map(([key]) => key)).toEqual([
			"doc:before:sd",
			"ls:before:h1",
			"ls:before:p1",
			"ls:before:pl",
			"pl:before:pi1",
			"pl:before:pi2",
			"pl:after:pi2",
			"ls:after:pl",
			"rs:before:tc1",
			"rs:after:tc1",
			"doc:after:sd",
		])
	})

	it("gaps task list items like bullet list items", ({ expect }) => {
		const { plugin, state } = makeState(
			docOf(taskList("tl", taskItem("ti1", para("a")))),
		)

		expect(gapRows(plugin, state).map(([key]) => key)).toEqual([
			"doc:before:tl",
			"tl:before:ti1",
			"tl:after:ti1",
			"doc:after:tl",
		])
	})
})

describe("gap decoration updates", () => {
	it("keeps the same plugin state for transactions that do not change the document", ({
		expect,
	}) => {
		const { plugin, state } = makeState(docOf(para("p1"), para("p2")))
		const next = state.apply(state.tr)

		expect(plugin.getState(next)).toBe(plugin.getState(state))
	})

	it("shifts gaps that follow inserted text", ({ expect }) => {
		const { plugin, state } = makeState(docOf(para("p1"), para("p2")))
		const next = state.apply(state.tr.insertText("xyz", 1))

		expect(decoRows(plugin, next)).toEqual([
			["doc:before:p1", 0],
			["doc:before:p2", 7],
			["doc:after:p2", 11],
		])
		expect(decoRows(plugin, next)).toEqual(gapRows(plugin, next))
	})

	it("drops gaps whose block was deleted", ({ expect }) => {
		const { plugin, state } = makeState(
			docOf(para("p1"), para("p2"), para("p3")),
		)
		const next = state.apply(state.tr.delete(4, 8))

		expect(decoRows(plugin, next)).toEqual([
			["doc:before:p1", 0],
			["doc:before:p3", 4],
			["doc:after:p3", 8],
		])
	})

	it("adds gaps for a block inserted between existing ones", ({ expect }) => {
		const { plugin, state } = makeState(docOf(para("p1"), para("p2")))
		const next = state.apply(state.tr.insert(4, para("pm")))

		expect(decoRows(plugin, next)).toEqual([
			["doc:before:p1", 0],
			["doc:before:pm", 4],
			["doc:before:p2", 8],
			["doc:after:p2", 12],
		])
	})

	it("re-places gaps whose mapped position no longer matches the document", ({
		expect,
	}) => {
		const { plugin, state } = makeState(docOf(para("p1"), para("p2")))
		// a widget at position 0 maps to 0 across a leading insertion, so
		// the gap before p1 only lands correctly if the update notices the
		// mismatch and recreates it
		const next = state.apply(state.tr.insert(0, para("p0")))

		expect(decoRows(plugin, next)).toEqual([
			["doc:before:p0", 0],
			["doc:before:p1", 4],
			["doc:before:p2", 8],
			["doc:after:p2", 12],
		])
	})

	it("rebuilds every gap on the refresh meta even without a document change", ({
		expect,
	}) => {
		const { plugin, state } = makeState(docOf(para("p1")))
		const before = gapState(plugin, state)
		const next = state.apply(state.tr.setMeta("refreshGapDecorations", true))

		expect(gapState(plugin, next)).not.toBe(before)
		expect(gapRows(plugin, next)).toEqual([
			["doc:before:p1", 0],
			["doc:after:p1", 4],
		])
	})

	it("keeps a gap of its own for sibling blocks that share a uid", ({
		expect,
	}) => {
		const { plugin, state } = makeState(docOf(para("dup"), para("dup")))

		expect(gapRows(plugin, state)).toEqual([
			["doc:before:dup", 0],
			["doc:before:dup#1", 4],
			["doc:after:dup", 8],
		])

		const next = state.apply(state.tr.insertText("z", 1))

		expect(gapRows(plugin, next)).toEqual([
			["doc:before:dup", 0],
			["doc:before:dup#1", 5],
			["doc:after:dup", 9],
		])
		expect(decoRows(plugin, next)).toEqual(gapRows(plugin, next))
	})

	it("exposes no decorations for a state the plugin is not installed in", ({
		expect,
	}) => {
		const plugin = gapPlugin()
		const foreign = EditorState.create({ doc: docOf(para("p1")) })

		expect(plugin.props.decorations?.call(plugin, foreign)).toBeUndefined()
	})
})

describe("refreshGapDecorations command", () => {
	function refreshCommand() {
		const addCommands = GapDecorations.config.addCommands

		if (!addCommands) {
			throw new Error("GapDecorations declares no commands")
		}

		const command = addCommands.call({} as never).refreshGapDecorations

		if (!command) {
			throw new Error("GapDecorations declares no refresh command")
		}

		return command
	}

	it("dispatches a transaction carrying the refresh meta", ({ expect }) => {
		const { state } = makeState(docOf(para("p1")))
		const tr = state.tr
		const dispatch = vi.fn()

		expect(
			refreshCommand()()({ tr, dispatch } as unknown as CommandProps),
		).toBe(true)
		expect(dispatch).toHaveBeenCalledWith(tr)
		expect(tr.getMeta("refreshGapDecorations")).toBe(true)
	})

	it("succeeds without dispatching when the command is only probed", ({
		expect,
	}) => {
		const { state } = makeState(docOf(para("p1")))
		const tr = state.tr

		expect(
			refreshCommand()()({
				tr,
				dispatch: undefined,
			} as unknown as CommandProps),
		).toBe(true)
		expect(tr.getMeta("refreshGapDecorations")).toBeUndefined()
	})
})

// the plugin view reads a global constructor and the reposition helper
// keeps module-level scheduling state, so these cannot interleave
describe("plugin view", { concurrent: false }, () => {
	it("skips the resize observer when the environment has none", ({
		expect,
	}) => {
		const plugin = gapPlugin()
		const view = plugin.spec.view?.({ dom: {} } as never)

		expect(() => view?.destroy?.()).not.toThrow()
	})

	it("observes the editor element and disconnects on destroy", ({ expect }) => {
		const observe = vi.fn()
		const disconnect = vi.fn()
		const dom = {}

		vi.stubGlobal(
			"ResizeObserver",
			class {
				observe = observe
				disconnect = disconnect
			},
		)

		const plugin = gapPlugin()
		const view = plugin.spec.view?.({ dom } as never)

		expect(observe).toHaveBeenCalledWith(dom)

		view?.destroy?.()

		expect(disconnect).toHaveBeenCalledOnce()
	})
})

// `until` reaches gap-decorations as a nuxt auto-import, which the node
// project resolves off globalThis
describe("refreshGapDecorationsInBackground", { concurrent: false }, () => {
	it("refreshes once the editor ref is populated", async ({ expect }) => {
		vi.stubGlobal("until", until)

		const refreshGapDecorations = vi.fn()
		const editor = shallowRef<Editor | null>(null)

		refreshGapDecorationsInBackground(editor)

		expect(refreshGapDecorations).not.toHaveBeenCalled()

		editor.value = { commands: { refreshGapDecorations } } as unknown as Editor

		await vi.waitFor(() => {
			expect(refreshGapDecorations).toHaveBeenCalledOnce()
		})
	})

	it("refreshes immediately when the editor is already available", async ({
		expect,
	}) => {
		vi.stubGlobal("until", until)

		const refreshGapDecorations = vi.fn()
		const editor = shallowRef<Editor | null>({
			commands: { refreshGapDecorations },
		} as unknown as Editor)

		refreshGapDecorationsInBackground(editor)

		await vi.waitFor(() => {
			expect(refreshGapDecorations).toHaveBeenCalledOnce()
		})
	})
})

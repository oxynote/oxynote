import type { CommandProps } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, type Plugin, type Transaction } from "@tiptap/pm/state"
import type { DecorationSet } from "@tiptap/pm/view"
import { describe, it } from "vitest"
import { HookDecorator } from "./hook-decorator"
import { docBuilder } from "../test-helpers"
import { CODE_BLOCK_NAME, METRIC_BLOCK_NAME } from "../blocks/node-names"
import { DocumentHookType, type DocumentHook } from "~/utils/api/document"

// minimal schema covering every placement rule: a plain block, a
// node-view block (by name), an atom block, and a metric block nested
// in its grid
const schema = new Schema<
	| "doc"
	| "paragraph"
	| "atomBlock"
	| "metricGrid"
	| typeof CODE_BLOCK_NAME
	| typeof METRIC_BLOCK_NAME
	| "text"
>({
	nodes: {
		doc: { content: "block+" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		atomBlock: {
			group: "block",
			atom: true,
			attrs: { uid: { default: null } },
		},
		metricGrid: { group: "block", content: "metricBlock+" },
		[CODE_BLOCK_NAME]: {
			group: "block",
			content: "text*",
			attrs: { uid: { default: null } },
		},
		[METRIC_BLOCK_NAME]: {
			content: "inline*",
			attrs: { uid: { default: null } },
		},
		text: { group: "inline" },
	},
})

const docOf = docBuilder(schema)

function para(uid: string | null, str: string): PMNode {
	return schema.nodes.paragraph.create({ uid }, schema.text(str))
}

function codeBlock(uid: string | null, str: string): PMNode {
	return schema.nodes[CODE_BLOCK_NAME].create({ uid }, schema.text(str))
}

function atomBlock(uid: string | null): PMNode {
	return schema.nodes.atomBlock.create({ uid })
}

function metricGrid(...blocks: PMNode[]): PMNode {
	return schema.nodes.metricGrid.create(null, blocks)
}

function metricBlock(uid: string | null, str: string): PMNode {
	return schema.nodes[METRIC_BLOCK_NAME].create({ uid }, schema.text(str))
}

function hook(blockId: string | null, score = "0"): DocumentHook {
	return {
		id: `hook-${blockId ?? "none"}`,
		type: DocumentHookType.ScheduledReminder,
		documentId: "doc-1",
		organizationId: "org-1",
		branchId: "branch-1",
		blockId,
		settings: {
			scale: "linear",
			duration: null,
			schedule: "2026-01-01T00:00:00Z",
		},
		state: { lastActiveAt: "2026-01-01T00:00:00Z" },
		score,
		createdAt: "2026-01-01T00:00:00Z",
	}
}

// addProseMirrorPlugins reads only this.options, so a minimal bound
// context stands in for the editor-managed extension instance that
// normally provides it
function hookPlugin(getHooks: () => DocumentHook[]): Plugin {
	const extension = HookDecorator.configure({ getHooks })
	const addProseMirrorPlugins = extension.config.addProseMirrorPlugins

	if (!addProseMirrorPlugins) {
		throw new Error("HookDecorator declares no prosemirror plugins")
	}

	const plugins = addProseMirrorPlugins.call({
		options: extension.options,
	} as never)
	const plugin = plugins[0]

	if (!plugin) {
		throw new Error("HookDecorator produced no prosemirror plugin")
	}

	return plugin
}

function makeState(docNode: PMNode, getHooks: () => DocumentHook[]) {
	const plugin = hookPlugin(getHooks)
	const state = EditorState.create({ doc: docNode, plugins: [plugin] })

	return { plugin, state }
}

// compresses the plugin's decorations into [from, to, kind] rows for
// readable assertions; widgets are told apart by their spec key
function decorationShape(
	plugin: Plugin,
	state: EditorState,
): [number, number, string][] {
	const decorations = plugin.props.decorations?.call(plugin, state)

	if (!decorations) {
		return []
	}

	return (decorations as DecorationSet)
		.find()
		.map((deco): [number, number, string] => {
			const spec = deco.spec as { key?: string }
			return [deco.from, deco.to, spec.key ? "widget" : "node"]
		})
		.sort((a, b) => a[0] - b[0] || a[1] - b[1])
}

describe("HookDecorator", () => {
	it("decorates a matching block with a node decoration and an inner widget", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(para("u1", "hello"), para("u2", "other")),
			() => [hook("u1")],
		)

		expect(decorationShape(plugin, state)).toEqual([
			[0, 7, "node"],
			[1, 1, "widget"],
		])
	})

	it.for([
		{
			name: "decorates a block whose hook score is zero",
			hooks: [hook("u1", "0")],
			expected: [
				[0, 4, "node"],
				[1, 1, "widget"],
			],
		},
		{
			name: "decorates a block whose decimal hook score evaluates to zero",
			hooks: [hook("u1", "0.00")],
			expected: [
				[0, 4, "node"],
				[1, 1, "widget"],
			],
		},
		{
			name: "ignores hooks with a non-zero score",
			hooks: [hook("u1", "100")],
			expected: [],
		},
		{
			name: "ignores hooks without a block id",
			hooks: [hook(null)],
			expected: [],
		},
		{
			name: "ignores hooks pointing at other blocks",
			hooks: [hook("u9")],
			expected: [],
		},
	])("$name", ({ hooks, expected }, { expect }) => {
		const { plugin, state } = makeState(docOf(para("u1", "hi")), () => hooks)

		expect(decorationShape(plugin, state)).toEqual(expected)
	})

	it("never decorates a block whose uid is empty", ({ expect }) => {
		const { plugin, state } = makeState(docOf(para("", "hi")), () => [hook("")])

		expect(decorationShape(plugin, state)).toEqual([])
	})

	it("places the widget at the node position for code blocks", ({ expect }) => {
		const { plugin, state } = makeState(
			docOf(para(null, "x"), codeBlock("cb", "code")),
			() => [hook("cb")],
		)

		expect(decorationShape(plugin, state)).toEqual([
			[3, 3, "widget"],
			[3, 9, "node"],
		])
	})

	it("places the widget at the node position for atom blocks", ({ expect }) => {
		const { plugin, state } = makeState(
			docOf(para(null, "x"), atomBlock("ab")),
			() => [hook("ab")],
		)

		expect(decorationShape(plugin, state)).toEqual([
			[3, 3, "widget"],
			[3, 4, "node"],
		])
	})

	it("places the widget before the parent grid for metric blocks", ({
		expect,
	}) => {
		const { plugin, state } = makeState(
			docOf(para(null, "x"), metricGrid(metricBlock("m1", "val"))),
			() => [hook("m1")],
		)

		expect(decorationShape(plugin, state)).toEqual([
			[3, 3, "widget"],
			[4, 9, "node"],
		])
	})

	it("keeps existing decorations when the document is unchanged", ({
		expect,
	}) => {
		let hooks = [hook("u1")]
		const { plugin, state } = makeState(docOf(para("u1", "hi")), () => hooks)

		hooks = []
		const next = state.apply(state.tr)

		expect(decorationShape(plugin, next)).toEqual([
			[0, 4, "node"],
			[1, 1, "widget"],
		])
	})

	it("rebuilds from fresh hooks when the document changes", ({ expect }) => {
		let hooks = [hook("u1")]
		const { plugin, state } = makeState(
			docOf(para("u1", "hi"), para("u2", "yo")),
			() => hooks,
		)

		hooks = [hook("u2")]
		const next = state.apply(state.tr.insertText("!", 1))

		expect(decorationShape(plugin, next)).toEqual([
			[5, 9, "node"],
			[6, 6, "widget"],
		])
	})

	describe("refreshHookDecorations", () => {
		it("dispatches a transaction that rebuilds the decorations", ({
			expect,
		}) => {
			let hooks: DocumentHook[] = []
			const { plugin, state } = makeState(docOf(para("u1", "hi")), () => hooks)
			const addCommands = HookDecorator.config.addCommands

			if (!addCommands) {
				throw new Error("HookDecorator declares no commands")
			}

			const refresh = addCommands.call({} as never).refreshHookDecorations

			if (!refresh) {
				throw new Error("refreshHookDecorations command missing")
			}

			hooks = [hook("u1")]
			const dispatched: Transaction[] = []
			const result = refresh()({
				tr: state.tr,
				dispatch: (tr: Transaction) => {
					dispatched.push(tr)
				},
			} as unknown as CommandProps)
			const tr = dispatched[0]

			if (!tr) {
				throw new Error("refresh command did not dispatch")
			}

			expect(result).toBe(true)
			expect(decorationShape(plugin, state.apply(tr))).toEqual([
				[0, 4, "node"],
				[1, 1, "widget"],
			])
		})
	})
})

import { Schema, type Node as PMNode, type Slice } from "@tiptap/pm/model"
import { EditorState, NodeSelection, TextSelection } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { describe, it } from "vitest"
import { TextSelectClipboard, TextSelectShortcuts } from "./text-select"
import { blockBuilder, makeDispatchEditor } from "./test-helpers"

const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		container: { group: "block", content: "block+" },
		atomBlock: { group: "block", atom: true },
		text: { group: "inline" },
	},
})

const block = blockBuilder(schema)

describe("TextSelectClipboard", () => {
	// instantiates the extension's plugin with a fake editor exposing the
	// schema, which is all transformCopied reads
	function transformCopied() {
		const create = TextSelectClipboard.config.addProseMirrorPlugins

		if (!create) {
			throw new Error("addProseMirrorPlugins is not defined")
		}

		const ctx = { editor: { schema } } as unknown as ThisParameterType<
			NonNullable<typeof create>
		>
		const plugin = create.call(ctx)[0]

		if (!plugin) {
			throw new Error("no prosemirror plugin was created")
		}

		const transform = plugin.props.transformCopied

		if (!transform) {
			throw new Error("transformCopied prop is not defined")
		}

		return (slice: Slice) =>
			transform.call(plugin, slice, undefined as unknown as EditorView)
	}

	function sliceShape(slice: Slice) {
		const texts: string[] = []

		slice.content.forEach((node) => {
			texts.push(node.isText ? (node.text ?? "") : node.type.name)
		})

		return { texts, openStart: slice.openStart, openEnd: slice.openEnd }
	}

	const rows: {
		name: string
		make: () => PMNode
		from: number
		to: number
		expected: string
	}[] = [
		{
			name: "flattens a partial paragraph selection to its text",
			make: () => block("doc", block("paragraph", "hello world")),
			from: 1,
			to: 6,
			expected: "hello",
		},
		{
			name: "joins text from multiple paragraphs with spaces",
			make: () =>
				block("doc", block("paragraph", "hello"), block("paragraph", "world")),
			from: 1,
			to: 13,
			expected: "hello world",
		},
		{
			name: "trims lines and drops whitespace-only ones",
			make: () =>
				block(
					"doc",
					block("paragraph", "  a  "),
					block("paragraph", "   "),
					block("paragraph", "b"),
				),
			from: 1,
			to: 14,
			expected: "a b",
		},
		{
			name: "flattens nested blocks",
			make: () =>
				block(
					"doc",
					block("container", block("paragraph", "a"), block("paragraph", "b")),
				),
			from: 0,
			to: 8,
			expected: "a b",
		},
	]

	it.for(rows)("$name", ({ make, from, to, expected }, { expect }) => {
		const result = transformCopied()(make().slice(from, to))

		expect(sliceShape(result)).toEqual({
			texts: [expected],
			openStart: 0,
			openEnd: 0,
		})
	})

	it("returns an empty slice when the selection carries no text", ({
		expect,
	}) => {
		const doc = block("doc", block("paragraph"))

		const result = transformCopied()(doc.slice(0, 2))

		expect(sliceShape(result)).toEqual({
			texts: [],
			openStart: 0,
			openEnd: 0,
		})
		expect(result.content.size).toBe(0)
	})

	it("returns an empty slice when the selection holds only whitespace", ({
		expect,
	}) => {
		const doc = block("doc", block("paragraph", "   "))

		const result = transformCopied()(doc.slice(0, 5))

		expect(sliceShape(result)).toEqual({
			texts: [],
			openStart: 0,
			openEnd: 0,
		})
		expect(result.content.size).toBe(0)
	})
})

describe("TextSelectShortcuts", () => {
	function shortcut(key: string) {
		const create = TextSelectShortcuts.config.addKeyboardShortcuts

		if (!create) {
			throw new Error("addKeyboardShortcuts is not defined")
		}

		const handlers = create.call(
			{} as unknown as ThisParameterType<NonNullable<typeof create>>,
		)
		const handler = handlers[key]

		if (!handler) {
			throw new Error(`no handler is registered for ${key}`)
		}

		return handler
	}

	function runShortcut(key: string, state: EditorState) {
		const { editor, dispatched, state: current } = makeDispatchEditor(state)
		const handled = shortcut(key)({ editor })

		return { handled, dispatched, selection: () => current().selection }
	}

	function stateAt(doc: PMNode, from: number, to = from) {
		return EditorState.create({
			doc,
			selection: TextSelection.create(doc, from, to),
		})
	}

	// paragraph texts span [1, 4) and [6, 9)
	const twoBlocks = block(
		"doc",
		block("paragraph", "abc"),
		block("paragraph", "def"),
	)

	// paragraph texts span [1, 4) and [8, 11) with an empty block between
	const withEmptyMiddle = block(
		"doc",
		block("paragraph", "abc"),
		block("paragraph"),
		block("paragraph", "def"),
	)

	describe("Mod-a", () => {
		it("selects only the current text block", ({ expect }) => {
			const { handled, dispatched, selection } = runShortcut(
				"Mod-a",
				stateAt(twoBlocks, 7),
			)

			expect(handled).toBe(true)
			expect(dispatched).toHaveLength(1)
			expect(selection().anchor).toBe(6)
			expect(selection().head).toBe(9)
		})

		it("does nothing for a node selection", ({ expect }) => {
			const doc = block("doc", block("atomBlock"), block("paragraph", "x"))
			const state = EditorState.create({
				doc,
				selection: NodeSelection.create(doc, 0),
			})

			const { handled, dispatched } = runShortcut("Mod-a", state)

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})
	})

	describe("Shift-ArrowUp", () => {
		it("extends the selection to the start of the current block", ({
			expect,
		}) => {
			const { handled, selection } = runShortcut(
				"Shift-ArrowUp",
				stateAt(twoBlocks, 8),
			)

			expect(handled).toBe(true)
			expect(selection().anchor).toBe(8)
			expect(selection().head).toBe(6)
		})

		it("jumps to the previous non-empty block from a block start", ({
			expect,
		}) => {
			const { selection } = runShortcut("Shift-ArrowUp", stateAt(twoBlocks, 6))

			expect(selection().anchor).toBe(6)
			expect(selection().head).toBe(1)
		})

		it("skips empty blocks when jumping upward", ({ expect }) => {
			const { selection } = runShortcut(
				"Shift-ArrowUp",
				stateAt(withEmptyMiddle, 8),
			)

			expect(selection().anchor).toBe(8)
			expect(selection().head).toBe(1)
		})

		it("clamps to the first text position when everything above is empty", ({
			expect,
		}) => {
			// the empty paragraph spans [0, 2) and "abc" spans [3, 6)
			const doc = block("doc", block("paragraph"), block("paragraph", "abc"))

			const { selection } = runShortcut("Shift-ArrowUp", stateAt(doc, 3))

			expect(selection().anchor).toBe(3)
			expect(selection().head).toBe(1)
		})

		it("keeps a full-document selection unchanged", ({ expect }) => {
			const { handled, dispatched } = runShortcut(
				"Shift-ArrowUp",
				stateAt(twoBlocks, 1, 9),
			)

			expect(handled).toBe(true)
			expect(dispatched).toHaveLength(0)
		})

		it("does nothing in a document without text blocks", ({ expect }) => {
			const state = EditorState.create({
				doc: block("doc", block("atomBlock")),
			})

			const { handled, dispatched } = runShortcut("Shift-ArrowUp", state)

			expect(handled).toBe(true)
			expect(dispatched).toHaveLength(0)
		})
	})

	describe("Shift-ArrowDown", () => {
		it("extends the selection to the end of the current block", ({
			expect,
		}) => {
			const { handled, selection } = runShortcut(
				"Shift-ArrowDown",
				stateAt(twoBlocks, 2),
			)

			expect(handled).toBe(true)
			expect(selection().anchor).toBe(2)
			expect(selection().head).toBe(4)
		})

		it("jumps to the next non-empty block from a block end", ({ expect }) => {
			const { selection } = runShortcut(
				"Shift-ArrowDown",
				stateAt(twoBlocks, 4),
			)

			expect(selection().anchor).toBe(4)
			expect(selection().head).toBe(9)
		})

		it("skips empty blocks when jumping downward", ({ expect }) => {
			const { selection } = runShortcut(
				"Shift-ArrowDown",
				stateAt(withEmptyMiddle, 4),
			)

			expect(selection().anchor).toBe(4)
			expect(selection().head).toBe(11)
		})

		it("clamps to the last text position when everything below is empty", ({
			expect,
		}) => {
			// "abc" spans [1, 4) and the empty paragraph spans [5, 7)
			const doc = block("doc", block("paragraph", "abc"), block("paragraph"))

			const { selection } = runShortcut("Shift-ArrowDown", stateAt(doc, 4))

			expect(selection().anchor).toBe(4)
			expect(selection().head).toBe(6)
		})

		it("keeps a full-document selection unchanged", ({ expect }) => {
			const { handled, dispatched } = runShortcut(
				"Shift-ArrowDown",
				stateAt(twoBlocks, 1, 9),
			)

			expect(handled).toBe(true)
			expect(dispatched).toHaveLength(0)
		})
	})
})

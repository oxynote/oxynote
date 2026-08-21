import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, NodeSelection } from "@tiptap/pm/state"
import { beforeEach, describe, it, vi } from "vitest"
import { CODE_BLOCK_NAME, METRIC_BLOCK_NAME } from "./blocks/node-names"
import {
	InputRules,
	ParagraphDeletionHandler,
	type ParagraphDeletionHandlerOptions,
} from "./input-utils"
import { blockBuilder, makeDispatchEditor, stateAt } from "./test-helpers"

// the block setup helpers are replaced with spies so the rules can be
// exercised without a schema carrying every real block node; the rest of
// each module (node definitions, name constants) stays real because
// input-utils reads node names off it
const spies = vi.hoisted(() => ({
	setUpCalloutBlockNode: vi.fn(),
	setUpSplitDocumentationNode: vi.fn(),
	setUpCodeBlockNode: vi.fn(),
	setUpMetricBlock: vi.fn(),
}))

vi.mock("./blocks/callout", async (importOriginal) => ({
	...(await importOriginal<typeof import("./blocks/callout")>()),
	setUpCalloutBlockNode: spies.setUpCalloutBlockNode,
}))

vi.mock("./blocks/split-documentation", async (importOriginal) => ({
	...(await importOriginal<typeof import("./blocks/split-documentation")>()),
	setUpSplitDocumentationNode: spies.setUpSplitDocumentationNode,
}))

vi.mock("./blocks/code-block", async (importOriginal) => ({
	...(await importOriginal<typeof import("./blocks/code-block")>()),
	setUpCodeBlockNode: spies.setUpCodeBlockNode,
}))

vi.mock("./blocks/metrics", async (importOriginal) => ({
	...(await importOriginal<typeof import("./blocks/metrics")>()),
	setUpMetricBlock: spies.setUpMetricBlock,
}))

const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		container: { group: "block", content: "block+" },
		[CODE_BLOCK_NAME]: {
			group: "block",
			content: "text*",
			marks: "",
			code: true,
		},
		[METRIC_BLOCK_NAME]: { group: "block", atom: true },
		text: { group: "inline" },
	},
})

const block = blockBuilder(schema)

// the block setup spies are module-level state shared by every test in
// this suite, so the tests cannot interleave
describe("InputRules", { concurrent: false }, () => {
	beforeEach(() => {
		Object.values(spies).forEach((spy) => {
			spy.mockClear()
		})
	})

	function rules() {
		const create = InputRules.config.addInputRules

		if (!create) {
			throw new Error("addInputRules is not defined")
		}

		return create.call(
			{} as unknown as ThisParameterType<NonNullable<typeof create>>,
		)
	}

	function ruleAt(index: number) {
		const rule = rules()[index]

		if (!rule) {
			throw new Error(`no input rule at index ${index}`)
		}

		return rule
	}

	// only the `state` field is read by the container guard wrapped around
	// every rule; the rest is handed straight to the block setup spies
	function runRule(index: number, state: EditorState) {
		const props = {
			state,
			range: { from: 1, to: 3 },
			match: ["```"],
			commands: {},
			chain: () => ({}),
		} as unknown as Parameters<ReturnType<typeof ruleAt>["handler"]>[0]

		ruleAt(index).handler(props)

		return props
	}

	it("registers a rule per supported block", ({ expect }) => {
		expect(rules()).toHaveLength(4)
	})

	const findRows: {
		name: string
		index: number
		matches: string[]
		rejects: string[]
	}[] = [
		{
			name: "the callout rule",
			index: 0,
			matches: ["!!", "text\n!!"],
			rejects: ["!", "!!x", "a!!"],
		},
		{
			name: "the split documentation rule",
			index: 1,
			matches: ["||", "text\n||"],
			rejects: ["|", "||x", "a||"],
		},
		{
			name: "the code block rule",
			index: 2,
			matches: ["```", "text\n```"],
			rejects: ["``", "```x", "a```"],
		},
		{
			name: "the metric rule",
			index: 3,
			matches: ["%%", "text\n%%"],
			rejects: ["%", "%%x", "a%%"],
		},
	]

	it.for(findRows)(
		"$name matches only its trigger",
		({ index, matches, rejects }, { expect }) => {
			const { find } = ruleAt(index)

			if (!(find instanceof RegExp)) {
				throw new Error("input rule find is not a regular expression")
			}

			expect(matches.map((text) => find.test(text))).toEqual(
				matches.map(() => true),
			)
			expect(rejects.map((text) => find.test(text))).toEqual(
				rejects.map(() => false),
			)
		},
	)

	const topLevelDoc = block("doc", block("paragraph", "xx"))
	const nestedDoc = block("doc", block("container", block("paragraph", "xx")))

	it("runs the callout rule in a top-level paragraph", ({ expect }) => {
		const props = runRule(0, stateAt(topLevelDoc, 2))

		expect(spies.setUpCalloutBlockNode).toHaveBeenCalledWith(
			props.range,
			props.commands,
		)
	})

	it("blocks the callout rule inside a disallowed container", ({ expect }) => {
		runRule(0, stateAt(nestedDoc, 3))

		expect(spies.setUpCalloutBlockNode).not.toHaveBeenCalled()
	})

	it("runs the code block rule in a top-level paragraph", ({ expect }) => {
		const props = runRule(2, stateAt(topLevelDoc, 2))

		expect(spies.setUpCodeBlockNode).toHaveBeenCalledWith(
			props.range,
			props.match,
			props.chain,
		)
	})

	it("blocks the code block rule inside a disallowed container", ({
		expect,
	}) => {
		runRule(2, stateAt(nestedDoc, 3))

		expect(spies.setUpCodeBlockNode).not.toHaveBeenCalled()
	})

	it.for([
		{
			name: "split documentation",
			index: 1,
			spy: spies.setUpSplitDocumentationNode,
		},
		{ name: "metric", index: 3, spy: spies.setUpMetricBlock },
	])(
		"runs the $name rule only at the document root",
		({ index, spy }, { expect }) => {
			const props = runRule(index, stateAt(topLevelDoc, 2))

			expect(spy).toHaveBeenCalledTimes(1)
			expect(spy).toHaveBeenCalledWith(
				props.state,
				expect.anything(),
				props.range.from,
			)

			spy.mockClear()

			runRule(index, stateAt(nestedDoc, 3))

			expect(spy).not.toHaveBeenCalled()
		},
	)
})

describe("ParagraphDeletionHandler", () => {
	function backspace(
		state: EditorState,
		options?: ParagraphDeletionHandlerOptions,
	) {
		const create = ParagraphDeletionHandler.config.addKeyboardShortcuts

		if (!create) {
			throw new Error("addKeyboardShortcuts is not defined")
		}

		const { editor, dispatched, state: current } = makeDispatchEditor(state)
		const ctx = {
			editor,
			options: options ?? ParagraphDeletionHandler.options,
		} as unknown as ThisParameterType<NonNullable<typeof create>>

		const handler = create.call(ctx).Backspace

		if (!handler) {
			throw new Error("no Backspace handler is registered")
		}

		const handled = handler({ editor })

		return { handled, dispatched, doc: () => current().doc, state: current }
	}

	// compresses a document into [nodeName, text] pairs
	function shape(doc: PMNode): [string, string][] {
		return doc.children.map((node) => [
			node.type.name,
			node.textBetween(0, node.content.size, "\n", "\n"),
		])
	}

	describe("without a matching context", () => {
		it("ignores a non-empty selection", ({ expect }) => {
			const doc = block("doc", block("paragraph", "abc"))

			const { handled, dispatched } = backspace(stateAt(doc, 1, 3))

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})

		it("ignores a node selection", ({ expect }) => {
			const doc = block(
				"doc",
				block(METRIC_BLOCK_NAME),
				block("paragraph", "a"),
			)
			const state = EditorState.create({
				doc,
				selection: NodeSelection.create(doc, 0),
			})

			const { handled, dispatched } = backspace(state)

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})

		it("ignores a cursor in the middle of a paragraph", ({ expect }) => {
			const doc = block("doc", block("paragraph", "abc"))

			const { handled, dispatched } = backspace(stateAt(doc, 3))

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})

		it("ignores a paragraph start preceded by another paragraph", ({
			expect,
		}) => {
			const doc = block(
				"doc",
				block("paragraph", "abc"),
				block("paragraph", "def"),
			)

			const { handled, dispatched } = backspace(stateAt(doc, 6))

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})

		it("ignores the document start when the first block is not a paragraph", ({
			expect,
		}) => {
			const doc = block(
				"doc",
				block(CODE_BLOCK_NAME, "x"),
				block("paragraph", "a"),
			)

			const { handled, dispatched } = backspace(stateAt(doc, 1))

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})

		it("ignores the document start when the first paragraph has content", ({
			expect,
		}) => {
			const doc = block("doc", block("paragraph", "abc"))

			const { handled, dispatched } = backspace(stateAt(doc, 1))

			expect(handled).toBe(false)
			expect(dispatched).toHaveLength(0)
		})
	})

	describe("at the document start", () => {
		it("removes a leading empty paragraph", ({ expect }) => {
			const doc = block("doc", block("paragraph"), block("paragraph", "abc"))

			const { handled, dispatched, doc: result } = backspace(stateAt(doc, 1))

			expect(handled).toBe(true)
			expect(dispatched).toHaveLength(1)
			expect(shape(result())).toEqual([["paragraph", "abc"]])
		})

		it("places the cursor in the new first block by default", ({ expect }) => {
			const doc = block("doc", block("paragraph"), block("paragraph", "abc"))

			const { state } = backspace(stateAt(doc, 1))

			expect(state().selection.head).toBe(1)
		})

		it("calls a custom onDeleted with the editor and transaction", ({
			expect,
		}) => {
			const onDeleted =
				vi.fn<NonNullable<ParagraphDeletionHandlerOptions["onDeleted"]>>()
			const doc = block("doc", block("paragraph"), block("paragraph", "abc"))

			const { handled } = backspace(stateAt(doc, 1), { onDeleted })

			expect(handled).toBe(true)
			expect(onDeleted).toHaveBeenCalledTimes(1)

			const args = onDeleted.mock.calls[0]?.[0]
			expect(args?.editor).toBeDefined()
			expect(args?.tr.docChanged).toBe(true)
		})

		it("tolerates a missing onDeleted callback", ({ expect }) => {
			const doc = block("doc", block("paragraph"), block("paragraph", "abc"))

			const { handled, doc: result } = backspace(stateAt(doc, 1), {})

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([["paragraph", "abc"]])
		})
	})

	describe("after a metric block", () => {
		it("merges the paragraph into the preceding paragraph", ({ expect }) => {
			const doc = block(
				"doc",
				block("paragraph", "abc"),
				block(METRIC_BLOCK_NAME),
				block("paragraph", "def"),
			)

			const { handled, doc: result, state } = backspace(stateAt(doc, 7))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([
				["paragraph", "abcdef"],
				[METRIC_BLOCK_NAME, ""],
			])
			expect(state().selection.head).toBe(7)
		})

		// the merge target is the last paragraph anywhere before the deleted
		// one, so content moves inside the previous container rather than
		// after it
		it("merges into a paragraph nested inside the previous block", ({
			expect,
		}) => {
			const doc = block(
				"doc",
				block("container", block("paragraph", "abc"), block(METRIC_BLOCK_NAME)),
				block("paragraph", "def"),
			)

			const { handled, doc: result } = backspace(stateAt(doc, 9))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([["container", "abcdef\n\n"]])
		})

		it("only drops an empty paragraph when there is content to merge into", ({
			expect,
		}) => {
			const doc = block(
				"doc",
				block("paragraph", "abc"),
				block(METRIC_BLOCK_NAME),
				block("paragraph"),
			)

			const { handled, doc: result, state } = backspace(stateAt(doc, 7))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([
				["paragraph", "abc"],
				[METRIC_BLOCK_NAME, ""],
			])
			expect(state().selection.head).toBe(4)
		})

		it("drops the paragraph when no earlier paragraph exists", ({ expect }) => {
			const doc = block(
				"doc",
				block(METRIC_BLOCK_NAME),
				block("paragraph", "def"),
			)

			const { handled, doc: result } = backspace(stateAt(doc, 2))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([[METRIC_BLOCK_NAME, ""]])
		})
	})

	describe("after a code block", () => {
		it("appends the paragraph text into the code block", ({ expect }) => {
			const doc = block(
				"doc",
				block(CODE_BLOCK_NAME, "x"),
				block("paragraph", "def"),
			)

			const { handled, doc: result, state } = backspace(stateAt(doc, 4))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([[CODE_BLOCK_NAME, "xdef"]])
			expect(state().selection.head).toBe(5)
		})

		it("appends into a code block nested at the end of the previous block", ({
			expect,
		}) => {
			const doc = block(
				"doc",
				block(
					"container",
					block("paragraph", "a"),
					block(CODE_BLOCK_NAME, "x"),
				),
				block("paragraph", "def"),
			)

			const { handled, doc: result } = backspace(stateAt(doc, 9))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([["container", "a\nxdef"]])
		})

		it("keeps the code block untouched for an empty paragraph", ({
			expect,
		}) => {
			const doc = block("doc", block(CODE_BLOCK_NAME, "x"), block("paragraph"))

			const { handled, doc: result, state } = backspace(stateAt(doc, 4))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([[CODE_BLOCK_NAME, "x"]])
			expect(state().selection.head).toBe(2)
		})

		it("merges into an earlier paragraph when the previous block only nests a code block", ({
			expect,
		}) => {
			const doc = block(
				"doc",
				block("paragraph", "abc"),
				block(CODE_BLOCK_NAME, "x"),
				block("paragraph", "def"),
			)

			const { handled, doc: result } = backspace(stateAt(doc, 9))

			expect(handled).toBe(true)
			expect(shape(result())).toEqual([
				["paragraph", "abc"],
				[CODE_BLOCK_NAME, "xdef"],
			])
		})
	})
})

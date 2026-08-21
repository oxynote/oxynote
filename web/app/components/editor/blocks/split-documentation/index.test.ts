import type { Editor, JSONContent } from "@tiptap/core"
import { getSchema, Mark, Node as TiptapNode } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import {
	BulletList,
	ListItem,
	OrderedList,
	TaskItem,
	TaskList,
} from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode, NodeType } from "@tiptap/pm/model"
import { Fragment, Schema } from "@tiptap/pm/model"
import type { Transaction } from "@tiptap/pm/state"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import { COMMENT_MARK_NAME } from "../../mark-names"
import { CalloutBlock } from "../callout"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	METRIC_BLOCK_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	TITLED_CODE_BLOCK_NAME,
} from "../node-names"
import {
	insertSplitDocumentation,
	setUpSplitDocumentationNode,
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationPlaceholderParents,
	SplitDocumentationRightSide,
} from "."
import {
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
} from "./parameter-list"
import {
	makeEditor,
	nodeKeyboardShortcut,
	paramItem,
	paramList,
} from "./test-helpers"
import {
	jsonDocBuilder,
	MetricBlockStub,
	nodeType as nodeTypeOf,
	paragraph,
	parseAttributes,
	startOfText,
} from "~/components/editor/test-helpers"

// the right side's content types come from other block packages whose
// real extensions cannot be schema-built here: the metric block reads
// chart styles off a canvas while building its attribute defaults, and
// the code block drags the whole lowlight bundle in. Only their names,
// groups, and nesting matter to this module, so stand-ins carry those.
const CodeBlockTitleStub = TiptapNode.create({
	name: CODE_BLOCK_TITLE_NAME,
	group: CODE_BLOCK_TITLE_NAME,
	content: "text*",
})

const CodeBlockStub = TiptapNode.create({
	name: CODE_BLOCK_NAME,
	group: CODE_BLOCK_NAME,
	content: "text*",
})

const TitledCodeBlockStub = TiptapNode.create({
	name: TITLED_CODE_BLOCK_NAME,
	group: TITLED_CODE_BLOCK_NAME,
	content: `${CODE_BLOCK_TITLE_NAME} ${CODE_BLOCK_NAME}`,
	isolating: true,
})

// the real document only accepts blocks, which keeps parameter lists,
// list items, and right side blocks locked inside a split
// documentation. Accepting them at the top level too is what makes the
// commands' "no such ancestor" guards reachable
const DocumentStub = Document.extend({
	content: `(block | ${TITLED_CODE_BLOCK_NAME} | ${METRIC_BLOCK_NAME} | ${SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME} | ${SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME})+`,
})

const CommentMarkStub = Mark.create({ name: COMMENT_MARK_NAME })

const schema = getSchema([
	DocumentStub,
	Text,
	Paragraph,
	Heading,
	BulletList,
	OrderedList,
	ListItem,
	TaskList,
	TaskItem,
	CalloutBlock,
	CommentMarkStub,
	CodeBlockTitleStub,
	CodeBlockStub,
	TitledCodeBlockStub,
	MetricBlockStub,
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
	ParameterList,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
	ParameterListHeader,
])

function nodeType(name: string): NodeType {
	return nodeTypeOf(schema, name)
}

function heading(text: string): JSONContent {
	return { type: "heading", content: [{ type: "text", text }] }
}

function bulletList(text: string): JSONContent {
	return {
		type: "bulletList",
		content: [{ type: "listItem", content: [paragraph(text)] }],
	}
}

function callout(text: string): JSONContent {
	return { type: "calloutBlock", content: [paragraph(text)] }
}

function titledCode(title: string, code: string): JSONContent {
	return {
		type: TITLED_CODE_BLOCK_NAME,
		content: [
			{ type: CODE_BLOCK_TITLE_NAME, content: [{ type: "text", text: title }] },
			{ type: CODE_BLOCK_NAME, content: [{ type: "text", text: code }] },
		],
	}
}

function metric(): JSONContent {
	return { type: METRIC_BLOCK_NAME }
}

function leftSide(...content: JSONContent[]): JSONContent {
	return { type: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, content }
}

function rightSide(...content: JSONContent[]): JSONContent {
	return { type: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, content }
}

function splitDocBlock(
	left: JSONContent,
	right: JSONContent,
	inversed = false,
): JSONContent {
	return {
		type: SPLIT_DOCUMENTATION_NAME,
		attrs: { inversed },
		content: [left, right],
	}
}

const doc = jsonDocBuilder(schema)

// a split documentation with a heading plus one paragraph on the left
// and a single titled code block on the right, followed by a trailing
// paragraph so deleting the whole block leaves a valid document
function defaultDoc(): PMNode {
	return doc(
		splitDocBlock(
			leftSide(heading("Title"), paragraph("body")),
			rightSide(titledCode("cmd", "run me")),
		),
		paragraph("after"),
	)
}

function stateAt(node: PMNode, cursorPos?: number): EditorState {
	return EditorState.create({
		doc: node,
		selection:
			cursorPos === undefined
				? undefined
				: TextSelection.create(node, cursorPos),
	})
}

// the absolute position of the nth node of the given type
function posOf(node: PMNode, typeName: string, index = 0): number {
	const positions: number[] = []

	node.descendants((child, pos) => {
		if (child.type.name === typeName) {
			positions.push(pos)
		}

		return true
	})

	const pos = positions[index]
	if (pos === undefined) {
		throw new Error(`no ${typeName} at index ${index} in the test document`)
	}

	return pos
}

function nodeOf(node: PMNode, typeName: string, index = 0): PMNode {
	const found = node.nodeAt(posOf(node, typeName, index))
	if (!found) {
		throw new Error(`no ${typeName} at index ${index} in the test document`)
	}

	return found
}

function childTypes(node: PMNode): string[] {
	const types: string[] = []

	node.forEach((child) => {
		types.push(child.type.name)
	})

	return types
}

// the header text of every parameter list in the document, in order
function listHeaders(node: PMNode): string[] {
	const headers: string[] = []

	node.descendants((child) => {
		if (child.type.name === SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME) {
			headers.push(child.child(0).textContent)
		}

		return true
	})

	return headers
}

// the title of every parameter list item in the document, in order
function itemTitles(node: PMNode): string[] {
	const titles: string[] = []

	node.descendants((child) => {
		if (child.type.name === SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME) {
			titles.push(child.child(0).child(0).textContent)
		}

		return true
	})

	return titles
}

interface CommandContext {
	state: EditorState
	tr: Transaction
	dispatch: ((tr: Transaction) => void) | undefined
}

type CommandFactory = (...args: any[]) => (props: CommandContext) => boolean

// the command factories read nothing off the extension context, so
// they can be pulled out and driven straight against a state
function splitDocCommand(name: string): CommandFactory {
	const addCommands = SplitDocumentation.config.addCommands as unknown as
		| ((this: unknown) => Record<string, CommandFactory>)
		| undefined

	if (!addCommands) {
		throw new Error("SplitDocumentation defines no commands")
	}

	const factory = addCommands.call(undefined)[name]
	if (!factory) {
		throw new Error(`SplitDocumentation defines no ${name} command`)
	}

	return factory
}

interface CommandRun {
	handled: boolean
	dispatched: boolean
	state: EditorState
}

// runs a command the way the editor would: the transaction it built is
// applied only when the command dispatched it
function runCommand(
	state: EditorState,
	name: string,
	args: unknown[],
	dispatchable = true,
): CommandRun {
	const tr = state.tr

	// collected rather than flagged: typescript does not track a boolean
	// assigned inside the dispatch callback
	const dispatched: Transaction[] = []

	const handled = splitDocCommand(name)(...args)({
		state,
		tr,
		dispatch: dispatchable
			? (dispatchedTr: Transaction) => {
					dispatched.push(dispatchedTr)
				}
			: undefined,
	})

	const applied = dispatched[0]

	return {
		handled,
		dispatched: applied !== undefined,
		state: applied ? state.apply(applied) : state,
	}
}

// a stateful editor stand-in that also exposes the two commands the
// split documentation shortcuts delegate to
function makeShortcutEditor(
	node: PMNode,
	cursorPos: number,
): {
	editor: Editor
	appendParameterListOnLeftSide: ReturnType<typeof vi.fn>
	appendBlockOnRightSide: ReturnType<typeof vi.fn>
} {
	const appendParameterListOnLeftSide = vi.fn(() => true)
	const appendBlockOnRightSide = vi.fn(() => true)
	const editor = makeEditor(node, cursorPos)

	Object.defineProperty(editor, "commands", {
		value: { appendParameterListOnLeftSide, appendBlockOnRightSide },
	})

	return { editor, appendParameterListOnLeftSide, appendBlockOnRightSide }
}

// a left side that already carries one parameter list, the starting
// point for the insert-relative-to-a-node commands
function docWithParameterList(...items: JSONContent[]): PMNode {
	const listItems = items.length ? items : [paramItem("id", "string", "the id")]

	return doc(
		splitDocBlock(
			leftSide(
				heading("Title"),
				paragraph("body"),
				paramList("Params", ...listItems),
			),
			rightSide(titledCode("cmd", "run me")),
		),
	)
}

// a right side holding one block of each kind, so the hovered node can
// be either the content-bearing or the atom one
function docWithTwoRightSideBlocks(): PMNode {
	return doc(
		splitDocBlock(
			leftSide(heading("Title"), paragraph("body")),
			rightSide(titledCode("cmd", "run me"), metric()),
		),
	)
}

// records the chained commands rather than running them, so the whole
// chain can be asserted without a live editor
function chainingEditor(): { editor: Editor; calls: unknown[][] } {
	const calls: unknown[][] = []
	const chain: Record<string, (...args: unknown[]) => unknown> = {}

	for (const name of [
		"focus",
		"deleteRange",
		"insertSplitDocumentation",
		"setTextSelection",
	]) {
		chain[name] = (...args: unknown[]) => {
			calls.push([name, ...args])

			return chain
		}
	}

	chain.run = () => {
		calls.push(["run"])

		return true
	}

	return { editor: { chain: () => chain } as unknown as Editor, calls }
}

// parameter-list.ts imports this module, so consumers can enter the
// package from either side. These reset the module registry, which is
// per-file shared state, so they cannot interleave
describe("split-documentation package index", { concurrent: false }, () => {
	it("describes the parameter list in the left side content expression when the package is entered through parameter-list", async ({
		expect,
	}) => {
		vi.resetModules()

		await import("./parameter-list")
		const { SplitDocumentationLeftSide } = await import(".")

		expect(SplitDocumentationLeftSide.config.content).toContain(
			SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
		)
	})

	it("lists the parameter list item among the placeholder parents when the package is entered through parameter-list", async ({
		expect,
	}) => {
		vi.resetModules()

		await import("./parameter-list")
		const { SplitDocumentationPlaceholderParents } = await import(".")

		expect(SplitDocumentationPlaceholderParents.map((v) => v.name)).toContain(
			SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
		)
	})

	it("loads for a consumer that reaches the package through the bubble menu", async ({
		expect,
	}) => {
		vi.resetModules()

		const { isBubbleMenuItemAllowedByContext } =
			await import("../../bubble-menu")

		expect(isBubbleMenuItemAllowedByContext).toBeTypeOf("function")
	})

	it("loads for a consumer that reaches the package through the drag handle config", async ({
		expect,
	}) => {
		vi.resetModules()

		const { DRAGGABLE_NODE_TYPES } = await import("../../drag-handle/config")

		expect(
			DRAGGABLE_NODE_TYPES.has(SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME),
		).toBe(true)
	})
})

// a hierarchy that puts a heading two levels below the split
// documentation, which the real schema never produces — the left side's
// grandparent guard exists for exactly that shape
const nestedHeadingSchema = new Schema({
	nodes: {
		doc: { content: `${SPLIT_DOCUMENTATION_NAME}+` },
		[SPLIT_DOCUMENTATION_NAME]: { content: "wrapper+" },
		wrapper: { content: "inner+" },
		inner: { content: "heading+" },
		heading: { content: "text*" },
		text: {},
	},
})

function fragmentOf(children: JSONContent[]): Fragment {
	return Fragment.from(children.map((json) => schema.nodeFromJSON(json)))
}

function pressLeftSideBackspace(
	node: PMNode,
	cursorPos: number,
): { editor: Editor; handled: boolean } {
	const editor = makeEditor(node, cursorPos)
	const backspace = nodeKeyboardShortcut(
		SplitDocumentationLeftSide,
		editor,
		"Backspace",
		schema,
	)

	return { editor, handled: backspace({ editor }) }
}

describe("SplitDocumentationLeftSide", () => {
	it("stays out of every other content group and keeps its own context", ({
		expect,
	}) => {
		expect(nodeType(SPLIT_DOCUMENTATION_LEFT_SIDE_NAME).spec).toMatchObject({
			group: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
			isolating: false,
			defining: true,
			selectable: false,
		})
	})

	it.for([
		{
			name: "accepts a heading followed by a paragraph",
			makeChildren: () => [heading("Title"), paragraph("body")],
			valid: true,
		},
		{
			name: "accepts lists and callouts as body blocks",
			makeChildren: () => [
				heading("Title"),
				bulletList("one"),
				callout("note"),
			],
			valid: true,
		},
		{
			name: "accepts parameter lists after the body blocks",
			makeChildren: () => [
				heading("Title"),
				paragraph("body"),
				paramList("Params", paramItem("id", "string", "the id")),
			],
			valid: true,
		},
		{
			name: "rejects a heading on its own",
			makeChildren: () => [heading("Title")],
			valid: false,
		},
		{
			name: "rejects body blocks without a leading heading",
			makeChildren: () => [paragraph("body")],
			valid: false,
		},
		{
			name: "rejects a parameter list standing in for the body blocks",
			makeChildren: () => [
				heading("Title"),
				paramList("Params", paramItem("id", "string", "the id")),
			],
			valid: false,
		},
		{
			name: "rejects a body block after a parameter list",
			makeChildren: () => [
				heading("Title"),
				paramList("Params", paramItem("id", "string", "the id")),
				paragraph("body"),
			],
			valid: false,
		},
		{
			name: "rejects right side blocks",
			makeChildren: () => [heading("Title"), titledCode("cmd", "run me")],
			valid: false,
		},
	])("$name", ({ makeChildren, valid }, { expect }) => {
		expect(
			nodeType(SPLIT_DOCUMENTATION_LEFT_SIDE_NAME).validContent(
				fragmentOf(makeChildren()),
			),
		).toBe(valid)
	})

	it("matches only left side markers when parsing html", ({ expect }) => {
		expect(
			nodeType(SPLIT_DOCUMENTATION_LEFT_SIDE_NAME).spec.parseDOM?.[0]?.tag,
		).toBe(`div[data-type="split-documentation-left-side"]`)
	})

	it("renders a div carrying the left side marker", ({ expect }) => {
		const node = nodeOf(defaultDoc(), SPLIT_DOCUMENTATION_LEFT_SIDE_NAME)

		expect(
			nodeType(SPLIT_DOCUMENTATION_LEFT_SIDE_NAME).spec.toDOM?.(node),
		).toEqual(["div", { "data-type": "split-documentation-left-side" }, 0])
	})

	it("renders through a vue node view", ({ expect }) => {
		const addNodeView = SplitDocumentationLeftSide.config.addNodeView as
			| (() => unknown)
			| undefined

		expect(addNodeView?.()).toBeTypeOf("function")
	})

	describe("Backspace", () => {
		it("deletes the whole split documentation from the start of its heading", ({
			expect,
		}) => {
			const node = defaultDoc()
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "Title"),
			)

			expect(handled).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)
			expect(childTypes(editor.state.doc)).toEqual(["paragraph"])
		})

		it("does nothing when the heading is not a direct child of the split documentation", ({
			expect,
		}) => {
			const nested = nestedHeadingSchema.nodeFromJSON({
				type: "doc",
				content: [
					{
						type: SPLIT_DOCUMENTATION_NAME,
						content: [
							{
								type: "wrapper",
								content: [
									{
										type: "inner",
										content: [
											{
												type: "heading",
												content: [{ type: "text", text: "Title" }],
											},
										],
									},
								],
							},
						],
					},
				],
			})
			const { editor, handled } = pressLeftSideBackspace(
				nested,
				startOfText(nested, "Title"),
			)

			expect(handled).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("swallows the key at the start of the only body block", ({ expect }) => {
			const node = defaultDoc()
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "body"),
			)

			expect(handled).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("swallows the key when only parameter lists follow the body block", ({
			expect,
		}) => {
			const node = doc(
				splitDocBlock(
					leftSide(
						heading("Title"),
						paragraph("body"),
						paramList("Params", paramItem("id", "string", "the id")),
					),
					rightSide(titledCode("cmd", "run me")),
				),
			)
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "body"),
			)

			expect(handled).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("lets the editor merge the body block when another one follows it", ({
			expect,
		}) => {
			const node = doc(
				splitDocBlock(
					leftSide(heading("Title"), paragraph("first"), paragraph("second")),
					rightSide(titledCode("cmd", "run me")),
				),
			)
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "first"),
			)

			expect(handled).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing at the start of a block nested in a callout", ({
			expect,
		}) => {
			const node = doc(
				splitDocBlock(
					leftSide(heading("Title"), callout("note")),
					rightSide(titledCode("cmd", "run me")),
				),
			)
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "note"),
			)

			expect(handled).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing when the cursor is not at the start of its block", ({
			expect,
		}) => {
			const node = defaultDoc()
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "body") + 1,
			)

			expect(handled).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing outside a split documentation", ({ expect }) => {
			const node = doc(paragraph("plain"))
			const { editor, handled } = pressLeftSideBackspace(
				node,
				startOfText(node, "plain"),
			)

			expect(handled).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})
	})
})

describe("SplitDocumentationRightSide", () => {
	it("stays out of every other content group and keeps its own context", ({
		expect,
	}) => {
		expect(nodeType(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME).spec).toMatchObject({
			group: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
			isolating: false,
			defining: true,
			selectable: false,
		})
	})

	it.for([
		{
			name: "accepts a single titled code block",
			makeChildren: () => [titledCode("cmd", "run me")],
			valid: true,
		},
		{
			name: "accepts titled code blocks and metric blocks in any order",
			makeChildren: () => [metric(), titledCode("cmd", "run me"), metric()],
			valid: true,
		},
		{
			name: "rejects an empty right side",
			makeChildren: () => [],
			valid: false,
		},
		{
			name: "rejects prose blocks",
			makeChildren: () => [paragraph("body")],
			valid: false,
		},
	])("$name", ({ makeChildren, valid }, { expect }) => {
		expect(
			nodeType(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME).validContent(
				fragmentOf(makeChildren()),
			),
		).toBe(valid)
	})

	it("matches only right side markers when parsing html", ({ expect }) => {
		expect(
			nodeType(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME).spec.parseDOM?.[0]?.tag,
		).toBe(`div[data-type="split-documentation-right-side"]`)
	})

	it("renders a div carrying the right side marker", ({ expect }) => {
		const node = nodeOf(defaultDoc(), SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME)

		expect(
			nodeType(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME).spec.toDOM?.(node),
		).toEqual(["div", { "data-type": "split-documentation-right-side" }, 0])
	})

	it("renders through a vue node view", ({ expect }) => {
		const addNodeView = SplitDocumentationRightSide.config.addNodeView as
			| (() => unknown)
			| undefined

		expect(addNodeView?.()).toBeTypeOf("function")
	})
})

describe("SplitDocumentationPlaceholderParents", () => {
	it("lists both sides and the parameter list item as parent placeholders", ({
		expect,
	}) => {
		expect(SplitDocumentationPlaceholderParents).toEqual([
			{ parent: true, name: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME },
			{ parent: true, name: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME },
			{ parent: true, name: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME },
		])
	})
})

// a left side whose heading is not its first child, which the real
// schema never produces — the Enter handler reaches for the sibling
// following the heading, and a trailing heading has none
const trailingHeadingSchema = new Schema({
	nodes: {
		doc: { content: `${SPLIT_DOCUMENTATION_NAME}+` },
		[SPLIT_DOCUMENTATION_NAME]: {
			content: `${SPLIT_DOCUMENTATION_LEFT_SIDE_NAME}+`,
		},
		[SPLIT_DOCUMENTATION_LEFT_SIDE_NAME]: { content: "(paragraph | heading)+" },
		paragraph: { content: "text*" },
		heading: { content: "text*" },
		text: {},
	},
})

describe("SplitDocumentation", () => {
	it("is a block level node holding one left and one right side", ({
		expect,
	}) => {
		expect(nodeType(SPLIT_DOCUMENTATION_NAME).spec).toMatchObject({
			group: "block",
			isolating: false,
			defining: true,
			selectable: false,
		})
	})

	it.for([
		{
			name: "accepts a left side followed by a right side",
			makeChildren: () => [
				leftSide(heading("Title"), paragraph("body")),
				rightSide(titledCode("cmd", "run me")),
			],
			valid: true,
		},
		{
			name: "rejects the sides in reverse order",
			makeChildren: () => [
				rightSide(titledCode("cmd", "run me")),
				leftSide(heading("Title"), paragraph("body")),
			],
			valid: false,
		},
		{
			name: "rejects a left side on its own",
			makeChildren: () => [leftSide(heading("Title"), paragraph("body"))],
			valid: false,
		},
		{
			name: "rejects a second right side",
			makeChildren: () => [
				leftSide(heading("Title"), paragraph("body")),
				rightSide(titledCode("cmd", "run me")),
				rightSide(titledCode("other", "run me too")),
			],
			valid: false,
		},
	])("$name", ({ makeChildren, valid }, { expect }) => {
		expect(
			nodeType(SPLIT_DOCUMENTATION_NAME).validContent(
				fragmentOf(makeChildren()),
			),
		).toBe(valid)
	})

	it("fills an empty block with a titled heading and a code block", ({
		expect,
	}) => {
		expect(
			nodeType(SPLIT_DOCUMENTATION_NAME).createAndFill()?.toJSON(),
		).toEqual({
			type: SPLIT_DOCUMENTATION_NAME,
			attrs: { inversed: false },
			content: [
				{
					type: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
					content: [
						{ type: "heading", attrs: { level: 1 } },
						{ type: "paragraph" },
					],
				},
				{
					type: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
					content: [
						{
							type: TITLED_CODE_BLOCK_NAME,
							content: [
								{ type: CODE_BLOCK_TITLE_NAME },
								{ type: CODE_BLOCK_NAME },
							],
						},
					],
				},
			],
		})
	})

	it.for([
		{
			name: "reads an inversed block back as inversed",
			value: "true",
			expected: true,
		},
		{
			name: "reads any other marker value as not inversed",
			value: "1",
			expected: false,
		},
	])("$name", ({ value, expected }, { expect }) => {
		expect(
			parseAttributes(nodeType(SPLIT_DOCUMENTATION_NAME), {
				"data-inversed": value,
			}),
		).toEqual({ inversed: expected })
	})

	it("reads a block without the marker as not inversed", ({ expect }) => {
		expect(parseAttributes(nodeType(SPLIT_DOCUMENTATION_NAME), {})).toEqual({
			inversed: false,
		})
	})

	it("omits the inversed marker while the sides are in their default order", ({
		expect,
	}) => {
		const node = nodeOf(defaultDoc(), SPLIT_DOCUMENTATION_NAME)

		expect(nodeType(SPLIT_DOCUMENTATION_NAME).spec.toDOM?.(node)).toEqual([
			"div",
			{ "data-type": "split-documentation" },
			0,
		])
	})

	it("renders the inversed marker back for an inversed block", ({ expect }) => {
		const node = nodeOf(
			doc(
				splitDocBlock(
					leftSide(heading("Title"), paragraph("body")),
					rightSide(titledCode("cmd", "run me")),
					true,
				),
			),
			SPLIT_DOCUMENTATION_NAME,
		)
		const rendered = nodeType(SPLIT_DOCUMENTATION_NAME).spec.toDOM?.(node) as [
			string,
			Record<string, string>,
			number,
		]

		expect(rendered).toEqual([
			"div",
			{ "data-inversed": "true", "data-type": "split-documentation" },
			0,
		])
		expect(
			parseAttributes(nodeType(SPLIT_DOCUMENTATION_NAME), rendered[1]),
		).toEqual(node.attrs)
	})

	it("matches only split documentation markers when parsing html", ({
		expect,
	}) => {
		expect(nodeType(SPLIT_DOCUMENTATION_NAME).spec.parseDOM?.[0]?.tag).toBe(
			`div[data-type="split-documentation"]`,
		)
	})

	it("renders through a vue node view", ({ expect }) => {
		const addNodeView = SplitDocumentation.config.addNodeView as
			| (() => unknown)
			| undefined

		expect(addNodeView?.()).toBeTypeOf("function")
	})

	describe("insertSplitDocumentation", () => {
		it("replaces the paragraph at the position with a filled block", ({
			expect,
		}) => {
			const node = doc(paragraph("||"), paragraph("after"))
			const run = runCommand(stateAt(node), "insertSplitDocumentation", [1])

			expect(run.handled).toBe(true)
			expect(run.dispatched).toBe(true)
			expect(childTypes(run.state.doc)).toEqual([
				SPLIT_DOCUMENTATION_NAME,
				"paragraph",
			])
			expect(run.state.selection.$from.parent.type.name).toBe("heading")
		})

		it("does nothing when the position is not inside a paragraph", ({
			expect,
		}) => {
			const node = doc(heading("Title"), paragraph("after"))
			const run = runCommand(stateAt(node), "insertSplitDocumentation", [1])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
			expect(childTypes(run.state.doc)).toEqual(["heading", "paragraph"])
		})

		it("reports the insertion as possible without dispatching it", ({
			expect,
		}) => {
			const node = doc(paragraph("||"), paragraph("after"))
			const run = runCommand(
				stateAt(node),
				"insertSplitDocumentation",
				[1],
				false,
			)

			expect(run.handled).toBe(true)
			expect(run.dispatched).toBe(false)
			expect(childTypes(run.state.doc)).toEqual(["paragraph", "paragraph"])
		})
	})

	describe("appendParameterListOnLeftSide", () => {
		it("appends an empty parameter list below the left side blocks", ({
			expect,
		}) => {
			const node = defaultDoc()
			const run = runCommand(stateAt(node), "appendParameterListOnLeftSide", [
				posOf(node, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
			])

			expect(run.handled).toBe(true)
			expect(
				childTypes(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME)),
			).toEqual([
				"heading",
				"paragraph",
				SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
			])
			expect(run.state.selection.$from.parent.type.name).toBe(
				SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
			)
		})

		it("does nothing when the position holds another node", ({ expect }) => {
			const node = defaultDoc()
			const run = runCommand(stateAt(node), "appendParameterListOnLeftSide", [
				posOf(node, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME),
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})
	})

	describe("invertSplitDocumentation", () => {
		it("marks the block at the given position as inversed", ({ expect }) => {
			const node = defaultDoc()
			const run = runCommand(stateAt(node), "invertSplitDocumentation", [
				posOf(node, SPLIT_DOCUMENTATION_NAME),
			])

			expect(run.handled).toBe(true)
			expect(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_NAME).attrs).toEqual({
				inversed: true,
			})
		})

		it("puts an already inversed block back into its default order", ({
			expect,
		}) => {
			const node = doc(
				splitDocBlock(
					leftSide(heading("Title"), paragraph("body")),
					rightSide(titledCode("cmd", "run me")),
					true,
				),
			)
			const run = runCommand(stateAt(node), "invertSplitDocumentation", [
				posOf(node, SPLIT_DOCUMENTATION_NAME),
			])

			expect(run.handled).toBe(true)
			expect(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_NAME).attrs).toEqual({
				inversed: false,
			})
		})

		it("falls back to the block around the cursor when the position holds another node", ({
			expect,
		}) => {
			const node = defaultDoc()
			const run = runCommand(
				stateAt(node, startOfText(node, "body")),
				"invertSplitDocumentation",
				[posOf(node, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME)],
			)

			expect(run.handled).toBe(true)
			expect(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_NAME).attrs).toEqual({
				inversed: true,
			})
		})

		it("inverts the block around the cursor when no position is given", ({
			expect,
		}) => {
			const node = defaultDoc()
			const run = runCommand(
				stateAt(node, startOfText(node, "cmd")),
				"invertSplitDocumentation",
				[],
			)

			expect(run.handled).toBe(true)
			expect(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_NAME).attrs).toEqual({
				inversed: true,
			})
		})

		it("does nothing when the cursor sits outside every split documentation", ({
			expect,
		}) => {
			const node = doc(paragraph("plain"))
			const run = runCommand(
				stateAt(node, startOfText(node, "plain")),
				"invertSplitDocumentation",
				[],
			)

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})
	})

	describe("appendBlockOnRightSide", () => {
		it("appends a titled code block and moves the cursor into its title", ({
			expect,
		}) => {
			const node = defaultDoc()
			const run = runCommand(stateAt(node), "appendBlockOnRightSide", [
				posOf(node, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME),
				"code",
			])

			expect(run.handled).toBe(true)
			expect(
				childTypes(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME)),
			).toEqual([TITLED_CODE_BLOCK_NAME, TITLED_CODE_BLOCK_NAME])
			expect(run.state.selection.$from.parent.type.name).toBe(
				CODE_BLOCK_TITLE_NAME,
			)
		})

		it("appends a metric block and leaves the cursor where it was", ({
			expect,
		}) => {
			const node = defaultDoc()
			const cursor = startOfText(node, "body")
			const run = runCommand(stateAt(node, cursor), "appendBlockOnRightSide", [
				posOf(node, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME),
				"metrics",
			])

			expect(run.handled).toBe(true)
			expect(
				childTypes(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME)),
			).toEqual([TITLED_CODE_BLOCK_NAME, METRIC_BLOCK_NAME])
			expect(run.state.selection.from).toBe(cursor)
		})

		it("does nothing when the position holds another node", ({ expect }) => {
			const node = defaultDoc()
			const run = runCommand(stateAt(node), "appendBlockOnRightSide", [
				posOf(node, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
				"code",
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})
	})

	describe("insertParameterListOnLeftSide", () => {
		it("inserts an empty list above the given one", ({ expect }) => {
			const node = docWithParameterList()
			const run = runCommand(stateAt(node), "insertParameterListOnLeftSide", [
				posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME),
				"above",
			])

			expect(run.handled).toBe(true)
			expect(listHeaders(run.state.doc)).toEqual(["", "Params"])
			expect(run.state.selection.$from.parent.type.name).toBe(
				SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
			)
		})

		it("inserts an empty list below the given one", ({ expect }) => {
			const node = docWithParameterList()
			const run = runCommand(stateAt(node), "insertParameterListOnLeftSide", [
				posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME),
				"below",
			])

			expect(run.handled).toBe(true)
			expect(listHeaders(run.state.doc)).toEqual(["Params", ""])
		})

		it("does nothing when the position holds another node", ({ expect }) => {
			const node = docWithParameterList()
			const run = runCommand(stateAt(node), "insertParameterListOnLeftSide", [
				posOf(node, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
				"above",
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})

		it("does nothing for a list outside any left side", ({ expect }) => {
			const node = doc(
				paramList("Params", paramItem("id", "string", "the id")),
				paragraph("after"),
			)
			const run = runCommand(stateAt(node), "insertParameterListOnLeftSide", [
				posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME),
				"above",
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})
	})

	describe("insertParameterListItemOnLeftSide", () => {
		it("inserts an empty item above the given one", ({ expect }) => {
			const node = docWithParameterList(
				paramItem("id", "string", "the id"),
				paramItem("name", "string", "the name"),
			)
			const run = runCommand(
				stateAt(node),
				"insertParameterListItemOnLeftSide",
				[posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME), "above"],
			)

			expect(run.handled).toBe(true)
			expect(itemTitles(run.state.doc)).toEqual(["", "id", "name"])
			expect(run.state.selection.$from.parent.type.name).toBe(
				SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
			)
		})

		it("inserts an empty item below the given one", ({ expect }) => {
			const node = docWithParameterList(
				paramItem("id", "string", "the id"),
				paramItem("name", "string", "the name"),
			)
			const run = runCommand(
				stateAt(node),
				"insertParameterListItemOnLeftSide",
				[posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME), "below"],
			)

			expect(run.handled).toBe(true)
			expect(itemTitles(run.state.doc)).toEqual(["id", "", "name"])
		})

		it("does nothing when the position holds another node", ({ expect }) => {
			const node = docWithParameterList()
			const run = runCommand(
				stateAt(node),
				"insertParameterListItemOnLeftSide",
				[posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME), "above"],
			)

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})

		it("does nothing for an item outside any parameter list", ({ expect }) => {
			const node = doc(paramItem("id", "string", "the id"), paragraph("after"))
			const run = runCommand(
				stateAt(node),
				"insertParameterListItemOnLeftSide",
				[posOf(node, SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME), "above"],
			)

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})
	})

	describe("insertBlockOnRightSide", () => {
		it("inserts a titled code block above the hovered one and selects its title", ({
			expect,
		}) => {
			const node = docWithTwoRightSideBlocks()
			const run = runCommand(stateAt(node), "insertBlockOnRightSide", [
				posOf(node, TITLED_CODE_BLOCK_NAME),
				"above",
				"code",
			])

			expect(run.handled).toBe(true)
			expect(
				childTypes(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME)),
			).toEqual([
				TITLED_CODE_BLOCK_NAME,
				TITLED_CODE_BLOCK_NAME,
				METRIC_BLOCK_NAME,
			])
			expect(run.state.selection.$from.parent.type.name).toBe(
				CODE_BLOCK_TITLE_NAME,
			)
		})

		it("inserts a metric block below the hovered one and leaves the cursor where it was", ({
			expect,
		}) => {
			const node = docWithTwoRightSideBlocks()
			const cursor = startOfText(node, "body")
			const run = runCommand(stateAt(node, cursor), "insertBlockOnRightSide", [
				posOf(node, TITLED_CODE_BLOCK_NAME),
				"below",
				"metrics",
			])

			expect(run.handled).toBe(true)
			expect(
				childTypes(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME)),
			).toEqual([TITLED_CODE_BLOCK_NAME, METRIC_BLOCK_NAME, METRIC_BLOCK_NAME])
			expect(run.state.selection.from).toBe(cursor)
		})

		it("finds the right side from a hovered metric block, which carries no content", ({
			expect,
		}) => {
			const node = docWithTwoRightSideBlocks()
			const run = runCommand(stateAt(node), "insertBlockOnRightSide", [
				posOf(node, METRIC_BLOCK_NAME),
				"above",
				"code",
			])

			expect(run.handled).toBe(true)
			expect(
				childTypes(nodeOf(run.state.doc, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME)),
			).toEqual([
				TITLED_CODE_BLOCK_NAME,
				TITLED_CODE_BLOCK_NAME,
				METRIC_BLOCK_NAME,
			])
		})

		it("does nothing when the position holds a block of another type", ({
			expect,
		}) => {
			const node = docWithTwoRightSideBlocks()
			const run = runCommand(stateAt(node), "insertBlockOnRightSide", [
				posOf(node, "paragraph"),
				"above",
				"code",
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})

		it("does nothing when the position holds no node at all", ({ expect }) => {
			const node = docWithTwoRightSideBlocks()
			const run = runCommand(stateAt(node), "insertBlockOnRightSide", [
				node.content.size,
				"above",
				"code",
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})

		it("does nothing for a block outside any right side", ({ expect }) => {
			const node = doc(titledCode("cmd", "run me"), paragraph("after"))
			const run = runCommand(stateAt(node), "insertBlockOnRightSide", [
				posOf(node, TITLED_CODE_BLOCK_NAME),
				"above",
				"code",
			])

			expect(run.handled).toBe(false)
			expect(run.dispatched).toBe(false)
		})
	})

	describe("Mod-e", () => {
		it("appends a parameter list when the cursor is on the left side", ({
			expect,
		}) => {
			const node = defaultDoc()
			const { editor, appendParameterListOnLeftSide, appendBlockOnRightSide } =
				makeShortcutEditor(node, startOfText(node, "body"))
			const modE = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Mod-e",
				schema,
			)

			expect(modE({ editor })).toBe(true)
			expect(appendParameterListOnLeftSide).toHaveBeenCalledExactlyOnceWith(
				posOf(node, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
			)
			expect(appendBlockOnRightSide).toHaveBeenCalledTimes(0)
		})

		it("appends a code block when the cursor is on the right side", ({
			expect,
		}) => {
			const node = defaultDoc()
			const { editor, appendParameterListOnLeftSide, appendBlockOnRightSide } =
				makeShortcutEditor(node, startOfText(node, "run me"))
			const modE = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Mod-e",
				schema,
			)

			expect(modE({ editor })).toBe(true)
			expect(appendBlockOnRightSide).toHaveBeenCalledExactlyOnceWith(
				posOf(node, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME),
				"code",
			)
			expect(appendParameterListOnLeftSide).toHaveBeenCalledTimes(0)
		})

		it("does nothing outside a split documentation", ({ expect }) => {
			const node = doc(paragraph("plain"))
			const { editor, appendParameterListOnLeftSide, appendBlockOnRightSide } =
				makeShortcutEditor(node, startOfText(node, "plain"))
			const modE = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Mod-e",
				schema,
			)

			expect(modE({ editor })).toBe(false)
			expect(appendParameterListOnLeftSide).toHaveBeenCalledTimes(0)
			expect(appendBlockOnRightSide).toHaveBeenCalledTimes(0)
		})
	})

	describe("Mod-m", () => {
		it("appends a metric block when the cursor is on the right side", ({
			expect,
		}) => {
			const node = defaultDoc()
			const { editor, appendParameterListOnLeftSide, appendBlockOnRightSide } =
				makeShortcutEditor(node, startOfText(node, "run me"))
			const modM = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Mod-m",
				schema,
			)

			expect(modM({ editor })).toBe(true)
			expect(appendBlockOnRightSide).toHaveBeenCalledExactlyOnceWith(
				posOf(node, SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME),
				"metrics",
			)
			expect(appendParameterListOnLeftSide).toHaveBeenCalledTimes(0)
		})

		it("does nothing when the cursor is on the left side", ({ expect }) => {
			const node = defaultDoc()
			const { editor, appendParameterListOnLeftSide, appendBlockOnRightSide } =
				makeShortcutEditor(node, startOfText(node, "body"))
			const modM = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Mod-m",
				schema,
			)

			expect(modM({ editor })).toBe(false)
			expect(appendBlockOnRightSide).toHaveBeenCalledTimes(0)
			expect(appendParameterListOnLeftSide).toHaveBeenCalledTimes(0)
		})

		it("does nothing outside a split documentation", ({ expect }) => {
			const node = doc(paragraph("plain"))
			const { editor, appendParameterListOnLeftSide, appendBlockOnRightSide } =
				makeShortcutEditor(node, startOfText(node, "plain"))
			const modM = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Mod-m",
				schema,
			)

			expect(modM({ editor })).toBe(false)
			expect(appendBlockOnRightSide).toHaveBeenCalledTimes(0)
			expect(appendParameterListOnLeftSide).toHaveBeenCalledTimes(0)
		})
	})

	describe("Enter", () => {
		it("moves the cursor into the empty paragraph already below the heading", ({
			expect,
		}) => {
			const node = doc(
				splitDocBlock(
					leftSide(heading("Title"), paragraph()),
					rightSide(titledCode("cmd", "run me")),
				),
			)
			const editor = makeEditor(
				node,
				startOfText(node, "Title") + "Title".length,
			)
			const enter = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Enter",
				schema,
			)

			expect(enter({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)
			expect(
				childTypes(
					nodeOf(editor.state.doc, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
				),
			).toEqual(["heading", "paragraph"])
			expect(editor.state.selection.$from.parent.type.name).toBe("paragraph")
		})

		it("inserts an empty paragraph when the next block already has text", ({
			expect,
		}) => {
			const node = defaultDoc()
			const editor = makeEditor(
				node,
				startOfText(node, "Title") + "Title".length,
			)
			const enter = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Enter",
				schema,
			)

			expect(enter({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)

			const left = nodeOf(editor.state.doc, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME)

			expect(childTypes(left)).toEqual(["heading", "paragraph", "paragraph"])
			expect(left.child(1).textContent).toBe("")
			expect(editor.state.selection.$from.parent.textContent).toBe("")
		})

		it("inserts an empty paragraph when the next block is a list", ({
			expect,
		}) => {
			const node = doc(
				splitDocBlock(
					leftSide(heading("Title"), bulletList("one")),
					rightSide(titledCode("cmd", "run me")),
				),
			)
			const editor = makeEditor(
				node,
				startOfText(node, "Title") + "Title".length,
			)
			const enter = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Enter",
				schema,
			)

			expect(enter({ editor })).toBe(true)
			expect(
				childTypes(
					nodeOf(editor.state.doc, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
				),
			).toEqual(["heading", "paragraph", "bulletList"])
		})

		it("inserts an empty paragraph when the heading is the last block", ({
			expect,
		}) => {
			const node = trailingHeadingSchema.nodeFromJSON({
				type: "doc",
				content: [
					{
						type: SPLIT_DOCUMENTATION_NAME,
						content: [
							{
								type: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
								content: [
									paragraph("body"),
									{
										type: "heading",
										content: [{ type: "text", text: "Title" }],
									},
								],
							},
						],
					},
				],
			})
			const editor = makeEditor(
				node,
				startOfText(node, "Title") + "Title".length,
			)
			const enter = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Enter",
				trailingHeadingSchema,
			)

			expect(enter({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)
			expect(
				childTypes(
					nodeOf(editor.state.doc, SPLIT_DOCUMENTATION_LEFT_SIDE_NAME),
				),
			).toEqual(["paragraph", "heading", "paragraph"])
			expect(editor.state.selection.$from.parent.textContent).toBe("")
		})

		it("does nothing when the cursor is not inside a heading", ({ expect }) => {
			const node = defaultDoc()
			const editor = makeEditor(node, startOfText(node, "body"))
			const enter = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Enter",
				schema,
			)

			expect(enter({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing for a heading outside a left side", ({ expect }) => {
			const node = doc(heading("Top"), paragraph("body"))
			const editor = makeEditor(node, startOfText(node, "Top") + "Top".length)
			const enter = nodeKeyboardShortcut(
				SplitDocumentation,
				editor,
				"Enter",
				schema,
			)

			expect(enter({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})
	})
})

describe("setUpSplitDocumentationNode", () => {
	it("replaces the paragraph holding the position with a filled block", ({
		expect,
	}) => {
		const state = stateAt(doc(paragraph("||"), paragraph("after")))
		const tr = state.tr

		expect(setUpSplitDocumentationNode(state, tr, 1)).toBe(true)
		expect(childTypes(tr.doc)).toEqual([SPLIT_DOCUMENTATION_NAME, "paragraph"])
		expect(tr.selection.$from.parent.type.name).toBe("heading")
	})

	it("leaves the transaction untouched when the position is not in a paragraph", ({
		expect,
	}) => {
		const state = stateAt(doc(heading("Title"), paragraph("after")))
		const tr = state.tr

		expect(setUpSplitDocumentationNode(state, tr, 1)).toBe(false)
		expect(tr.docChanged).toBe(false)
	})

	it("leaves the transaction untouched when the schema has no split documentation", ({
		expect,
	}) => {
		const bare = new Schema({
			nodes: {
				doc: { content: "block+" },
				paragraph: { group: "block", content: "inline*" },
				text: { group: "inline" },
			},
		})
		const state = EditorState.create({
			doc: bare.nodeFromJSON({
				type: "doc",
				content: [
					{ type: "paragraph", content: [{ type: "text", text: "||" }] },
				],
			}),
		})
		const tr = state.tr

		expect(setUpSplitDocumentationNode(state, tr, 1)).toBe(false)
		expect(tr.docChanged).toBe(false)
	})
})

describe("insertSplitDocumentation", () => {
	it("replaces the trigger range with a block and puts the cursor at its start", ({
		expect,
	}) => {
		const { editor, calls } = chainingEditor()

		insertSplitDocumentation(editor, { from: 4, to: 9 })

		expect(calls).toEqual([
			["focus"],
			["deleteRange", { from: 4, to: 9 }],
			["insertSplitDocumentation", 4],
			["setTextSelection", 4],
			["run"],
		])
	})
})

import type { Editor, JSONContent, Range, SingleCommands } from "@tiptap/core"
import { getSchema } from "@tiptap/core"
import Document from "@tiptap/extension-document"
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
import { Fragment } from "@tiptap/pm/model"
import { describe, it, vi } from "vitest"
import { CALLOUT_BLOCK_NAME } from "../node-names"
import { CalloutBlock, setUpCalloutBlockNode } from "."
import {
	jsonDocBuilder,
	makeCursorEditor,
	nodeKeyboardShortcut,
	nodeType,
	paragraph,
	parseAttributes,
	startOfText,
} from "~/components/editor/test-helpers"

const schema = getSchema([
	Document,
	Text,
	Paragraph,
	BulletList,
	OrderedList,
	ListItem,
	TaskList,
	TaskItem,
	CalloutBlock,
])

function calloutType(): NodeType {
	return nodeType(schema, CALLOUT_BLOCK_NAME)
}

const docNode = jsonDocBuilder(schema)

function callout(...content: JSONContent[]): JSONContent {
	return { type: CALLOUT_BLOCK_NAME, content }
}

function pressBackspace(editor: Editor): boolean {
	return nodeKeyboardShortcut(
		CalloutBlock,
		editor,
		"Backspace",
		schema,
	)({
		editor,
	})
}

function topLevelTypes(doc: PMNode): string[] {
	const names: string[] = []

	doc.forEach((child) => {
		names.push(child.type.name)
	})

	return names
}

describe("CalloutBlock", () => {
	it("defines an isolating, defining, unselectable block", ({ expect }) => {
		expect(calloutType().spec).toMatchObject({
			content: "(paragraph | bulletList | orderedList | taskList)+",
			defining: true,
			selectable: false,
			isolating: true,
			group: "block",
		})
	})

	it("matches only callout block markers when parsing html", ({ expect }) => {
		expect(calloutType().spec.parseDOM?.[0]?.tag).toBe(
			`div[data-type="callout-block"]`,
		)
	})

	it.for([
		{ name: "accepts a paragraph", child: paragraph("hi"), valid: true },
		{
			name: "accepts a bullet list",
			child: {
				type: "bulletList",
				content: [{ type: "listItem", content: [paragraph("hi")] }],
			},
			valid: true,
		},
		{
			name: "accepts a task list",
			child: {
				type: "taskList",
				content: [{ type: "taskItem", content: [paragraph("hi")] }],
			},
			valid: true,
		},
		{
			name: "rejects a nested callout",
			child: callout(paragraph("hi")),
			valid: false,
		},
	])("$name as content", ({ child, valid }, { expect }) => {
		expect(
			calloutType().validContent(Fragment.from(schema.nodeFromJSON(child))),
		).toBe(valid)
	})

	it("rejects an empty callout", ({ expect }) => {
		expect(calloutType().validContent(Fragment.empty)).toBe(false)
	})

	it("defaults the icon and leaves the previous icon unset", ({ expect }) => {
		expect(calloutType().createAndFill()?.attrs).toEqual({
			icon: "lucide:text",
			previousIcon: null,
		})
	})

	it("renders the icon and the previous icon as data attributes", ({
		expect,
	}) => {
		const node = calloutType().createAndFill({
			icon: "lucide:star",
			previousIcon: "lucide:text",
		})
		if (!node) {
			throw new Error("createAndFill produced no node")
		}

		expect(calloutType().spec.toDOM?.(node)).toEqual([
			"div",
			{
				"data-icon": "lucide:star",
				"data-previous-icon": "lucide:text",
				"data-type": "callout-block",
			},
			0,
		])
	})

	it("omits the previous icon when it is unset", ({ expect }) => {
		const node = calloutType().createAndFill()
		if (!node) {
			throw new Error("createAndFill produced no node")
		}

		expect(calloutType().spec.toDOM?.(node)).toEqual([
			"div",
			{ "data-icon": "lucide:text", "data-type": "callout-block" },
			0,
		])
	})

	it("parses both icons back off the data attributes", ({ expect }) => {
		expect(
			parseAttributes(calloutType(), {
				"data-icon": "lucide:star",
				"data-previous-icon": "lucide:text",
			}),
		).toEqual({ icon: "lucide:star", previousIcon: "lucide:text" })
	})

	// tiptap drops null results from an attribute parser, so the node
	// falls back to the attribute defaults
	it("parses an element without data attributes into no attributes", ({
		expect,
	}) => {
		expect(parseAttributes(calloutType(), {})).toEqual({})
	})

	describe("Backspace", () => {
		it("deletes the callout when pressed at the start of its only child", ({
			expect,
		}) => {
			const doc = docNode(paragraph("before"), callout(paragraph("inside")))
			const { editor, dispatch, state } = makeCursorEditor(
				doc,
				startOfText(doc, "inside"),
			)

			expect(pressBackspace(editor)).toBe(true)
			expect(dispatch).toHaveBeenCalledTimes(1)
			expect(topLevelTypes(state().doc)).toEqual(["paragraph"])
		})

		it("keeps a callout that holds more than one child", ({ expect }) => {
			const doc = docNode(callout(paragraph("first"), paragraph("second")))
			const { editor, dispatch, state } = makeCursorEditor(
				doc,
				startOfText(doc, "first"),
			)

			expect(pressBackspace(editor)).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
			expect(topLevelTypes(state().doc)).toEqual([CALLOUT_BLOCK_NAME])
		})

		it("does nothing when the cursor is not at the child start", ({
			expect,
		}) => {
			const doc = docNode(callout(paragraph("inside")))
			const { editor, dispatch } = makeCursorEditor(
				doc,
				startOfText(doc, "inside") + 2,
			)

			expect(pressBackspace(editor)).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing at the start of a top-level paragraph", ({ expect }) => {
			const doc = docNode(paragraph("outside"))
			const { editor, dispatch } = makeCursorEditor(
				doc,
				startOfText(doc, "outside"),
			)

			expect(pressBackspace(editor)).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		// the handler only looks one level up, so a list nested in the
		// callout shields the callout from the deletion
		it("does nothing at the start of a list item inside the callout", ({
			expect,
		}) => {
			const doc = docNode(
				callout({
					type: "bulletList",
					content: [{ type: "listItem", content: [paragraph("item")] }],
				}),
			)
			const { editor, dispatch } = makeCursorEditor(
				doc,
				startOfText(doc, "item"),
			)

			expect(pressBackspace(editor)).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})
	})
})

describe("setUpCalloutBlockNode", () => {
	it("replaces the range with a callout holding one empty paragraph", ({
		expect,
	}) => {
		const range: Range = { from: 3, to: 8 }
		const commands = {
			deleteRange: vi.fn(),
			insertContent: vi.fn(),
		} as unknown as SingleCommands

		setUpCalloutBlockNode(range, commands)

		expect(commands.deleteRange).toHaveBeenCalledTimes(1)
		expect(commands.deleteRange).toHaveBeenCalledWith(range)
		expect(commands.insertContent).toHaveBeenCalledTimes(1)
		expect(commands.insertContent).toHaveBeenCalledWith({
			type: CALLOUT_BLOCK_NAME,
			content: [{ type: "paragraph" }],
		})
	})
})

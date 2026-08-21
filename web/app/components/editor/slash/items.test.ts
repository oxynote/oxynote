import type { Editor, JSONContent } from "@tiptap/core"
import { Schema } from "@tiptap/pm/model"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import {
	CALLOUT_BLOCK_NAME,
	CODE_BLOCK_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
} from "../blocks/node-names"
import {
	allowSlashItemsByContext,
	CommandGroup,
	commandGroupSortIndex,
	filterSlashItems,
} from "./items"
import { paragraph } from "~/components/editor/test-helpers"

// minimal stand-ins for the real editor nodes — the context whitelist
// only inspects ancestor node names, so the content rules can stay
// loose as long as the names match the real extensions
const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		heading: { group: "block", content: "inline*" },
		bulletList: { group: "block", content: "block+" },
		orderedList: { group: "block", content: "block+" },
		taskList: { group: "block", content: "block+" },
		[CODE_BLOCK_NAME]: { group: "block", content: "text*" },
		[CALLOUT_BLOCK_NAME]: { group: "block", content: "block+" },
		[SPLIT_DOCUMENTATION_LEFT_SIDE_NAME]: {
			group: "block",
			content: "block+",
		},
		[SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME]: {
			group: "block",
			content: "block+",
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME]: {
			group: "block",
			content: "block+",
		},
		text: { group: "inline" },
	},
})

function block(type: string, ...content: JSONContent[]): JSONContent {
	return { type, content }
}

// one document containing every context the whitelist distinguishes;
// each text is unique so a cursor can be addressed by it
const pmDoc = schema.nodeFromJSON({
	type: "doc",
	content: [
		paragraph("top"),
		{ type: CODE_BLOCK_NAME, content: [{ type: "text", text: "code" }] },
		block("bulletList", paragraph("bullet")),
		block("orderedList", paragraph("ordered")),
		block("taskList", paragraph("task")),
		block(CALLOUT_BLOCK_NAME, paragraph("callout")),
		block(
			SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
			paragraph("left"),
			block("heading", { type: "text", text: "left-heading" }),
			block("bulletList", paragraph("left-bullet")),
			block(SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME, paragraph("left-param")),
		),
		block(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, paragraph("right")),
	],
})

// places the cursor at the start of the text node with the given text
function stateAt(text: string): EditorState {
	let found = -1

	pmDoc.descendants((node, pos) => {
		if (found !== -1) {
			return false
		}

		if (node.isText && node.text === text) {
			found = pos
			return false
		}

		return true
	})

	if (found === -1) {
		throw new Error(`text "${text}" not found in the test document`)
	}

	return EditorState.create({
		doc: pmDoc,
		selection: TextSelection.create(pmDoc, found),
	})
}

function titles(query: string, cursorText: string): string[] {
	const editor = { state: stateAt(cursorText) } as unknown as Editor

	return filterSlashItems({ query, editor }).map((item) => item.title)
}

describe("commandGroupSortIndex", () => {
	it.for([
		{ group: CommandGroup.Text, expected: 0 },
		{ group: CommandGroup.List, expected: 1 },
		{ group: CommandGroup.BasicBlock, expected: 2 },
		{ group: CommandGroup.PowerBlock, expected: 3 },
	])(
		"sorts the $group group at index $expected",
		({ group, expected }, { expect }) => {
			expect(commandGroupSortIndex(group)).toBe(expected)
		},
	)
})

describe("allowSlashItemsByContext", () => {
	it.for([
		{ name: "allows items in a top-level paragraph", text: "top" },
		{ name: "allows items inside a callout", text: "callout" },
		{
			name: "allows items in a split documentation left side paragraph",
			text: "left",
		},
	])("$name", ({ text }, { expect }) => {
		expect(allowSlashItemsByContext(stateAt(text))).toBe(true)
	})

	it.for([
		{ name: "blocks items inside a code block", text: "code" },
		{ name: "blocks items inside a bulleted list", text: "bullet" },
		{ name: "blocks items inside a numbered list", text: "ordered" },
		{ name: "blocks items inside a checklist", text: "task" },
		{
			name: "blocks items in a split documentation left side heading",
			text: "left-heading",
		},
		{
			name: "blocks items in a list nested in a left side",
			text: "left-bullet",
		},
		{
			name: "blocks items in a parameter list nested in a left side",
			text: "left-param",
		},
		{
			name: "blocks items in a split documentation right side",
			text: "right",
		},
	])("$name", ({ text }, { expect }) => {
		expect(allowSlashItemsByContext(stateAt(text))).toBe(false)
	})
})

describe("filterSlashItems", () => {
	it("returns every command group, already ordered, for an empty query at the top level", ({
		expect,
	}) => {
		const editor = { state: stateAt("top") } as unknown as Editor
		const items = filterSlashItems({ query: "", editor })

		expect(new Set(items.map((item) => item.group))).toEqual(
			new Set([
				CommandGroup.Text,
				CommandGroup.List,
				CommandGroup.BasicBlock,
				CommandGroup.PowerBlock,
			]),
		)

		const indexes = items.map((item) => commandGroupSortIndex(item.group))
		expect(indexes).toEqual([...indexes].sort((a, b) => a - b))
	})

	it("matches the query case-insensitively", ({ expect }) => {
		expect(titles("HEAD", "top")).toEqual([
			"Heading 1",
			"Heading 2",
			"Heading 3",
		])
	})

	it("matches the query anywhere in the title", ({ expect }) => {
		expect(titles("list", "top")).toEqual([
			"Bulleted list",
			"Numbered list",
			"Checklist",
		])
	})

	it("returns no items for an unmatched query", ({ expect }) => {
		expect(titles("zzz", "top")).toEqual([])
	})

	it("offers only list and callout items inside a split documentation left side", ({
		expect,
	}) => {
		expect(titles("", "left")).toEqual([
			"Bulleted list",
			"Numbered list",
			"Checklist",
			"Callout",
		])
	})

	it("offers only list items inside a callout", ({ expect }) => {
		expect(titles("", "callout")).toEqual([
			"Bulleted list",
			"Numbered list",
			"Checklist",
		])
	})

	it("returns no items inside a code block even for a matching query", ({
		expect,
	}) => {
		expect(titles("heading", "code")).toEqual([])
	})

	it("hides context-blocked items even when the query matches them", ({
		expect,
	}) => {
		expect(titles("callout", "callout")).toEqual([])
	})

	it.for([
		{ title: "Heading 1", level: 1 },
		{ title: "Heading 2", level: 2 },
		{ title: "Heading 3", level: 3 },
	])(
		"returns a $title command that sets heading level $level",
		({ title, level }, { expect }) => {
			const setNode = vi.fn(() => chain)
			const chain = {
				focus: () => chain,
				deleteRange: () => chain,
				setNode,
				run: () => true,
			}

			const state = stateAt("top")
			const editor = {
				state,
				extensionManager: { extensions: [] },
				chain: () => chain,
			} as unknown as Editor

			const item = filterSlashItems({ query: title, editor }).find(
				(v) => v.title === title,
			)
			if (!item) {
				throw new Error(`the ${title} item is missing from the slash menu`)
			}

			item.command({
				editor,
				range: { from: state.selection.from, to: state.selection.from },
			})

			expect(setNode).toHaveBeenCalledTimes(1)
			expect(setNode).toHaveBeenCalledWith("heading", { level })
		},
	)
})

import type { AnyExtension, InputRule, JSONContent } from "@tiptap/core"
import { Editor, getExtensionField } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { Schema } from "@tiptap/pm/model"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import Heading from "@tiptap/extension-heading"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import {
	BulletList,
	OrderedList,
	TaskItem,
	TaskList,
} from "@tiptap/extension-list"
import { InputRules } from "../input-utils"
import { extensionContext } from "../test-helpers/editor"
import {
	CALLOUT_BLOCK_NAME,
	CODE_BLOCK_NAME,
	FILE_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
} from "../blocks/node-names"
import { FileBlock } from "../blocks/file"
import { ImageBlock } from "../blocks/image"
import {
	allItems,
	allowSlashItemsByContext,
	CommandGroup,
	commandGroupSortIndex,
	filterSlashItems,
} from "./items"
import { paragraph } from "~/components/editor/test-helpers"
import messages from "~/../i18n/locales/en/editor.json"

// node tests have no i18n runtime; titles resolve straight from the
// locale file the app ships
function t(key: string): string {
	const message = key
		.split(".")
		.reduce<unknown>(
			(node, part) =>
				node && typeof node === "object"
					? (node as Record<string, unknown>)[part]
					: undefined,
			messages,
		)
	if (typeof message !== "string") {
		throw new Error(`no translation for "${key}"`)
	}

	return message
}

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

	return filterSlashItems({ query, editor, t }).map((item) =>
		t(item.titleI18nKey),
	)
}

// a live editor holding the two upload atoms, for the commands that
// replace the paragraph the menu was opened in
function makeUploadEditor(): Editor {
	return new Editor({
		extensions: [Document, Text, Paragraph, ImageBlock, FileBlock],
		content: {
			type: "doc",
			content: [paragraph("before"), paragraph("/"), paragraph("after")],
		},
	})
}

function topLevelTypes(editor: Editor): string[] {
	const types: string[] = []

	editor.state.doc.forEach((child) => {
		types.push(child.type.name)
	})

	return types
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
		const items = filterSlashItems({ query: "", editor, t })

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

			const item = filterSlashItems({ query: title, editor, t }).find(
				(v) => t(v.titleI18nKey) === title,
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

	it.for([
		{ title: "File", expected: FILE_BLOCK_NAME },
		{ title: "Image", expected: IMAGE_BLOCK_NAME },
	])(
		"returns a $title command that swaps the paragraph for an empty $expected",
		({ title, expected }, { expect }) => {
			const editor = makeUploadEditor()
			// the slash was typed in the middle paragraph
			editor.commands.setTextSelection(10)
			const from = editor.state.selection.from

			const item = filterSlashItems({ query: title, editor, t }).find(
				(v) => t(v.titleI18nKey) === title,
			)
			if (!item) {
				throw new Error(`the ${title} item is missing from the slash menu`)
			}

			item.command({ editor, range: { from, to: from } })

			expect(topLevelTypes(editor)).toEqual([
				"paragraph",
				expected,
				"paragraph",
			])
			expect(editor.state.doc.child(1).attrs.src).toBeNull()
			expect(editor.state.doc.child(1).attrs.uploading).toBe(false)
		},
	)

	it("returns a File command that leaves a schema without the block alone", ({
		expect,
	}) => {
		const editor = new Editor({
			extensions: [Document, Text, Paragraph],
			content: { type: "doc", content: [paragraph("/")] },
		})
		editor.commands.setTextSelection(1)

		const item = filterSlashItems({ query: "File", editor, t }).find(
			(v) => t(v.titleI18nKey) === "File",
		)
		if (!item) {
			throw new Error("the File item is missing from the slash menu")
		}

		item.command({ editor, range: { from: 1, to: 1 } })

		expect(topLevelTypes(editor)).toEqual(["paragraph"])
	})
})

// every input rule that can turn typed markdown into a block, built the
// way the editor builds them: the heading levels the editor allows, the
// list rules (the checklist one sits on the item, not the list), the
// divider, and the custom block rules
function blockInputRules(): InputRule[] {
	const sources: [AnyExtension, string][] = [
		[Heading.configure({ levels: [1, 2, 3] }), Heading.name],
		[BulletList, BulletList.name],
		[OrderedList, OrderedList.name],
		[TaskList, TaskList.name],
		[TaskItem.configure({ nested: true }), TaskItem.name],
		[HorizontalRule, HorizontalRule.name],
		[InputRules, InputRules.name],
	]

	return sources.flatMap(([extension, name]) => {
		const build = getExtensionField<(() => InputRule[]) | undefined>(
			extension,
			"addInputRules",
			extensionContext(extension, name),
		)

		return build?.() ?? []
	})
}

function matchedBySomeRule(rules: InputRule[], text: string): boolean {
	return rules.some((rule) =>
		typeof rule.find === "function"
			? rule.find(text) !== null
			: rule.find.test(text),
	)
}

describe("allItems", () => {
	it("describes every command", ({ expect }) => {
		allItems.forEach((item) => {
			expect(t(item.titleI18nKey).length).toBeGreaterThan(0)
			expect(t(item.descriptionI18nKey).length).toBeGreaterThan(0)
		})
	})

	// the shortcut field is shown as "type this to get the same block", so
	// it has to be text an input rule actually reacts to, trailing space
	// included where the rule waits for one
	it.for(allItems.filter((item) => item.shortcut))(
		"types markdown the editor recognises for $titleI18nKey",
		({ shortcut }, { expect }) => {
			expect(matchedBySomeRule(blockInputRules(), shortcut ?? "")).toBe(true)
		},
	)
})

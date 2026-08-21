// the split-documentation package cycle must be entered through the
// index, as app code does — see the matching note in test-helpers.ts
import "."

import type { NodeType, Node as PMNode } from "@tiptap/pm/model"
import { Fragment } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import {
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
} from "../node-names"
import {
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeaderTitle,
} from "./parameter-list"
import {
	docNode,
	makeEditor,
	nodeKeyboardShortcut,
	paramItem,
	paramList,
	paramListSchema,
	splitDoc,
} from "./test-helpers"
import {
	nodeType as nodeTypeOf,
	startOfText,
} from "~/components/editor/test-helpers"

function nodeType(name: string): NodeType {
	return nodeTypeOf(paramListSchema, name)
}

function firstParamList(doc: PMNode): PMNode | null {
	let found: PMNode | null = null

	doc.descendants((node) => {
		if (found) {
			return false
		}

		if (node.type.name === SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME) {
			found = node
			return false
		}

		return true
	})

	return found
}

// compresses the first parameter list's items into readable
// { title, type, body } rows
function itemsShape(
	doc: PMNode,
): { title: string; type: string; body: string }[] {
	const list = firstParamList(doc)
	if (!list) {
		throw new Error("no parameter list in the test document")
	}

	const items: { title: string; type: string; body: string }[] = []

	list.forEach((child) => {
		if (child.type.name !== SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME) {
			return
		}

		const header = child.child(0)

		items.push({
			title: header.child(0).textContent,
			type: header.child(1).textContent,
			body: child.child(1).textContent,
		})
	})

	return items
}

describe("ParameterListItemHeaderTitle", () => {
	describe("Backspace", () => {
		it("deletes the item when pressed at the start of its title", ({
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
			const editor = makeEditor(doc, startOfText(doc, "name"))
			const backspace = nodeKeyboardShortcut(
				ParameterListItemHeaderTitle,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)
			expect(itemsShape(editor.state.doc)).toEqual([
				{ title: "id", type: "string", body: "the id" },
			])
		})

		it("keeps the last remaining item but still handles the key", ({
			expect,
		}) => {
			const doc = docNode(
				splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
			)
			const editor = makeEditor(doc, startOfText(doc, "id"))
			const backspace = nodeKeyboardShortcut(
				ParameterListItemHeaderTitle,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
			expect(itemsShape(editor.state.doc)).toEqual([
				{ title: "id", type: "string", body: "the id" },
			])
		})

		it("does nothing when the cursor is not at the title start", ({
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
			const editor = makeEditor(doc, startOfText(doc, "name") + 1)
			const backspace = nodeKeyboardShortcut(
				ParameterListItemHeaderTitle,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing outside a split documentation block", ({ expect }) => {
			const doc = docNode(
				paramList(
					"Params",
					paramItem("id", "string", "the id"),
					paramItem("name", "string", "the name"),
				),
			)
			const editor = makeEditor(doc, startOfText(doc, "name"))
			const backspace = nodeKeyboardShortcut(
				ParameterListItemHeaderTitle,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})
	})
})

describe("ParameterListItem", () => {
	describe("Enter", () => {
		it("inserts an empty item after the current one and moves the cursor into it", ({
			expect,
		}) => {
			const doc = docNode(
				splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
			)
			const editor = makeEditor(doc, startOfText(doc, "the id"))
			const enter = nodeKeyboardShortcut(ParameterListItem, editor, "Enter")

			expect(enter({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)
			expect(itemsShape(editor.state.doc)).toEqual([
				{ title: "id", type: "string", body: "the id" },
				{ title: "", type: "", body: "" },
			])

			// the cursor lands in the new item's empty title
			const { $from } = editor.state.selection
			expect($from.parent.type.name).toBe(
				SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
			)
			expect($from.parent.textContent).toBe("")
		})

		it("inserts the new item after the current one from its title too", ({
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
			const editor = makeEditor(doc, startOfText(doc, "id"))
			const enter = nodeKeyboardShortcut(ParameterListItem, editor, "Enter")

			expect(enter({ editor })).toBe(true)
			expect(itemsShape(editor.state.doc)).toEqual([
				{ title: "id", type: "string", body: "the id" },
				{ title: "", type: "", body: "" },
				{ title: "name", type: "string", body: "the name" },
			])
		})

		it("does nothing when the cursor is outside any item", ({ expect }) => {
			const doc = docNode(
				splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
			)
			const editor = makeEditor(doc, startOfText(doc, "Params"))
			const enter = nodeKeyboardShortcut(ParameterListItem, editor, "Enter")

			expect(enter({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
			expect(itemsShape(editor.state.doc)).toHaveLength(1)
		})
	})
})

describe("ParameterListHeader", () => {
	describe("Backspace", () => {
		it("deletes the whole list when pressed at the start of its header", ({
			expect,
		}) => {
			const doc = docNode(
				splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
			)
			const editor = makeEditor(doc, startOfText(doc, "Params"))
			const backspace = nodeKeyboardShortcut(
				ParameterListHeader,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(true)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(1)
			expect(firstParamList(editor.state.doc)).toBeNull()
		})

		it("does nothing when the cursor is not at the header start", ({
			expect,
		}) => {
			const doc = docNode(
				splitDoc(paramList("Params", paramItem("id", "string", "the id"))),
			)
			const editor = makeEditor(doc, startOfText(doc, "Params") + 1)
			const backspace = nodeKeyboardShortcut(
				ParameterListHeader,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
		})

		it("does nothing outside a split documentation block", ({ expect }) => {
			const doc = docNode(
				paramList("Params", paramItem("id", "string", "the id")),
			)
			const editor = makeEditor(doc, startOfText(doc, "Params"))
			const backspace = nodeKeyboardShortcut(
				ParameterListHeader,
				editor,
				"Backspace",
			)

			expect(backspace({ editor })).toBe(false)
			expect(editor.view.dispatch).toHaveBeenCalledTimes(0)
			expect(firstParamList(editor.state.doc)).not.toBeNull()
		})
	})
})

describe("ParameterList", () => {
	it("fills a complete list skeleton from an empty create", ({ expect }) => {
		const filled = nodeType(
			SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
		).createAndFill()
		if (!filled) {
			throw new Error("createAndFill produced no node")
		}

		function shape(node: PMNode): unknown {
			const children: unknown[] = []

			node.forEach((child) => {
				children.push(shape(child))
			})

			return children.length
				? { type: node.type.name, content: children }
				: { type: node.type.name }
		}

		expect(shape(filled)).toEqual({
			type: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
			content: [
				{ type: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME },
				{
					type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
					content: [
						{
							type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
							content: [
								{
									type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
								},
								{
									type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
								},
							],
						},
						{ type: "paragraph" },
					],
				},
			],
		})
	})

	it.for([
		{
			name: "accepts a header followed by items",
			makeChildren: () => [
				{ type: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME },
				paramItem("id", "string", "the id"),
			],
			valid: true,
		},
		{
			name: "rejects a list without items",
			makeChildren: () => [
				{ type: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME },
			],
			valid: false,
		},
		{
			name: "rejects a list without a header",
			makeChildren: () => [paramItem("id", "string", "the id")],
			valid: false,
		},
	])("$name", ({ makeChildren, valid }, { expect }) => {
		const children = makeChildren().map((json) =>
			paramListSchema.nodeFromJSON(json),
		)

		expect(
			nodeType(SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME).validContent(
				Fragment.from(children),
			),
		).toBe(valid)
	})

	it.for([
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	])(
		"defines %s as an isolating, defining, unselectable node in its own group",
		(name, { expect }) => {
			expect(nodeType(name).spec).toMatchObject({
				isolating: true,
				defining: true,
				selectable: false,
				group: name,
			})
		},
	)
})

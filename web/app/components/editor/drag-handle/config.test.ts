import type { Editor } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import {
	cursorOffsetByType,
	DEBUG_SHOW_GAPS,
	DRAG_COMPOSITE_DRAG_NODES,
	DRAG_CONTAINER_NODE_TYPES,
	DRAG_DISABLED_EXCEPT_RULES,
	DRAG_HANDLE_APPROX_WIDTH_PX,
	DRAG_HANDLE_IGNORE_CLASS,
	DRAG_HANDLE_IGNORE_SELF_CLASS,
	DRAGGABLE_NODE_TYPES,
	gapZoneConfigByType,
	handlePositionByNodeType,
	LIST_ITEM_TO_DEFAULT_LIST_TYPE,
	LIST_ITEM_TYPES,
	LIST_NODE_TYPES,
	NODE_TO_WRAPPER_TYPE,
} from "./config"
import {
	CODE_BLOCK_NAME,
	FIGMA_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	MERMAID_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	METRIC_GRID_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	TITLED_CODE_BLOCK_NAME,
	CALLOUT_BLOCK_NAME,
} from "../blocks/node-names"

type NodeName =
	| "doc"
	| "paragraph"
	| "heading"
	| "listItem"
	| "horizontalRule"
	| typeof IMAGE_BLOCK_NAME
	| typeof FIGMA_BLOCK_NAME
	| typeof TITLED_CODE_BLOCK_NAME
	| "text"

// only the node types the two position helpers branch on: everything
// else in the document model is irrelevant to them
const schema = new Schema<NodeName>({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "text*" },
		heading: {
			group: "block",
			content: "text*",
			attrs: { level: { default: 1 } },
		},
		listItem: { group: "block", content: "paragraph+" },
		horizontalRule: { group: "block", atom: true },
		[IMAGE_BLOCK_NAME]: { group: "block", atom: true },
		[FIGMA_BLOCK_NAME]: { group: "block", atom: true },
		[TITLED_CODE_BLOCK_NAME]: { group: "block", content: "text*" },
		text: { group: "inline" },
	},
})

// handlePositionByNodeType and highlightOverlayByNodeType read nothing
// from the editor but state.doc, so a bare doc holder stands in
function editorWith(...blocks: PMNode[]): Editor {
	const doc = schema.nodes.doc.create(null, blocks)

	return { state: { doc } } as unknown as Editor
}

function block(type: NodeName, attrs?: Record<string, unknown>): PMNode {
	return schema.nodes[type].create(attrs ?? null)
}

describe("DRAG_HANDLE_APPROX_WIDTH_PX", () => {
	it("stays small enough not to reach neighbouring nodes", ({ expect }) => {
		expect(DRAG_HANDLE_APPROX_WIDTH_PX).toBe(9)
		expect(DRAG_HANDLE_APPROX_WIDTH_PX).toBeLessThanOrEqual(10)
	})
})

describe("DEBUG_SHOW_GAPS", () => {
	it("is off so gap zones stay invisible outside debugging", ({ expect }) => {
		expect(DEBUG_SHOW_GAPS).toBe(false)
	})
})

describe("DRAG_COMPOSITE_DRAG_NODES", () => {
	it.for([
		{ typeName: TITLED_CODE_BLOCK_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME, expected: true },
		{ typeName: CALLOUT_BLOCK_NAME, expected: true },
		{ typeName: METRIC_BLOCK_NAME, expected: true },
		{ typeName: "taskItem", expected: true },
		{ typeName: "listItem", expected: true },
		{ typeName: "blockquote", expected: true },
		{ typeName: "paragraph", expected: false },
		{ typeName: METRIC_GRID_NAME, expected: false },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME, expected: false },
	])(
		"reports $expected for $typeName",
		({ typeName, expected }, { expect }) => {
			expect(DRAG_COMPOSITE_DRAG_NODES.has(typeName)).toBe(expected)
		},
	)

	it("absorbs both list item flavours so their children are not targets", ({
		expect,
	}) => {
		expect([...DRAG_COMPOSITE_DRAG_NODES]).toHaveLength(7)
	})
})

describe("DRAG_CONTAINER_NODE_TYPES", () => {
	it.for([
		{ typeName: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME, expected: true },
		{ typeName: "bulletList", expected: true },
		{ typeName: "orderedList", expected: true },
		{ typeName: "taskList", expected: true },
		{ typeName: METRIC_GRID_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_NAME, expected: false },
		{ typeName: "paragraph", expected: false },
		{ typeName: CALLOUT_BLOCK_NAME, expected: false },
	])(
		"reports $expected for $typeName",
		({ typeName, expected }, { expect }) => {
			expect(DRAG_CONTAINER_NODE_TYPES.has(typeName)).toBe(expected)
		},
	)

	it("never marks a composite node as a gap container", ({ expect }) => {
		for (const typeName of DRAG_CONTAINER_NODE_TYPES) {
			expect(DRAG_COMPOSITE_DRAG_NODES.has(typeName)).toBe(false)
		}
	})
})

describe("gapZoneConfigByType", () => {
	it("returns a vertical gap for a metric block inside a metric grid", ({
		expect,
	}) => {
		expect(
			gapZoneConfigByType(METRIC_BLOCK_NAME, undefined, METRIC_GRID_NAME),
		).toEqual({
			height: "100%",
			width: "1rem",
			verticalGap: true,
			includeBeforeFirst: true,
			xOffsetFirst: 0,
			xOffsetMiddle: 0,
			xOffsetLast: 0,
			debugColor: "rgba(128, 0, 200, 0.3)",
		})
	})

	it("returns the horizontal metric block gap outside a metric grid", ({
		expect,
	}) => {
		expect(
			gapZoneConfigByType(METRIC_BLOCK_NAME, undefined, CALLOUT_BLOCK_NAME),
		).toEqual({
			height: "1.5rem",
			includeBeforeFirst: true,
			debugColor: "rgba(128, 0, 64, 0.2)",
		})
	})

	it("ignores the metric grid parent for other child types", ({ expect }) => {
		expect(
			gapZoneConfigByType("paragraph", undefined, METRIC_GRID_NAME),
		).toEqual({
			height: "1.5rem",
			includeBeforeFirst: true,
			debugColor: "rgba(0, 64, 128, 0.2)",
		})
	})

	it.for([
		{
			name: "heading gaps carry a top offset",
			childNodeType: "heading",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				yOffsetFirst: 3,
				yOffsetMiddle: 3,
				debugColor: "rgba(128, 64, 0, 0.2)",
			},
		},
		{
			name: "paragraph gaps are a plain row",
			childNodeType: "paragraph",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 64, 128, 0.2)",
			},
		},
		{
			name: "image block gaps are a plain row",
			childNodeType: IMAGE_BLOCK_NAME,
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 64, 128, 0.2)",
			},
		},
		{
			name: "split documentation gaps are a plain row",
			childNodeType: SPLIT_DOCUMENTATION_NAME,
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 64, 0, 0.2)",
			},
		},
		{
			name: "titled code block gaps are taller",
			childNodeType: TITLED_CODE_BLOCK_NAME,
			expected: {
				height: "2rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 255, 0, 0.2)",
			},
		},
		{
			name: "mermaid block gaps are taller",
			childNodeType: MERMAID_BLOCK_NAME,
			expected: {
				height: "2rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 255, 0, 0.2)",
			},
		},
		{
			name: "figma block gaps are taller",
			childNodeType: FIGMA_BLOCK_NAME,
			expected: {
				height: "2rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 255, 0, 0.2)",
			},
		},
		{
			name: "code block gaps pull the last decoration up",
			childNodeType: CODE_BLOCK_NAME,
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				yOffsetFirst: 3,
				yOffsetLast: -3,
				debugColor: "rgba(0, 255, 0, 0.2)",
			},
		},
		{
			name: "blockquote gaps are a plain row",
			childNodeType: "blockquote",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 255, 64, 0.2)",
			},
		},
		{
			name: "parameter list gaps offset the middle decorations",
			childNodeType: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
			expected: {
				height: "2rem",
				includeBeforeFirst: true,
				yOffsetFirst: 0,
				yOffsetMiddle: 4,
				yOffsetLast: 0,
				debugColor: "rgba(0, 0, 255, 0.2)",
			},
		},
		{
			name: "parameter list item gaps offset the middle decorations further",
			childNodeType: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
			expected: {
				height: "2rem",
				includeBeforeFirst: true,
				yOffsetFirst: 0,
				yOffsetMiddle: 8,
				yOffsetLast: 0,
				debugColor: "rgba(255, 0, 0, 0.2)",
			},
		},
		{
			name: "callout block gaps are a plain row",
			childNodeType: CALLOUT_BLOCK_NAME,
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 0, 64, 0.2)",
			},
		},
		{
			name: "metric grid gaps are a plain row",
			childNodeType: METRIC_GRID_NAME,
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 64, 64, 0.2)",
			},
		},
		{
			name: "horizontal rule gaps are a plain row",
			childNodeType: "horizontalRule",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 128, 128, 0.2)",
			},
		},
		{
			name: "bullet list gaps are a plain row",
			childNodeType: "bulletList",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(64, 0, 64, 0.2)",
			},
		},
		{
			name: "ordered list gaps are a plain row",
			childNodeType: "orderedList",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(64, 128, 0, 0.2)",
			},
		},
		{
			name: "task list gaps are a plain row",
			childNodeType: "taskList",
			expected: {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 128, 0, 0.2)",
			},
		},
	])("$name", ({ childNodeType, expected }, { expect }) => {
		expect(gapZoneConfigByType(childNodeType)).toEqual(expected)
	})

	it.for([
		{
			name: "returns the root list item gap when no indentation is given",
			childNodeType: "listItem",
			indentationLevel: undefined,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: 0,
				debugColor: "rgba(128, 0, 128, 0.2)",
			},
		},
		{
			name: "returns the root list item gap at indentation level zero",
			childNodeType: "listItem",
			indentationLevel: 0,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: 0,
				debugColor: "rgba(128, 0, 128, 0.2)",
			},
		},
		{
			name: "insets nested list item gaps to the left",
			childNodeType: "listItem",
			indentationLevel: 1,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: -6,
				leftInset: -15,
				debugColor: "rgba(128, 0, 168, 0.2)",
			},
		},
		{
			name: "clamps the nested list item debug colour at deep indentation",
			childNodeType: "listItem",
			indentationLevel: 9,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: -6,
				leftInset: -15,
				debugColor: "rgba(128, 0, 255, 0.2)",
			},
		},
		{
			name: "returns the root task item gap when no indentation is given",
			childNodeType: "taskItem",
			indentationLevel: undefined,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: 0,
				debugColor: "rgba(128, 0, 128, 0.2)",
			},
		},
		{
			name: "returns the root task item gap at indentation level zero",
			childNodeType: "taskItem",
			indentationLevel: 0,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: 0,
				debugColor: "rgba(128, 0, 128, 0.2)",
			},
		},
		{
			name: "raises the last nested task item gap",
			childNodeType: "taskItem",
			indentationLevel: 2,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: -6,
				debugColor: "rgba(128, 0, 208, 0.2)",
			},
		},
		{
			name: "clamps the nested task item debug colour at deep indentation",
			childNodeType: "taskItem",
			indentationLevel: 20,
			expected: {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: -6,
				debugColor: "rgba(128, 0, 255, 0.2)",
			},
		},
	])("$name", ({ childNodeType, indentationLevel, expected }, { expect }) => {
		expect(gapZoneConfigByType(childNodeType, indentationLevel)).toEqual(
			expected,
		)
	})

	it.for([
		{ childNodeType: "doc" },
		{ childNodeType: "text" },
		{ childNodeType: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME },
		{ childNodeType: "unknownBlock" },
	])(
		"returns null for the unhandled type $childNodeType",
		({ childNodeType }, { expect }) => {
			expect(gapZoneConfigByType(childNodeType)).toBeNull()
		},
	)
})

describe("cursorOffsetByType", () => {
	it.for([
		{
			name: "split documentation",
			nodeTypeName: SPLIT_DOCUMENTATION_NAME,
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "root list item",
			nodeTypeName: "listItem",
			indentationLevel: 0,
			expected: { before: 5.5, after: -2, left: -20 },
		},
		{
			name: "nested list item",
			nodeTypeName: "listItem",
			indentationLevel: 1,
			expected: { before: 3, after: -2, left: -20 },
		},
		{
			name: "root task item",
			nodeTypeName: "taskItem",
			indentationLevel: 0,
			expected: { before: 6, after: -1, left: 5 },
		},
		{
			name: "nested task item",
			nodeTypeName: "taskItem",
			indentationLevel: 3,
			expected: { before: 6, after: -1.5, left: 5 },
		},
		{
			name: "titled code block",
			nodeTypeName: TITLED_CODE_BLOCK_NAME,
			indentationLevel: 0,
			expected: { before: 9, after: -7 },
		},
		{
			name: "mermaid block",
			nodeTypeName: MERMAID_BLOCK_NAME,
			indentationLevel: 0,
			expected: { before: 9, after: -7 },
		},
		{
			name: "code block",
			nodeTypeName: CODE_BLOCK_NAME,
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "callout block",
			nodeTypeName: CALLOUT_BLOCK_NAME,
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "blockquote",
			nodeTypeName: "blockquote",
			indentationLevel: 0,
			expected: { before: 11, after: -7 },
		},
		{
			name: "metric block",
			nodeTypeName: METRIC_BLOCK_NAME,
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "metric grid",
			nodeTypeName: METRIC_GRID_NAME,
			indentationLevel: 0,
			expected: { before: 12, after: -7 },
		},
		{
			name: "heading",
			nodeTypeName: "heading",
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "paragraph",
			nodeTypeName: "paragraph",
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "bullet list",
			nodeTypeName: "bulletList",
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "ordered list",
			nodeTypeName: "orderedList",
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "task list",
			nodeTypeName: "taskList",
			indentationLevel: 0,
			expected: { before: 10, after: -7 },
		},
		{
			name: "horizontal rule",
			nodeTypeName: "horizontalRule",
			indentationLevel: 0,
			expected: { before: 12, after: -12 },
		},
		{
			name: "an unhandled node type",
			nodeTypeName: IMAGE_BLOCK_NAME,
			indentationLevel: 0,
			expected: { before: 7, after: -7 },
		},
		{
			name: "a missing node type",
			nodeTypeName: null,
			indentationLevel: 0,
			expected: { before: 7, after: -7 },
		},
	])(
		"offsets the cursor for $name",
		({ nodeTypeName, indentationLevel, expected }, { expect }) => {
			expect(cursorOffsetByType(nodeTypeName, indentationLevel)).toEqual(
				expected,
			)
		},
	)
})

describe("LIST_NODE_TYPES", () => {
	it.for([
		{ typeName: "bulletList", expected: true },
		{ typeName: "orderedList", expected: true },
		{ typeName: "taskList", expected: true },
		{ typeName: "listItem", expected: false },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME, expected: false },
	])(
		"reports $expected for $typeName",
		({ typeName, expected }, { expect }) => {
			expect(LIST_NODE_TYPES.has(typeName)).toBe(expected)
		},
	)
})

describe("LIST_ITEM_TO_DEFAULT_LIST_TYPE", () => {
	it.for([
		{ typeName: "listItem", expected: "bulletList" },
		{ typeName: "taskItem", expected: "taskList" },
		{ typeName: "paragraph", expected: undefined },
	])("maps $typeName to $expected", ({ typeName, expected }, { expect }) => {
		expect(LIST_ITEM_TO_DEFAULT_LIST_TYPE.get(typeName)).toBe(expected)
	})

	it("only maps types that are list items and to types that are lists", ({
		expect,
	}) => {
		for (const [itemType, listType] of LIST_ITEM_TO_DEFAULT_LIST_TYPE) {
			expect.soft(LIST_ITEM_TYPES.has(itemType)).toBe(true)
			expect.soft(LIST_NODE_TYPES.has(listType)).toBe(true)
		}
	})
})

describe("NODE_TO_WRAPPER_TYPE", () => {
	it.for([
		{ typeName: METRIC_BLOCK_NAME, expected: METRIC_GRID_NAME },
		{ typeName: "paragraph", expected: undefined },
		{ typeName: METRIC_GRID_NAME, expected: undefined },
	])("maps $typeName to $expected", ({ typeName, expected }, { expect }) => {
		expect(NODE_TO_WRAPPER_TYPE.get(typeName)).toBe(expected)
	})
})

describe("LIST_ITEM_TYPES", () => {
	it.for([
		{ typeName: "listItem", expected: true },
		{ typeName: "taskItem", expected: true },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME, expected: false },
		{ typeName: "bulletList", expected: false },
	])(
		"reports $expected for $typeName",
		({ typeName, expected }, { expect }) => {
			expect(LIST_ITEM_TYPES.has(typeName)).toBe(expected)
		},
	)
})

describe("DRAG_HANDLE_IGNORE_CLASS", () => {
	it("is the class node detection looks for on ignored subtrees", ({
		expect,
	}) => {
		expect(DRAG_HANDLE_IGNORE_CLASS).toBe("drag-handle-ignore")
	})
})

describe("DRAG_HANDLE_IGNORE_SELF_CLASS", () => {
	it("is a distinct class from the subtree-wide ignore class", ({ expect }) => {
		expect(DRAG_HANDLE_IGNORE_SELF_CLASS).toBe("drag-handle-ignore-self")
		expect(DRAG_HANDLE_IGNORE_SELF_CLASS).not.toBe(DRAG_HANDLE_IGNORE_CLASS)
	})
})

describe("DRAGGABLE_NODE_TYPES", () => {
	it.for([
		{ typeName: "paragraph", expected: true },
		{ typeName: IMAGE_BLOCK_NAME, expected: true },
		{ typeName: "heading", expected: true },
		{ typeName: "listItem", expected: true },
		{ typeName: "taskItem", expected: true },
		{ typeName: "blockquote", expected: true },
		{ typeName: CALLOUT_BLOCK_NAME, expected: true },
		{ typeName: CODE_BLOCK_NAME, expected: true },
		{ typeName: TITLED_CODE_BLOCK_NAME, expected: true },
		{ typeName: MERMAID_BLOCK_NAME, expected: true },
		{ typeName: FIGMA_BLOCK_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME, expected: true },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME, expected: true },
		{ typeName: METRIC_BLOCK_NAME, expected: true },
		{ typeName: "horizontalRule", expected: true },
		{ typeName: "doc", expected: false },
		{ typeName: "text", expected: false },
		{ typeName: METRIC_GRID_NAME, expected: false },
		{ typeName: "bulletList", expected: false },
		{ typeName: "taskList", expected: false },
		{ typeName: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, expected: false },
		{ typeName: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, expected: false },
	])(
		"reports $expected for $typeName",
		({ typeName, expected }, { expect }) => {
			expect(DRAGGABLE_NODE_TYPES.has(typeName)).toBe(expected)
		},
	)
})

describe("DRAG_DISABLED_EXCEPT_RULES", () => {
	it.for([
		{
			name: "task items only allow nested list items",
			typeName: "taskItem",
			expected: ["taskItem", "listItem"],
		},
		{
			name: "list items only allow nested list items",
			typeName: "listItem",
			expected: ["taskItem", "listItem"],
		},
		{
			name: "metric grids only allow metric blocks",
			typeName: METRIC_GRID_NAME,
			expected: [METRIC_BLOCK_NAME],
		},
		{
			name: "split documentation allows its own block vocabulary",
			typeName: SPLIT_DOCUMENTATION_NAME,
			expected: [
				SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
				SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
				CODE_BLOCK_NAME,
				TITLED_CODE_BLOCK_NAME,
				CALLOUT_BLOCK_NAME,
				"listItem",
				"taskItem",
				"paragraph",
				METRIC_BLOCK_NAME,
			],
		},
	])("$name", ({ typeName, expected }, { expect }) => {
		expect([...(DRAG_DISABLED_EXCEPT_RULES.get(typeName) ?? [])]).toEqual(
			expected,
		)
	})

	it.for([
		{ typeName: CALLOUT_BLOCK_NAME },
		{ typeName: METRIC_BLOCK_NAME },
		{ typeName: "blockquote" },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME },
		{ typeName: TITLED_CODE_BLOCK_NAME },
	])(
		"disables every descendant of $typeName with an empty exception set",
		({ typeName }, { expect }) => {
			const rule = DRAG_DISABLED_EXCEPT_RULES.get(typeName)

			expect(rule).toBeInstanceOf(Set)
			expect(rule?.size).toBe(0)
		},
	)

	it.for([
		{ typeName: "paragraph" },
		{ typeName: "heading" },
		{ typeName: "bulletList" },
		{ typeName: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME },
		{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME },
		{ typeName: "doc" },
	])(
		"has no rule for $typeName so its descendants stay draggable",
		({ typeName }, { expect }) => {
			expect(DRAG_DISABLED_EXCEPT_RULES.has(typeName)).toBe(false)
			expect(DRAG_DISABLED_EXCEPT_RULES.get(typeName)).toBeUndefined()
		},
	)

	it("only ever lists draggable node types as exceptions", ({ expect }) => {
		for (const [typeName, exceptions] of DRAG_DISABLED_EXCEPT_RULES) {
			for (const exception of exceptions) {
				expect
					.soft(
						DRAGGABLE_NODE_TYPES.has(exception),
						`${typeName} -> ${exception}`,
					)
					.toBe(true)
			}
		}
	})
})

describe("handlePositionByNodeType", () => {
	it.for([
		{
			name: "level one headings sit flush with the text",
			node: () => block("heading", { level: 1 }),
			expected: { placement: "left-start", yOffset: 0, xOffset: 0 },
		},
		{
			name: "level two headings drop by two pixels",
			node: () => block("heading", { level: 2 }),
			expected: { placement: "left-start", yOffset: 2, xOffset: 0 },
		},
		{
			name: "level three headings drop by three pixels",
			node: () => block("heading", { level: 3 }),
			expected: { placement: "left-start", yOffset: 3, xOffset: 0 },
		},
		{
			name: "deeper headings keep the default offset",
			node: () => block("heading", { level: 4 }),
			expected: { placement: "left-start", yOffset: -1, xOffset: 0 },
		},
		{
			name: "list items are pushed left of the marker",
			node: () => block("listItem", undefined),
			expected: { placement: "left-start", yOffset: -1, xOffset: -25 },
		},
		{
			name: "image blocks drop by four pixels",
			node: () => block(IMAGE_BLOCK_NAME),
			expected: { placement: "left-start", yOffset: 4, xOffset: 0 },
		},
		{
			name: "figma blocks drop by four pixels",
			node: () => block(FIGMA_BLOCK_NAME),
			expected: { placement: "left-start", yOffset: 4, xOffset: 0 },
		},
		{
			name: "horizontal rules are centred on the rule",
			node: () => block("horizontalRule"),
			expected: { placement: "left", yOffset: 0, xOffset: 0 },
		},
		{
			name: "titled code blocks drop by three pixels",
			node: () => block(TITLED_CODE_BLOCK_NAME),
			expected: { placement: "left-start", yOffset: 3, xOffset: 0 },
		},
		{
			name: "unhandled node types keep the default offset",
			node: () => block("paragraph"),
			expected: { placement: "left-start", yOffset: -1, xOffset: 0 },
		},
	])("$name", ({ node, expected }, { expect }) => {
		expect(handlePositionByNodeType(editorWith(node()), 0)).toEqual({
			strategy: "absolute",
			...expected,
		})
	})

	it("falls back to the default config when no node sits at the position", ({
		expect,
	}) => {
		const editor = editorWith(block("heading", { level: 1 }))

		expect(
			handlePositionByNodeType(editor, editor.state.doc.content.size),
		).toEqual({
			strategy: "absolute",
			placement: "left-start",
			yOffset: -1,
			xOffset: 0,
		})
	})
})

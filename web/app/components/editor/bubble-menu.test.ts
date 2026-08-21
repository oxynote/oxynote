import type { Editor } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, NodeSelection } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { describe, it } from "vitest"
import {
	CODE_BLOCK_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
	TITLED_CODE_BLOCK_NAME,
} from "./blocks/node-names"
import {
	isBubbleMenuItemAllowedByContext,
	shouldShowBubbleMenu,
} from "./bubble-menu"
import { COMMENT_MARK_NAME } from "./mark-names"
import { blockBuilder, stateAt } from "./test-helpers"

// minimal schema reusing the real node and mark names the whitelist
// checks against
const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		heading: { group: "block", content: "inline*" },
		[CODE_BLOCK_NAME]: { group: "block", content: "text*", marks: "" },
		[TITLED_CODE_BLOCK_NAME]: { group: "block", content: "block+" },
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME]: {
			group: "block",
			content: "inline*",
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME]: {
			group: "block",
			content: "inline*",
		},
		[SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME]: {
			group: "block",
			content: "inline*",
		},
		atomBlock: { group: "block", atom: true },
		text: { group: "inline" },
	},
	marks: {
		[COMMENT_MARK_NAME]: {},
	},
})

const block = blockBuilder(schema)

function commentText(text: string) {
	return schema.text(text, [schema.mark(COMMENT_MARK_NAME)])
}

// "plain " spans [1, 7), the comment-marked "noted" spans [7, 12), and
// the heading text spans [14, 18)
const commentDoc = block(
	"doc",
	block("paragraph", "plain ", commentText("noted")),
	block("heading", "head"),
)

// a heading whose entire text carries the comment mark; text spans [1, 5)
const commentHeadingDoc = block("doc", block("heading", commentText("head")))

describe("isBubbleMenuItemAllowedByContext", () => {
	it.for([
		"bold",
		"code",
		"italic",
		"link",
		"strike",
		"underline",
		COMMENT_MARK_NAME,
	])("allows %s in a plain paragraph", (item, { expect }) => {
		expect(
			isBubbleMenuItemAllowedByContext(stateAt(commentDoc, 2, 4), item),
		).toBe(true)
	})

	it("rejects items outside the whitelist in a plain paragraph", ({
		expect,
	}) => {
		expect(
			isBubbleMenuItemAllowedByContext(stateAt(commentDoc, 2, 4), "heading"),
		).toBe(false)
	})

	it("rejects the comment item over comment-marked text", ({ expect }) => {
		expect(
			isBubbleMenuItemAllowedByContext(
				stateAt(commentDoc, 8, 10),
				COMMENT_MARK_NAME,
			),
		).toBe(false)
	})

	it("still allows formatting items over comment-marked text", ({ expect }) => {
		expect(
			isBubbleMenuItemAllowedByContext(stateAt(commentDoc, 8, 10), "bold"),
		).toBe(true)
	})

	const restrictedRows: {
		name: string
		make: () => PMNode
		pos: number
	}[] = [
		{
			name: "a heading",
			make: () => commentDoc,
			pos: 15,
		},
		{
			name: "a code block",
			make: () => block("doc", block(CODE_BLOCK_NAME, "x")),
			pos: 1,
		},
		{
			name: "a titled code block",
			make: () =>
				block(
					"doc",
					block(TITLED_CODE_BLOCK_NAME, block(CODE_BLOCK_NAME, "x")),
				),
			pos: 2,
		},
		{
			name: "the parameter list header",
			make: () =>
				block(
					"doc",
					block(SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME, "x"),
				),
			pos: 1,
		},
		{
			name: "a parameter item title header",
			make: () =>
				block(
					"doc",
					block(SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME, "x"),
				),
			pos: 1,
		},
		{
			name: "a parameter item type header",
			make: () =>
				block(
					"doc",
					block(SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME, "x"),
				),
			pos: 1,
		},
	]

	it.for(restrictedRows)(
		"allows only the comment item inside $name",
		({ make, pos }, { expect }) => {
			const state = stateAt(make(), pos)

			expect(isBubbleMenuItemAllowedByContext(state, COMMENT_MARK_NAME)).toBe(
				true,
			)
			expect(isBubbleMenuItemAllowedByContext(state, "bold")).toBe(false)
		},
	)

	it("rejects every item inside a heading over comment-marked text", ({
		expect,
	}) => {
		const state = stateAt(commentHeadingDoc, 2, 3)

		expect(isBubbleMenuItemAllowedByContext(state, COMMENT_MARK_NAME)).toBe(
			false,
		)
		expect(isBubbleMenuItemAllowedByContext(state, "bold")).toBe(false)
	})
})

describe("shouldShowBubbleMenu", () => {
	function showOpts(
		state: EditorState,
		overrides: { focused?: boolean; editable?: boolean } = {},
	) {
		return {
			editor: { isEditable: overrides.editable ?? true } as unknown as Editor,
			state,
			view: {
				hasFocus: () => overrides.focused ?? true,
			} as unknown as EditorView,
			from: state.selection.from,
			to: state.selection.to,
		}
	}

	it("shows for a focused text selection in a paragraph", ({ expect }) => {
		expect(shouldShowBubbleMenu(showOpts(stateAt(commentDoc, 2, 4)))).toBe(true)
	})

	it("shows in a heading where only the comment item is allowed", ({
		expect,
	}) => {
		expect(shouldShowBubbleMenu(showOpts(stateAt(commentDoc, 15, 17)))).toBe(
			true,
		)
	})

	it("hides when the view has no focus", ({ expect }) => {
		expect(
			shouldShowBubbleMenu(
				showOpts(stateAt(commentDoc, 2, 4), { focused: false }),
			),
		).toBe(false)
	})

	it("hides for an empty selection", ({ expect }) => {
		expect(shouldShowBubbleMenu(showOpts(stateAt(commentDoc, 2)))).toBe(false)
	})

	it("hides when the editor is not editable", ({ expect }) => {
		expect(
			shouldShowBubbleMenu(
				showOpts(stateAt(commentDoc, 2, 4), { editable: false }),
			),
		).toBe(false)
	})

	it("hides for a node selection", ({ expect }) => {
		const doc = block("doc", block("atomBlock"), block("paragraph", "x"))
		const state = EditorState.create({
			doc,
			selection: NodeSelection.create(doc, 0),
		})

		expect(shouldShowBubbleMenu(showOpts(state))).toBe(false)
	})

	it("hides inside a heading over comment-marked text", ({ expect }) => {
		expect(
			shouldShowBubbleMenu(showOpts(stateAt(commentHeadingDoc, 2, 3))),
		).toBe(false)
	})
})

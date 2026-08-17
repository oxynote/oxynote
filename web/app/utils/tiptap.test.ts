import { InputRule } from "@tiptap/core"
import { Schema } from "@tiptap/pm/model"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import {
	clearTiptapScrollElementHighlightOverlays,
	highlightTiptapScrollElement,
	isCursorInsideTiptapMark,
	isCursorInsideTiptapNode,
	isNodeInsideTiptapNode,
	preventTiptapInputRuleInNode,
	tiptapNodePosByCursor,
} from "./tiptap"

// minimal schema with a nestable container to exercise the ancestor
// walks: doc > paragraph("plain bold") + callout > paragraph("inside")
const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		callout: { group: "block", content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		text: { group: "inline" },
	},
	marks: {
		bold: {},
	},
})

// positions: paragraph opens at 0, "plain " spans [1, 7), "bold"
// spans [7, 11); the callout opens at 12, its paragraph at 13, and
// "inside" spans [14, 20)
const pmDoc = schema.nodes.doc.create(null, [
	schema.nodes.paragraph.create(null, [
		schema.text("plain "),
		schema.text("bold", [schema.marks.bold.create()]),
	]),
	schema.nodes.callout.create(null, [
		schema.nodes.paragraph.create(null, [schema.text("inside")]),
	]),
])

function stateWithSelection(from: number, to = from) {
	return EditorState.create({
		doc: pmDoc,
		selection: TextSelection.create(pmDoc, from, to),
	})
}

describe("highlightTiptapScrollElement", () => {
	// node has no window, so this exercises the SSR guard directly
	it("does nothing without a window", ({ expect }) => {
		expect(() => {
			highlightTiptapScrollElement("id", {} as HTMLElement)
		}).not.toThrow()
	})

	it("does nothing without a document even when a window exists", ({
		expect,
	}) => {
		vi.stubGlobal("window", {})

		expect(() => {
			highlightTiptapScrollElement("id", {} as HTMLElement)
		}).not.toThrow()
	})
})

describe("clearTiptapScrollElementHighlightOverlays", () => {
	// node has no window, so this exercises the SSR guard directly
	it("does nothing without a window", ({ expect }) => {
		expect(() => {
			clearTiptapScrollElementHighlightOverlays()
		}).not.toThrow()
	})

	it("does nothing without a document even when a window exists", ({
		expect,
	}) => {
		vi.stubGlobal("window", {})

		expect(() => {
			clearTiptapScrollElementHighlightOverlays()
		}).not.toThrow()
	})
})

describe("isCursorInsideTiptapNode", () => {
	it("detects the cursor inside a matching container", ({ expect }) => {
		expect(isCursorInsideTiptapNode(stateWithSelection(15), ["callout"])).toBe(
			true,
		)
	})

	it("rejects a cursor outside the container", ({ expect }) => {
		expect(isCursorInsideTiptapNode(stateWithSelection(3), ["callout"])).toBe(
			false,
		)
	})

	it("rejects every cursor for an empty name list", ({ expect }) => {
		expect(isCursorInsideTiptapNode(stateWithSelection(15), [])).toBe(false)
	})
})

describe("isCursorInsideTiptapMark", () => {
	it("detects a selection over marked text", ({ expect }) => {
		expect(isCursorInsideTiptapMark(stateWithSelection(8, 10), "bold")).toBe(
			true,
		)
	})

	it("rejects a selection over plain text", ({ expect }) => {
		expect(isCursorInsideTiptapMark(stateWithSelection(2, 4), "bold")).toBe(
			false,
		)
	})

	it("rejects marks unknown to the schema", ({ expect }) => {
		expect(
			isCursorInsideTiptapMark(stateWithSelection(8, 10), "underline"),
		).toBe(false)
	})
})

describe("tiptapNodePosByCursor", () => {
	it("returns the position and node of the closest matching ancestor", ({
		expect,
	}) => {
		const result = tiptapNodePosByCursor(stateWithSelection(15), ["callout"])

		expect(result?.pos).toBe(12)
		expect(result?.node.type.name).toBe("callout")
	})

	it("prefers the deepest matching ancestor", ({ expect }) => {
		const result = tiptapNodePosByCursor(stateWithSelection(15), [
			"callout",
			"paragraph",
		])

		expect(result?.pos).toBe(13)
		expect(result?.node.type.name).toBe("paragraph")
	})

	it("returns null without a matching ancestor", ({ expect }) => {
		expect(tiptapNodePosByCursor(stateWithSelection(3), ["callout"])).toBeNull()
	})
})

describe("isNodeInsideTiptapNode", () => {
	it("detects a position nested inside the container", ({ expect }) => {
		expect(isNodeInsideTiptapNode(stateWithSelection(0), 14, ["callout"])).toBe(
			true,
		)
	})

	it("rejects a position outside the container", ({ expect }) => {
		expect(isNodeInsideTiptapNode(stateWithSelection(0), 1, ["callout"])).toBe(
			false,
		)
	})
})

describe("preventTiptapInputRuleInNode", () => {
	function makeRule() {
		const handler = vi.fn()

		return { rule: new InputRule({ find: /abc$/, handler }), handler }
	}

	function props(state: EditorState) {
		return { state } as unknown as Parameters<InputRule["handler"]>[0]
	}

	it("runs the rule when every ancestor is allowed", ({ expect }) => {
		const { rule, handler } = makeRule()
		const wrapped = preventTiptapInputRuleInNode(rule, ["callout", "paragraph"])
		const ruleProps = props(stateWithSelection(15))

		wrapped.handler(ruleProps)

		expect(handler).toHaveBeenCalledTimes(1)
		expect(handler).toHaveBeenCalledWith(ruleProps)
	})

	it("blocks the rule when an ancestor is not allowed", ({ expect }) => {
		const { rule, handler } = makeRule()
		const wrapped = preventTiptapInputRuleInNode(rule, ["callout"])

		wrapped.handler(props(stateWithSelection(15)))

		expect(handler).toHaveBeenCalledTimes(0)
	})

	it("runs the rule anywhere without container restrictions", ({ expect }) => {
		const { rule, handler } = makeRule()
		const wrapped = preventTiptapInputRuleInNode(rule, [])

		wrapped.handler(props(stateWithSelection(15)))

		expect(handler).toHaveBeenCalledTimes(1)
	})

	it("blocks nested cursors in root-only mode", ({ expect }) => {
		const { rule, handler } = makeRule()
		const wrapped = preventTiptapInputRuleInNode(rule, [], true)

		wrapped.handler(props(stateWithSelection(15)))

		expect(handler).toHaveBeenCalledTimes(0)
	})

	it("runs the rule for top-level cursors in root-only mode", ({ expect }) => {
		const { rule, handler } = makeRule()
		const wrapped = preventTiptapInputRuleInNode(rule, [], true)

		wrapped.handler(props(stateWithSelection(3)))

		expect(handler).toHaveBeenCalledTimes(1)
	})
})

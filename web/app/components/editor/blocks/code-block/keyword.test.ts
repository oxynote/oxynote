import type { Node as PMNode } from "@tiptap/pm/model"
import { Schema } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { EditorState } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import { CODE_BLOCK_TITLE_NAME } from "../node-names"
import { fixUserSelectionAroundKeywordColor, KeywordColor } from "./keyword"
import { decorationClassShape } from "~/components/editor/test-helpers"

const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		[CODE_BLOCK_TITLE_NAME]: { group: "block", content: "inline*" },
		paragraph: { group: "block", content: "inline*" },
		text: { group: "inline" },
	},
	marks: {
		bold: {},
	},
})

// the extension's plugin factory reads nothing from the extension
// context, so it can be invoked without a live editor
function makePlugin(): Plugin {
	const addProseMirrorPlugins = KeywordColor.config
		.addProseMirrorPlugins as unknown as (this: unknown) => Plugin[]

	const plugin = addProseMirrorPlugins.call(undefined)[0]
	if (!plugin) {
		throw new Error("KeywordColor produced no plugin")
	}

	return plugin
}

function stateWithBlocks(...blocks: PMNode[]): {
	state: EditorState
	plugin: Plugin
} {
	const plugin = makePlugin()
	const state = EditorState.create({
		doc: schema.nodes.doc.create(null, blocks),
		plugins: [plugin],
	})

	return { state, plugin }
}

function title(...inline: PMNode[]): PMNode {
	return schema.nodes[CODE_BLOCK_TITLE_NAME].create(null, inline)
}

function stateWithTitle(text: string): { state: EditorState; plugin: Plugin } {
	return stateWithBlocks(title(schema.text(text)))
}

describe("KeywordColor", () => {
	it.for([
		{ method: "GET", class: "text-http-method-get" },
		{ method: "POST", class: "text-http-method-post" },
		{ method: "PUT", class: "text-http-method-put" },
		{ method: "PATCH", class: "text-http-method-patch" },
		{ method: "DELETE", class: "text-http-method-delete" },
		{ method: "OPTIONS", class: "text-http-method-options" },
		{ method: "HEAD", class: "text-http-method-head" },
		{ method: "TRACE", class: "text-http-method-trace" },
		{ method: "CONNECT", class: "text-http-method-connect" },
	])("colors the $method method with $class", (pattern, { expect }) => {
		const { state, plugin } = stateWithTitle(`${pattern.method} /users`)

		expect(decorationClassShape(state, plugin)).toEqual([
			[pattern.class, pattern.method],
		])
	})

	it.for([
		{
			name: "dims a domain followed by a path",
			text: "api.example.com/users",
			expected: [["opacity-50", "api.example.com"]],
		},
		{
			name: "dims a domain followed by a port",
			text: "example.com:8080",
			expected: [["opacity-50", "example.com"]],
		},
		{
			name: "dims a domain at the end of the title",
			text: "example.com",
			expected: [["opacity-50", "example.com"]],
		},
		{
			name: "ignores a domain followed by a space",
			text: "example.com and more",
			expected: [],
		},
		{
			name: "colors a colon-style path parameter without its slash",
			text: "/users/:userId",
			expected: [["text-url-path-param", ":userId"]],
		},
		{
			name: "colors a brace-style path parameter without its slash",
			text: "/orgs/{orgId}/repos",
			expected: [["text-url-path-param", "{orgId}"]],
		},
		{
			name: "ignores a method embedded in another word",
			text: "FORGET IT",
			expected: [],
		},
		{
			name: "ignores lowercase method names",
			text: "get /users",
			expected: [],
		},
		{
			name: "decorates a full request line in position order",
			text: "GET https://api.example.com/users/:id",
			expected: [
				["text-http-method-get", "GET"],
				["opacity-50", "api.example.com"],
				["text-url-path-param", ":id"],
			],
		},
	])("$name", ({ text, expected }, { expect }) => {
		const { state, plugin } = stateWithTitle(text)

		expect(decorationClassShape(state, plugin)).toEqual(expected)
	})

	it("matches keywords across text nodes split by marks", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(
			title(
				schema.text("GE"),
				schema.text("T /x", [schema.marks.bold.create()]),
			),
		)

		expect(decorationClassShape(state, plugin)).toEqual([
			["text-http-method-get", "GET"],
		])
	})

	it("ignores keywords outside code block titles", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(
			schema.nodes.paragraph.create(null, [
				schema.text("GET api.example.com/users"),
			]),
		)

		expect(decorationClassShape(state, plugin)).toEqual([])
	})

	it("produces no decorations for an empty title", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(title())

		expect(decorationClassShape(state, plugin)).toEqual([])
	})

	it("keeps the decoration set when the document is unchanged", ({
		expect,
	}) => {
		const { state, plugin } = stateWithTitle("GET /users")

		const next = state.apply(state.tr)

		expect(plugin.getState(next)).toBe(plugin.getState(state))
	})

	it("rebuilds decorations when the document changes", ({ expect }) => {
		const { state, plugin } = stateWithTitle("GE")

		// completing "GET" inside the title turns the text into a match
		const next = state.apply(state.tr.insertText("T", 3))

		expect(decorationClassShape(state, plugin)).toEqual([])
		expect(decorationClassShape(next, plugin)).toEqual([
			["text-http-method-get", "GET"],
		])
	})
})

describe("fixUserSelectionAroundKeywordColor", () => {
	it("clears the selection on the next animation frame", ({ expect }) => {
		const removeAllRanges = vi.fn()
		const requestAnimationFrame = vi.fn((cb: FrameRequestCallback) => {
			cb(0)
			return 0
		})

		vi.stubGlobal("requestAnimationFrame", requestAnimationFrame)
		vi.stubGlobal("window", { getSelection: () => ({ removeAllRanges }) })

		fixUserSelectionAroundKeywordColor()

		expect(requestAnimationFrame).toHaveBeenCalledTimes(1)
		expect(removeAllRanges).toHaveBeenCalledTimes(1)
	})

	it("does nothing when no selection exists", ({ expect }) => {
		vi.stubGlobal(
			"requestAnimationFrame",
			(cb: FrameRequestCallback): number => {
				cb(0)
				return 0
			},
		)
		vi.stubGlobal("window", { getSelection: () => null })

		expect(() => {
			fixUserSelectionAroundKeywordColor()
		}).not.toThrow()
	})
})

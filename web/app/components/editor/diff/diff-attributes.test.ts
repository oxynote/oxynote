import { getSchema } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Schema } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import { DiffAttributes } from "./diff-attributes"
import { nodeType } from "../test-helpers"

function makeSchema(): Schema {
	return getSchema([
		Document,
		Paragraph,
		Text,
		Heading,
		DiffAttributes.configure({ types: ["paragraph"] }),
	])
}

// the parse rules only read attributes, so a getAttribute stub stands in
// for a DOM element in the node environment
function stubElement(attributes: Record<string, string>): HTMLElement {
	return {
		getAttribute: (name: string) => attributes[name] ?? null,
	} as unknown as HTMLElement
}

describe("DiffAttributes", () => {
	it("registers null-defaulted diff attributes on the configured types", ({
		expect,
	}) => {
		const paragraph = nodeType(makeSchema(), "paragraph")

		expect(paragraph.create().attrs).toEqual({
			diffStatus: null,
			modifiedIndex: null,
			originalIndex: null,
			oldNode: null,
			modifiedTextContent: null,
		})
	})

	it("leaves unconfigured node types without diff attributes", ({ expect }) => {
		const heading = nodeType(makeSchema(), "heading")

		expect(heading.create().attrs).toEqual({ level: 1 })
	})

	it("renders diffStatus as a data-diff-status attribute", ({ expect }) => {
		const paragraph = nodeType(makeSchema(), "paragraph")
		const node = paragraph.create({ diffStatus: "added" })

		expect(paragraph.spec.toDOM?.(node)).toEqual([
			"p",
			{ "data-diff-status": "added" },
			0,
		])
	})

	it("keeps the diff metadata attributes out of the rendered HTML", ({
		expect,
	}) => {
		const paragraph = nodeType(makeSchema(), "paragraph")
		const node = paragraph.create({
			modifiedIndex: 2,
			originalIndex: 1,
			oldNode: { type: "paragraph" },
			modifiedTextContent: "text",
		})

		expect(paragraph.spec.toDOM?.(node)).toEqual(["p", {}, 0])
	})

	it("parses diffStatus from the data-diff-status attribute", ({ expect }) => {
		const rule = nodeType(makeSchema(), "paragraph").spec.parseDOM?.[0]

		expect(
			rule?.getAttrs?.(stubElement({ "data-diff-status": "removed" })),
		).toEqual({ diffStatus: "removed" })
	})

	it("parses no diffStatus from an element without the attribute", ({
		expect,
	}) => {
		const rule = nodeType(makeSchema(), "paragraph").spec.parseDOM?.[0]

		expect(rule?.getAttrs?.(stubElement({}))).toEqual({})
	})
})

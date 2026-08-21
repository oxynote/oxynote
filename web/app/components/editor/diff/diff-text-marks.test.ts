import { getSchema } from "@tiptap/core"
import Bold from "@tiptap/extension-bold"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Schema } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import { DiffTextAddedMark, DiffTextRemovedMark } from "./diff-text-marks"
import { markType } from "../test-helpers"

function makeSchema(): Schema {
	return getSchema([
		Document,
		Paragraph,
		Text,
		Bold,
		DiffTextAddedMark,
		DiffTextRemovedMark,
	])
}

describe("DiffTextAddedMark", () => {
	it("renders as a span with the diff-text-added class", ({ expect }) => {
		const type = markType(makeSchema(), DIFF_TEXT_ADDED_MARK_NAME)

		expect(type.spec.toDOM?.(type.create(), true)).toEqual([
			"span",
			{ class: "diff-text-added" },
			0,
		])
	})

	it("stacks with the removed mark and regular marks on the same text", ({
		expect,
	}) => {
		const schema = makeSchema()
		const added = markType(schema, DIFF_TEXT_ADDED_MARK_NAME).create()
		const removed = markType(schema, DIFF_TEXT_REMOVED_MARK_NAME).create()
		const bold = markType(schema, "bold").create()

		const set = added.addToSet([bold, removed])

		expect(set.map((mark) => mark.type.name).sort()).toEqual([
			"bold",
			DIFF_TEXT_ADDED_MARK_NAME,
			DIFF_TEXT_REMOVED_MARK_NAME,
		])
	})
})

describe("DiffTextRemovedMark", () => {
	it("renders as a span with the diff-text-removed class", ({ expect }) => {
		const type = markType(makeSchema(), DIFF_TEXT_REMOVED_MARK_NAME)

		expect(type.spec.toDOM?.(type.create(), true)).toEqual([
			"span",
			{ class: "diff-text-removed" },
			0,
		])
	})

	it("stacks with the added mark on the same text", ({ expect }) => {
		const schema = makeSchema()
		const removed = markType(schema, DIFF_TEXT_REMOVED_MARK_NAME).create()
		const added = markType(schema, DIFF_TEXT_ADDED_MARK_NAME).create()

		const set = removed.addToSet([added])

		expect(set.map((mark) => mark.type.name).sort()).toEqual([
			DIFF_TEXT_ADDED_MARK_NAME,
			DIFF_TEXT_REMOVED_MARK_NAME,
		])
	})
})

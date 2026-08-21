import type { Editor } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import type { DecorationSet } from "@tiptap/pm/view"
import { describe, it } from "vitest"
import { ref, toValue } from "vue"
import { CalloutBlock } from "./blocks/callout"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	MERMAID_BLOCK_NAME,
} from "./blocks/node-names"
import { SplitDocumentationLeftSide } from "./blocks/split-documentation"
import {
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
} from "./blocks/split-documentation/parameter-list"
import {
	defaultContentPlaceholder,
	defaultNamePlaceholder,
	explicitContentPlaceholder,
	placeholderEmptyNodeClass,
} from "./placeholder"
import { blockBuilder } from "./test-helpers"

const t = (path: string) => path

// the vitest projects wire unimport only for app/utils and app/composables,
// so the vue auto-import placeholder.ts relies on falls back to a global
// lookup. A plain assignment rather than vi.stubGlobal because
// unstubGlobals would tear it down under a concurrently running test
Object.assign(globalThis, { toValue })

// minimal schema reusing the real block node names, so the placeholder
// ancestor checks and node filters match exactly like in the app schema
const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		heading: { group: "block", content: "inline*" },
		[CalloutBlock.name]: { group: "block", content: "block+" },
		[SplitDocumentationLeftSide.name]: { group: "block", content: "block+" },
		[ParameterListItem.name]: { group: "block", content: "block+" },
		text: { group: "inline" },
	},
})

const block = blockBuilder(schema)

type PlaceholderExtension = ReturnType<typeof defaultNamePlaceholder>

function placeholderFn(ext: PlaceholderExtension) {
	const { placeholder } = ext.options

	if (typeof placeholder !== "function") {
		throw new Error("placeholder option is not a function")
	}

	return placeholder
}

function editorFor(doc: PMNode, isEmpty = false): Editor {
	return {
		state: EditorState.create({ doc }),
		isEmpty,
	} as unknown as Editor
}

function nodeAt(doc: PMNode, pos: number): PMNode {
	const node = doc.nodeAt(pos)

	if (!node) {
		throw new Error(`no node at position ${pos}`)
	}

	return node
}

// instantiates the extension's prosemirror plugin the same way tiptap does
// (options resolved through the extension instance) and returns the
// decoration set it produces for the given state
function decorationsOf(
	ext: PlaceholderExtension,
	state: EditorState,
	editorFlags: { isEditable?: boolean; isEmpty?: boolean } = {},
) {
	const create = ext.config.addProseMirrorPlugins

	if (!create) {
		throw new Error("addProseMirrorPlugins is not defined")
	}

	const editor = {
		isEditable: editorFlags.isEditable ?? true,
		isEmpty: editorFlags.isEmpty ?? false,
		state,
	} as unknown as Editor

	const ctx = {
		editor,
		options: ext.options,
	} as unknown as ThisParameterType<NonNullable<typeof create>>

	const plugin = create.call(ctx)[0]

	if (!plugin) {
		throw new Error("no prosemirror plugin was created")
	}

	const decorations = plugin.props.decorations

	if (!decorations) {
		throw new Error("decorations prop is not defined")
	}

	return decorations.call(plugin, state) as DecorationSet | null
}

// Decoration keeps its rendered attributes on an internal field with no
// public accessor, so the assertion helper reaches into it deliberately
function shape(set: DecorationSet | null) {
	if (!set) {
		return null
	}

	return set.find().map((decoration) => {
		const { attrs } = (
			decoration as unknown as {
				type: { attrs: Record<string, string | undefined> }
			}
		).type

		return {
			from: decoration.from,
			to: decoration.to,
			class: attrs.class,
			placeholder: attrs["data-placeholder"],
		}
	})
}

function stateWithAnchor(doc: PMNode, anchor: number) {
	return EditorState.create({
		doc,
		selection: TextSelection.create(doc, anchor),
	})
}

describe("defaultNamePlaceholder", () => {
	it("returns the default name placeholder", ({ expect }) => {
		const doc = block("doc", block("paragraph"))
		const fn = placeholderFn(defaultNamePlaceholder(t))

		const result = fn({
			editor: editorFor(doc),
			node: nodeAt(doc, 0),
			pos: 0,
			hasAnchor: true,
		})

		expect(result).toBe("editor.placeholders.name-default")
	})

	it("decorates every empty top-level node regardless of the anchor", ({
		expect,
	}) => {
		// paragraphs span [0, 2), [2, 5), and [5, 7)
		const doc = block(
			"doc",
			block("paragraph"),
			block("paragraph", "x"),
			block("paragraph"),
		)

		const set = decorationsOf(
			defaultNamePlaceholder(t),
			stateWithAnchor(doc, 1),
		)

		expect(shape(set)).toEqual([
			{
				from: 0,
				to: 2,
				class: placeholderEmptyNodeClass,
				placeholder: "editor.placeholders.name-default",
			},
			{
				from: 5,
				to: 7,
				class: placeholderEmptyNodeClass,
				placeholder: "editor.placeholders.name-default",
			},
		])
	})

	it("skips the empty children of an empty node", ({ expect }) => {
		// the callout spans [0, 4) with an empty paragraph at [1, 3)
		const doc = block("doc", block(CalloutBlock.name, block("paragraph")))

		const set = decorationsOf(
			defaultNamePlaceholder(t),
			stateWithAnchor(doc, 2),
		)

		expect(shape(set)).toEqual([
			{
				from: 0,
				to: 4,
				class: placeholderEmptyNodeClass,
				placeholder: "editor.placeholders.name-default",
			},
		])
	})

	it("adds the empty editor class when the document is empty", ({ expect }) => {
		const doc = block("doc", block("paragraph"))

		const set = decorationsOf(
			defaultNamePlaceholder(t),
			stateWithAnchor(doc, 1),
			{ isEmpty: true },
		)

		expect(shape(set)).toEqual([
			{
				from: 0,
				to: 2,
				class: `${placeholderEmptyNodeClass} is-editor-empty`,
				placeholder: "editor.placeholders.name-default",
			},
		])
	})

	it("decorates nothing when the editor is not editable", ({ expect }) => {
		const doc = block("doc", block("paragraph"))

		const set = decorationsOf(
			defaultNamePlaceholder(t),
			stateWithAnchor(doc, 1),
			{ isEditable: false },
		)

		expect(shape(set)).toBe(null)
	})
})

describe("defaultContentPlaceholder", () => {
	const rows: {
		name: string
		make: () => PMNode
		pos: number
		disabled?: boolean
		location?: "document" | "comment"
		isEmpty?: boolean
		expected: string
	}[] = [
		{
			name: "returns the heading key for a top-level heading",
			make: () => block("doc", block("heading")),
			pos: 0,
			expected: "editor.placeholders.content.heading",
		},
		{
			name: "returns the left-side heading key inside the left side",
			make: () =>
				block("doc", block(SplitDocumentationLeftSide.name, block("heading"))),
			pos: 1,
			expected:
				"editor.placeholders.content.split-documentation.left-side-heading",
		},
		{
			name: "returns the read-only left-side heading key when disabled",
			make: () =>
				block("doc", block(SplitDocumentationLeftSide.name, block("heading"))),
			pos: 1,
			disabled: true,
			expected:
				"editor.placeholders.content.split-documentation.left-side-heading-empty",
		},
		{
			name: "returns the item key for a paragraph in a parameter list item",
			make: () =>
				block("doc", block(ParameterListItem.name, block("paragraph"))),
			pos: 1,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.paragraph",
		},
		{
			name: "returns the read-only item key when disabled",
			make: () =>
				block("doc", block(ParameterListItem.name, block("paragraph"))),
			pos: 1,
			disabled: true,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.paragraph-empty",
		},
		{
			name: "returns the left-side paragraph key inside the left side",
			make: () =>
				block(
					"doc",
					block(SplitDocumentationLeftSide.name, block("paragraph")),
				),
			pos: 1,
			expected:
				"editor.placeholders.content.split-documentation.left-side-paragraph",
		},
		{
			name: "returns the read-only left-side paragraph key when disabled",
			make: () =>
				block(
					"doc",
					block(SplitDocumentationLeftSide.name, block("paragraph")),
				),
			pos: 1,
			disabled: true,
			expected:
				"editor.placeholders.content.split-documentation.left-side-paragraph-empty",
		},
		{
			name: "prefers the parameter item key over the surrounding left side",
			make: () =>
				block(
					"doc",
					block(
						SplitDocumentationLeftSide.name,
						block(ParameterListItem.name, block("paragraph")),
					),
				),
			pos: 2,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.paragraph",
		},
		{
			name: "returns the callout content key for a paragraph in a callout",
			make: () => block("doc", block(CalloutBlock.name, block("paragraph"))),
			pos: 1,
			expected: "editor.placeholders.content.callout.content",
		},
		{
			name: "returns the paragraph key for a top-level paragraph",
			make: () => block("doc", block("paragraph")),
			pos: 0,
			expected: "editor.placeholders.content.paragraph",
		},
		{
			name: "returns the comment key for an empty comment editor",
			make: () => block("doc", block("paragraph")),
			pos: 0,
			location: "comment",
			isEmpty: true,
			expected: "editor.placeholders.content.comment",
		},
		{
			name: "returns an empty string for a non-empty comment editor",
			make: () => block("doc", block("paragraph")),
			pos: 0,
			location: "comment",
			expected: "",
		},
		{
			name: "returns an empty string for unhandled node types",
			make: () => block("doc", block(CalloutBlock.name, block("paragraph"))),
			pos: 0,
			expected: "",
		},
	]

	it.for(rows)(
		"$name",
		({ make, pos, disabled, location, isEmpty, expected }, { expect }) => {
			const doc = make()
			const fn = placeholderFn(
				defaultContentPlaceholder(t, disabled ?? false, location),
			)

			const result = fn({
				editor: editorFor(doc, isEmpty ?? false),
				node: nodeAt(doc, pos),
				pos,
				hasAnchor: true,
			})

			expect(result).toBe(expected)
		},
	)

	it("unwraps a ref passed as isEditingDisabled", ({ expect }) => {
		const doc = block(
			"doc",
			block(SplitDocumentationLeftSide.name, block("heading")),
		)
		const fn = placeholderFn(defaultContentPlaceholder(t, ref(true)))

		const result = fn({
			editor: editorFor(doc),
			node: nodeAt(doc, 1),
			pos: 1,
			hasAnchor: true,
		})

		expect(result).toBe(
			"editor.placeholders.content.split-documentation.left-side-heading-empty",
		)
	})

	it("decorates the empty node under the anchor", ({ expect }) => {
		// paragraphs span [0, 2) and [2, 5)
		const doc = block("doc", block("paragraph"), block("paragraph", "x"))

		const set = decorationsOf(
			defaultContentPlaceholder(t, false),
			stateWithAnchor(doc, 1),
		)

		expect(shape(set)).toEqual([
			{
				from: 0,
				to: 2,
				class: placeholderEmptyNodeClass,
				placeholder: "editor.placeholders.content.paragraph",
			},
		])
	})

	it("decorates empty nodes inside a callout without the anchor", ({
		expect,
	}) => {
		// the callout spans [3, 7) with an empty paragraph at [4, 6); the
		// anchor stays in the first paragraph
		const doc = block(
			"doc",
			block("paragraph", "x"),
			block(CalloutBlock.name, block("paragraph")),
		)

		const set = decorationsOf(
			defaultContentPlaceholder(t, false),
			stateWithAnchor(doc, 1),
		)

		expect(shape(set)).toEqual([
			{
				from: 4,
				to: 6,
				class: placeholderEmptyNodeClass,
				placeholder: "editor.placeholders.content.callout.content",
			},
		])
	})

	it("decorates empty nodes inside the split documentation parents", ({
		expect,
	}) => {
		// the left side spans [3, 7) with an empty heading at [4, 6)
		const doc = block(
			"doc",
			block("paragraph", "x"),
			block(SplitDocumentationLeftSide.name, block("heading")),
		)

		const set = decorationsOf(
			defaultContentPlaceholder(t, false),
			stateWithAnchor(doc, 1),
		)

		expect(shape(set)).toEqual([
			{
				from: 4,
				to: 6,
				class: placeholderEmptyNodeClass,
				placeholder:
					"editor.placeholders.content.split-documentation.left-side-heading",
			},
		])
	})

	it("decorates nothing when the editor is not editable", ({ expect }) => {
		const doc = block("doc", block("paragraph"))

		const set = decorationsOf(
			defaultContentPlaceholder(t, false),
			stateWithAnchor(doc, 1),
			{ isEditable: false },
		)

		expect(shape(set)).toBe(null)
	})
})

describe("explicitContentPlaceholder", () => {
	const rows: {
		name: string
		nodeType: string
		disabled: boolean
		expected: string
	}[] = [
		{
			name: "an editable code block",
			nodeType: CODE_BLOCK_NAME,
			disabled: false,
			expected: "editor.placeholders.content.extended-code-block.content",
		},
		{
			name: "a read-only code block",
			nodeType: CODE_BLOCK_NAME,
			disabled: true,
			expected: "editor.placeholders.content.extended-code-block.content-empty",
		},
		{
			name: "an editable code block title",
			nodeType: CODE_BLOCK_TITLE_NAME,
			disabled: false,
			expected: "editor.placeholders.content.extended-code-block.header-title",
		},
		{
			name: "a read-only code block title",
			nodeType: CODE_BLOCK_TITLE_NAME,
			disabled: true,
			expected:
				"editor.placeholders.content.extended-code-block.header-title-empty",
		},
		{
			name: "an editable parameter list header",
			nodeType: ParameterListHeader.name,
			disabled: false,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.header-name",
		},
		{
			name: "a read-only parameter list header",
			nodeType: ParameterListHeader.name,
			disabled: true,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.header-name-empty",
		},
		{
			name: "an editable parameter item type header",
			nodeType: ParameterListItemHeaderType.name,
			disabled: false,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.header-type",
		},
		{
			name: "a read-only parameter item type header",
			nodeType: ParameterListItemHeaderType.name,
			disabled: true,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.header-type-empty",
		},
		{
			name: "an editable parameter item title header",
			nodeType: ParameterListItemHeaderTitle.name,
			disabled: false,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.header-name",
		},
		{
			name: "a read-only parameter item title header",
			nodeType: ParameterListItemHeaderTitle.name,
			disabled: true,
			expected:
				"editor.placeholders.content.split-documentation.parameter-list.item.header-name-empty",
		},
		{
			name: "an editable mermaid block",
			nodeType: MERMAID_BLOCK_NAME,
			disabled: false,
			expected: "editor.placeholders.content.mermaid.content",
		},
		{
			name: "a read-only mermaid block",
			nodeType: MERMAID_BLOCK_NAME,
			disabled: true,
			expected: "editor.placeholders.content.mermaid.content-empty",
		},
	]

	it.for(rows)(
		"returns the $expected key for $name",
		({ nodeType, disabled, expected }, { expect }) => {
			expect(explicitContentPlaceholder(t, nodeType, disabled)).toBe(expected)
		},
	)

	it("returns an empty string for unknown node types", ({ expect }) => {
		expect(explicitContentPlaceholder(t, "paragraph", false)).toBe("")
	})

	it("unwraps a ref passed as isEditingDisabled", ({ expect }) => {
		expect(explicitContentPlaceholder(t, CODE_BLOCK_NAME, ref(true))).toBe(
			"editor.placeholders.content.extended-code-block.content-empty",
		)
	})
})

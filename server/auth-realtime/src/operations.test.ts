import { describe, it } from "vitest"
import * as Y from "yjs"
import { replaceYdocContent } from "./ydocument.js"
import {
	applyOperations,
	findByUid,
	pmBlockToY,
	type InsertOp,
	type Operation,
	type PMInline,
	type PMNode,
} from "./operations.js"

const SAMPLE_NAME = "Runbook"
const SAMPLE_ICON = "lucide:file"

function textParagraph(uid: string, text: string): PMNode {
	return {
		type: "paragraph",
		attrs: { uid },
		content: [{ type: "text", text }],
	}
}

function emptyParagraph(uid: string): PMNode {
	return { type: "paragraph", attrs: { uid } }
}

// a realistic editor document: an intro paragraph, a callout wrapping a
// nested paragraph, and a heading last. Tests that care about TipTap's
// trailing empty paragraph append one themselves.
function sampleBlocks(): PMNode[] {
	return [
		textParagraph("intro", "Intro line"),
		{
			type: "calloutBlock",
			attrs: { uid: "callout", icon: "lucide:zap" },
			content: [textParagraph("nested", "Nested line")],
		},
		{
			type: "heading",
			attrs: { uid: "head", level: 2 },
			content: [{ type: "text", text: "Section" }],
		},
	]
}

// builds a live document through the same path the Hocuspocus server
// uses, so every block carries the uid the uid-addressed operations
// look up.
function docWith(blocks: PMNode[]): Y.Doc {
	const doc = new Y.Doc()
	replaceYdocContent(doc, {
		name: SAMPLE_NAME,
		content: { type: "doc", content: blocks },
		icon: SAMPLE_ICON,
	})

	return doc
}

// a paragraph whose only child is a block element. The transformer
// cannot produce one — paragraphs hold inline content — so it is built
// by hand to exercise the element branch of the trailing-node check.
function paragraphWrappingElement(uid: string): Y.XmlElement {
	const para = new Y.XmlElement("paragraph")
	para.setAttribute("uid", uid)

	const inner = new Y.XmlElement("paragraph")
	inner.setAttribute("uid", `${uid}-inner`)
	para.insert(0, [inner])

	return para
}

// a paragraph holding a zero-length Y.XmlText, which is still the
// trailing-node affordance as far as opAppend is concerned.
function paragraphWithBlankText(uid: string): Y.XmlElement {
	const para = new Y.XmlElement("paragraph")
	para.setAttribute("uid", uid)
	para.insert(0, [new Y.XmlText()])

	return para
}

// the uids of the content fragment's direct children, which is what
// most position assertions come down to.
function topUids(doc: Y.Doc): (string | undefined)[] {
	return doc
		.getXmlFragment("content")
		.toArray()
		.map((node) =>
			node instanceof Y.XmlElement
				? node.getAttribute("uid")
				: undefined,
		)
}

// resolves a block for assertions, throwing rather than returning null
// so callers keep a non-nullable element.
function blockByUid(doc: Y.Doc, uid: string): Y.XmlElement {
	const found = findByUid(doc.getXmlFragment("content"), uid)
	if (!found) {
		throw new Error(`test fixture has no block: ${uid}`)
	}

	return found.element
}

// getAttributes is declared as a string map, but the runtime keeps
// whatever value it was given — numbers and arrays included — so
// assertions widen it once here.
function attrsOf(el: Y.XmlElement): Record<string, unknown> {
	return el.getAttributes() as Record<string, unknown>
}

// Y.XmlText.toDelta is declared as returning any; narrowing it in one
// place keeps every call site free of unsafe values.
function inlineDelta(el: Y.XmlElement): unknown[] {
	const child = el.get(0)
	if (!(child instanceof Y.XmlText)) {
		throw new Error("block carries no inline text")
	}

	return child.toDelta() as unknown[]
}

// neither Y.XmlElement nor Y.Text declares a string-returning toString
// in its type definitions, though both serialize properly at runtime.
// Narrowing it once here keeps the assertions readable.
function serialize(node: Y.XmlElement | Y.Text): string {
	return (node as unknown as { toString(): string }).toString()
}

// a detached Y.XmlElement keeps its attributes and children in prelim
// state, invisible to the public getters, so assertions read it after
// attaching it to a throwaway host — which is what the operations do
// with it anyway.
function attached(el: Y.XmlElement): Y.XmlElement {
	new Y.Doc().getXmlFragment("content").insert(0, [el])

	return el
}

describe("applyOperations", () => {
	it("keeps applying later operations after one of them fails", ({
		expect,
	}) => {
		const doc = docWith(sampleBlocks())

		const result = applyOperations(doc, [
			{
				kind: "prepend",
				block: textParagraph("first", "First"),
			},
			{ kind: "delete", block_uid: "ghost" },
			{
				kind: "append",
				block: textParagraph("last", "Last"),
			},
		])

		expect(result.applied).toBe(2)
		expect(result.errors).toEqual([
			{ index: 1, message: "block_uid not found: ghost" },
		])
		expect(topUids(doc)).toEqual([
			"first",
			"intro",
			"callout",
			"head",
			"last",
		])
	})

	it("emits one consolidated update for the whole batch", ({
		expect,
	}) => {
		const doc = docWith(sampleBlocks())
		let updates = 0
		doc.on("update", () => {
			updates++
		})

		applyOperations(doc, [
			{ kind: "set_name", name: "Renamed" },
			{ kind: "set_icon", icon: "lucide:zap" },
			{
				kind: "prepend",
				block: textParagraph("first", "First"),
			},
		])

		expect(updates).toBe(1)
	})

	it("reports nothing applied for an empty batch", ({ expect }) => {
		const doc = docWith(sampleBlocks())

		expect(applyOperations(doc, [])).toEqual({
			applied: 0,
			errors: [],
		})
		expect(topUids(doc)).toEqual(["intro", "callout", "head"])
	})

	// an operation kind this service does not implement means the two
	// sides have drifted apart; counting the no-op as applied would tell
	// the caller its edit landed
	it("reports an error for an operation kind it does not implement", ({
		expect,
	}) => {
		const doc = docWith(sampleBlocks())

		const result = applyOperations(doc, [
			{ kind: "transmogrify" } as unknown as Operation,
		])

		expect(result.applied).toBe(0)
		expect(result.errors).toEqual([
			{
				index: 0,
				message: "unknown operation kind: transmogrify",
			},
		])
		expect(topUids(doc)).toEqual(["intro", "callout", "head"])
	})

	describe("insert", () => {
		const positionCases: {
			name: string
			input: InsertOp["position"]
			expected: string[]
		}[] = [
			{
				name: "before",
				input: "before",
				expected: ["intro", "added", "callout", "head"],
			},
			{
				name: "after",
				input: "after",
				expected: ["intro", "callout", "added", "head"],
			},
		]

		it.for(positionCases)(
			"places the block $name the reference block",
			({ input, expected }, { expect }) => {
				const doc = docWith(sampleBlocks())

				const result = applyOperations(doc, [
					{
						kind: "insert",
						position: input,
						reference_uid: "callout",
						block: textParagraph(
							"added",
							"Added",
						),
					},
				])

				expect(result.applied).toBe(1)
				expect(topUids(doc)).toEqual(expected)
			},
		)

		it("inserts next to a nested block inside that block's own parent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "insert",
					position: "after",
					reference_uid: "nested",
					block: textParagraph(
						"sibling",
						"Sibling",
					),
				},
			])

			const callout = blockByUid(doc, "callout")
			expect(
				callout
					.toArray()
					.map((node) =>
						node instanceof Y.XmlElement
							? node.getAttribute(
									"uid",
								)
							: undefined,
					),
			).toEqual(["nested", "sibling"])
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

		// the position arrives as unvalidated JSON, and taking anything
		// that is not "before" as "after" would put the block on the
		// wrong side of the reference without saying so
		it("reports an error for a position that is neither before nor after", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: "beforeX",
					reference_uid: "callout",
					block: textParagraph("added", "Added"),
				} as unknown as Operation,
			])

			expect(result.applied).toBe(0)
			expect(result.errors[0]?.message).toBe(
				'insert position must be "before" or "after", got: beforeX',
			)
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

		it("reports an error when the reference block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: "after",
					reference_uid: "ghost",
					block: textParagraph("added", "Added"),
				},
			])

			expect(result).toEqual({
				applied: 0,
				errors: [
					{
						index: 0,
						message: "reference_uid not found: ghost",
					},
				],
			})
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})
	})

	describe("append", () => {
		it("adds the block after a last block that is not a paragraph", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
				"added",
			])
		})

		it("adds the block before TipTap's trailing empty paragraph", ({
			expect,
		}) => {
			const doc = docWith([
				...sampleBlocks(),
				emptyParagraph("trailing"),
			])

			applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
				"added",
				"trailing",
			])
		})

		it("adds the block after a last paragraph that still carries text", ({
			expect,
		}) => {
			const doc = docWith([
				...sampleBlocks(),
				textParagraph("outro", "Outro line"),
			])

			applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
				"outro",
				"added",
			])
		})

		it("adds the block after a last paragraph that wraps an element", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())
			const frag = doc.getXmlFragment("content")
			frag.insert(frag.length, [
				paragraphWrappingElement("wrapper"),
			])

			applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
				"wrapper",
				"added",
			])
		})

		it("adds the block before a last paragraph holding only blank text", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())
			const frag = doc.getXmlFragment("content")
			frag.insert(frag.length, [
				paragraphWithBlankText("trailing"),
			])

			applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
				"added",
				"trailing",
			])
		})

		it("adds the block after a trailing node that is not an element", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())
			const frag = doc.getXmlFragment("content")
			const stray = new Y.XmlText()
			frag.insert(frag.length, [stray])
			stray.insert(0, "stray")

			applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
				undefined,
				"added",
			])
		})

		it("adds the block as the only child of an empty fragment", ({
			expect,
		}) => {
			const doc = new Y.Doc()

			const result = applyOperations(doc, [
				{
					kind: "append",
					block: textParagraph("added", "Added"),
				},
			])

			expect(result.applied).toBe(1)
			expect(topUids(doc)).toEqual(["added"])
			expect(serialize(blockByUid(doc, "added"))).toBe(
				'<paragraph uid="added">Added</paragraph>',
			)
		})
	})

	describe("prepend", () => {
		it("adds the block at index 0", ({ expect }) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "prepend",
					block: textParagraph("added", "Added"),
				},
			])

			expect(result.applied).toBe(1)
			expect(topUids(doc)).toEqual([
				"added",
				"intro",
				"callout",
				"head",
			])
		})
	})

	describe("replace", () => {
		it("swaps the block in place, keeping its index", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "callout",
					block: textParagraph("fresh", "Fresh"),
				},
			])

			expect(result.applied).toBe(1)
			expect(topUids(doc)).toEqual(["intro", "fresh", "head"])
			expect(
				findByUid(
					doc.getXmlFragment("content"),
					"nested",
				),
			).toBeNull()
		})

		it("swaps a nested block inside that block's own parent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "nested",
					block: textParagraph("fresh", "Fresh"),
				},
			])

			const found = findByUid(
				doc.getXmlFragment("content"),
				"fresh",
			)
			expect(found?.index).toBe(0)
			expect(found?.parent).toBe(blockByUid(doc, "callout"))
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

		it("reports an error when the block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "ghost",
					block: textParagraph("fresh", "Fresh"),
				},
			])

			expect(result).toEqual({
				applied: 0,
				errors: [
					{
						index: 0,
						message: "block_uid not found: ghost",
					},
				],
			})
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})
	})

	describe("update_text", () => {
		it("swaps the inline content while keeping the block's type, attrs and uid", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "update_text",
					block_uid: "head",
					content: [
						{
							type: "text",
							text: "Rewritten",
						},
					],
				},
			])

			const head = blockByUid(doc, "head")
			expect(result.applied).toBe(1)
			expect(head.nodeName).toBe("heading")
			expect(attrsOf(head)).toEqual({ uid: "head", level: 2 })
			expect(head.length).toBe(1)
			expect(inlineDelta(head)).toEqual([
				{ insert: "Rewritten" },
			])
		})

		it("turns marks into format attributes on the inline text", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "update_text",
					block_uid: "intro",
					content: [
						{
							type: "text",
							text: "plain ",
						},
						{
							type: "text",
							text: "bold",
							marks: [
								{
									type: "bold",
								},
							],
						},
						{
							type: "text",
							text: "link",
							marks: [
								{
									type: "link",
									attrs: {
										href: "https://oxynote.test",
									},
								},
							],
						},
					],
				},
			])

			expect(inlineDelta(blockByUid(doc, "intro"))).toEqual([
				{ insert: "plain " },
				{ insert: "bold", attributes: { bold: true } },
				{
					insert: "link",
					attributes: {
						link: {
							href: "https://oxynote.test",
						},
					},
				},
			])
		})

		it("leaves the block without children when the content is empty", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "update_text",
					block_uid: "intro",
					content: [],
				},
			])

			const intro = blockByUid(doc, "intro")
			expect(intro.length).toBe(0)
			expect(serialize(intro)).toBe(
				'<paragraph uid="intro"></paragraph>',
			)
		})

		it("leaves the block without children when no inline node carries text", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "update_text",
					block_uid: "intro",
					content: [
						{
							type: "hardBreak",
						} as unknown as PMInline,
						{ type: "text", text: "" },
					],
				},
			])

			expect(blockByUid(doc, "intro").length).toBe(0)
		})

		it("reports an error when the block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "update_text",
					block_uid: "ghost",
					content: [
						{
							type: "text",
							text: "Rewritten",
						},
					],
				},
			])

			expect(result).toEqual({
				applied: 0,
				errors: [
					{
						index: 0,
						message: "block_uid not found: ghost",
					},
				],
			})
		})
	})

	describe("update_attrs", () => {
		it("sets the named attributes and keeps the untouched ones", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "update_attrs",
					block_uid: "callout",
					attrs: {
						icon: "lucide:bug",
						queries: [{ query: "up" }],
					},
				},
			])

			expect(result.applied).toBe(1)
			expect(attrsOf(blockByUid(doc, "callout"))).toEqual({
				uid: "callout",
				icon: "lucide:bug",
				queries: [{ query: "up" }],
			})
		})

		it("refuses to change the uid while applying the other attributes", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "update_attrs",
					block_uid: "head",
					attrs: { uid: "stolen", level: 3 },
				},
			])

			expect(attrsOf(blockByUid(doc, "head"))).toEqual({
				uid: "head",
				level: 3,
			})
			expect(
				findByUid(
					doc.getXmlFragment("content"),
					"stolen",
				),
			).toBeNull()
		})

		it("reports an error when the block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "update_attrs",
					block_uid: "ghost",
					attrs: { level: 3 },
				},
			])

			expect(result).toEqual({
				applied: 0,
				errors: [
					{
						index: 0,
						message: "block_uid not found: ghost",
					},
				],
			})
		})
	})

	describe("delete", () => {
		it("removes the block and everything nested inside it", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "callout" },
			])

			expect(result.applied).toBe(1)
			expect(topUids(doc)).toEqual(["intro", "head"])
			expect(
				findByUid(
					doc.getXmlFragment("content"),
					"nested",
				),
			).toBeNull()
		})

		it("reports an error when the block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "ghost" },
			])

			expect(result).toEqual({
				applied: 0,
				errors: [
					{
						index: 0,
						message: "block_uid not found: ghost",
					},
				],
			})
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})
	})

	describe("set_name", () => {
		it("replaces the name fragment with one paragraph carrying the text and a generated uid", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{ kind: "set_name", name: "Renamed" },
			])

			const frag = doc.getXmlFragment("name")
			const para = frag.get(0)
			expect(result.applied).toBe(1)
			expect(frag.length).toBe(1)
			expect(para).toBeInstanceOf(Y.XmlElement)

			if (!(para instanceof Y.XmlElement)) {
				return
			}

			expect(para.nodeName).toBe("paragraph")
			expect(serialize(para)).toContain(">Renamed<")

			const uid = para.getAttribute("uid")
			expect(typeof uid).toBe("string")
			expect(uid).not.toBe("")
		})

		it("leaves the paragraph without a text child for an empty name", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [{ kind: "set_name", name: "" }])

			const frag = doc.getXmlFragment("name")
			const para = frag.get(0)
			expect(frag.length).toBe(1)
			expect(para).toBeInstanceOf(Y.XmlElement)

			if (!(para instanceof Y.XmlElement)) {
				return
			}

			expect(para.length).toBe(0)
			expect(para.getAttribute("uid")).not.toBe("")
		})
	})

	describe("set_icon", () => {
		const iconCases: {
			name: string
			input: string
			expected: string
		}[] = [
			{
				name: "a new identifier",
				input: "lucide:bug",
				expected: "lucide:bug",
			},
			{ name: "an empty string", input: "", expected: "" },
		]

		it.for(iconCases)(
			"replaces the icon text with $name",
			({ input, expected }, { expect }) => {
				const doc = docWith(sampleBlocks())
				expect(serialize(doc.getText("icon"))).toBe(
					SAMPLE_ICON,
				)

				const result = applyOperations(doc, [
					{ kind: "set_icon", icon: input },
				])

				expect(result.applied).toBe(1)
				expect(serialize(doc.getText("icon"))).toBe(
					expected,
				)
			},
		)
	})
})

describe("findByUid", () => {
	it("returns a top-level block with the content fragment as its parent", ({
		expect,
	}) => {
		const doc = docWith(sampleBlocks())
		const frag = doc.getXmlFragment("content")

		const found = findByUid(frag, "head")

		expect(found?.parent).toBe(frag)
		expect(found?.index).toBe(2)
		expect(found?.element.nodeName).toBe("heading")
	})

	it("returns a nested block with its enclosing element as parent", ({
		expect,
	}) => {
		const doc = docWith(sampleBlocks())
		const callout = blockByUid(doc, "callout")

		const found = findByUid(doc.getXmlFragment("content"), "nested")

		expect(found?.parent).toBe(callout)
		expect(found?.index).toBe(0)
		expect(found?.element).toBe(callout.get(0))
	})

	it("returns null when no block carries the uid", ({ expect }) => {
		const doc = docWith(sampleBlocks())

		expect(
			findByUid(doc.getXmlFragment("content"), "ghost"),
		).toBeNull()
	})

	it("skips children that are not elements", ({ expect }) => {
		const doc = new Y.Doc()
		const frag = doc.getXmlFragment("content")
		const stray = new Y.XmlText()
		frag.insert(0, [stray])
		stray.insert(0, "stray")
		frag.insert(1, [paragraphWrappingElement("wrapper")])

		const found = findByUid(frag, "wrapper-inner")

		expect(found?.index).toBe(0)
		expect(found?.parent).toBe(frag.get(1))
	})
})

describe("pmBlockToY", () => {
	it("returns a detached element carrying the block's node name, attributes and children", ({
		expect,
	}) => {
		const el = pmBlockToY({
			type: "calloutBlock",
			attrs: { uid: "callout", icon: "lucide:zap" },
			content: [textParagraph("nested", "Nested line")],
		})

		expect(el.nodeName).toBe("calloutBlock")
		expect(el.doc).toBeNull()
		expect(el.parent).toBeNull()

		const live = attached(el)
		expect(attrsOf(live)).toEqual({
			uid: "callout",
			icon: "lucide:zap",
		})
		expect(serialize(live)).toBe(
			'<calloutblock icon="lucide:zap" uid="callout">' +
				'<paragraph uid="nested">Nested line</paragraph>' +
				"</calloutblock>",
		)
	})

	it("preserves attribute values that are not strings", ({ expect }) => {
		const el = pmBlockToY({
			type: "metricBlock",
			attrs: {
				uid: "metric",
				queries: [{ query: "up", legend: "uptime" }],
			},
		})

		expect(attrsOf(attached(el))).toEqual({
			uid: "metric",
			queries: [{ query: "up", legend: "uptime" }],
		})
	})

	it("throws when the transformer yields no element for the block", ({
		expect,
	}) => {
		// a bare inline text node is not a block, so the transformer
		// puts a Y.XmlText at the top of the fragment
		expect(() =>
			pmBlockToY({ type: "text", text: "loose" }),
		).toThrow("pmBlockToY: transformer produced no XmlElement")
	})
})

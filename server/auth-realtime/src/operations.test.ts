import { describe, it } from "vitest"
import * as Y from "yjs"
import { replaceYdocContent } from "./ydocument.js"
import {
	applyOperations,
	type InsertOp,
	type MoveOp,
	type Operation,
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

// the block carrying uid anywhere in the content, if there is one.
function findBlock(doc: Y.Doc, uid: string): Y.XmlElement | undefined {
	const [found] = doc
		.getXmlFragment("content")
		.createTreeWalker(
			(node) =>
				node instanceof Y.XmlElement &&
				node.getAttribute("uid") === uid,
		)

	return found instanceof Y.XmlElement ? found : undefined
}

// resolves a block for assertions, throwing rather than returning
// undefined so callers keep a non-nullable element.
function blockByUid(doc: Y.Doc, uid: string): Y.XmlElement {
	const found = findBlock(doc, uid)
	if (!found) {
		throw new Error(`test fixture has no block: ${uid}`)
	}

	return found
}

// getAttributes is declared as a string map, but the runtime keeps
// whatever value it was given — numbers and arrays included — so
// assertions widen it once here.
function attrsOf(el: Y.XmlElement): Record<string, unknown> {
	return el.getAttributes()
}

// neither Y.XmlElement nor Y.Text declares a string-returning toString
// in its type definitions, though both serialize properly at runtime.
// Narrowing it once here keeps the assertions readable.
function serialize(node: Y.XmlElement | Y.Text): string {
	return (node as unknown as { toString(): string }).toString()
}

describe("applyOperations", () => {
	it("applies nothing when one operation fails", ({ expect }) => {
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

		expect(result).toEqual({
			errors: [
				{
					index: 1,
					message: "block_uid not found: ghost",
				},
			],
		})
		expect(topUids(doc)).toEqual(["intro", "callout", "head"])
	})

	it("reports only the first failure of a batch", ({ expect }) => {
		const doc = docWith(sampleBlocks())

		const result = applyOperations(doc, [
			{ kind: "delete", block_uid: "ghost" },
			{ kind: "delete", block_uid: "phantom" },
		])

		expect(result).toEqual({
			errors: [
				{
					index: 0,
					message: "block_uid not found: ghost",
				},
			],
		})
	})

	it("applies a batch whose operations build on each other", ({
		expect,
	}) => {
		const doc = docWith(sampleBlocks())

		const result = applyOperations(doc, [
			{
				kind: "prepend",
				block: textParagraph("first", "First"),
			},
			{
				kind: "insert",
				position: "after",
				reference_uid: "first",
				block: textParagraph("second", "Second"),
			},
		])

		expect(result).toEqual({ errors: [] })
		expect(topUids(doc)).toEqual([
			"first",
			"second",
			"intro",
			"callout",
			"head",
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

		expect(result.errors).toEqual([
			{
				index: 0,
				message: "unknown operation kind: transmogrify",
			},
		])
		expect(topUids(doc)).toEqual(["intro", "callout", "head"])
	})

	describe("insert", () => {
		it("refuses a block the parent does not take, leaving the document as it was", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: "after",
					reference_uid: "intro",
					block: {
						type: "metricBlock",
						attrs: { uid: "metric" },
					},
				},
			])

			expect(result.errors[0]?.message).toContain(
				"metricBlock is not allowed there in the document root",
			)
			expect(result.errors[0]?.message).toContain(
				"Choose a reference block in a container that takes it.",
			)
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

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

				expect(result.errors).toEqual([])
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
		it("refuses a block the document root does not take", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "append",
					block: {
						type: "metricBlock",
						attrs: { uid: "metric" },
					},
				},
			])

			expect(result.errors[0]?.message).toContain(
				"Insert it before or after a block in a container that takes it.",
			)
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

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

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual(["added"])
			expect(serialize(blockByUid(doc, "added"))).toBe(
				'<paragraph uid="added">Added</paragraph>',
			)
		})
		it("keeps attribute values that are not strings", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			applyOperations(doc, [
				{
					kind: "append",
					block: {
						type: "metricGrid",
						attrs: { uid: "grid" },
						content: [
							{
								type: "metricBlock",
								attrs: {
									uid: "metric",
									queries: [
										{
											query: "up",
										},
									],
								},
							},
						],
					},
				},
			])

			// the simulation flag is the one metric attribute whose
			// schema default is not null, so it is written out too.
			expect(attrsOf(blockByUid(doc, "metric"))).toEqual({
				uid: "metric",
				queries: [{ query: "up" }],
				simulationActive: false,
			})
		})

		it("reports an error for a bare text node", ({ expect }) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "append",
					block: { type: "text", text: "loose" },
				},
			])

			expect(result.errors).toEqual([
				{ index: 0, message: "text is not a block" },
			])
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})
	})

	describe("prepend", () => {
		it("refuses a block the document root does not take", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "prepend",
					block: {
						type: "metricBlock",
						attrs: { uid: "metric" },
					},
				},
			])

			expect(result.errors).toHaveLength(1)
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

		it("adds the block at index 0", ({ expect }) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "prepend",
					block: textParagraph("added", "Added"),
				},
			])

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual([
				"added",
				"intro",
				"callout",
				"head",
			])
		})
	})

	describe("replace", () => {
		it("refuses a block its container does not take, leaving the old one", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "nested",
					block: {
						type: "heading",
						attrs: {
							uid: "nested",
							level: 2,
						},
						content: [
							{
								type: "text",
								text: "Heading",
							},
						],
					},
				},
			])

			expect(result.errors[0]?.message).toContain(
				"heading is not allowed there in calloutBlock",
			)
			expect(result.errors[0]?.message).not.toContain(
				"Choose",
			)
			expect(blockByUid(doc, "nested").nodeName).toBe(
				"paragraph",
			)
		})

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

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual(["intro", "fresh", "head"])
			expect(findBlock(doc, "nested")).toBeUndefined()
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

			expect(blockByUid(doc, "callout").get(0)).toBe(
				blockByUid(doc, "fresh"),
			)
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

			expect(result.errors).toEqual([])
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
			expect(findBlock(doc, "stolen")).toBeUndefined()
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
		it("refuses to delete a container's last block", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "nested" },
			])

			expect(result.errors[0]?.message).toContain(
				"calloutBlock would end without",
			)
			expect(blockByUid(doc, "callout").length).toBe(1)
		})

		it("removes the block and everything nested inside it", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "callout" },
			])

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual(["intro", "head"])
			expect(findBlock(doc, "nested")).toBeUndefined()
		})

		it("reports an error when the block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "ghost" },
			])

			expect(result).toEqual({
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

	describe("move", () => {
		const positionCases: {
			name: string
			input: MoveOp["position"]
			expected: string[]
		}[] = [
			{
				name: "before",
				input: "before",
				expected: ["callout", "intro", "head"],
			},
			{
				name: "after",
				input: "after",
				expected: ["callout", "head", "intro"],
			},
		]

		it.for(positionCases)(
			"lands the block $name the reference block",
			({ input, expected }, { expect }) => {
				const doc = docWith(sampleBlocks())

				const result = applyOperations(doc, [
					{
						kind: "move",
						block_uid: "intro",
						position: input,
						reference_uid: "head",
					},
				])

				expect(result.errors).toEqual([])
				expect(topUids(doc)).toEqual(expected)
			},
		)

		it("moves a block up past its earlier siblings", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "head",
					position: "before",
					reference_uid: "intro",
				},
			])

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual([
				"head",
				"intro",
				"callout",
			])
		})

		it("keeps the block's uid, attrs and nested content across the move", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "callout",
					position: "after",
					reference_uid: "head",
				},
			])

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual([
				"intro",
				"head",
				"callout",
			])

			const callout = blockByUid(doc, "callout")
			expect(attrsOf(callout)).toEqual({
				uid: "callout",
				icon: "lucide:zap",
			})

			expect(blockByUid(doc, "nested").parent).toBe(callout)
		})

		it("moves a block into a different parent next to a nested reference", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "intro",
					position: "after",
					reference_uid: "nested",
				},
			])

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual(["callout", "head"])

			expect(blockByUid(doc, "intro").parent).toBe(
				blockByUid(doc, "callout"),
			)
			expect(attrsOf(blockByUid(doc, "intro"))).toEqual({
				uid: "intro",
			})
		})

		it("refuses a move into a container that does not take the block, leaving the document as it was", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "head",
					position: "after",
					reference_uid: "nested",
				},
			])

			expect(result.errors[0]?.message).toContain(
				"heading is not allowed there in calloutBlock",
			)
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
			expect(blockByUid(doc, "callout").length).toBe(1)
		})

		it("moves a nested block out to the document root", ({
			expect,
		}) => {
			const doc = docWith([
				textParagraph("intro", "Intro line"),
				{
					type: "calloutBlock",
					attrs: {
						uid: "callout",
						icon: "lucide:zap",
					},
					content: [
						textParagraph(
							"nested",
							"Nested line",
						),
						textParagraph(
							"second",
							"Second line",
						),
					],
				},
			])

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "nested",
					position: "before",
					reference_uid: "intro",
				},
			])

			expect(result.errors).toEqual([])
			expect(topUids(doc)).toEqual([
				"nested",
				"intro",
				"callout",
			])
			expect(blockByUid(doc, "callout").length).toBe(1)
		})

		it("refuses a move that would leave its container empty", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "nested",
					position: "before",
					reference_uid: "intro",
				},
			])

			expect(result.errors[0]?.message).toContain(
				"calloutBlock would end without",
			)
			expect(blockByUid(doc, "callout").length).toBe(1)
		})

		it("reports an error for a position that is neither before nor after", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "intro",
					position: "under" as MoveOp["position"],
					reference_uid: "head",
				},
			])

			expect(result).toEqual({
				errors: [
					{
						index: 0,
						message: `move position must be "before" or "after", got: under`,
					},
				],
			})
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

		it("reports an error for a block moved relative to itself", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "intro",
					position: "after",
					reference_uid: "intro",
				},
			])

			expect(result).toEqual({
				errors: [
					{
						index: 0,
						message: "cannot move a block relative to itself: intro",
					},
				],
			})
			expect(topUids(doc)).toEqual([
				"intro",
				"callout",
				"head",
			])
		})

		it("reports an error when the moved block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "ghost",
					position: "after",
					reference_uid: "head",
				},
			])

			expect(result).toEqual({
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

		it("reports an error when the reference block is absent", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "intro",
					position: "after",
					reference_uid: "ghost",
				},
			])

			expect(result).toEqual({
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

		it("reports an error when the reference sits inside the moved block", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "callout",
					position: "after",
					reference_uid: "nested",
				},
			])

			expect(result).toEqual({
				errors: [
					{
						index: 0,
						message: "reference_uid is inside the moved block: nested",
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
			expect(result.errors).toEqual([])
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

				expect(result.errors).toEqual([])
				expect(serialize(doc.getText("icon"))).toBe(
					expected,
				)
			},
		)
	})
})

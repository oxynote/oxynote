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
// quoted returns a blockquote holding one paragraph
function quoted(uid: string, paragraphUid: string): PMNode {
	return {
		type: "blockquote",
		attrs: { uid },
		content: [textParagraph(paragraphUid, "Quoted line")],
	}
}

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

function gridBlock(uid: string, attrs: Record<string, unknown> = {}): PMNode {
	return {
		type: "metricBlock",
		attrs: { uid: uid, title: uid, ...attrs },
	}
}

function grid(uid: string, ...blocks: PMNode[]): PMNode {
	return { type: "blockGrid", attrs: { uid: uid }, content: blocks }
}

// a grid g whose standard blocks a and b sit side by side
function pairGrid(): PMNode {
	return grid(
		"g",
		gridBlock("a", {
			widePosX: 0,
			widePosY: 0,
			width: 12,
			height: 1,
		}),
		gridBlock("b", {
			widePosX: 12,
			widePosY: 0,
			width: 12,
			height: 1,
		}),
	)
}

// placesIn lists the cells that each block of the grid stores, as
// "uid x,y wxh". The list is sorted by uid.
function placesIn(doc: Y.Doc, gridUid: string): string[] {
	return blockByUid(doc, gridUid)
		.toArray()
		.flatMap((child) => {
			if (!(child instanceof Y.XmlElement)) {
				return []
			}

			const { uid, widePosX, widePosY, width, height } =
				attrsOf(child)

			return [
				`${String(uid)} ${String(widePosX)},${String(widePosY)} ${String(width)}x${String(height)}`,
			]
		})
		.sort()
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

		it("refuses a grid below the document root, leaving the document as it was", ({
			expect,
		}) => {
			const doc = docWith([quoted("quote", "quoted")])

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: "after",
					reference_uid: "quoted",
					block: {
						type: "blockGrid",
						attrs: { uid: "grid" },
						content: [
							{
								type: "metricBlock",
								attrs: {
									uid: "metric",
								},
							},
						],
					},
				},
			])

			expect(result.errors[0]?.message).toContain(
				"blockGrid is not allowed there in blockquote",
			)
			expect(blockByUid(doc, "quote").length).toBe(1)
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
		it("reports an error for a position that is no side of a block", ({
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
				'insert position must be "before", "after", "above" or "below", got: beforeX',
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
						type: "blockGrid",
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

		it("writes only the attributes whose value changes", ({
			expect,
		}) => {
			const doc = docWith([
				grid(
					"g",
					gridBlock("a", {
						queries: [{ query: "up" }],
					}),
				),
			])
			const changes: string[] = []

			blockByUid(doc, "a").observe((event) => {
				changes.push(...event.attributesChanged)
			})

			const result = applyOperations(doc, [
				{
					kind: "update_attrs",
					block_uid: "a",
					attrs: {
						title: "Renamed",
						queries: [{ query: "up" }],
					},
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(changes).toEqual(["title"])
			expect(attrsOf(blockByUid(doc, "a"))).toMatchObject({
				title: "Renamed",
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

		it("refuses to move a grid below the document root, leaving the document as it was", ({
			expect,
		}) => {
			const doc = docWith([
				{
					type: "blockGrid",
					attrs: { uid: "grid" },
					content: [
						{
							type: "metricBlock",
							attrs: {
								uid: "metric",
							},
						},
					],
				},
				quoted("quote", "quoted"),
			])

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "grid",
					position: "after",
					reference_uid: "quoted",
				},
			])

			expect(result.errors[0]?.message).toContain(
				"blockGrid is not allowed there in blockquote",
			)
			expect(topUids(doc)).toEqual(["grid", "quote"])
			expect(blockByUid(doc, "quote").length).toBe(1)
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

		it("reports an error for a position that is no side of a block", ({
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
						message: `move position must be "before", "after", "above" or "below", got: under`,
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

	describe("in a grid", () => {
		it.for([
			{
				name: "puts a block inserted after another at its right edge",
				input: { position: "after", reference: "a" },
				expected: [
					"a 0,0 12x1",
					"b 12,1 12x1",
					"n 12,0 12x1",
				],
			},
			{
				name: "puts a block inserted before another at its left edge",
				input: { position: "before", reference: "b" },
				expected: [
					"a 0,0 12x1",
					"b 12,1 12x1",
					"n 12,0 12x1",
				],
			},
			{
				name: "puts a block inserted below another under it",
				input: { position: "below", reference: "a" },
				expected: [
					"a 0,0 12x1",
					"b 12,0 12x1",
					"n 0,1 12x1",
				],
			},
			{
				name: "puts a block inserted above another over it",
				input: { position: "above", reference: "b" },
				expected: [
					"a 0,0 12x1",
					"b 12,1 12x1",
					"n 12,0 12x1",
				],
			},
		] as const)("$name", ({ input, expected }, { expect }) => {
			const doc = docWith([pairGrid()])

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: input.position,
					reference_uid: input.reference,
					block: gridBlock("n", {
						widePosX: 5,
						widePosY: 9,
						stackPos: 4,
					}),
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(placesIn(doc, "g")).toEqual(expected)
			expect(
				attrsOf(blockByUid(doc, "n")).stackPos,
			).toBeUndefined()
		})

		it("refuses above or below next to a block outside a grid", ({
			expect,
		}) => {
			const doc = docWith(sampleBlocks())

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: "below",
					reference_uid: "intro",
					block: textParagraph("n", "New"),
				},
			])

			expect(result).toEqual({
				errors: [
					{
						index: 0,
						message: "position below only places a block next to a block in a grid; use before or after here",
					},
				],
			})
		})

		it("moves a block within its grid by its cells alone", ({
			expect,
		}) => {
			const doc = docWith([pairGrid()])
			const moved = blockByUid(doc, "b")

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "b",
					position: "before",
					reference_uid: "a",
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(placesIn(doc, "g")).toEqual([
				"a 12,0 12x1",
				"b 0,0 12x1",
			])
			expect(blockByUid(doc, "b")).toBe(moved)
		})

		it("moves a block from another grid in without its old places and deletes the grid it empties", ({
			expect,
		}) => {
			const doc = docWith([
				pairGrid(),
				grid(
					"h",
					gridBlock("m", {
						widePosX: 0,
						widePosY: 0,
						width: 8,
						height: 1,
						stackPos: 0,
					}),
				),
			])

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "m",
					position: "below",
					reference_uid: "b",
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(placesIn(doc, "g")).toEqual([
				"a 0,0 12x1",
				"b 12,0 12x1",
				"m 12,1 8x1",
			])
			expect(
				attrsOf(blockByUid(doc, "m")).stackPos,
			).toBeUndefined()
			expect(findBlock(doc, "h")).toBeUndefined()
		})

		it.for([
			{
				name: "makes room for a block that an update makes wider",
				input: { width: 24 },
				expected: ["a 0,0 24x1", "b 12,1 12x1"],
			},
			{
				name: "makes room below a block an update makes taller",
				input: { height: 2 },
				expected: ["a 0,0 12x2", "b 12,0 12x1"],
			},
		])("$name", ({ input, expected }, { expect }) => {
			const doc = docWith([pairGrid()])

			const result = applyOperations(doc, [
				{
					kind: "update_attrs",
					block_uid: "a",
					attrs: input,
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(placesIn(doc, "g")).toEqual(expected)
		})

		it("leaves the other blocks alone when an update keeps the size", ({
			expect,
		}) => {
			const doc = docWith([pairGrid()])
			const changes: string[] = []

			blockByUid(doc, "b").observe((event) => {
				changes.push(...event.attributesChanged)
			})

			const result = applyOperations(doc, [
				{
					kind: "update_attrs",
					block_uid: "a",
					attrs: { title: "Renamed" },
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(changes).toEqual([])
			expect(attrsOf(blockByUid(doc, "a")).title).toBe(
				"Renamed",
			)
		})

		it("changes a block replaced by itself in place and keeps its live places", ({
			expect,
		}) => {
			const doc = docWith([pairGrid()])
			const replaced = blockByUid(doc, "a")

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "a",
					block: gridBlock("a", {
						title: "Renamed",
						widePosX: 12,
						widePosY: 4,
						width: 12,
						height: 1,
					}),
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(blockByUid(doc, "a")).toBe(replaced)
			expect(attrsOf(replaced).title).toBe("Renamed")
			expect(placesIn(doc, "g")).toEqual([
				"a 0,0 12x1",
				"b 12,0 12x1",
			])
		})

		it("removes the attributes a block replaced by itself leaves out", ({
			expect,
		}) => {
			const doc = docWith([
				grid(
					"g",
					gridBlock("a", {
						decimals: 2,
						widePosX: 0,
						widePosY: 0,
						width: 12,
						height: 1,
					}),
				),
			])

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "a",
					block: gridBlock("a", {
						width: 12,
						height: 1,
					}),
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(
				attrsOf(blockByUid(doc, "a")).decimals,
			).toBeUndefined()
			expect(placesIn(doc, "g")).toEqual(["a 0,0 12x1"])
		})

		it("leaves an object attribute alone when a block replaced by itself sends it with its keys reordered", ({
			expect,
		}) => {
			const cells = {
				widePosX: 0,
				widePosY: 0,
				width: 12,
				height: 1,
			}
			const doc = docWith([
				grid(
					"g",
					gridBlock("a", {
						queries: [
							{
								expr: "up",
								refId: "A",
							},
						],
						...cells,
					}),
				),
			])
			const changes: string[] = []

			blockByUid(doc, "a").observe((event) => {
				changes.push(...event.attributesChanged)
			})

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "a",
					block: gridBlock("a", {
						title: "Renamed",
						queries: [
							{
								refId: "A",
								expr: "up",
							},
						],
						simulationActive: false,
						...cells,
					}),
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(changes).toEqual(["title"])
		})

		it("gives a block of another type with the same id the replaced one's places", ({
			expect,
		}) => {
			const doc = docWith([pairGrid()])

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "b",
					block: {
						type: "logBlock",
						attrs: {
							uid: "b",
							width: 24,
						},
					},
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(blockByUid(doc, "b").nodeName).toBe("logBlock")
			expect(placesIn(doc, "g")).toEqual([
				"a 0,0 12x1",
				"b 0,1 24x1",
			])
		})

		it("gives a block that replaces another its places", ({
			expect,
		}) => {
			const doc = docWith([
				grid(
					"g",
					gridBlock("a", {
						widePosX: 0,
						widePosY: 0,
						width: 12,
						height: 1,
						stackPos: 1,
					}),
					gridBlock("b", {
						widePosX: 12,
						widePosY: 0,
						width: 12,
						height: 1,
						stackPos: 0,
					}),
				),
			])

			const result = applyOperations(doc, [
				{
					kind: "replace",
					block_uid: "a",
					block: gridBlock("n"),
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(placesIn(doc, "g")).toEqual([
				"b 12,0 12x1",
				"n 0,0 12x1",
			])
			expect(attrsOf(blockByUid(doc, "n")).stackPos).toBe(1)
		})

		it("keeps a user's edit to a block the assistant moves at the same time", ({
			expect,
		}) => {
			const server = docWith([pairGrid()])
			const user = new Y.Doc()

			Y.applyUpdate(user, Y.encodeStateAsUpdate(server))
			;(
				blockByUid(user, "a") as Y.XmlElement<
					Record<string, any>
				>
			).setAttribute("title", "Renamed")

			const result = applyOperations(server, [
				{
					kind: "move",
					block_uid: "a",
					position: "after",
					reference_uid: "b",
				},
			])

			Y.applyUpdate(
				server,
				Y.encodeStateAsUpdate(
					user,
					Y.encodeStateVector(server),
				),
			)
			Y.applyUpdate(
				user,
				Y.encodeStateAsUpdate(
					server,
					Y.encodeStateVector(user),
				),
			)

			expect(result).toEqual({ errors: [] })

			for (const doc of [server, user]) {
				expect(
					attrsOf(blockByUid(doc, "a")).title,
				).toBe("Renamed")
				expect(placesIn(doc, "g")).toEqual([
					"a 12,1 12x1",
					"b 12,0 12x1",
				])
			}
		})

		it("deletes the grid a delete takes the last block from", ({
			expect,
		}) => {
			const doc = docWith([
				grid("g", gridBlock("a")),
				textParagraph("p", "Line"),
			])

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "a" },
			])

			expect(result).toEqual({ errors: [] })
			expect(topUids(doc)).toEqual(["p"])
		})

		it("keeps an emptied grid that is the document's only block", ({
			expect,
		}) => {
			const doc = docWith([grid("g", gridBlock("a"))])

			const result = applyOperations(doc, [
				{ kind: "delete", block_uid: "a" },
			])

			expect(result).toEqual({ errors: [] })
			expect(topUids(doc)).toEqual(["g"])
		})

		it("deletes the grid a move takes the last block out of", ({
			expect,
		}) => {
			const doc = docWith([
				grid("g", gridBlock("a")),
				{
					type: "splitDocumentation",
					attrs: { uid: "s" },
					content: [
						{
							type: "splitDocumentationLeftSide",
							content: [
								{
									type: "heading",
									attrs: {
										uid: "h",
										level: 2,
									},
									content: [
										{
											type: "text",
											text: "Title",
										},
									],
								},
								textParagraph(
									"l",
									"Left",
								),
							],
						},
						{
							type: "splitDocumentationRightSide",
							content: [
								gridBlock("r"),
							],
						},
					],
				},
			])

			const result = applyOperations(doc, [
				{
					kind: "move",
					block_uid: "a",
					position: "after",
					reference_uid: "r",
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(topUids(doc)).toEqual(["s"])
			expect(findBlock(doc, "a")).toBeDefined()
		})

		it("refuses to write a grid without blocks", ({ expect }) => {
			const doc = docWith([textParagraph("p", "Line")])

			const result = applyOperations(doc, [
				{
					kind: "insert",
					position: "after",
					reference_uid: "p",
					block: {
						type: "blockGrid",
						attrs: { uid: "n" },
						content: [],
					},
				},
			])

			expect(result.errors[0]?.message).toContain(
				"blockGrid must hold at least one block",
			)
			expect(topUids(doc)).toEqual(["p"])
		})
	})

	describe("set_stack_order", () => {
		it.for([
			{
				name: "gives every block its place in the list",
				input: ["b", "c", "a"],
				expected: { a: 2, b: 0, c: 1 },
			},
			{
				name: "puts a block the list leaves out after the block it follows in the wide layout",
				input: ["c", "a"],
				expected: { a: 1, b: 2, c: 0 },
			},
		])("$name", ({ input, expected }, { expect }) => {
			const doc = docWith([
				grid(
					"g",
					gridBlock("a", {
						widePosX: 0,
						widePosY: 0,
						width: 8,
						height: 1,
					}),
					gridBlock("b", {
						widePosX: 8,
						widePosY: 0,
						width: 8,
						height: 1,
					}),
					gridBlock("c", {
						widePosX: 16,
						widePosY: 0,
						width: 8,
						height: 1,
					}),
				),
			])

			const result = applyOperations(doc, [
				{
					kind: "set_stack_order",
					grid_uid: "g",
					order: input,
				},
			])

			expect(result).toEqual({ errors: [] })
			expect({
				a: attrsOf(blockByUid(doc, "a")).stackPos,
				b: attrsOf(blockByUid(doc, "b")).stackPos,
				c: attrsOf(blockByUid(doc, "c")).stackPos,
			}).toEqual(expected)
		})

		it("writes only the places that change", ({ expect }) => {
			const doc = docWith([
				grid(
					"g",
					gridBlock("a", { stackPos: 0 }),
					gridBlock("b", { stackPos: 1 }),
				),
			])
			const changes: string[] = []

			blockByUid(doc, "a").observe((event) => {
				changes.push(...event.attributesChanged)
			})

			const result = applyOperations(doc, [
				{
					kind: "set_stack_order",
					grid_uid: "g",
					order: ["a", "b"],
				},
			])

			expect(result).toEqual({ errors: [] })
			expect(changes).toEqual([])
		})

		it.for([
			{
				name: "a block outside the grid",
				input: { grid: "g", order: ["a", "intro"] },
				expected: "order names a block outside the grid: intro",
			},
			{
				name: "a block twice",
				input: { grid: "g", order: ["a", "a"] },
				expected: "order names a block twice: a",
			},
			{
				name: "a block that is no grid",
				input: { grid: "intro", order: [] },
				expected: "grid_uid is not a grid: intro",
			},
			{
				name: "an order that is no list",
				input: {
					grid: "g",
					order: "a" as unknown as string[],
				},
				expected: "order must be a list of block uids",
			},
		])("refuses $name", ({ input, expected }, { expect }) => {
			const doc = docWith([
				textParagraph("intro", "Intro"),
				pairGrid(),
			])

			const result = applyOperations(doc, [
				{
					kind: "set_stack_order",
					grid_uid: input.grid,
					order: input.order,
				},
			])

			expect(result).toEqual({
				errors: [{ index: 0, message: expected }],
			})
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

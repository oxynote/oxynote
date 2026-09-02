import { describe, it } from "vitest"
import {
	CalloutBlock,
	CodeBlock,
	CodeBlockTitle,
	getEditorExtensions,
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
	TitledCodeBlock,
	AddedMark,
	CommentMark,
	DeletedMark,
} from "./index.js"
import { FigmaBlock } from "./figma.js"
import { ImageBlock } from "./image.js"
import { MermaidBlock } from "./mermaid.js"
import { MetricBlock, MetricGrid } from "./metric.js"
import { transformer } from "../ydocument.js"

interface PMNode {
	type: string
	attrs?: Record<string, unknown>
	content?: PMNode[]
	text?: string
	marks?: { type: string; attrs?: Record<string, unknown> }[]
}

// the schema's real job is to survive the trip every document takes
// between the editor and core: prosemirror JSON into a Y.Doc and back
// out. A node the schema does not know is silently dropped on the way
// through, so a round trip is what proves it is registered.
function roundTrip(block: PMNode): PMNode {
	const ydoc = transformer.toYdoc(
		{ type: "doc", content: [block] },
		"content",
	)
	const back = transformer.fromYdoc(ydoc, "content") as PMNode

	const first = back.content?.[0]
	if (!first) {
		throw new Error("the round trip produced an empty document")
	}

	return first
}

function paragraph(uid: string, text: string): PMNode {
	return {
		type: "paragraph",
		attrs: { uid },
		content: [{ type: "text", text }],
	}
}

function heading(uid: string, text: string): PMNode {
	return {
		type: "heading",
		attrs: { uid, level: 2 },
		content: [{ type: "text", text }],
	}
}

function metricBlock(uid: string): PMNode {
	return {
		type: "metricBlock",
		attrs: {
			uid,
			title: "Requests",
			queries: [{ expr: "up", legend: "up" }],
			thresholds: [{ value: 10, color: "red" }],
			decimals: 2,
			unitType: "short",
			simulationPreset: "http_latency",
		},
	}
}

function titledCodeBlock(uid: string): PMNode {
	return {
		type: "titledCodeBlock",
		attrs: { uid },
		content: [
			{
				type: "codeBlockTitle",
				attrs: { uid: `${uid}-title` },
				content: [{ type: "text", text: "restart.sh" }],
			},
			{
				type: "codeBlock",
				attrs: { uid: `${uid}-code`, language: "bash" },
				content: [
					{
						type: "text",
						text: "systemctl restart",
					},
				],
			},
		],
	}
}

describe("getEditorExtensions", () => {
	it("gives every extension a unique name", ({ expect }) => {
		const names = getEditorExtensions().map((e) => e.name)

		expect(new Set(names).size).toBe(names.length)
	})

	// a document cannot be parsed at all without them, and they are the
	// two the custom blocks never declare themselves
	it("includes the document and text nodes every schema needs", ({
		expect,
	}) => {
		const names = getEditorExtensions().map((e) => e.name)

		expect(names).toContain("doc")
		expect(names).toContain("text")
	})

	// operations.ts addresses every block by its uid, so a block missing
	// from this list is one the assistant can never edit
	it.for([
		{ name: "callout", input: "calloutBlock" },
		{ name: "metric", input: "metricBlock" },
		{ name: "metric grid", input: "metricGrid" },
		{ name: "image", input: "imageBlock" },
		{ name: "figma", input: "figmaBlock" },
		{ name: "mermaid", input: "mermaidBlock" },
		{ name: "titled code", input: "titledCodeBlock" },
		{ name: "split documentation", input: "splitDocumentation" },
		{
			name: "parameter list",
			input: "splitDocumentationParameterList",
		},
	])("gives the $name block a uid", ({ input }, { expect }) => {
		const uid = getEditorExtensions().find(
			(e) => e.name === "uniqueID",
		)
		const types = (uid?.options as { types?: string[] } | undefined)
			?.types

		expect(types).toContain(input)
	})

	// a name here that no extension answers to would silently leave that
	// block without uids
	it("lists only registered extensions as uid carriers", ({ expect }) => {
		const extensions = getEditorExtensions()
		const names = new Set(extensions.map((e) => e.name))
		const uid = extensions.find((e) => e.name === "uniqueID")
		const types =
			(uid?.options as { types?: string[] } | undefined)
				?.types ?? []

		expect(types.filter((t) => !names.has(t))).toEqual([])
	})
})

describe("editor schema", () => {
	describe("round trip", () => {
		it.for([
			{
				name: "a paragraph",
				input: paragraph("p1", "Restart it"),
			},
			{ name: "a heading", input: heading("h1", "Runbook") },
			{
				name: "an image",
				input: {
					type: "imageBlock",
					attrs: {
						uid: "i1",
						src: "https://example/x.png",
						alt: "x",
						width: 400,
					},
				},
			},
			{
				name: "a figma embed",
				input: {
					type: "figmaBlock",
					attrs: {
						uid: "f1",
						src: "https://figma/x",
						width: 800,
						height: 600,
					},
				},
			},
			{
				name: "a horizontal rule",
				input: {
					type: "horizontalRule",
					attrs: { uid: "r1" },
				},
			},
		])("keeps $name intact", ({ input }, { expect }) => {
			expect(roundTrip(input)).toEqual(input)
		})

		// the reason cloneXmlElement exists: Y.XmlElement.clone() drops
		// everything that is not a string, which would strip a metric
		// block's queries on every fork and merge
		it("keeps a metric block's array and number attributes", ({
			expect,
		}) => {
			const result = roundTrip(metricBlock("m1"))

			expect(result.attrs?.queries).toEqual([
				{ expr: "up", legend: "up" },
			])
			expect(result.attrs?.thresholds).toEqual([
				{ value: 10, color: "red" },
			])
			expect(result.attrs?.decimals).toBe(2)
		})

		it("keeps a metric block's simulation preset", ({ expect }) => {
			const result = roundTrip(metricBlock("m1"))

			expect(result.attrs?.simulationPreset).toBe(
				"http_latency",
			)
		})

		it("keeps a mermaid block's source text", ({ expect }) => {
			const result = roundTrip({
				type: "mermaidBlock",
				attrs: { uid: "d1" },
				content: [
					{
						type: "text",
						text: "graph TD;\n  A-->B;",
					},
				],
			})

			expect(result.content?.[0]?.text).toBe(
				"graph TD;\n  A-->B;",
			)
		})

		it("keeps a callout's icon and its nested paragraph", ({
			expect,
		}) => {
			const result = roundTrip({
				type: "calloutBlock",
				attrs: { uid: "c1", icon: "lucide:siren" },
				content: [
					paragraph("c1-p", "Page the on-call"),
				],
			})

			expect(result.attrs?.icon).toBe("lucide:siren")
			expect(result.content?.[0]?.content?.[0]?.text).toBe(
				"Page the on-call",
			)
		})

		it("keeps the blocks inside a metric grid", ({ expect }) => {
			const result = roundTrip({
				type: "metricGrid",
				attrs: { uid: "g1" },
				content: [metricBlock("m1"), metricBlock("m2")],
			})

			expect(result.content).toHaveLength(2)
			expect(result.content?.[1]?.attrs?.uid).toBe("m2")
		})

		it("keeps a titled code block's title and language", ({
			expect,
		}) => {
			const result = roundTrip(titledCodeBlock("t1"))

			expect(result.content?.[0]?.content?.[0]?.text).toBe(
				"restart.sh",
			)
			expect(result.content?.[1]?.attrs?.language).toBe(
				"bash",
			)
		})

		// the widest node in the schema: a heading and prose on the
		// left, code or metrics on the right
		it("keeps both sides of a split documentation block", ({
			expect,
		}) => {
			const result = roundTrip({
				type: "splitDocumentation",
				attrs: { uid: "s1" },
				content: [
					{
						type: "splitDocumentationLeftSide",
						attrs: { uid: "s1-l" },
						content: [
							heading(
								"s1-h",
								"Endpoint",
							),
							paragraph(
								"s1-p",
								"Returns a page",
							),
						],
					},
					{
						type: "splitDocumentationRightSide",
						attrs: { uid: "s1-r" },
						content: [
							titledCodeBlock("s1-c"),
						],
					},
				],
			})

			expect(result.content?.[0]?.type).toBe(
				"splitDocumentationLeftSide",
			)
			expect(result.content?.[1]?.type).toBe(
				"splitDocumentationRightSide",
			)
			expect(result.content?.[0]?.content?.[0]?.type).toBe(
				"heading",
			)
		})

		it("keeps a parameter list's header and items", ({
			expect,
		}) => {
			const result = roundTrip({
				type: "splitDocumentationParameterList",
				attrs: { uid: "pl1" },
				content: [
					{
						type: "splitDocumentationParameterListHeader",
						attrs: { uid: "pl1-h" },
						content: [
							{
								type: "text",
								text: "Parameters",
							},
						],
					},
					{
						type: "splitDocumentationParameterListItem",
						attrs: { uid: "pl1-i" },
						content: [
							{
								type: "splitDocumentationParameterListItemHeader",
								attrs: {
									uid: "pl1-ih",
								},
								content: [
									{
										type: "splitDocumentationParameterListItemHeaderTitle",
										attrs: {
											uid: "pl1-t",
										},
										content: [
											{
												type: "text",
												text: "limit",
											},
										],
									},
									{
										type: "splitDocumentationParameterListItemHeaderType",
										attrs: {
											uid: "pl1-ty",
										},
										content: [
											{
												type: "text",
												text: "number",
											},
										],
									},
								],
							},
							paragraph(
								"pl1-d",
								"How many to return",
							),
						],
					},
				],
			})

			expect(result.content?.[0]?.content?.[0]?.text).toBe(
				"Parameters",
			)
			expect(
				result.content?.[1]?.content?.[0]?.content?.[0]
					?.content?.[0]?.text,
			).toBe("limit")
		})
	})

	describe("marks", () => {
		// comment marks carry the id the comment sidebar resolves; a
		// dropped attribute orphans the thread
		it("keeps a comment mark's id", ({ expect }) => {
			const result = roundTrip({
				type: "paragraph",
				attrs: { uid: "p1" },
				content: [
					{
						type: "text",
						text: "flagged",
						marks: [
							{
								type: "comment",
								attrs: {
									commentId: "cmt-1",
								},
							},
						],
					},
				],
			})

			expect(result.content?.[0]?.marks?.[0]?.type).toBe(
				"comment",
			)
			expect(
				result.content?.[0]?.marks?.[0]?.attrs
					?.commentId,
			).toBe("cmt-1")
		})

		// the diff view marks additions and deletions inline
		it.for([
			{ name: "added", input: "added" },
			{ name: "deleted", input: "deleted" },
		])("keeps a $name mark", ({ input }, { expect }) => {
			const result = roundTrip({
				type: "paragraph",
				attrs: { uid: "p1" },
				content: [
					{
						type: "text",
						text: "changed",
						marks: [{ type: input }],
					},
				],
			})

			expect(result.content?.[0]?.marks?.[0]?.type).toBe(
				input,
			)
		})

		it.for([
			{ name: "bold", input: "bold" },
			{ name: "italic", input: "italic" },
			{ name: "strike", input: "strike" },
			{ name: "underline", input: "underline" },
			{ name: "code", input: "code" },
		])("keeps a $name mark", ({ input }, { expect }) => {
			const result = roundTrip({
				type: "paragraph",
				attrs: { uid: "p1" },
				content: [
					{
						type: "text",
						text: "styled",
						marks: [{ type: input }],
					},
				],
			})

			expect(result.content?.[0]?.marks?.[0]?.type).toBe(
				input,
			)
		})

		it("keeps a link's href", ({ expect }) => {
			const result = roundTrip({
				type: "paragraph",
				attrs: { uid: "p1" },
				content: [
					{
						type: "text",
						text: "docs",
						marks: [
							{
								type: "link",
								attrs: {
									href: "https://example/docs",
								},
							},
						],
					},
				],
			})

			expect(
				result.content?.[0]?.marks?.[0]?.attrs?.href,
			).toBe("https://example/docs")
		})
	})
})

// the HTML side of the schema. This service never serializes a document
// to HTML — the transformer works on prosemirror JSON — so these
// callbacks exist only to keep the definitions identical to web's. A
// data-type that drifts apart is a block web writes and cannot parse
// back, which is why the strings are pinned here rather than left to the
// two copies to agree by luck.
type Attrs = Record<string, unknown>

interface AttributeSpec {
	default?: unknown
	parseHTML?: (element: {
		getAttribute(name: string): string | null
	}) => unknown
	renderHTML?: (attrs: Attrs) => Attrs
}

interface NodeSpec {
	renderHTML?: (
		this: { options: Attrs },
		props: { HTMLAttributes: Attrs; node?: { attrs: Attrs } },
	) => [string, Attrs, ...unknown[]]
	parseHTML?: (this: { options: Attrs }) => { tag: string }[]
	addAttributes?: (this: {
		options: Attrs
	}) => Record<string, AttributeSpec>
}

function specOf(node: { config: unknown }): NodeSpec {
	return node.config as NodeSpec
}

function render(
	node: { config: unknown },
	attrs: Attrs = {},
	options: Attrs = {},
): [string, Attrs, ...unknown[]] {
	const rendered = specOf(node).renderHTML?.call(
		{ options },
		{ HTMLAttributes: {}, node: { attrs } },
	)
	if (!rendered) {
		throw new Error("the node declares no renderHTML")
	}

	return rendered
}

function parseSelector(node: { config: unknown }): string | undefined {
	return specOf(node).parseHTML?.call({ options: {} })[0]?.tag
}

function attributesOf(node: {
	config: unknown
}): Record<string, AttributeSpec> {
	return specOf(node).addAttributes?.call({ options: {} }) ?? {}
}

describe("schema html contract", () => {
	it.for([
		{
			name: "callout",
			input: CalloutBlock,
			expected: "callout-block",
		},
		{
			name: "metric grid",
			input: MetricGrid,
			expected: "metric-grid",
		},
		{
			name: "metric",
			input: MetricBlock,
			expected: "metric-block",
		},
		{ name: "figma", input: FigmaBlock, expected: "figma-block" },
		{
			name: "mermaid",
			input: MermaidBlock,
			expected: "mermaid-block",
		},
		{ name: "code", input: CodeBlock, expected: "code-block" },
		{
			name: "code title",
			input: CodeBlockTitle,
			expected: "code-block-title",
		},
		{
			name: "titled code",
			input: TitledCodeBlock,
			expected: "titled-code-block",
		},
		{
			name: "split documentation",
			input: SplitDocumentation,
			expected: "split-documentation",
		},
		{
			name: "split left side",
			input: SplitDocumentationLeftSide,
			expected: "split-documentation-left-side",
		},
		{
			name: "split right side",
			input: SplitDocumentationRightSide,
			expected: "split-documentation-right-side",
		},
		{
			name: "parameter list",
			input: ParameterList,
			expected: "split-documentation-parameter-list",
		},
		{
			name: "parameter list header",
			input: ParameterListHeader,
			expected: "split-documentation-parameter-list-header",
		},
		{
			name: "parameter list item",
			input: ParameterListItem,
			expected: "split-documentation-parameter-list-item",
		},
		{
			name: "parameter list item header",
			input: ParameterListItemHeader,
			expected: "split-documentation-parameter-list-item-header",
		},
		{
			name: "parameter list item title",
			input: ParameterListItemHeaderTitle,
			expected: "split-documentation-parameter-list-item-header-title",
		},
		{
			name: "parameter list item type",
			input: ParameterListItemHeaderType,
			expected: "split-documentation-parameter-list-item-header-type",
		},
	])(
		"tags the $name block as $expected and parses it back",
		({ input, expected }, { expect }) => {
			expect(render(input)[1]["data-type"]).toBe(expected)
			expect(parseSelector(input)).toContain(expected)
		},
	)

	it.for([
		{ name: "image", input: ImageBlock, expected: "img" },
		{ name: "mermaid", input: MermaidBlock, expected: "pre" },
		{ name: "code", input: CodeBlock, expected: "pre" },
		{ name: "callout", input: CalloutBlock, expected: "div" },
	])(
		"renders the $name block as a $expected element",
		({ input, expected }, { expect }) => {
			expect(render(input)[0]).toBe(expected)
		},
	)

	describe("callout icon", () => {
		it("reads the icon off data-icon", ({ expect }) => {
			const icon = attributesOf(CalloutBlock).icon

			expect(
				icon?.parseHTML?.({
					getAttribute: (name) =>
						name === "data-icon"
							? "lucide:zap"
							: null,
				}),
			).toBe("lucide:zap")
		})

		it("writes the icon back to data-icon", ({ expect }) => {
			const icon = attributesOf(CalloutBlock).icon

			expect(
				icon?.renderHTML?.({ icon: "lucide:siren" }),
			).toEqual({
				"data-icon": "lucide:siren",
			})
		})

		it("defaults to the plain text icon", ({ expect }) => {
			expect(attributesOf(CalloutBlock).icon?.default).toBe(
				"lucide:text",
			)
		})
	})

	describe("figma attributes", () => {
		// the width and height arrive as strings on the element and are
		// stored as numbers, so a document round-trips as numbers
		it.for([
			{ name: "width", input: "width" },
			{ name: "height", input: "height" },
		])("parses $name into a number", ({ input }, { expect }) => {
			const attr = attributesOf(FigmaBlock)[input]

			expect(
				attr?.parseHTML?.({
					getAttribute: () => "800",
				}),
			).toBe(800)
		})

		it.for([
			{ name: "src", input: "src" },
			{ name: "width", input: "width" },
			{ name: "height", input: "height" },
		])(
			"parses a missing $name as null",
			({ input }, { expect }) => {
				const attr = attributesOf(FigmaBlock)[input]

				expect(
					attr?.parseHTML?.({
						getAttribute: () => null,
					}),
				).toBeNull()
			},
		)

		it.for([
			{ name: "src", input: "src", expected: "data-src" },
			{
				name: "width",
				input: "width",
				expected: "data-width",
			},
			{
				name: "height",
				input: "height",
				expected: "data-height",
			},
		])(
			"writes $name back to $expected",
			({ input, expected }, { expect }) => {
				const attr = attributesOf(FigmaBlock)[input]

				expect(
					attr?.renderHTML?.({ [input]: 800 }),
				).toEqual({
					[expected]: 800,
				})
			},
		)

		// an unset value writes no attribute at all rather than an
		// empty one, which is what keeps the markup clean
		it.for([
			{ name: "src", input: "src" },
			{ name: "width", input: "width" },
			{ name: "height", input: "height" },
		])(
			"writes nothing when $name is unset",
			({ input }, { expect }) => {
				const attr = attributesOf(FigmaBlock)[input]

				expect(attr?.renderHTML?.({})).toEqual({})
			},
		)
	})

	describe("marks", () => {
		it.for([
			{
				name: "added",
				input: AddedMark,
				expected: "data-added",
			},
			{
				name: "deleted",
				input: DeletedMark,
				expected: "data-deleted",
			},
		])(
			"flags a $name range with $expected",
			({ input, expected }, { expect }) => {
				expect(render(input)[1][expected]).toBe("true")
				expect(parseSelector(input)).toContain(expected)
			},
		)

		it("writes a comment's id to data-comment-id", ({ expect }) => {
			const attr = attributesOf(CommentMark).commentId

			expect(
				attr?.renderHTML?.({ commentId: "cmt-1" }),
			).toEqual({
				"data-comment-id": "cmt-1",
			})
		})

		it("reads a comment's id off data-comment-id", ({ expect }) => {
			const attr = attributesOf(CommentMark).commentId

			expect(
				attr?.parseHTML?.({
					getAttribute: (name) =>
						name === "data-comment-id"
							? "cmt-1"
							: null,
				}),
			).toBe("cmt-1")
		})
	})

	describe("code block language", () => {
		it("puts the language on the inner code element", ({
			expect,
		}) => {
			const rendered = render(
				CodeBlock,
				{ language: "bash" },
				{ languageClassPrefix: "language-" },
			)
			const code = rendered[2] as [
				string,
				Attrs,
				...unknown[],
			]

			expect(code[1].class).toBe("language-bash")
		})

		it("leaves the class unset when no language is chosen", ({
			expect,
		}) => {
			const rendered = render(
				CodeBlock,
				{},
				{ languageClassPrefix: "language-" },
			)
			const code = rendered[2] as [
				string,
				Attrs,
				...unknown[],
			]

			expect(code[1].class).toBeNull()
		})
	})
})

describe("split documentation orientation", () => {
	it.for([
		{ name: "set", input: "true", expected: true },
		{ name: "absent", input: null, expected: false },
		{ name: "some other value", input: "yes", expected: false },
	])(
		"reads an orientation that is $name as $expected",
		({ input, expected }, { expect }) => {
			const attr = attributesOf(SplitDocumentation).inversed

			expect(
				attr?.parseHTML?.({
					getAttribute: () => input,
				}),
			).toBe(expected)
		},
	)

	it("writes the orientation back only when it is inverted", ({
		expect,
	}) => {
		const attr = attributesOf(SplitDocumentation).inversed

		expect(attr?.renderHTML?.({ inversed: true })).toEqual({
			"data-inversed": "true",
		})
		expect(attr?.renderHTML?.({ inversed: false })).toEqual({})
	})

	it("defaults to the uninverted orientation", ({ expect }) => {
		expect(attributesOf(SplitDocumentation).inversed?.default).toBe(
			false,
		)
	})
})

describe("comment mark rendering", () => {
	it("renders a comment as a span carrying its attributes", ({
		expect,
	}) => {
		const rendered = specOf(CommentMark).renderHTML?.call(
			{ options: {} },
			{ HTMLAttributes: { "data-comment-id": "cmt-1" } },
		)

		expect(rendered?.[0]).toBe("span")
		expect(rendered?.[1]["data-comment-id"]).toBe("cmt-1")
	})
})

describe("code block without configured options", () => {
	// the prefix comes from the extension's options, and a caller that
	// renders the node without them still has to produce a usable class
	it("falls back to an unprefixed language class", ({ expect }) => {
		const rendered = render(CodeBlock, { language: "bash" }, {})
		const code = rendered[2] as [string, Attrs, ...unknown[]]

		expect(code[1].class).toBe("bash")
	})
})

describe("code block options", () => {
	// the definition extends CodeBlockLowlight, so its own defaults are
	// layered over whatever the parent extension supplied — and it has to
	// hold up when there is no parent to layer over
	it.for([
		{ name: "a parent extension", input: true },
		{ name: "no parent at all", input: false },
	])("resolves its defaults with $name", ({ input }, { expect }) => {
		const addOptions = (
			CodeBlock.config as {
				addOptions?: (this: {
					parent?: () => Record<string, unknown>
				}) => Record<string, unknown>
			}
		).addOptions

		const options = addOptions?.call(
			input
				? { parent: () => ({ tabSize: 8 }) }
				: { parent: undefined },
		)

		expect(options?.languageClassPrefix).toBe("language-")
		expect(options?.tabSize).toBe(input ? 8 : 2)
	})
})

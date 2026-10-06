import { describe, it } from "vitest"
import { checkBlock, checkChildren } from "./validate.js"
import type { PMNode } from "./operations.js"

function paragraph(text: string): PMNode {
	return { type: "paragraph", content: [{ type: "text", text }] }
}

function titledCode(): PMNode {
	return {
		type: "titledCodeBlock",
		content: [
			{
				type: "codeBlockTitle",
				content: [{ type: "text", text: "GET /x" }],
			},
			{
				type: "codeBlock",
				content: [{ type: "text", text: "x" }],
			},
		],
	}
}

describe("checkBlock", () => {
	it("accepts a split documentation block of both sides", ({
		expect,
	}) => {
		expect(() => {
			checkBlock({
				type: "splitDocumentation",
				content: [
					{
						type: "splitDocumentationLeftSide",
						content: [
							{
								type: "heading",
								attrs: {
									level: 1,
								},
								content: [
									{
										type: "text",
										text: "API",
									},
								],
							},
							paragraph(
								"Does a thing",
							),
						],
					},
					{
						type: "splitDocumentationRightSide",
						content: [titledCode()],
					},
				],
			})
		}).not.toThrow()
	})

	it("refuses an unknown block type", ({ expect }) => {
		expect(() => {
			checkBlock({ type: "wibble" })
		}).toThrow("wibble is not a known block type")
	})

	it("refuses a nested block its container does not take", ({
		expect,
	}) => {
		expect(() => {
			checkBlock({
				type: "calloutBlock",
				content: [paragraph("a"), titledCode()],
			})
		}).toThrow(
			"titledCodeBlock is not allowed there in calloutBlock",
		)
	})

	it("refuses a mark the text's node does not allow", ({ expect }) => {
		expect(() => {
			checkBlock({
				type: "codeBlock",
				content: [
					{
						type: "text",
						text: "x",
						marks: [{ type: "bold" }],
					},
				],
			})
		}).toThrow(
			"the bold mark is not allowed in codeBlock: write the text without it",
		)
	})

	it("accepts marks the text's node allows", ({ expect }) => {
		expect(() => {
			checkBlock({
				type: "paragraph",
				content: [
					{
						type: "text",
						text: "x",
						marks: [
							{ type: "bold" },
							{ type: "italic" },
						],
					},
				],
			})
		}).not.toThrow()
	})

	it("refuses a mark the schema does not know", ({ expect }) => {
		expect(() => {
			checkBlock({
				type: "paragraph",
				content: [
					{
						type: "text",
						text: "x",
						marks: [{ type: "sparkle" }],
					},
				],
			})
		}).toThrow("the sparkle mark is not allowed in paragraph")
	})
})

describe("checkChildren", () => {
	it("accepts children the parent takes in that order", ({ expect }) => {
		expect(() => {
			checkChildren("doc", [
				"paragraph",
				"metricGrid",
				"heading",
			])
		}).not.toThrow()
	})

	it("refuses a block at the document root that belongs in a container", ({
		expect,
	}) => {
		expect(() => {
			checkChildren("doc", ["paragraph", "metricBlock"])
		}).toThrow(
			/^metricBlock is not allowed there in the document root, which takes .+ or another block at that point\.$/,
		)
	})

	it("ends the refusal with the caller's advice", ({ expect }) => {
		expect(() => {
			checkChildren("doc", ["metricBlock"], "Try elsewhere.")
		}).toThrow(/at that point\. Try elsewhere\.$/)
	})

	it("refuses a block out of the order its parent takes", ({
		expect,
	}) => {
		expect(() => {
			checkChildren("splitDocumentationLeftSide", [
				"heading",
				"paragraph",
				"splitDocumentationParameterList",
				"paragraph",
			])
		}).toThrow(
			"paragraph is not allowed there in splitDocumentationLeftSide, which takes splitDocumentationParameterList at that point",
		)
	})

	it("refuses a parent left without the blocks it needs", ({
		expect,
	}) => {
		expect(() => {
			checkChildren("splitDocumentationRightSide", [])
		}).toThrow(
			"splitDocumentationRightSide would end without titledCodeBlock, metricBlock, or mermaidBlock. Add one there first, or delete the block that holds it.",
		)
	})

	it("refuses emptying the document root without naming a holder", ({
		expect,
	}) => {
		expect(() => {
			checkChildren("doc", [])
		}).toThrow(
			/^the document root would end without .+\. Add one there first\.$/,
		)
	})

	it("refuses a block after a parent's content is complete", ({
		expect,
	}) => {
		expect(() => {
			checkChildren("titledCodeBlock", [
				"codeBlockTitle",
				"codeBlock",
				"codeBlock",
			])
		}).toThrow("which takes nothing more at that point")
	})

	it("refuses an unknown child type", ({ expect }) => {
		expect(() => {
			checkChildren("doc", ["wibble"])
		}).toThrow("wibble is not allowed there in the document root")
	})

	it("refuses an unknown parent type", ({ expect }) => {
		expect(() => {
			checkChildren("wibble", [])
		}).toThrow("wibble is not a known block type")
	})
})

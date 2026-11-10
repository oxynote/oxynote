import type { JSONContent } from "@tiptap/core"
import { describe, expect, it } from "vitest"
import { comparableGridAttrs, hashBlockContent } from "./content-hash"
import { text } from "../test-helpers"

function paragraph(
	content: JSONContent[],
	attrs?: JSONContent["attrs"],
): JSONContent {
	return { type: "paragraph", attrs, content }
}

describe("comparableGridAttrs", () => {
	it.for([
		{
			name: "drops the places and keeps the size it stores",
			input: {
				title: "a",
				widePosX: 12,
				widePosY: 3,
				width: 8,
				height: 2,
				stackPos: 1,
			},
			expected: { title: "a", width: 8, height: 2 },
		},
		{
			name: "reads a named width and a missing height as columns and rows",
			input: { title: "a", width: "wide" },
			expected: { title: "a", width: 24, height: 1 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(comparableGridAttrs(input)).toEqual(expected)
	})
})

describe("hashBlockContent", () => {
	it("returns identical hashes for structurally equal nodes", () => {
		const a = paragraph([text("hello")], { uid: "u1" })
		const b = paragraph([text("hello")], { uid: "u1" })

		expect(hashBlockContent(a)).toBe(hashBlockContent(b))
	})

	it("returns different hashes when the text differs", () => {
		const a = paragraph([text("hello")])
		const b = paragraph([text("hellp")])

		expect(hashBlockContent(a)).not.toBe(hashBlockContent(b))
	})

	it("ignores comment marks and node comment attributes by default", () => {
		const commented = paragraph(
			[text("hello", [{ type: "comment", attrs: { commentId: "c1" } }])],
			{ uid: "u1", nodeCommentId: "nc1" },
		)
		const clean = paragraph([text("hello")], { uid: "u1" })

		expect(hashBlockContent(commented)).toBe(hashBlockContent(clean))
	})

	// the server switches a simulation off on whichever branch is open.
	// That is not a user change, so the hash ignores the flag
	it("ignores a metric block's simulation flag by default", () => {
		const simulating = {
			type: "metricBlock",
			attrs: {
				uid: "m1",
				simulationPreset: "cpu_usage",
				simulationActive: true,
			},
		}
		const answered = {
			type: "metricBlock",
			attrs: {
				uid: "m1",
				simulationPreset: "cpu_usage",
				simulationActive: false,
			},
		}

		expect(hashBlockContent(simulating)).toBe(hashBlockContent(answered))
	})

	it("keeps a metric block's simulation preset significant", () => {
		const a = {
			type: "metricBlock",
			attrs: { uid: "m1", simulationPreset: "cpu_usage" },
		}
		const b = {
			type: "metricBlock",
			attrs: { uid: "m1", simulationPreset: "error_rate" },
		}

		expect(hashBlockContent(a)).not.toBe(hashBlockContent(b))
	})

	// a drop writes cells into every block that it moves. In a grid saved
	// before blocks stored cells, it writes them into every block.
	it("ignores where a grid block sits and how it stores its size", () => {
		const saved = {
			type: "metricBlock",
			attrs: { uid: "m1", width: "compact" },
		}
		const placed = {
			type: "metricBlock",
			attrs: {
				uid: "m1",
				widePosX: 16,
				widePosY: 3,
				width: 8,
				height: 1,
			},
		}

		expect(hashBlockContent(placed)).toBe(hashBlockContent(saved))
	})

	it("keeps a grid block's size significant", () => {
		const a = {
			type: "logBlock",
			attrs: { uid: "l1", width: 8, height: 1 },
		}
		const b = {
			type: "logBlock",
			attrs: { uid: "l1", width: 10, height: 1 },
		}

		expect(hashBlockContent(a)).not.toBe(hashBlockContent(b))
	})

	it("merges text runs split by a comment boundary", () => {
		const split = paragraph([
			text("hel", [{ type: "comment", attrs: { commentId: "c1" } }]),
			text("lo"),
		])
		const whole = paragraph([text("hello")])

		expect(hashBlockContent(split)).toBe(hashBlockContent(whole))
	})

	it("keeps text runs split by other marks distinct", () => {
		const split = paragraph([text("hel", [{ type: "bold" }]), text("lo")])
		const whole = paragraph([text("hello")])

		expect(hashBlockContent(split)).not.toBe(hashBlockContent(whole))
	})

	it("keeps non-comment marks significant", () => {
		const bold = paragraph([text("hello", [{ type: "bold" }])])
		const plain = paragraph([text("hello")])

		expect(hashBlockContent(bold)).not.toBe(hashBlockContent(plain))
	})

	it("keeps other attributes significant", () => {
		const a = paragraph([text("hello")], { uid: "u1" })
		const b = paragraph([text("hello")], { uid: "u2" })

		expect(hashBlockContent(a)).not.toBe(hashBlockContent(b))
	})

	it("is insensitive to attribute key order", () => {
		const a = paragraph([text("hello")], { uid: "u1", level: 2 })
		const b = paragraph([text("hello")], { level: 2, uid: "u1" })

		expect(hashBlockContent(a)).toBe(hashBlockContent(b))
	})

	it("strips excluded marks and attributes at any depth", () => {
		const commented: JSONContent = {
			type: "blockquote",
			content: [
				paragraph(
					[text("deep", [{ type: "comment", attrs: { commentId: "c1" } }])],
					{ nodeCommentId: "nc1" },
				),
			],
		}
		const clean: JSONContent = {
			type: "blockquote",
			content: [paragraph([text("deep")], {})],
		}

		expect(hashBlockContent(commented)).toBe(hashBlockContent(clean))
	})

	it("applies custom exclusions instead of the defaults", () => {
		const bold = paragraph([text("hello", [{ type: "bold" }])])
		const plain = paragraph([text("hello")])
		const commented = paragraph([text("hello", [{ type: "comment" }])])

		expect(hashBlockContent(bold, { excludeMarks: ["bold"] })).toBe(
			hashBlockContent(plain, { excludeMarks: ["bold"] }),
		)

		// with custom options the default comment exclusions no longer apply
		expect(hashBlockContent(commented, { excludeMarks: ["bold"] })).not.toBe(
			hashBlockContent(plain, { excludeMarks: ["bold"] }),
		)
	})
})

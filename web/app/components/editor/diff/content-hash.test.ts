import type { JSONContent } from "@tiptap/core"
import { describe, expect, it } from "vitest"
import { hashBlockContent } from "./content-hash"
import { text } from "../test-helpers"

function paragraph(
	content: JSONContent[],
	attrs?: JSONContent["attrs"],
): JSONContent {
	return { type: "paragraph", attrs, content }
}

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

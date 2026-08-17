import { describe, it } from "vitest"
import { sanitizeSearchResult } from "./sanitize"

describe("sanitizeSearchResult", () => {
	it("keeps mark tags for highlighting", ({ expect }) => {
		expect(sanitizeSearchResult("found a <mark>hit</mark> here")).toBe(
			"found a <mark>hit</mark> here",
		)
	})

	it("strips every other tag but keeps its text", ({ expect }) => {
		expect(sanitizeSearchResult("<b>bold</b> and <mark>hit</mark>")).toBe(
			"bold and <mark>hit</mark>",
		)
	})

	it("strips attributes from mark tags", ({ expect }) => {
		expect(sanitizeSearchResult('<mark class="x" onclick="y">hit</mark>')).toBe(
			"<mark>hit</mark>",
		)
	})

	it("drops script content entirely", ({ expect }) => {
		expect(sanitizeSearchResult("<script>alert(1)</script>safe")).toBe("safe")
	})
})

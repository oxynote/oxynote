import { describe, it } from "vitest"
import { sanitizeSearchResult } from "./sanitize"

describe("sanitizeSearchResult", () => {
	it("returns an empty string for empty input", ({ expect }) => {
		expect(sanitizeSearchResult("")).toBe("")
	})

	// node has no window, so this exercises the server-side fallback
	it("strips every tag on the server side", ({ expect }) => {
		expect(sanitizeSearchResult("<b>bold</b> and <mark>hit</mark>")).toBe(
			"bold and hit",
		)
	})
})

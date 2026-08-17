import { describe, it } from "vitest"
import { postAuthDocumentUrl, postEmailVerificationUrl } from "./auth"

describe("postAuthDocumentUrl", () => {
	it.for([
		{
			name: "joins the base url with the decoded next path",
			baseUrl: "https://app.test",
			next: "%2Fdocs%2Fabc%3Fx%3D1",
			expected: "https://app.test/docs/abc?x=1",
		},
		{
			name: "falls back to the default path without a next path",
			baseUrl: "https://app.test",
			next: undefined,
			expected: "https://app.test/",
		},
		{
			name: "treats a missing base url as empty",
			baseUrl: undefined,
			next: "%2Fdocs",
			expected: "/docs",
		},
		{
			name: "returns the default path alone without base url and next",
			baseUrl: undefined,
			next: undefined,
			expected: "/",
		},
	])("$name", ({ baseUrl, next, expected }, { expect }) => {
		expect(postAuthDocumentUrl(baseUrl, next)).toBe(expected)
	})

	it("uses a custom default path when given", ({ expect }) => {
		expect(postAuthDocumentUrl("https://app.test", undefined, "/home")).toBe(
			"https://app.test/home",
		)
	})
})

describe("postEmailVerificationUrl", () => {
	it("builds the verification url with the email encoded", ({ expect }) => {
		expect(postEmailVerificationUrl("https://app.test", "a+b@test.io")).toBe(
			"https://app.test/verify-email?new=a%2Bb%40test.io",
		)
	})

	it("treats a missing base url as empty", ({ expect }) => {
		expect(postEmailVerificationUrl(undefined, "a@test.io")).toBe(
			"/verify-email?new=a%40test.io",
		)
	})
})

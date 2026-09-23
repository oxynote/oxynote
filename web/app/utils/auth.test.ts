import { describe, it } from "vitest"
import {
	acceptInvitationUrl,
	isInvitationRedirect,
	postAuthDocumentUrl,
	postEmailVerificationUrl,
} from "./auth"

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

describe("acceptInvitationUrl", () => {
	it("carries everything the accept page renders, encoded", ({ expect }) => {
		expect(
			acceptInvitationUrl("https://app.test", {
				id: "inv-1",
				email: "a+b@test.io",
				inviter: "Ada Lovelace",
				organizationName: "Acme & Co",
				organizationId: "org-1",
			}),
		).toBe(
			"https://app.test/accept-invite?id=inv-1&email=a%2Bb%40test.io&inviter=Ada+Lovelace&orgName=Acme+%26+Co&orgId=org-1",
		)
	})
})

describe("isInvitationRedirect", () => {
	it.for([
		{
			name: "an encoded accept-invite path",
			input: encodeURIComponent("/accept-invite?id=inv-1"),
			expected: true,
		},
		{ name: "another page", input: "%2Fdocs%2Fabc", expected: false },
		{ name: "a malformed encoding", input: "%E0%A4%A", expected: false },
		{ name: "no next path", input: undefined, expected: false },
		{ name: "a repeated query parameter", input: ["a", "b"], expected: false },
	])("answers $expected for $name", ({ input, expected }, { expect }) => {
		expect(isInvitationRedirect(input)).toBe(expected)
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

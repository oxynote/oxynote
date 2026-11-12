import { describe, it } from "vitest"
import { isRole } from "./member-roles"

describe("isRole", () => {
	it.for([
		{ name: "admin", input: "admin", expected: true },
		{ name: "editor", input: "editor", expected: true },
		{ name: "viewer", input: "viewer", expected: true },
		{ name: "a built-in better-auth role", input: "owner", expected: false },
		{ name: "an empty string", input: "", expected: false },
		{ name: "a value that is not a string", input: 1, expected: false },
	])("answers $expected for $name", ({ input, expected }, { expect }) => {
		expect(isRole(input)).toBe(expected)
	})
})

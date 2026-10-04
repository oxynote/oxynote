import { describe, it } from "vitest"
import { hookErrorMessage } from "./hook-errors"

describe("hookErrorMessage", () => {
	it.for([
		{
			name: "a check that failed",
			input: "document_hook.missing_repository",
			expected: "editor.hooks.errors.codes.missing-repository",
		},
		{
			name: "settings the processor refused",
			input: "document_hook.invalid_image",
			expected: "editor.hooks.errors.codes.invalid-image",
		},
		{
			name: "a service the hook could not reach",
			input: "document_hook.upstream_unavailable",
			expected: "editor.hooks.errors.codes.upstream-unavailable",
		},
	])("names the message of $name", ({ input, expected }, { expect }) => {
		expect(hookErrorMessage({ data: { code: input } }, t)).toBe(
			`message:${expected}`,
		)
	})

	it("names nothing for an unknown code", ({ expect }) => {
		expect(hookErrorMessage({ data: { code: "general" } }, t)).toBeUndefined()
	})

	it("names nothing for an error without a body", ({ expect }) => {
		expect(hookErrorMessage(new Error("boom"), t)).toBeUndefined()
		expect(hookErrorMessage(undefined, t)).toBeUndefined()
	})
})

function t(key: string): string {
	return `message:${key}`
}

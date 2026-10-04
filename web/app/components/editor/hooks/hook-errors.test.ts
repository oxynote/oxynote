import { describe, it } from "vitest"
import { hookErrorMessage } from "./hook-errors"

describe("hookErrorMessage", () => {
	it.for([
		{
			name: "names the message of a check that failed",
			input: { data: { code: "document_hook.missing_repository" } },
			expected: "message:editor.hooks.errors.codes.missing-repository",
		},
		{
			name: "names the message of settings the processor refused",
			input: { data: { code: "document_hook.invalid_image" } },
			expected: "message:editor.hooks.errors.codes.invalid-image",
		},
		{
			name: "names the message of a service the hook could not reach",
			input: { data: { code: "document_hook.upstream_unavailable" } },
			expected: "message:editor.hooks.errors.codes.upstream-unavailable",
		},
		{
			name: "names nothing for an unknown code",
			input: { data: { code: "general" } },
			expected: undefined,
		},
		{
			name: "names nothing for an error without a body",
			input: new Error("boom"),
			expected: undefined,
		},
		{
			name: "names nothing for a missing error",
			input: undefined,
			expected: undefined,
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(hookErrorMessage(input, t)).toBe(expected)
	})
})

function t(key: string): string {
	return `message:${key}`
}

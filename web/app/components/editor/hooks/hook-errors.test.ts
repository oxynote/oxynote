import { describe, it } from "vitest"
import { hookErrorMessage } from "./hook-errors"

describe("hookErrorMessage", () => {
	it.for([
		{
			input: "document_hook.missing_repository",
			expected: "editor.hooks.errors.codes.missing-repository",
		},
		{
			input: "document_hook.invalid_image",
			expected: "editor.hooks.errors.codes.invalid-image",
		},
		{
			input: "document_hook.upstream_unavailable",
			expected: "editor.hooks.errors.codes.upstream-unavailable",
		},
	])("names the message of $input", ({ input, expected }, { expect }) => {
		expect(hookErrorMessage({ data: { code: input } }, i18n())).toBe(
			`message:${expected}`,
		)
	})

	it("names nothing for an unknown code", ({ expect }) => {
		expect(
			hookErrorMessage({ data: { code: "general" } }, i18n()),
		).toBeUndefined()
	})

	it("names nothing for an error without a body", ({ expect }) => {
		expect(hookErrorMessage(new Error("boom"), i18n())).toBeUndefined()
		expect(hookErrorMessage(undefined, i18n())).toBeUndefined()
	})
})

function i18n() {
	return {
		t: (key: string) => `message:${key}`,
	} as unknown as Parameters<typeof hookErrorMessage>[1]
}

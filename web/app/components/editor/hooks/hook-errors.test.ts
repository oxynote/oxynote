import { describe, it } from "vitest"
import { hookErrorKey } from "./hook-errors"

describe("hookErrorKey", () => {
	it("names the status a refused hook would start in", ({ expect }) => {
		expect(
			hookErrorKey({
				statusCode: 422,
				data: { code: "document_hook.missing_repository" },
			}),
		).toBe("editor.hooks.errors.codes.missing-repository")
	})

	it("names the setting core rejects", ({ expect }) => {
		expect(
			hookErrorKey({
				statusCode: 400,
				data: { code: "registry.invalid_reference" },
			}),
		).toBe("editor.hooks.errors.codes.invalid-image")
	})

	it("tells an unreachable service apart", ({ expect }) => {
		expect(
			hookErrorKey({
				statusCode: 424,
				data: { code: "document_hook.upstream_unavailable" },
			}),
		).toBe("editor.hooks.errors.codes.upstream-unavailable")
	})

	it("names nothing for an unknown code", ({ expect }) => {
		expect(
			hookErrorKey({ statusCode: 500, data: { code: "general" } }),
		).toBeUndefined()
	})

	it("names nothing for an error without a body", ({ expect }) => {
		expect(hookErrorKey(new Error("boom"))).toBeUndefined()
		expect(hookErrorKey(undefined)).toBeUndefined()
	})
})

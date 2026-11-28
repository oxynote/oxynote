import { describe, it } from "vitest"
import { isNewerRelease, isReleaseVersion } from "./version"

describe("isReleaseVersion", () => {
	it.for([
		{ input: "0.11.0", expected: true },
		{ input: "12.3.45", expected: true },
		{ input: "dev", expected: false },
		{ input: "", expected: false },
		{ input: "v0.11.0", expected: false },
		{ input: "0.11", expected: false },
		{ input: "0.12.0-rc.1", expected: false },
	])("returns $expected for $input", ({ input, expected }, { expect }) => {
		expect(isReleaseVersion(input)).toBe(expected)
	})
})

describe("isNewerRelease", () => {
	it.for([
		{ input: { candidate: "0.12.0", current: "0.11.0" }, expected: true },
		{ input: { candidate: "0.11.1", current: "0.11.0" }, expected: true },
		{ input: { candidate: "1.0.0", current: "0.99.9" }, expected: true },
		{ input: { candidate: "0.10.0", current: "0.9.3" }, expected: true },
		{ input: { candidate: "0.11.0", current: "0.11.0" }, expected: false },
		{ input: { candidate: "0.11.0", current: "0.12.0" }, expected: false },
		{ input: { candidate: "0.9.3", current: "0.10.0" }, expected: false },
		{ input: { candidate: "0.12.0", current: "dev" }, expected: false },
		{ input: { candidate: "dev", current: "0.11.0" }, expected: false },
	])(
		"returns $expected for $input.candidate over $input.current",
		({ input, expected }, { expect }) => {
			expect(isNewerRelease(input.candidate, input.current)).toBe(expected)
		},
	)
})

import { describe, it } from "vitest"
import { tagColorFor } from "./tag-color"

describe("tagColorFor", () => {
	it("tints a hex colour for both modes", ({ expect }) => {
		expect(tagColorFor("#123456")).toEqual({
			lightBg: "color-mix(in srgb, #123456 13%, transparent)",
			lightFg: "color-mix(in srgb, #123456 80%, black)",
			darkBg: "color-mix(in srgb, #123456 18%, transparent)",
			darkBorder: "color-mix(in srgb, #123456 40%, transparent)",
			darkFg: "color-mix(in srgb, #123456 60%, white)",
		})
	})

	it("tints a colour the picker resolved from a theme variable", ({
		expect,
	}) => {
		const treatment = tagColorFor("oklch(0.577 0.245 27.325)")

		expect(treatment.lightBg).toBe(
			"color-mix(in srgb, oklch(0.577 0.245 27.325) 13%, transparent)",
		)
	})
})

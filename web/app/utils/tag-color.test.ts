import { describe, it } from "vitest"
import { tagColorFor } from "./tag-color"

describe("tagColorFor", () => {
	it("tints a palette colour for both modes", ({ expect }) => {
		expect(tagColorFor("green")).toEqual({
			lightBg: "color-mix(in srgb, var(--selectable-color-6) 13%, transparent)",
			lightFg: "color-mix(in srgb, var(--selectable-color-6) 80%, black)",
			darkBg: "color-mix(in srgb, var(--selectable-color-6) 18%, transparent)",
			darkBorder:
				"color-mix(in srgb, var(--selectable-color-6) 40%, transparent)",
			darkFg: "color-mix(in srgb, var(--selectable-color-6) 60%, white)",
		})
	})
})

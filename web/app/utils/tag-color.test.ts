import { describe, it, vi } from "vitest"
import { pickTagColor, tagColorFor } from "./tag-color"

const PALETTE = ["#ff0000", "#00ff00", "#0000ff", "#ffff00"]

// the draw is the only randomness, so a fixed fraction of the range names
// exactly which colour comes back
function stubDraw(fraction: number) {
	vi.spyOn(Math, "random").mockReturnValue(fraction)
}

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

// the draw is spied on Math itself, which every test in the file shares
describe("pickTagColor", { concurrent: false }, () => {
	it("draws from the colours no tag holds yet", ({ expect }) => {
		stubDraw(0)

		expect(pickTagColor(PALETTE, ["#ff0000"])).toBe("#00ff00")
	})

	it("counts a colour outside the palette as no use at all", ({ expect }) => {
		stubDraw(0)

		expect(pickTagColor(PALETTE, ["#123456"])).toBe("#ff0000")
	})

	// red is held three times and the rest once, so the bands come out as
	// red [0,1), green [1,4), blue [4,7), yellow [7,10)
	it.for([
		{
			name: "gives the most used colour the narrowest band",
			draw: 0.05,
			expected: "#ff0000",
		},
		{
			name: "gives a least used colour a band three times as wide",
			draw: 0.15,
			expected: "#00ff00",
		},
		{
			name: "reaches the last colour at the top of the range",
			draw: 0.95,
			expected: "#ffff00",
		},
	])("$name once every colour is taken", ({ draw, expected }, { expect }) => {
		stubDraw(draw)

		expect(
			pickTagColor(PALETTE, [
				"#ff0000",
				"#ff0000",
				"#ff0000",
				"#00ff00",
				"#0000ff",
				"#ffff00",
			]),
		).toBe(expected)
	})

	it("returns nothing without a palette", ({ expect }) => {
		expect(pickTagColor([], ["#ff0000"])).toBeUndefined()
	})
})

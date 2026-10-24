import { describe, it, vi } from "vitest"
import { paletteColor, pickLeastUsedColor } from "./color"

const PALETTE = ["#ff0000", "#00ff00", "#0000ff", "#ffff00"]

describe("paletteColor", () => {
	it.for([
		{ input: "red", expected: "var(--selectable-color-1)" },
		{ input: "green", expected: "var(--selectable-color-6)" },
		{ input: "pink", expected: "var(--selectable-color-16)" },
	])(
		"paints $input with its theme variable",
		({ input, expected }, { expect }) => {
			expect(paletteColor(input)).toBe(expected)
		},
	)

	it("returns a colour saved as css as it is", ({ expect }) => {
		expect(paletteColor("oklch(57.7% .245 27.325)")).toBe(
			"oklch(57.7% .245 27.325)",
		)
	})
})

// the draw is spied on Math itself, which every test in the file shares
describe("pickLeastUsedColor", { concurrent: false }, () => {
	it("draws from the colours nothing holds yet", ({ expect }) => {
		stubDraw(0)

		expect(pickLeastUsedColor(PALETTE, ["#ff0000"])).toBe("#00ff00")
	})

	it("counts a colour outside the palette as no use at all", ({ expect }) => {
		stubDraw(0)

		expect(pickLeastUsedColor(PALETTE, ["#123456"])).toBe("#ff0000")
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
			pickLeastUsedColor(PALETTE, [
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
		expect(pickLeastUsedColor([], ["#ff0000"])).toBeUndefined()
	})
})

// the draw is the only randomness, so a fixed fraction of the range names
// exactly which colour comes back
function stubDraw(fraction: number) {
	vi.spyOn(Math, "random").mockReturnValue(fraction)
}

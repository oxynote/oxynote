// PALETTE_COLORS names every colour a tag, a variable group or a metric
// threshold can carry, in picker order. What is stored is the name, and the
// n-th name paints as the n-th --selectable-color variable. Core validates
// tags and groups against its own copy of the names
// (server/core/internal/tag/color.go).
export const PALETTE_COLORS = [
	"red",
	"orange",
	"amber",
	"yellow",
	"lime",
	"green",
	"emerald",
	"teal",
	"cyan",
	"sky",
	"blue",
	"indigo",
	"violet",
	"purple",
	"fuchsia",
	"pink",
]

// DEFAULT_PALETTE_COLOR is the colour something new takes when nothing
// suggests another.
export const DEFAULT_PALETTE_COLOR = "green"

// paletteColor resolves a stored palette name to the css colour that
// paints it. A threshold saved before thresholds stored names holds a css
// colour already and is returned as it is.
export function paletteColor(color: string): string {
	const slot = PALETTE_COLORS.indexOf(color)
	if (slot < 0) {
		return color
	}

	return `var(--selectable-color-${slot + 1})`
}

// pickLeastUsedColor suggests a colour for something new, such as a tag or
// a variable group, drawing at random from the ones nothing holds yet. Once
// the palette is exhausted every colour stays in play, weighted so the
// least used are the most likely, so new things keep spreading out instead
// of piling onto whichever colour comes first.
export function pickLeastUsedColor(
	palette: string[],
	used: string[],
): string | undefined {
	const counts = new Map(palette.map((color) => [color, 0]))

	used.forEach((color) => {
		const count = counts.get(color)
		if (count === undefined) {
			return
		}

		counts.set(color, count + 1)
	})

	const unused = palette.filter((color) => counts.get(color) === 0)
	if (unused.length) {
		return unused[Math.floor(Math.random() * unused.length)]
	}

	const most = Math.max(...counts.values())
	const weights = palette.map((color) => most + 1 - (counts.get(color) ?? 0))
	const target =
		Math.random() * weights.reduce((sum, weight) => sum + weight, 0)

	let running = 0
	let picked = palette[0]

	// each colour owns a band of the total width, and the target lands in
	// the band of the last colour starting at or before it
	palette.forEach((color, index) => {
		if (running <= target) {
			picked = color
		}

		running += weights[index] ?? 0
	})

	return picked
}

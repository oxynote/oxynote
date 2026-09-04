// TagColorTreatment is how a tag's colour is rendered as a pill. A tag
// stores one colour; light and dark each tint it differently, so both are
// derived here and handed to css together.
export interface TagColorTreatment {
	lightBg: string
	lightFg: string
	darkBg: string
	darkBorder: string
	darkFg: string
}

// tagColorFor tints a tag's colour into its pill treatment: 13% behind
// darkened text in light mode, 18% behind a 40% border in dark. A tag can
// carry any colour the picker offers, so the tints are computed rather
// than looked up.
export function tagColorFor(color: string): TagColorTreatment {
	return {
		lightBg: `color-mix(in srgb, ${color} 13%, transparent)`,
		lightFg: `color-mix(in srgb, ${color} 80%, black)`,
		darkBg: `color-mix(in srgb, ${color} 18%, transparent)`,
		darkBorder: `color-mix(in srgb, ${color} 40%, transparent)`,
		darkFg: `color-mix(in srgb, ${color} 60%, white)`,
	}
}

// pickTagColor suggests a colour for a new tag, drawing at random from the
// ones no tag holds yet. Once the palette is exhausted every colour stays
// in play, weighted so the least used are the most likely — new tags keep
// spreading out instead of piling onto whichever colour comes first.
//
// Both arguments have to be in the same notation for a colour to count as
// used; the caller normalises them.
export function pickTagColor(
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

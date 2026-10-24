import { paletteColor } from "./color"

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

// tagColorFor tints a tag's palette colour into its pill treatment: 13%
// behind darkened text in light mode, 18% behind a 40% border in dark.
export function tagColorFor(name: string): TagColorTreatment {
	const color = paletteColor(name)

	return {
		lightBg: `color-mix(in srgb, ${color} 13%, transparent)`,
		lightFg: `color-mix(in srgb, ${color} 80%, black)`,
		darkBg: `color-mix(in srgb, ${color} 18%, transparent)`,
		darkBorder: `color-mix(in srgb, ${color} 40%, transparent)`,
		darkFg: `color-mix(in srgb, ${color} 60%, white)`,
	}
}

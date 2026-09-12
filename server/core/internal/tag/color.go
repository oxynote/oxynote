package tag

import "slices"

// _palette is every colour a tag may carry, in the order the frontend's
// picker shows them. The picker paints the --selectable-color-N
// variables in web/app/assets/css/main.css, Tailwind's *-600 oklch
// shades, and stores the hex a canvas hands back for each: these are
// those values, so the list has to follow any change to the variables.
var _palette = []Color{
	{Name: "red", Hex: "#e7000b"},
	{Name: "orange", Hex: "#f54a00"},
	{Name: "amber", Hex: "#e17100"},
	{Name: "yellow", Hex: "#d08700"},
	{Name: "lime", Hex: "#5ea500"},
	{Name: "green", Hex: "#00a63e"},
	{Name: "emerald", Hex: "#009966"},
	{Name: "teal", Hex: "#009689"},
	{Name: "cyan", Hex: "#0092b8"},
	{Name: "sky", Hex: "#0084d1"},
	{Name: "blue", Hex: "#155dfc"},
	{Name: "indigo", Hex: "#4f39f6"},
	{Name: "violet", Hex: "#7f22fe"},
	{Name: "purple", Hex: "#9810fa"},
	{Name: "fuchsia", Hex: "#c800de"},
	{Name: "pink", Hex: "#e60076"},
}

// Color is one palette entry: the name a person or model picks it by and
// the hex value a tag stores.
type Color struct {
	// Name is the colour's palette name, such as "green".
	Name string

	// Hex is the colour as a lowercase "#rrggbb" triplet.
	Hex string
}

// ColorHex returns the hex value of the palette colour with the given
// name, or "" when the palette has no such name.
func ColorHex(name string) string {
	i := slices.IndexFunc(_palette, func(c Color) bool { return c.Name == name })
	if i < 0 {
		return ""
	}

	return _palette[i].Hex
}

// ColorName returns the palette name of the given hex value, or "" when
// the palette has no such colour. The match is exact: the palette is
// lowercase and so is every colour the frontend stores.
func ColorName(hex string) string {
	i := slices.IndexFunc(_palette, func(c Color) bool { return c.Hex == hex })
	if i < 0 {
		return ""
	}

	return _palette[i].Name
}

// ColorNames returns the palette names in palette order.
func ColorNames() []string {
	out := make([]string, 0, len(_palette))

	for _, c := range _palette {
		out = append(out, c.Name)
	}

	return out
}

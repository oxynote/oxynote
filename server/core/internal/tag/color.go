package tag

import "slices"

// _palette is every colour a tag or a variable group may carry, in the
// order the frontend's picker shows them. Only the name is stored, and the
// frontend resolves it to a theme colour (web/app/utils/color.ts), so both
// lists have to name the same colours.
var _palette = []string{
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
}

// ValidColor reports whether the given colour is a palette name.
func ValidColor(name string) bool {
	return slices.Contains(_palette, name)
}

// ColorNames returns the palette names in palette order.
func ColorNames() []string {
	return slices.Clone(_palette)
}

// Package icon holds the icons a document can use, which are the ones the
// editor's picker offers.
package icon

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// _iconsJSON is the picker's icon list, in the picker's order.
// scripts/check-icons.mjs keeps it equal to web/app/utils/icon.ts.
//
//go:embed icons.json
var _iconsJSON []byte

// _entries is the parsed icon list. The file is embedded, so a parse
// failure is a bug caught by every test of this package.
var _entries = func() []Entry {
	var entries []Entry
	if err := json.Unmarshal(_iconsJSON, &entries); err != nil {
		panic(fmt.Sprintf("parsing the embedded icon list: %v", err))
	}

	return entries
}()

// _styleSuffix matches the style and variant suffixes an icon id ends
// with, such as -fill, -24-filled or -2.
var _styleSuffix = regexp.MustCompile(`(-(fill|line|filled|outline|solid|\d+))+$`)

// Entry is one icon the picker offers.
type Entry struct {
	// ID specifies the Iconify id, such as mingcute:rocket-fill.
	ID string `json:"id"`

	// Name specifies the words the picker shows and searches.
	Name string `json:"name"`
}

// Words returns the lowercase words of the icon's name and id.
func (e Entry) Words() []string {
	return slices.Concat(words(e.Name), extractWords(e.ID))
}

// List returns the icons the picker offers, in the picker's order. The
// caller must not modify the result.
func List() []Entry {
	return _entries
}

// Find reports whether id is an icon the picker offers.
func Find(id string) bool {
	return slices.ContainsFunc(_entries, func(e Entry) bool { return e.ID == id })
}

// extractWords returns the lowercase words of an icon id without its
// collection and style suffix.
func extractWords(id string) []string {
	if _, rest, ok := strings.Cut(id, ":"); ok {
		id = rest
	}

	return words(_styleSuffix.ReplaceAllString(strings.ToLower(id), ""))
}

// words splits s into lowercase words of letters and digits.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

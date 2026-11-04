// Package icon holds the icons a document can use, which are the ones the
// editor's picker offers.
package icon

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

// _iconsJSON is the picker's icon list, in the picker's order.
// scripts/check-icons.mjs keeps it equal to web/app/utils/icon.ts.
//
//go:embed icons.json
var _iconsJSON []byte

// Entry is one icon the picker offers.
type Entry struct {
	// ID specifies the Iconify id, such as mingcute:rocket-fill.
	ID string `json:"id"`

	// Name specifies the words the picker shows and searches.
	Name string `json:"name"`
}

// _entries is the parsed icon list. The file is embedded, so a parse
// failure is a bug caught by every test of this package.
var _entries = func() []Entry {
	var entries []Entry
	if err := json.Unmarshal(_iconsJSON, &entries); err != nil {
		panic(fmt.Sprintf("parsing the embedded icon list: %v", err))
	}

	return entries
}()

// List returns the icons the picker offers, in the picker's order. The
// caller must not modify the result.
func List() []Entry {
	return _entries
}

// Find reports whether id is an icon the picker offers.
func Find(id string) bool {
	return slices.ContainsFunc(_entries, func(e Entry) bool { return e.ID == id })
}

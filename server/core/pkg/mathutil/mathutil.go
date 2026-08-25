// Package mathutil implements helper values and functions for the exact
// decimal arithmetic the domain is expressed in.
package mathutil

import "github.com/shopspring/decimal"

// _hundred is the integer Hundred is built from.
const _hundred = 100

// Hundred is the decimal 100 — the full-percentage reference every ratio in
// the codebase is measured against, so that a full score and a full percent
// are the same value everywhere.
//
//nolint:gochecknoglobals // used as a constant
var Hundred = decimal.NewFromInt(_hundred)

// SafeDiv divides a by b, returning zero when b is zero instead of
// panicking.
func SafeDiv(a, b decimal.Decimal) decimal.Decimal {
	if b.IsZero() {
		return decimal.Zero
	}

	return a.Div(b)
}

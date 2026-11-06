// Package icon checks icon ids against the ones the editor's picker
// offers.
package icon

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	docicon "github.com/oxynote/oxynote/server/core/internal/document/icon"
)

const (
	// _maxIDLength is the longest icon id Validate looks at. Every
	// offered id is far shorter, and scoring a long one is slow.
	_maxIDLength = 128

	// _maxSuggestions is the number of close icons an error names.
	_maxSuggestions = 8

	// _exactScore is what a query word scores against an equal word.
	_exactScore = 2

	// _partialScore is what a query word scores against a word that
	// holds it or that it holds.
	_partialScore = 1

	// _minPartialWord is the shortest word that matches as part of
	// another. Shorter ones, such as net or js, are inside too many names.
	_minPartialWord = 4
)

// Validate returns nil when id is an icon the picker offers, or an
// error naming the offered icons whose names are closest to it.
func Validate(id string) error {
	if len(id) > _maxIDLength {
		return fmt.Errorf("icon is %d bytes, but an icon id is at most %d; use an id from the icon picker", len(id), _maxIDLength)
	}

	if docicon.Find(id) {
		return nil
	}

	closest := suggest(docicon.List(), id)
	if len(closest) == 0 {
		return fmt.Errorf("icon %q is not available and no available icon has a similar name; try a plainer word, such as mingcute:document-2-fill", id)
	}

	return fmt.Errorf("icon %q is not available; use one of %s", id, strings.Join(closest, ", "))
}

// suggest returns the ids of the icons whose words best match the words
// of id, best first, ties in list order.
func suggest(entries []docicon.Entry, id string) []string {
	query := docicon.Entry{ID: id}.Words()

	type scored struct {
		id    string
		score int
	}

	var hits []scored

	for _, e := range entries {
		iconWords := e.Words()
		total := 0

		for _, q := range query {
			best := 0

			for _, w := range iconWords {
				best = max(best, wordScore(q, w))
			}

			total += best
		}

		if total > 0 {
			hits = append(hits, scored{e.ID, total})
		}
	}

	slices.SortStableFunc(hits, func(a, b scored) int { return cmp.Compare(b.score, a.score) })

	out := make([]string, 0, _maxSuggestions)
	for _, h := range hits[:min(len(hits), _maxSuggestions)] {
		out = append(out, h.id)
	}

	return out
}

// wordScore returns the score of one word against another: exact when
// equal, partial when one holds the other and both are long enough, and
// 0 otherwise.
func wordScore(a, b string) int {
	switch {
	case a == b:
		return _exactScore
	case len(a) >= _minPartialWord && len(b) >= _minPartialWord && (strings.Contains(a, b) || strings.Contains(b, a)):
		return _partialScore
	default:
		return 0
	}
}

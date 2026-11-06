package icon

import (
	"errors"
	"slices"
	"strings"
	"testing"

	docicon "github.com/oxynote/oxynote/server/core/internal/document/icon"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		ID  string
		Err error
	}{
		"Offered icon": {ID: "mingcute:rocket-fill"},
		"Id over the length limit": {
			ID:  strings.Repeat("a-", 65),
			Err: errors.New("icon is 130 bytes, but an icon id is at most 128; use an id from the icon picker"),
		},
		"Offered icon from another collection": {ID: "simple-icons:kubernetes"},
		"Unknown icon with close ones": {
			ID:  "lucide:rocket",
			Err: errors.New(`icon "lucide:rocket" is not available; use one of mingcute:rocket-fill, mingcute:rocket-2-fill`),
		},
		"Unknown icon with nothing close": {
			ID:  "mingcute:deploy-fill",
			Err: errors.New(`icon "mingcute:deploy-fill" is not available and no available icon has a similar name; try a plainer word, such as mingcute:document-2-fill`),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, Validate(c.ID))
		})
	}
}

func Test_suggest(t *testing.T) {
	t.Parallel()

	entries := []docicon.Entry{
		{ID: "mingcute:book-fill", Name: "book"},
		{ID: "mingcute:folder-open-fill", Name: "folder open"},
		{ID: "mingcute:book-open-fill", Name: "book open"},
		{ID: "devicon-plain:postgresql", Name: "postgresql"},
		{ID: "mingcute:cube-fill", Name: "cube"},
	}

	cc := map[string]struct {
		Entries []docicon.Entry
		ID      string
		Result  []string
	}{
		"Exact words rank first and ties keep list order": {
			Entries: entries,
			ID:      "lucide:book-open",
			Result:  []string{"mingcute:book-open-fill", "mingcute:book-fill", "mingcute:folder-open-fill"},
		},
		"Word inside a name": {
			Entries: entries,
			ID:      "mingcute:postgres-fill",
			Result:  []string{"devicon-plain:postgresql"},
		},
		"Nothing close": {
			Entries: entries,
			ID:      "mingcute:deploy-fill",
			Result:  []string{},
		},
		"Result stops at the cap": {
			Entries: slices.Repeat([]docicon.Entry{{ID: "x:book", Name: "book"}}, _maxSuggestions+2),
			ID:      "book",
			Result:  slices.Repeat([]string{"x:book"}, _maxSuggestions),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, suggest(c.Entries, c.ID))
		})
	}
}

func Test_wordScore(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		A      string
		B      string
		Result int
	}{
		"Equal words":         {A: "c", B: "c", Result: 2},
		"One holds the other": {A: "postgres", B: "postgresql", Result: 1},
		"Other holds the one": {A: "postgresql", B: "postgres", Result: 1},
		"Short word inside":   {A: "net", B: "kubernetes"},
		"Unrelated words":     {A: "deploy", B: "rocket"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, wordScore(c.A, c.B))
		})
	}
}

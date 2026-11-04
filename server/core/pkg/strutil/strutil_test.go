package strutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_Ellipsize(t *testing.T) {
	cc := map[string]struct {
		Input  string
		MaxLen int
		Result string
	}{
		"Shorter than limit":     {Input: "abc", MaxLen: 5, Result: "abc"},
		"Exactly at limit":       {Input: "abcde", MaxLen: 5, Result: "abcde"},
		"Longer than limit":      {Input: "abcdef", MaxLen: 5, Result: "abcde…"},
		"Zero limit passthrough": {Input: "abcdef", MaxLen: 0, Result: "abcdef"},
		"Negative limit":         {Input: "abcdef", MaxLen: -1, Result: "abcdef"},
		"Multibyte runes":        {Input: "ąčęėįš", MaxLen: 3, Result: "ąčę…"},
		"Empty input":            {Input: "", MaxLen: 3, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, Ellipsize(c.Input, c.MaxLen))
		})
	}
}

func Test_Preview(t *testing.T) {
	cc := map[string]struct {
		Input  string
		MaxLen int
		Result string
	}{
		"Newlines collapse":  {Input: "a\nb\nc", MaxLen: 10, Result: "a b c"},
		"Whitespace trimmed": {Input: "  x  ", MaxLen: 10, Result: "x"},
		"Long text elided":   {Input: "abcdef", MaxLen: 3, Result: "abc…"},
		"Empty input":        {Input: "", MaxLen: 3, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, Preview(c.Input, c.MaxLen))
		})
	}
}

func Test_ValidateName(t *testing.T) {
	t.Parallel()

	errEmpty := errors.New("empty")
	errLong := errors.New("long")

	cc := map[string]struct {
		Name string
		Err  error
	}{
		"Empty name":                 {Err: errEmpty},
		"Name at the length limit":   {Name: strings.Repeat("é", 5)},
		"Name over the length limit": {Name: strings.Repeat("é", 6), Err: errLong},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, ValidateName(c.Name, 5, errEmpty, errLong))
		})
	}
}

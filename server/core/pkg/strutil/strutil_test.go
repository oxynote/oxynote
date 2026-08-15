package strutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

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

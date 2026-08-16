package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_parseOrigins(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Val    string
		Result []string
	}{
		"Unset value leaves origin checking disabled": {
			Val: "",
		},
		"Single origin": {
			Val:    "https://one.test",
			Result: []string{"https://one.test"},
		},
		"Several origins": {
			Val:    "https://one.test,https://two.test",
			Result: []string{"https://one.test", "https://two.test"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, parseOrigins(c.Val))
		})
	}
}

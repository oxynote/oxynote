package processor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_expandMacros(t *testing.T) {
	t.Parallel()

	// the expansion echoes what it was given, so a case can assert on the
	// name and arguments the scanner produced. The raw name emits the
	// invocation as the scanner saw it.
	expand := func(name string, args []string, raw string) (string, bool) {
		if name == "skip" {
			return "", false
		}

		if name == "raw" {
			return raw, true
		}

		var b strings.Builder

		b.WriteString("<" + name)

		for _, a := range args {
			b.WriteString("|" + a)
		}

		b.WriteString(">")

		return b.String(), true
	}

	cc := map[string]struct {
		Query    string
		Expected string
	}{
		"Query without macros is untouched": {
			Query:    "SELECT 1 FROM t WHERE a = '$_x'",
			Expected: "SELECT 1 FROM t WHERE a = '$_x'",
		},
		"Bare argument": {
			Query:    `$__timeFilter("time")`,
			Expected: "<timeFilter|time>",
		},
		"No arguments": {
			Query:    "$__timeFrom()",
			Expected: "<timeFrom>",
		},
		// the old regexp stopped at the first ")", cutting the call in half
		// and leaving a stray parenthesis in the emitted SQL.
		"Argument that is itself a call": {
			Query:    "$__timeFilter(COALESCE(a, b))",
			Expected: "<timeFilter|COALESCE(a, b)>",
		},
		"Commas inside a nested call do not split the arguments": {
			Query:    "$__timeGroup(COALESCE(a, b), '5m')",
			Expected: "<timeGroup|COALESCE(a, b)|5m>",
		},
		"Invocation reaches the expansion exactly as written": {
			Query:    `SELECT $__raw( 'time' , COALESCE(a, b)) FROM t`,
			Expected: `SELECT $__raw( 'time' , COALESCE(a, b)) FROM t`,
		},
		"Unhandled macro is left as it was": {
			Query:    "SELECT $__skip(a) FROM t",
			Expected: "SELECT $__skip(a) FROM t",
		},
		"Unclosed invocation is left as it was": {
			Query:    "SELECT $__timeFilter(a FROM t",
			Expected: "SELECT $__timeFilter(a FROM t",
		},
		"Macro among other text": {
			Query:    "SELECT * FROM t WHERE $__timeFilter(ts) ORDER BY ts",
			Expected: "SELECT * FROM t WHERE <timeFilter|ts> ORDER BY ts",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, expandMacros(c.Query, expand))
		})
	}
}

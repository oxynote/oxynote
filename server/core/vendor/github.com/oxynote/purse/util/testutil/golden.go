package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ProcessGoldenJSON works in three steps:
//  1. Golden JSON files are loaded from the specified directory.
//  2. Golden JSON is unmarshalled into an instance provided by the
//     constructor function.
//  3. The instance is marshalled into JSON and then compared against
//     the golden JSON.
//
// NOTE: constructor function must return a pointer.
func ProcessGoldenJSON(t *testing.T, dirname string, fn func() any) {
	t.Helper()

	ff, err := os.ReadDir(dirname)
	require.NoError(t, err)
	require.NotEmpty(t, ff)

	var count int

	for _, f := range ff {
		if filepath.Ext(f.Name()) != ".golden" {
			continue
		}

		cn := strings.TrimSuffix(f.Name(), ".golden")
		count++

		t.Run(cn, func(t *testing.T) {
			t.Helper()

			golden, err := os.ReadFile(filepath.Join("testdata", f.Name()))
			if !assert.NoError(t, err) {
				return
			}

			val := fn()

			err = json.Unmarshal(golden, val)
			if !assert.NoError(t, err) {
				return
			}

			out, err := json.Marshal(val)
			if !assert.NoError(t, err) {
				return
			}

			assert.JSONEq(t, string(golden), string(out))
		})
	}

	// ensure that at least one file test was executed
	require.NotZero(t, count)
}

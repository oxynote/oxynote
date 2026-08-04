// Package testutil implements helper functions for more convenient testing
// and may be imported in '_test.go' files only.
// Usage of some of these functions would have to be replaced by the ones
// provided by the official testing toolkit packages, should they decide
// to include helper functions of similar logic and kind.
package testutil

import (
	"bufio"
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertEqualError uses testify's assert package to check if errors
// are equal or, if assert.AnError is expected, whether an error exists
// or not.
func AssertEqualError(t *testing.T, exp, err error) {
	t.Helper()

	if exp != nil {
		if exp == assert.AnError { //nolint:goerr113,errorlint // direct check is needed
			assert.Error(t, err)
			return
		}

		assert.Equal(t, exp, err)

		return
	}

	assert.NoError(t, err)
}

// RequireEqualError uses testify's require package to check if errors
// are equal or, if assert.AnError is expected, whether an error exists
// or not.
func RequireEqualError(t *testing.T, exp, err error) {
	t.Helper()

	if exp != nil {
		if exp == assert.AnError { //nolint:goerr113,errorlint // direct check is needed
			require.Error(t, err)
			return
		}

		require.Equal(t, exp, err)

		return
	}

	require.NoError(t, err)
}

// NewBuffer creates a fresh instance of concurrent writer and its buffer.
func NewBuffer() (*Writer, *bytes.Buffer) {
	b := &bytes.Buffer{}
	out := bufio.NewWriter(b)

	return &Writer{out: out}, b
}

// Writer wraps the underlying bufio.Writer to allow concurrently safe
// access.
type Writer struct {
	out *bufio.Writer
	mu  sync.Mutex
}

// Write writes the contents of p into the buffer.
func (w *Writer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.out.Write(p)
}

// Flush writes any buffered data to the underlying io.Writer.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.out.Flush()
}

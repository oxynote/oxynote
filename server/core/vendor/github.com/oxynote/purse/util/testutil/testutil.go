// Package testutil implements helper functions for more convenient testing
// and may be imported in '_test.go' files only.
// Usage of some of these functions would have to be replaced by the ones
// provided by the official testing toolkit packages, should they decide
// to include helper functions of similar logic and kind.
package testutil

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jarcoal/httpmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _rand = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // this is used only for testing

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

// FilterEqual asserts that two objects are equal.
// All ignored types found on any of the provided objects will not be compared.
func FilterEqual(v1, v2 any, ignoreTypes ...any) error {
	diff := cmp.Diff(v1, v2, cmpopts.IgnoreTypes(ignoreTypes...),
		cmp.Exporter(func(reflect.Type) bool { return true }))
	if diff != "" {
		return fmt.Errorf("not equal:\n%s", diff)
	}

	return nil
}

// AssertFilterEqual asserts that two objects are equal.
// All ignored types found on any of the provided objects will not be compared.
func AssertFilterEqual(t *testing.T, v1, v2 any, ignoreTypes ...any) {
	t.Helper()

	if err := FilterEqual(v1, v2, ignoreTypes...); err != nil {
		t.Error(err)
	}
}

// RequireFilterEqual asserts that two objects are equal.
// All ignored types found on any of the provided objects will not be compared.
func RequireFilterEqual(t *testing.T, v1, v2 any, ignoreTypes ...any) {
	t.Helper()

	if err := FilterEqual(v1, v2, ignoreTypes...); err != nil {
		t.Error(err)
		t.FailNow()
	}
}

// RandomDecimal returns a random decimal value.
func RandomDecimal() decimal.Decimal {
	return decimal.NewFromInt(_rand.Int63())
}

// MockHTTP returns mocked http environment.
func MockHTTP() (*http.Client, *httpmock.MockTransport) {
	t := httpmock.NewMockTransport()
	return &http.Client{Transport: t}, t
}

// QueryResponder asserts the required query values are present in the
// request.
func QueryResponder(t *testing.T, resp httpmock.Responder, q url.Values) httpmock.Responder {
	return func(r *http.Request) (*http.Response, error) {
		for k, v := range q {
			// the value might be an array, so we need to check
			// all values
			if len(v) != 0 && v[0] != "" {
				assert.ElementsMatch(t, v, r.URL.Query()[k], "key: %s", k)
				continue
			}

			// if it's first value is zero or there are no
			// values, only check if the key exists
			assert.NotZero(t, r.URL.Query().Get(k))
		}

		return resp(r)
	}
}

// NewBuffer creates a fresh instance of concurrent writer and its buffer.
func NewBuffer() (*Writer, *bytes.Buffer) {
	b := &bytes.Buffer{}
	out := bufio.NewWriter(b)

	return NewWriter(out), b
}

// Writer wraps the underlying bufio.Writer to allow concurrently safe
// access.
type Writer struct {
	out *bufio.Writer
	mu  sync.Mutex
}

// NewWriter creates a fresh instance of concurrent writer.
func NewWriter(out *bufio.Writer) *Writer {
	return &Writer{out: out}
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

// AddChiCtx adds the provided key and value into chi's context namespace.
func AddChiCtx(ctx context.Context, k, v string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(k, v)

	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

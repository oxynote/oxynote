package errutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_New(t *testing.T) {
	err := New(http.StatusBadRequest, "hello", "bad request %s", "123")
	require.IsType(t, &statusError{}, err) //nolint:testifylint // a direct type check is needed
	sErr, _ := err.(*statusError)          //nolint:errorlint // we need a direct check
	assert.NoError(t, sErr.err)
	assert.Equal(t, "bad request 123", sErr.Message)
	assert.Equal(t, "hello", sErr.InternalCode)
	assert.Equal(t, http.StatusBadRequest, sErr.statusCode)
}

func Test_NewPlain(t *testing.T) {
	err := NewPlain(http.StatusBadRequest)
	require.IsType(t, &statusError{}, err) //nolint:testifylint // a direct type check is needed
	sErr, _ := err.(*statusError)          //nolint:errorlint // we need a direct check
	assert.NoError(t, sErr.err)
	assert.Equal(t, "bad request", sErr.Message)
	assert.Equal(t, http.StatusBadRequest, sErr.statusCode)
	assert.Equal(t, "general", sErr.InternalCode)
}

func Test_wrap(t *testing.T) {
	cc := map[string]struct {
		Err     error
		Msg     string
		Args    []any
		Message string
	}{
		"Message formatted with args": {
			Err:     assert.AnError,
			Msg:     "bad request %s",
			Args:    []any{"123"},
			Message: "bad request 123",
		},
		// without args the message is used verbatim, so a literal percent
		// sign is not treated as a format verb.
		"Message without args": {
			Msg:     "usage above 90% threshold",
			Message: "usage above 90% threshold",
		},
		"Empty message": {
			Err:     assert.AnError,
			Message: "bad request",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := wrap(c.Err, http.StatusBadRequest, "hello", c.Msg, c.Args...)
			require.IsType(t, &statusError{}, err) //nolint:testifylint // a direct type check is needed
			sErr, _ := err.(*statusError)          //nolint:errorlint // we need a direct check
			assert.Equal(t, c.Message, sErr.Message)
			assert.Equal(t, "hello", sErr.InternalCode)
			assert.Equal(t, http.StatusBadRequest, sErr.statusCode)
			assert.Equal(t, c.Err, sErr.err)
		})
	}
}

func Test_statusError_Error(t *testing.T) {
	cc := map[string]struct {
		Err    error
		Result string
	}{
		"Without wrapped error": {
			Result: "bad request",
		},
		"With wrapped error": {
			Err:    errors.New("invalid JSON body"),
			Result: "bad request: invalid JSON body",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sErr := statusError{
				statusCode:   400,
				Message:      "bad request",
				InternalCode: "test",
				err:          c.Err,
			}

			assert.Equal(t, c.Result, sErr.Error())
		})
	}
}

func Test_statusError_Unwrap(t *testing.T) {
	sErr := statusError{err: assert.AnError}
	assert.Equal(t, assert.AnError, sErr.Unwrap())
}

func Test_IsNotFound(t *testing.T) {
	cc := map[string]struct {
		Err    error
		Result bool
	}{
		"Nil error": {
			Err:    nil,
			Result: false,
		},
		"Other error": {
			Err:    assert.AnError,
			Result: false,
		},
		"No rows error": {
			Err:    sql.ErrNoRows,
			Result: true,
		},
		"Not found error": {
			Err:    ErrNotFound,
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, IsNotFound(c.Err))
		})
	}
}

func Test_Detect(t *testing.T) {
	cc := map[string]struct {
		Err      error
		Message  string
		Code     int
		Passthru bool
	}{
		"No rows found error": {
			Err:     sql.ErrNoRows,
			Message: strings.ToLower(http.StatusText(http.StatusNotFound)),
			Code:    http.StatusNotFound,
		},
		"No cookie error": {
			Err:     http.ErrNoCookie,
			Message: strings.ToLower(http.StatusText(http.StatusUnauthorized)),
			Code:    http.StatusUnauthorized,
		},
		"Context cancelled error": {
			Err:     context.Canceled,
			Message: strings.ToLower(http.StatusText(http.StatusGatewayTimeout)),
			Code:    http.StatusGatewayTimeout,
		},
		"Context deadline exceeded error": {
			Err:     context.DeadlineExceeded,
			Message: strings.ToLower(http.StatusText(http.StatusGatewayTimeout)),
			Code:    http.StatusGatewayTimeout,
		},
		"Status error": {
			Err:     wrap(errors.New("error"), http.StatusBadRequest, "test", "error"),
			Message: "error",
			Code:    http.StatusBadRequest,
		},
		"Wrapped status error": {
			Err:     fmt.Errorf("test: %w", New(http.StatusBadRequest, "test", "hello")),
			Message: "hello",
			Code:    http.StatusBadRequest,
		},
		"Wrapped internal status error": {
			Err:     fmt.Errorf("test: %w", NewPlain(http.StatusServiceUnavailable)),
			Message: strings.ToLower(http.StatusText(http.StatusServiceUnavailable)),
			Code:    http.StatusServiceUnavailable,
		},
		"Internal server status error": {
			Err: wrap(errors.New("error"), http.StatusInternalServerError, "test", "error"),
			Message: strings.ToLower(
				http.StatusText(http.StatusInternalServerError),
			),
			Code: http.StatusInternalServerError,
		},
		"Undefined error": {
			Err: errors.New("error"),
			Message: strings.ToLower(
				http.StatusText(http.StatusInternalServerError),
			),
			Code: http.StatusInternalServerError,
		},
		"Passthru": {
			Err: NewPlain(http.StatusConflict),
			Message: strings.ToLower(
				http.StatusText(http.StatusConflict),
			),
			Code:     http.StatusConflict,
			Passthru: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := Detect(c.Err, c.Passthru)
			require.IsType(t, &statusError{}, err) //nolint:testifylint // a direct type check is needed
			sErr, _ := err.(*statusError)          //nolint:errorlint // it's a test
			assert.Equal(t, c.Message, sErr.Message)
			assert.Equal(t, c.Code, sErr.statusCode)
		})
	}
}

func Test_StatusCode(t *testing.T) {
	cc := map[string]struct {
		Err    error
		Detect bool
		Result int
	}{
		"Undefined error": {
			Err:    errors.New("error"),
			Result: http.StatusInternalServerError,
		},
		"Status error": {
			Err:    New(400, "hello", "error"),
			Result: 400,
		},
		"Detected no rows error": {
			Err:    sql.ErrNoRows,
			Detect: true,
			Result: 404,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, StatusCode(c.Err, c.Detect))
		})
	}
}

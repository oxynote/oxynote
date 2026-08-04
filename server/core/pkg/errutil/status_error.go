// Package errutil provides helper functions and types to simplify and join
// error handling with http status code and message.
package errutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrNotFound is returned when the target resource is not found.
var ErrNotFound = NewPlain(http.StatusNotFound)

// statusError is a custom error type used to carry error
// message, custom code and HTTP status code.
type statusError struct {
	Message      string `json:"message"`
	InternalCode string `json:"code"` // json is shorter
	statusCode   int
	err          error
}

// New creates a new status error.
func New(statusCode int, internalCode, msg string, args ...any) error {
	return wrap(nil, statusCode, internalCode, msg, args...)
}

// NewPlain creates a new status error with status code as a message.
func NewPlain(statusCode int) error {
	return wrap(nil, statusCode, "general", "")
}

// wrap creates a new status error wrapping it around another error.
func wrap(err error, statusCode int, internalCode, msg string, args ...any) error {
	if msg == "" {
		msg = strings.ToLower(http.StatusText(statusCode))
	}

	return &statusError{
		Message:      fmt.Sprintf(msg, args...),
		InternalCode: internalCode,
		statusCode:   statusCode,
		err:          err,
	}
}

// Error converts the error to string.
func (e *statusError) Error() string {
	msg := e.Message
	if e.err != nil {
		return fmt.Sprintf("%s: %s", msg, e.err)
	}

	return msg
}

// Unwrap returns the wrapped error, if it exists.
func (e *statusError) Unwrap() error {
	return e.err
}

// IsNotFound is a special used to check if the error indicates
// that something is not found.
// It is useful in situation when working with a database
// whose error hooks cannot handle empty row list cases.
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound)
}

// Detect wraps the provided error with additional information
// useful for applications.
// The bool parameter determines whether the error should be returned as is
// if it doesn't match any error checks.
func Detect(err error, passthru bool) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case errors.Is(err, http.ErrNoCookie):
		return wrap(
			err,
			http.StatusUnauthorized,
			"account.not_authenticated",
			strings.ToLower(http.StatusText(http.StatusUnauthorized)),
		)
	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):

		return NewPlain(http.StatusGatewayTimeout)
	}

	if passthru {
		return err
	}

	var (
		serr       *statusError
		statusCode = http.StatusInternalServerError
	)

	if errors.As(err, &serr) {
		// ensure that we aren't leaking any critical information
		if serr.statusCode < 500 {
			return serr
		}

		statusCode = serr.statusCode
	}

	return wrap(err, statusCode, "general", strings.ToLower(http.StatusText(statusCode)))
}

// StatusCode returns a status code associated with the error.
func StatusCode(err error, detect bool) int {
	if detect {
		err = Detect(err, true)
	}

	var serr *statusError
	if !errors.As(err, &serr) {
		return http.StatusInternalServerError
	}

	return serr.statusCode
}

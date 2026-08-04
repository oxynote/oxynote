// Package errutil provides helper functions and types to simplify and join
// error handling with http status code and message.
package errutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// statusError is a custom error type used to carry error
// message, custom code and HTTP status code.
type statusError struct {
	Message      string `json:"message"`
	InternalCode string `json:"code"` // json is shorter
	statusCode   int
	err          error
}

// Wrap creates a new status error wrapping it around another error.
func Wrap(err error, statusCode int, internalCode, msg string, args ...any) error {
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

// Detect wraps the provided error with additional information
// useful for applications.
func Detect(err error) error {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {

		return Wrap(nil, http.StatusGatewayTimeout, "general", "")
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

	return Wrap(err, statusCode, "general", "")
}

// StatusCode returns a status code associated with the error.
func StatusCode(err error) int {
	var serr *statusError
	if !errors.As(err, &serr) {
		return http.StatusInternalServerError
	}

	return serr.statusCode
}

package processor

import (
	"errors"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// _jsonNullSize is the encoded size of a JSON "null" literal.
	_jsonNullSize = 4

	// _jsonBoolSize is the encoded size of the longest JSON boolean
	// literal ("false").
	_jsonBoolSize = 5

	// _jsonQuotesSize accounts for the quotes wrapping a JSON string.
	_jsonQuotesSize = 2

	// _jsonNumericSize is a rough encoded size of a JSON number.
	_jsonNumericSize = 8

	// _columnScanRows caps how far column classification looks for a value
	// that says what a column holds.
	_columnScanRows = 20
)

// _errClosed is returned when a closed processor is asked for a
// connection.
var _errClosed = errors.New("the data source processor is closed")

// _errRateLimited is returned when the data source limits the requests it
// answers and refused this one.
var _errRateLimited = errutil.New(
	http.StatusTooManyRequests,
	"data_source.rate_limited",
	"The data source is rate limiting requests. Try again shortly.",
)

// _errUnauthorized is returned when the data source refused the login.
var _errUnauthorized = errutil.New(
	http.StatusUnauthorized,
	"data_source.unauthorized",
	"Unauthorized access to the data source.",
)

// _errUnreachable is returned when the data source cannot be reached.
var _errUnreachable = errutil.New(
	http.StatusBadRequest,
	"data_source.unreachable",
	"The data source is unreachable.",
)

// IsConnectionFailure reports whether err says the data source was down or
// refused the login.
func IsConnectionFailure(err error) bool {
	return errors.Is(err, _errUnauthorized) || errors.Is(err, _errUnreachable)
}

// IsOutage reports whether err says the data source is down, refused the
// login or is rate limiting requests. A different query does not fix any
// of them.
func IsOutage(err error) bool {
	return IsConnectionFailure(err) || errors.Is(err, _errRateLimited)
}

// IsInvalidQuery reports whether err says the data source rejected the
// query.
func IsInvalidQuery(err error) bool {
	var target invalidQueryError

	return errors.As(err, &target)
}

// ConnectionStatus represents the status of a connection test.
type ConnectionStatus string

const (
	// ConnectionStatusSuccess indicates that the connection test was successful.
	ConnectionStatusSuccess ConnectionStatus = "success"

	// ConnectionStatusUnauthorized indicates that the connection test failed due to unauthorized access.
	ConnectionStatusUnauthorized ConnectionStatus = "unauthorized"

	// ConnectionStatusUnreachable indicates that the data source is unreachable.
	ConnectionStatusUnreachable ConnectionStatus = "unreachable"

	// ConnectionStatusVersionNotSupported indicates that the data source version is not supported.
	ConnectionStatusVersionNotSupported ConnectionStatus = "version_not_supported"

	// ConnectionStatusNotReadOnly indicates that the connection is not read-only.
	ConnectionStatusNotReadOnly ConnectionStatus = "not_read_only"

	// ConnectionStatusInvalidEncryptionKey indicates that the stored
	// credentials cannot be decrypted with the configured keys.
	ConnectionStatusInvalidEncryptionKey ConnectionStatus = "invalid_encryption_key"

	// ConnectionStatusInvalidCACertificate indicates that the credentials
	// carry a CA certificate that does not parse, or that the data source
	// connects without TLS for it to verify.
	ConnectionStatusInvalidCACertificate ConnectionStatus = "invalid_ca_certificate"
)

// Error returns an error corresponding to the ConnectionStatus.
func (cs ConnectionStatus) Error() error {
	switch cs {
	case ConnectionStatusUnauthorized:
		return _errUnauthorized
	case ConnectionStatusUnreachable:
		return _errUnreachable
	case ConnectionStatusVersionNotSupported:
		return errutil.New(http.StatusBadRequest, "data_source.version_not_supported", "The data source version is not supported.")
	case ConnectionStatusNotReadOnly:
		return errutil.New(http.StatusBadRequest, "data_source.not_read_only", "The data source connection must be read-only.")
	case ConnectionStatusInvalidEncryptionKey:
		return errutil.New(
			http.StatusBadRequest,
			"data_source.invalid_encryption_key",
			"The data source credentials cannot be decrypted with the configured keys and must be entered again.",
		)
	case ConnectionStatusInvalidCACertificate:
		return errutil.New(
			http.StatusBadRequest,
			"data_source.invalid_ca_certificate",
			"The CA certificate must be a PEM certificate, and the data source must connect over TLS.",
		)
	default:
		return nil
	}
}

// columnIsNumeric reports whether column i carries numbers, reading past the
// leading NULLs. Classifying on the first row alone turns a column whose first
// value happens to be NULL into a label, which drops the series from the chart
// even though every later row carries a number.
func columnIsNumeric(rows [][]any, i int, parse func(any) (float64, bool)) bool {
	for r := 0; r < len(rows) && r < _columnScanRows; r++ {
		if i >= len(rows[r]) || rows[r][i] == nil {
			continue
		}

		_, ok := parse(rows[r][i])

		return ok
	}

	return false
}

// Input represents the input required by a data source processor.
//
//go:generate ../../../scripts/codegen/mock -t internal Input
type Input interface {
	// URL returns the endpoint URL of the data source.
	URL() string

	// Credentials returns the credentials associated with the data source.
	Credentials() credentials.Credentials
}

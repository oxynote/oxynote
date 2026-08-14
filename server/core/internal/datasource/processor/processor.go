package processor

import (
	"fmt"
	"net/http"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
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
)

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
)

// Error returns an error corresponding to the ConnectionStatus.
func (cs ConnectionStatus) Error() error {
	switch cs {
	case ConnectionStatusUnauthorized:
		return errutil.New(http.StatusUnauthorized, "data_source.unauthorized", "Unauthorized access to the data source.")
	case ConnectionStatusUnreachable:
		return errutil.New(http.StatusBadRequest, "data_source.unreachable", "The data source is unreachable.")
	case ConnectionStatusVersionNotSupported:
		return errutil.New(http.StatusBadRequest, "data_source.version_not_supported", "The data source version is not supported.")
	case ConnectionStatusNotReadOnly:
		return errutil.New(http.StatusBadRequest, "data_source.not_read_only", "The data source connection must be read-only.")
	default:
		return nil
	}
}

// Input represents the input required by a data source processor.
//
//go:generate ../../../scripts/codegen/mock -t internal Input
type Input interface {
	// URL returns the endpoint URL of the data source.
	URL() string

	// Credentials returns the credentials associated with the data source.
	Credentials() Credentials
}

// Credentials represents the credentials of a data source in JSON format.
type Credentials []byte

// MarshalJSON returns the JSON representation of the credentials.
func (c *Credentials) MarshalJSON() ([]byte, error) {
	return []byte(*c), nil
}

// UnmarshalJSON sets the credentials from the given JSON data.
func (c *Credentials) UnmarshalJSON(data []byte) error {
	*c = data

	return nil
}

// Encrypt encrypts the credentials using the provided signing key.
func (c *Credentials) Encrypt(signingKey string) ([]byte, error) {
	state, err := cryptoutil.EncryptText(string(*c), []byte(signingKey))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	return []byte(state), nil
}

// Decrypt decrypts the given encrypted credentials using the provided signing key.
func (c *Credentials) Decrypt(signingKey string) error {
	decrypted, err := cryptoutil.DecryptText(string(*c), []byte(signingKey))
	if err != nil {
		return fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	*c = Credentials(decrypted)

	return nil
}

// CredentialsUpdateInput represents the input for updating credentials.
type CredentialsUpdateInput []byte

// UnmarshalJSON sets the credentials from the given JSON data.
func (cui *CredentialsUpdateInput) UnmarshalJSON(data []byte) error {
	*cui = data

	return nil
}

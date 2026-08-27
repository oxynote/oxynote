package processor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/guregu/null/v5"
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

	// ConnectionStatusInvalidSigningSecret indicates that the stored credentials
	// cannot be decrypted with the configured signing secret.
	ConnectionStatusInvalidSigningSecret ConnectionStatus = "invalid_signing_secret"
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
	case ConnectionStatusInvalidSigningSecret:
		return errutil.New(
			http.StatusBadRequest,
			"data_source.invalid_signing_secret",
			"The data source credentials cannot be decrypted and must be entered again.",
		)
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
type Credentials struct {
	// data is the credentials themselves, as JSON.
	data []byte

	// unreadable indicates that the stored credentials could not be
	// decrypted, which is what a signing secret rotated since they were
	// written leaves behind. Only Decrypt can decide it, so credentials
	// carrying it are the ones that came out of a failed read and no
	// others.
	unreadable bool
}

// NewCredentials creates a fresh instance of Credentials carrying the given
// JSON.
func NewCredentials(data []byte) Credentials {
	return Credentials{data: data}
}

// IsValid reports whether the credentials can be read.
//
// Only credentials Decrypt could not make sense of are invalid. Empty
// credentials are valid and ordinary: they say the data source authenticates
// through its URL, or not at all.
func (c *Credentials) IsValid() bool {
	return !c.unreadable
}

// MarshalJSON returns the JSON representation of the credentials. Empty
// credentials marshal as null: emitting nothing at all would make every
// enclosing struct fail to marshal.
func (c *Credentials) MarshalJSON() ([]byte, error) {
	if c == nil || len(c.data) == 0 {
		return []byte("null"), nil
	}

	return c.data, nil
}

// UnmarshalJSON sets the credentials from the given JSON data.
func (c *Credentials) UnmarshalJSON(data []byte) error {
	*c = Credentials{data: data}

	return nil
}

// Scan transforms a database entry into credentials.
func (c *Credentials) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		*c = Credentials{data: v}
	case string:
		*c = Credentials{data: []byte(v)}
	default:
		return errors.New("invalid credentials type")
	}

	return nil
}

// Encrypt encrypts the credentials using the provided signing key.
//
// Credentials that could not be read encrypt to nothing, so writing them back
// would replace the stored ciphertext — still readable once the right secret
// returns — with an encryption of the emptiness left in its place. They are
// refused instead.
func (c *Credentials) Encrypt(signingKey string) ([]byte, error) {
	if !c.IsValid() {
		return nil, errors.New("cannot encrypt credentials that could not be decrypted")
	}

	state, err := cryptoutil.EncryptText(string(c.data), []byte(signingKey))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	return []byte(state), nil
}

// Decrypt decrypts the stored credentials using the provided signing key.
//
// Credentials it cannot read are emptied and marked unreadable rather than
// left holding ciphertext nobody can use, so every later reader is told the
// same thing the error says here.
func (c *Credentials) Decrypt(signingKey string) error {
	decrypted, err := cryptoutil.DecryptText(string(c.data), []byte(signingKey))
	if err != nil {
		*c = Credentials{unreadable: true}

		return fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	*c = Credentials{data: []byte(decrypted)}

	return nil
}

// CredentialsUpdateInput represents the input for updating credentials.
type CredentialsUpdateInput []byte

// UnmarshalJSON sets the credentials from the given JSON data.
func (cui *CredentialsUpdateInput) UnmarshalJSON(data []byte) error {
	*cui = data

	return nil
}

// BasicCredentials represents the username and password every data source
// authenticates with.
type BasicCredentials struct {
	// Username is the username for the data source.
	Username string `json:"username"`

	// Password is the password for the data source.
	Password string `json:"password"`
}

// BasicCredentialsUpdate represents the input for updating credentials. An
// absent field leaves the stored value as it is.
type BasicCredentialsUpdate struct {
	// Username is the username for the data source.
	Username null.String `json:"username"`

	// Password is the password for the data source.
	Password null.String `json:"password"`
}

// UpdateBasicCredentials applies the update input to the stored credentials.
// Credentials emptied by the update are dropped altogether, so a data source
// falls back to whatever its URL carries.
func UpdateBasicCredentials(rawCreds Credentials, inp CredentialsUpdateInput) (Credentials, error) {
	var creds BasicCredentials

	// credentials that could not be read are no base to merge onto: what
	// they hold is ciphertext nobody can make sense of, so the update
	// supplies the whole pair rather than half of it.
	if rawCreds.IsValid() && len(rawCreds.data) > 0 {
		if err := json.Unmarshal(rawCreds.data, &creds); err != nil {
			return Credentials{}, fmt.Errorf("error unmarshaling credentials: %w", err)
		}
	}

	var update BasicCredentialsUpdate

	if err := json.Unmarshal(inp, &update); err != nil {
		return Credentials{}, fmt.Errorf("error unmarshaling credentials update input: %w", err)
	}

	if update.Username.Valid {
		creds.Username = update.Username.String
	}

	if update.Password.Valid {
		creds.Password = update.Password.String
	}

	if creds.Username == "" && creds.Password == "" {
		return Credentials{}, nil
	}

	data, err := json.Marshal(creds) //nolint:gosec // credentials are encrypted before storage
	if err != nil {
		return Credentials{}, fmt.Errorf("error marshaling updated credentials: %w", err)
	}

	return Credentials{data: data}, nil
}

// _columnScanRows caps how far column classification looks for a value that
// says what a column holds.
const _columnScanRows = 20

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

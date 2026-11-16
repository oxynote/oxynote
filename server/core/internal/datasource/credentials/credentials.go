// Package credentials holds the credentials a data source authenticates
// with: their kinds, how they are stored and how an update changes them.
package credentials

import (
	"bytes"
	"cmp"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// All available Kind constants.
const (
	// KindBasic authenticates with a username and password.
	KindBasic Kind = "basic"

	// KindToken authenticates with a token.
	KindToken Kind = "token"
)

var (
	// ErrInvalid is returned when the credentials have a kind the data
	// source does not accept.
	ErrInvalid = errutil.New(
		http.StatusBadRequest,
		"data_source.invalid_credentials",
		"The credentials are not valid for this data source.",
	)

	// ErrInvalidKind is returned when the credentials have no kind, or
	// one that does not exist.
	ErrInvalidKind = errutil.New(
		http.StatusBadRequest,
		"data_source.invalid_credentials",
		"The credentials have no known kind.",
	)

	// ErrInvalidCACertificate is returned when the credentials carry a CA
	// certificate that cannot verify anything: it does not parse, or the
	// data source connects without TLS.
	ErrInvalidCACertificate = errors.New("the ca certificate cannot be used")
)

// Kind says how a data source authenticates.
type Kind string

// Validate reports whether the kind is a known one.
func (k Kind) Validate() error {
	switch k {
	case KindBasic, KindToken:
		return nil
	default:
		return ErrInvalidKind
	}
}

// MarshalText returns the kind as text.
func (k Kind) MarshalText() ([]byte, error) {
	return []byte(k), nil
}

// UnmarshalText parses a kind, trimmed and lowercased. An unknown kind is
// kept, so Validate reports it with ErrInvalidKind and not as bad JSON.
func (k *Kind) UnmarshalText(text []byte) error {
	*k = Kind(strings.ToLower(strings.TrimSpace(string(text))))

	return nil
}

// Credentials represents the credentials of a data source in JSON format.
type Credentials struct {
	// data is the credentials themselves, as JSON.
	data []byte

	// unreadable indicates that the stored credentials could not be
	// decrypted, which is what a key dropped since they were written
	// leaves behind. Only Decrypt can decide it, so credentials
	// carrying it are the ones that came out of a failed read and no
	// others.
	unreadable bool
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
//
//nolint:revive // a value receiver would let a struct holding credentials by value emit the secret
func (c *Credentials) MarshalJSON() ([]byte, error) {
	if c == nil || len(c.data) == 0 {
		return []byte("null"), nil
	}

	return c.data, nil
}

// UnmarshalJSON sets the credentials from a copy of the given JSON data.
func (c *Credentials) UnmarshalJSON(data []byte) error {
	*c = Credentials{data: bytes.Clone(data)}

	return nil
}

// Scan transforms a database entry into credentials.
func (c *Credentials) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		// the driver owns the bytes and may reuse them for the next row.
		*c = Credentials{data: bytes.Clone(v)}
	case string:
		*c = Credentials{data: []byte(v)}
	default:
		return errors.New("invalid credentials type")
	}

	return nil
}

// Encrypt encrypts the credentials under the keyring's newest key, bound to
// the associated data, which is the owning row's id.
//
// Credentials that could not be read encrypt to nothing, so writing them back
// would replace the stored ciphertext — still readable once the right key
// returns — with an encryption of the emptiness left in its place. They are
// refused instead.
func (c *Credentials) Encrypt(keys *cryptoutil.Keyring, aad []byte) ([]byte, error) {
	if !c.IsValid() {
		return nil, errors.New("cannot encrypt credentials that could not be decrypted")
	}

	sealed, err := keys.Encrypt(c.data, aad)
	if err != nil {
		// NOCOV: crypto/rand failures cannot be simulated in tests.
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	return sealed, nil
}

// Decrypt decrypts the stored credentials with whichever key of the keyring
// sealed them, given the same associated data Encrypt was handed.
//
// Credentials it cannot read are emptied and marked unreadable rather than
// left holding ciphertext nobody can use, so every later reader is told the
// same thing the error says here.
func (c *Credentials) Decrypt(keys *cryptoutil.Keyring, aad []byte) error {
	decrypted, err := keys.Decrypt(c.data, aad)
	if err != nil {
		*c = Credentials{unreadable: true}

		return fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	*c = Credentials{data: decrypted}

	return nil
}

// Read returns the credentials as the kind they were stored with. It
// returns nil for empty credentials and for credentials no key decrypted.
func (c *Credentials) Read() (Auth, error) {
	kind, err := c.kind()
	if err != nil {
		return nil, err
	}

	switch kind {
	case KindBasic:
		var b Basic
		if err := json.Unmarshal(c.data, &b); err != nil {
			return nil, fmt.Errorf("unmarshaling basic credentials: %w", err)
		}

		return b, nil
	case KindToken:
		var t Token
		if err := json.Unmarshal(c.data, &t); err != nil {
			return nil, fmt.Errorf("unmarshaling token credentials: %w", err)
		}

		return t, nil
	}

	return nil, nil //nolint:nilnil // empty credentials hold no kind of credentials
}

// Basic returns the credentials of a data source that accepts only a
// username and password. Empty credentials hold neither.
func (c *Credentials) Basic() (Basic, error) {
	kind, err := c.kind()
	if err != nil {
		return Basic{}, err
	}

	switch kind {
	case "":
		return Basic{}, nil
	case KindBasic:
		var b Basic
		if err := json.Unmarshal(c.data, &b); err != nil {
			return Basic{}, fmt.Errorf("unmarshaling basic credentials: %w", err)
		}

		return b, nil
	default:
		return Basic{}, ErrInvalid
	}
}

// Public returns what may be shown of the credentials: their kind and
// which of them are set, never their values. Empty credentials show
// nothing, and so do credentials no key decrypted.
func (c *Credentials) Public() (Public, error) {
	auth, err := c.Read()
	if err != nil || auth == nil {
		return Public{}, err
	}

	return Public{
		Kind:       auth.Kind(),
		Configured: auth.Configured(),
	}, nil
}

// kind returns the kind the credentials were stored with. Empty
// credentials have none, and neither do credentials no key decrypted.
func (c *Credentials) kind() (Kind, error) {
	if len(c.data) == 0 {
		return "", nil
	}

	var header struct {
		// Kind says how the data source authenticates.
		Kind Kind `json:"kind"`
	}

	if err := json.Unmarshal(c.data, &header); err != nil {
		return "", fmt.Errorf("unmarshaling credentials: %w", err)
	}

	if err := header.Kind.Validate(); err != nil {
		return "", err
	}

	return header.Kind, nil
}

// Public describes stored credentials without their values.
type Public struct {
	// Kind specifies the kind of the credentials.
	Kind Kind `json:"kind"`

	// Configured specifies the credentials that are set.
	Configured map[string]bool `json:"configured"`
}

// UpdateInput represents the input for updating credentials. Its fields
// are those of the kind it names, and an absent field leaves the stored
// value as it is.
type UpdateInput []byte

// UnmarshalJSON sets the update input from a copy of the given JSON data.
func (ui *UpdateInput) UnmarshalJSON(data []byte) error {
	*ui = bytes.Clone(data)

	return nil
}

// Update applies the update input to the stored credentials. An input
// without a kind keeps the stored kind. A new kind keeps only the CA
// certificate, the one field all kinds share. Only the given kinds are
// accepted. Credentials left empty are dropped, so the data source falls
// back to its URL.
func Update(stored Credentials, inp UpdateInput, kinds ...Kind) (Credentials, error) {
	var header struct {
		// Kind says how the data source authenticates.
		Kind Kind `json:"kind"`
	}

	if err := json.Unmarshal(inp, &header); err != nil {
		return Credentials{}, fmt.Errorf("unmarshaling credentials update input: %w", err)
	}

	// credentials that cannot be read are no base to merge onto. The
	// update supplies the whole set instead.
	base := []byte("{}")

	storedKind, kerr := stored.kind()
	if kerr == nil && storedKind != "" {
		base = stored.data
	}

	kind := cmp.Or(header.Kind, storedKind)
	if err := kind.Validate(); err != nil {
		return Credentials{}, err
	}

	if !slices.Contains(kinds, kind) {
		return Credentials{}, ErrInvalid
	}

	var updated Auth

	switch kind {
	case KindBasic:
		var b Basic
		if err := json.Unmarshal(base, &b); err != nil {
			return Credentials{}, fmt.Errorf("unmarshaling stored basic credentials: %w", err)
		}

		if err := json.Unmarshal(inp, &b); err != nil {
			return Credentials{}, fmt.Errorf("unmarshaling basic credentials update input: %w", err)
		}

		updated = b
	case KindToken:
		var t Token
		if err := json.Unmarshal(base, &t); err != nil {
			return Credentials{}, fmt.Errorf("unmarshaling stored token credentials: %w", err)
		}

		if err := json.Unmarshal(inp, &t); err != nil {
			return Credentials{}, fmt.Errorf("unmarshaling token credentials update input: %w", err)
		}

		updated = t
	}

	if len(updated.Configured()) == 0 {
		return Credentials{}, nil
	}

	data, err := json.Marshal(updated)
	if err != nil {
		// NOCOV: the credentials hold only strings, which always marshal.
		return Credentials{}, fmt.Errorf("marshaling updated credentials: %w", err)
	}

	return Credentials{data: data}, nil
}

// Auth is the credentials of one kind.
type Auth interface {
	// Kind returns the kind of the credentials.
	Kind() Kind

	// Configured reports which credentials are set, never their values.
	Configured() map[string]bool

	// HasCACertificate reports whether the server is verified with a CA
	// certificate of its own.
	HasCACertificate() bool

	// RootCAs returns the pool holding the CA certificate.
	RootCAs() (*x509.CertPool, error)

	// Authorize wraps the round tripper so that each request carries the
	// credentials.
	Authorize(rt http.RoundTripper) http.RoundTripper
}

// Package datasource manages outbound data-source connections and queries.
package datasource

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _maxNameLength is the most characters a data source name holds, the
// same limit the web form sets.
const _maxNameLength = 128

var (
	// ErrInvalidName is returned when a data source is given a name that
	// is empty or only white space.
	ErrInvalidName = errutil.New(
		http.StatusBadRequest,
		"data_source.invalid_name",
		"data source name cannot be empty",
	)

	// ErrNameTooLong is returned when a data source name holds more than
	// _maxNameLength characters.
	ErrNameTooLong = errutil.New(
		http.StatusBadRequest,
		"data_source.name_too_long",
		"data source name cannot be longer than 128 characters",
	)

	// ErrInvalidType is returned when a data source has an unknown type.
	ErrInvalidType = errutil.New(
		http.StatusBadRequest,
		"data_source.invalid_type",
		"invalid data source type",
	)

	// ErrInvalidURL is returned when a data source URL is empty or does
	// not parse.
	ErrInvalidURL = errutil.New(
		http.StatusBadRequest,
		"data_source.invalid_url",
		"invalid data source url",
	)
)

const (
	// TypePrometheus represents a Prometheus data source.
	TypePrometheus Type = "prometheus"

	// TypePostgreSQL represents a PostgreSQL data source.
	TypePostgreSQL Type = "postgresql"

	// TypeSupabase represents a Supabase project, whose database is a
	// PostgreSQL one.
	TypeSupabase Type = "supabase"

	// TypeMariaDB represents a MariaDB data source.
	TypeMariaDB Type = "mariadb"

	// TypeMySQL represents a MySQL data source.
	TypeMySQL Type = "mysql"

	// TypeLoki represents a Loki data source.
	TypeLoki Type = "loki"
)

// Type represents the type of the data source.
type Type string

// Validate reports whether the type is a known one.
func (ty Type) Validate() error {
	switch ty {
	case TypePrometheus, TypePostgreSQL, TypeSupabase, TypeMariaDB, TypeMySQL, TypeLoki:
		return nil
	default:
		return ErrInvalidType
	}
}

// credentialsKinds returns the credential kinds the type accepts. An
// unknown type accepts none.
func (ty Type) credentialsKinds() []credentials.Kind {
	switch ty {
	case TypePrometheus, TypeLoki:
		return []credentials.Kind{credentials.KindBasic, credentials.KindToken}
	case TypePostgreSQL, TypeSupabase, TypeMariaDB, TypeMySQL:
		return []credentials.Kind{credentials.KindBasic}
	default:
		return nil
	}
}

// MarshalText returns the type as text.
func (ty Type) MarshalText() ([]byte, error) {
	return []byte(ty), nil
}

// UnmarshalText parses a type, trimmed and lowercased. An unknown type is
// kept, so Validate reports it with ErrInvalidType and not as bad JSON.
func (ty *Type) UnmarshalText(text []byte) error {
	*ty = Type(strings.ToLower(strings.TrimSpace(string(text))))

	return nil
}

// DataSource represents a generic data source.
type DataSource struct {
	// ID is the unique identifier for the data source.
	ID xid.ID `json:"id" db:"id"`

	// OrganizationID is the ID of the organization that owns the data source.
	OrganizationID string `json:"-" db:"fk_organization_id"`

	// Name is the name of the data source.
	Name string `json:"name" db:"name"`

	// Type indicates the type of the data source (e.g., Prometheus).
	Type Type `json:"type" db:"type"`

	// URL is the endpoint URL of the data source.
	URL string `json:"url" db:"url"`

	// Credentials holds the authentication credentials for the data source.
	Credentials credentials.Credentials `json:"-" db:"credentials"`

	// Status indicates the current connection status of the data source.
	Status processor.ConnectionStatus `json:"status" db:"status"`

	// CreatedAt is the timestamp when the data source was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp when the data source was last updated.
	UpdatedAt null.Time `json:"updatedAt" db:"updated_at"`
}

// NewDataSource creates a new instance of DataSource. The name is kept
// without the white space around it. It fails on an input Validate
// refuses.
func NewDataSource(ci CreateInput, organizationID string) (*DataSource, error) {
	ci = ci.sanitize()

	if err := ci.Validate(); err != nil {
		return nil, err
	}

	return &DataSource{
		ID:             xid.New(),
		OrganizationID: organizationID,
		Type:           ci.Type,
		Name:           ci.Name,
		URL:            ci.URL,
		Credentials:    ci.Credentials,
		Status:         processor.ConnectionStatusSuccess,
		CreatedAt:      timeutil.Now(),
	}, nil
}

// NewDemoDataSources creates the demo data sources a fresh organization
// starts with. They share one URL; each type picks what it answers as.
func NewDemoDataSources(organizationID string) ([]*DataSource, error) {
	inputs := []CreateInput{
		{
			Type: TypePrometheus,
			Name: "Demo Metrics",
			URL:  demo.URL,
		},
		{
			Type: TypeLoki,
			Name: "Demo Logs",
			URL:  demo.URL,
		},
	}

	dss := make([]*DataSource, 0, len(inputs))

	for _, ci := range inputs {
		ds, err := NewDataSource(ci, organizationID)
		if err != nil {
			// NOCOV: the demo inputs are constants that pass Validate.
			return nil, err
		}

		dss = append(dss, ds)
	}

	return dss, nil
}

// Info returns a subset of data source fields for external use.
func (ds DataSource) Info() Info {
	return Info{
		ID:   ds.ID,
		Name: ds.Name,
		Type: ds.Type,
	}
}

// ApplyUpdate returns the data source with the input's changes. A new
// name is kept without the white space around it. It fails on an input
// Validate refuses. The receiver is left untouched.
func (ds DataSource) ApplyUpdate(ui UpdateInput) (DataSource, error) {
	ui = ui.sanitize()

	if err := ui.Validate(); err != nil {
		return DataSource{}, err
	}

	if ui.Name.Valid {
		ds.Name = ui.Name.String
	}

	// credentials belong to the endpoint they were entered for. A new URL
	// starts without them, or with only those the update brings.
	if ui.URL.Valid && ui.URL.String != ds.URL {
		ds.URL = ui.URL.String
		ds.Credentials = credentials.Credentials{}
	}

	creds, err := ds.updatedCredentials(ui.Credentials)
	if err != nil {
		return DataSource{}, err
	}

	ds.Credentials = creds
	ds.UpdatedAt = null.TimeFrom(timeutil.Now())

	return ds, nil
}

// updatedCredentials returns the data source credentials with the update
// applied, or the current ones when the update carries none.
func (ds DataSource) updatedCredentials(inp null.Value[credentials.UpdateInput]) (credentials.Credentials, error) {
	if !inp.Valid {
		return ds.Credentials, nil
	}

	kinds := ds.Type.credentialsKinds()
	if kinds == nil {
		return credentials.Credentials{}, errors.New("invalid data source type")
	}

	return credentials.Update(ds.Credentials, inp.V, kinds...)
}

// Info is the subset of data source fields returned to the AI.
type Info struct {
	// ID is the unique identifier for the data source.
	ID xid.ID `json:"id"`

	// Name is the name of the data source.
	Name string `json:"name"`

	// Type indicates the type of the data source (e.g., Prometheus).
	Type Type `json:"type"`
}

// CreateInput represents the data required to create a new data source.
type CreateInput struct {
	// Type is the type of the hook.
	Type Type `json:"type"`

	// Name is the name of the data source.
	Name string `json:"name"`

	// URL is the endpoint URL of the data source.
	URL string `json:"url"`

	// Credentials holds the authentication credentials for the data source.
	Credentials credentials.Credentials `json:"credentials"`
}

// Validate checks that the type is a known one, that the name, once
// sanitized, is usable, that the URL, once sanitized, parses and that the
// credentials can be read. A connection test does not always read the
// credentials first: the demo data source passes without them.
func (ci CreateInput) Validate() error {
	if err := ci.Type.Validate(); err != nil {
		return err
	}

	sanitized := ci.sanitize()

	if err := validateName(sanitized.Name); err != nil {
		return err
	}

	if err := validateURL(sanitized.URL); err != nil {
		return err
	}

	auth, err := sanitized.Credentials.Read()
	if err != nil {
		return credentials.ErrInvalid
	}

	if auth != nil && !slices.Contains(sanitized.Type.credentialsKinds(), auth.Kind()) {
		return credentials.ErrInvalid
	}

	return nil
}

// sanitize returns the input with the white space around its name and
// URL removed.
func (ci CreateInput) sanitize() CreateInput {
	ci.Name = strings.TrimSpace(ci.Name)
	ci.URL = strings.TrimSpace(ci.URL)

	return ci
}

// UpdateInput represents the data required to update an existing freshness hook.
type UpdateInput struct {
	// Name is the name of the data source.
	Name null.String `json:"name"`

	// URL is the endpoint URL of the data source.
	URL null.String `json:"url"`

	// Credentials holds the authentication credentials for the data source.
	Credentials null.Value[credentials.UpdateInput] `json:"credentials"`
}

// Validate checks that a set name, once sanitized, is usable and that a
// set URL, once sanitized, parses.
func (ui UpdateInput) Validate() error {
	sanitized := ui.sanitize()

	if sanitized.Name.Valid {
		if err := validateName(sanitized.Name.String); err != nil {
			return err
		}
	}

	if sanitized.URL.Valid {
		if err := validateURL(sanitized.URL.String); err != nil {
			return err
		}
	}

	return nil
}

// sanitize returns the input with the white space around its set name and
// URL removed.
func (ui UpdateInput) sanitize() UpdateInput {
	if ui.Name.Valid {
		ui.Name = null.StringFrom(strings.TrimSpace(ui.Name.String))
	}

	if ui.URL.Valid {
		ui.URL = null.StringFrom(strings.TrimSpace(ui.URL.String))
	}

	return ui
}

// validateName reports whether a trimmed data source name is neither empty
// nor too long.
func validateName(name string) error {
	return strutil.ValidateName(name, _maxNameLength, ErrInvalidName, ErrNameTooLong)
}

// validateURL reports whether a trimmed data source URL is neither empty
// nor one url.Parse refuses.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return ErrInvalidURL
	}

	if _, err := url.Parse(rawURL); err != nil {
		return ErrInvalidURL
	}

	return nil
}

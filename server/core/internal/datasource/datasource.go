// Package datasource manages outbound data-source connections and queries.
package datasource

import (
	"errors"
	"net/http"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// ErrUnavailableDataSource is returned when the data source is unavailable.
var ErrUnavailableDataSource = errutil.New(http.StatusBadRequest, "data_source.unavailable", "The data source is currently unavailable.")

// Type represents the type of the data source.
type Type string

const (
	// TypePrometheus represents a Prometheus data source.
	TypePrometheus Type = "prometheus"

	// TypePostgreSQL represents a PostgreSQL data source.
	TypePostgreSQL Type = "postgresql"

	// TypeMariaDB represents a MariaDB data source.
	TypeMariaDB Type = "mariadb"

	// TypeMySQL represents a MySQL data source.
	TypeMySQL Type = "mysql"
)

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
	Credentials processor.Credentials `json:"-" db:"credentials"`

	// Status indicates the current connection status of the data source.
	Status processor.ConnectionStatus `json:"status" db:"status"`

	// CreatedAt is the timestamp when the data source was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp when the data source was last updated.
	UpdatedAt null.Time `json:"updatedAt" db:"updated_at"`
}

// NewDataSource creates a new instance of DataSource.
func NewDataSource(ci CreateInput, organizationID string) *DataSource {
	return &DataSource{
		ID:             xid.New(),
		OrganizationID: organizationID,
		Type:           ci.Type,
		Name:           ci.Name,
		URL:            ci.URL,
		Credentials:    ci.Credentials,
		Status:         processor.ConnectionStatusSuccess,
		CreatedAt:      timeutil.Now(),
	}
}

// Info returns a subset of data source fields for external use.
func (ds *DataSource) Info() Info {
	return Info{
		ID:   ds.ID,
		Name: ds.Name,
		Type: ds.Type,
	}
}

// ApplyUpdate applies the given update input to the data source.
func (ds *DataSource) ApplyUpdate(ui UpdateInput) error {
	if ui.Name.Valid {
		ds.Name = ui.Name.String
	}

	if ui.URL.Valid {
		ds.URL = ui.URL.String
	}

	err := ds.updateCredentials(ui.Credentials)
	if err != nil {
		return err
	}

	ds.UpdatedAt = null.TimeFrom(timeutil.Now())

	return nil
}

// updateCredentials updates the data source credentials.
func (ds *DataSource) updateCredentials(inp null.Value[processor.CredentialsUpdateInput]) error {
	if !inp.Valid {
		return nil
	}

	switch ds.Type {
	case TypePrometheus:
		creds, err := processor.UpdatePrometheusCredentials(ds.Credentials, inp.V)
		if err != nil {
			return err
		}

		ds.Credentials = creds
	case TypePostgreSQL:
		creds, err := processor.UpdatePostgreSQLCredentials(ds.Credentials, inp.V)
		if err != nil {
			return err
		}

		ds.Credentials = creds
	case TypeMariaDB, TypeMySQL:
		creds, err := processor.UpdateMySQLCredentials(ds.Credentials, inp.V)
		if err != nil {
			return err
		}

		ds.Credentials = creds
	default:
		return errors.New("invalid data source type")
	}

	return nil
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
	Credentials processor.Credentials `json:"credentials"`
}

// UpdateInput represents the data required to update an existing freshness hook.
type UpdateInput struct {
	// Name is the name of the data source.
	Name null.String `json:"name"`

	// URL is the endpoint URL of the data source.
	URL null.String `json:"url"`

	// Credentials holds the authentication credentials for the data source.
	Credentials null.Value[processor.CredentialsUpdateInput] `json:"credentials"`
}

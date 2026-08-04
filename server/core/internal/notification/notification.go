package notification

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// Query constants.
const (
	FilterReadEq = "read_eq"

	SortCreatedAt = "created_at"
)

// NotificationCore is a single notification.
type NotificationCore struct {
	// Code specifies a code of the notification.
	Code NotificationCode `json:"code" db:"code"`

	// Metadata specifies a collection of additional data associated
	// with the notification.
	Metadata Metadata `json:"metadata" db:"metadata"`
}

// Notification is a single notification.
type Notification struct {
	NotificationCore

	// ID is the unique identifier of the notification.
	ID xid.ID `json:"id" db:"id"`

	// UserID is the tenant identifier of the notification.
	UserID string `json:"userId" db:"fk_user_id"`

	// OrganizationID is the organization identifier of the notification.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// Read specifies whether the notification has been read.
	Read bool `json:"read" db:"read"`

	// CreatedAt is the creation time of the notification.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// newNotification creates a new notification.
func newNotification(organizationID, userID string, nc NotificationCore) *Notification {
	return &Notification{
		NotificationCore: nc,
		ID:               xid.New(),
		UserID:           userID,
		OrganizationID:   organizationID,
		CreatedAt:        timeutil.Now(),
	}
}

// Metadata is a collection of additional data associated with the notification.
type Metadata map[string]any

// Value transforms metadata type into a database entry.
func (md Metadata) Value() (driver.Value, error) {
	// NOCOV: error case cannot happen since the scheme
	// is already validated.
	return json.Marshal(md)
}

// Scan transforms a database entry into a metadata type.
func (md *Metadata) Scan(src any) error {
	var pv []byte

	switch v := src.(type) {
	case []byte:
		pv = v
	case string:
		pv = []byte(v)
	default:
		return errors.New("invalid metadata type")
	}

	data := &Metadata{}
	if err := json.Unmarshal(pv, data); err != nil {
		return err
	}

	*md = *data

	return nil
}

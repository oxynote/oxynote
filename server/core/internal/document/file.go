package document

import (
	"time"

	"github.com/oxynote/purse/util/timeutil"
	"github.com/rs/xid"
)

// Location represents the location of the file.
type Location string

const (
	// LocationDocument indicates that the file is stored in a document.
	LocationDocument Location = "document"

	// LocationComment indicates that the file is stored in a comment.
	LocationComment Location = "comment"
)

// Valid checks if the location is valid.
func (l Location) Valid() bool {
	switch l {
	case LocationDocument, LocationComment:
		return true
	default:
		return false
	}
}

// File represents a file attached to a document.
type File struct {
	// ID is the identifier of this file.
	ID string `json:"id" db:"id"`

	// Location indicates where the file is stored.
	Location Location `json:"location" db:"location"`

	// DocumentID is the identifier for the document this file belongs to.
	DocumentID xid.ID `json:"documentId" db:"fk_document_id"`

	// OrganizationID is the identifier for the organization this file belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// CreatedAt is the timestamp when the file was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// NewFile creates a new file instance.
func NewFile(id string, loc Location, documentID xid.ID, organizationID string) File {
	return File{
		ID:             id,
		Location:       loc,
		DocumentID:     documentID,
		OrganizationID: organizationID,
		CreatedAt:      timeutil.Now(),
	}
}

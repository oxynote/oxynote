// Package file defines files attached to documents and their storage records.
package file

import (
	"fmt"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _folderFormat is the storage folder format for a document's files.
const _folderFormat = "organizations/%s/documents/%s/files"

// Folder returns the storage folder holding the given document's files.
func Folder(organizationID string, documentID xid.ID) string {
	return fmt.Sprintf(_folderFormat, organizationID, documentID)
}

// Key returns the storage key of the given document's file. The key
// is the file id alone: a re-upload into the same block then overwrites
// its object in place, so no object is ever left without a row naming it.
func Key(organizationID string, documentID xid.ID, id string) string {
	return path.Join(Folder(organizationID, documentID), id)
}

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

	// StorageKey is the full object key of the file in the object storage.
	// It is stored rather than derived so that the object stays reachable
	// after the owning document or organization is gone.
	StorageKey string `json:"storageKey" db:"storage_key"`

	// DocumentID is the identifier for the document this file belongs to.
	// Null when the document has been deleted; the manager will clean up
	// such files.
	DocumentID null.Value[xid.ID] `json:"documentId" db:"fk_document_id"`

	// OrganizationID is the identifier for the organization this file
	// belongs to. Null when the organization has been deleted.
	OrganizationID null.String `json:"organizationId" db:"fk_organization_id"`

	// Name is the file name the upload carried, which the file is served
	// back under.
	Name string `json:"name" db:"name"`

	// Size is the size of the stored object in bytes.
	Size int64 `json:"size" db:"size"`

	// ContentType is the media type detected from the object's bytes at
	// upload time. It is what the file is served as: a client's own claim
	// about the type never reaches the row.
	ContentType string `json:"contentType" db:"content_type"`

	// CreatedAt is the timestamp when the file was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UnreferencedAt is the timestamp when the file was first observed
	// to be referenced by no document content, history entry or
	// comment. Null while the file is still referenced.
	UnreferencedAt null.Time `json:"unreferencedAt" db:"unreferenced_at"`
}

// NewFile creates a new file instance.
func NewFile(
	id string,
	loc Location,
	storageKey string,
	documentID xid.ID,
	organizationID string,
	name string,
	size int64,
	contentType string,
) File {
	return File{
		ID:             id,
		Location:       loc,
		StorageKey:     storageKey,
		DocumentID:     null.ValueFrom(documentID),
		OrganizationID: null.StringFrom(organizationID),
		Name:           name,
		Size:           size,
		ContentType:    contentType,
		CreatedAt:      timeutil.Now(),
	}
}

// Viewable reports whether a browser may render the file inline. The
// set is a fixed allowlist of types a browser shows rather than runs:
// HTML, SVG, XML and scripts are deliberately absent, since served inline
// they would execute under the API's origin.
func Viewable(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	switch mt {
	case "application/pdf", "image/png", "image/jpeg", "image/gif", "image/webp", "text/plain":
		return true
	}

	return strings.HasPrefix(mt, "video/") || strings.HasPrefix(mt, "audio/")
}

// Disposition returns the Content-Disposition header value the file is
// served with: inline for a viewable type, attachment for everything
// else, carrying the file's name either way. The name is formatted as a
// media type parameter, which quotes it and percent-encodes non-ASCII
// and control characters into an RFC 2231 filename*, so no name can
// break out of the header.
func (f File) Disposition() string {
	disposition := "attachment"

	if Viewable(f.ContentType) {
		disposition = "inline"
	}

	return mime.FormatMediaType(disposition, map[string]string{"filename": f.Name})
}

// Orphaned reports whether the file lost its owner, which happens when the
// document or the organization is deleted by a path that cascades around
// the file itself.
func (f File) Orphaned() bool {
	return !f.DocumentID.Valid || !f.OrganizationID.Valid
}

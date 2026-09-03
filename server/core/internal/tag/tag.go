// Package tag provides the domain types for the tags a document carries.
package tag

import (
	"net/http"
	"slices"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sliceutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

var (
	// ErrInvalidTagName is returned when a tag is created without a name.
	ErrInvalidTagName = errutil.New(http.StatusBadRequest, "tag.invalid_name", "tag name cannot be empty")

	// ErrInvalidTagColor is returned when a tag carries a colour the frontend
	// cannot render as a dot.
	ErrInvalidTagColor = errutil.New(http.StatusBadRequest, "tag.invalid_color", "tag colour must be a hex triplet")
)

// Tag represents a label a document can carry. A document may carry many
// tags and a tag may be carried by many documents.
type Tag struct {
	// ID is the unique identifier of the tag.
	ID xid.ID `json:"id" db:"id"`

	// OrganizationID is the organization the tag belongs to.
	OrganizationID string `json:"organizationId" db:"organization_id"`

	// TagName is the display name of the tag.
	TagName string `json:"tagName" db:"tag_name"`

	// Color is the tag's colour as a hex triplet, including the leading hash.
	Color string `json:"color" db:"color"`

	// SortIndex is the position of the tag among its organization's tags.
	SortIndex int `json:"sortIndex" db:"sort_index"`

	// CreatedAt is the moment the tag was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// CreatedBy is the user who created the tag.
	CreatedBy null.String `json:"createdBy" db:"created_by"`
}

// CreateInput is the input structure for creating a tag.
type CreateInput struct {
	// TagName is the display name of the tag.
	TagName string `json:"tagName"`

	// Color is the tag's colour as a hex triplet, including the leading hash.
	Color string `json:"color"`
}

// Validate reports whether the input can become a tag.
func (ci CreateInput) Validate() error {
	if ci.TagName == "" {
		return ErrInvalidTagName
	}

	if !isHexColorValid(ci.Color) {
		return ErrInvalidTagColor
	}

	return nil
}

// isHexColorValid reports whether the value is a "#rrggbb" hex triplet.
func isHexColorValid(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}

	for _, c := range color[1:] {
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'

		if !isDigit && !isLower && !isUpper {
			return false
		}
	}

	return true
}

// NewTag creates a fresh instance of Tag from the given input.
func NewTag(inp CreateInput, organizationID, userID string) Tag {
	return Tag{
		ID:             xid.New(),
		OrganizationID: organizationID,
		TagName:        inp.TagName,
		Color:          inp.Color,
		CreatedAt:      timeutil.Now(),
		CreatedBy:      null.StringFrom(userID),
	}
}

// VisibilityInput is the input for changing whether one user sees a tag.
type VisibilityInput struct {
	// Hidden keeps the tag out of that user's sidebar without deleting it.
	Hidden bool `json:"hidden"`
}

// SwapInput is the input for moving a tag to another position.
type SwapInput struct {
	// ID is the identifier of the tag to swap.
	ID xid.ID `json:"id"`

	// SortIndex is the index the tag should end up at.
	SortIndex int `json:"sortIndex"`
}

// AssignInput is the input for attaching a tag to a document.
type AssignInput struct {
	// TagID is the identifier of the tag to attach.
	TagID xid.ID `json:"tagId"`
}

// Summaries is a collection of Summary objects.
type Summaries []Summary

// Swap returns a copy of the Summaries slice with the summary moved to the
// given sort index. The receiver is left untouched.
func (ss Summaries) Swap(id xid.ID, sortIndex int) (Summaries, error) {
	if sortIndex < 0 || sortIndex >= len(ss) {
		return nil, errutil.New(
			http.StatusBadRequest,
			"tag_summary.invalid_sort_index",
			"sort index is out of range",
		)
	}

	index := slices.IndexFunc(ss, func(s Summary) bool {
		return s.ID == id
	})
	if index == -1 {
		return nil, errutil.ErrNotFound
	}

	nss := slices.Clone(ss)
	sliceutil.Move(nss, index, sortIndex)

	return nss, nil
}

// Summary represents a tag together with the documents carrying it.
type Summary struct {
	// ID is the unique identifier of the tag.
	ID xid.ID `json:"id" db:"id"`

	// TagName is the display name of the tag.
	TagName string `json:"tagName" db:"tag_name"`

	// Color is the tag's colour as a hex triplet, including the leading hash.
	Color string `json:"color" db:"color"`

	// Hidden reports whether the user who asked for the tree keeps this tag
	// out of their sidebar. It is a per-user preference, so two members of
	// the same organization see different values.
	Hidden bool `json:"hidden" db:"hidden"`

	// Documents are the documents carrying this tag, each with its own
	// subtree. A document carrying several tags appears under each of them.
	Documents document.Summaries `json:"documents,omitempty" db:"documents"`
}

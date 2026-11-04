// Package tag provides the domain types for the tags a document carries.
package tag

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sliceutil"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _maxTagNameLength is the most characters a tag name holds, since the
// whole tag tree is sent to every member of the organization.
const _maxTagNameLength = 64

var (
	// ErrInvalidTagName is returned when a tag is given a name that is
	// empty or only white space.
	ErrInvalidTagName = errutil.New(http.StatusBadRequest, "tag.invalid_name", "tag name cannot be empty")

	// ErrTagNameTooLong is returned when a tag name holds more than
	// _maxTagNameLength characters.
	ErrTagNameTooLong = errutil.New(http.StatusBadRequest, "tag.name_too_long", "tag name cannot be longer than 64 characters")

	// ErrInvalidTagColor is returned when a tag carries a colour outside
	// the palette.
	ErrInvalidTagColor = errutil.New(http.StatusBadRequest, "tag.invalid_color", "tag colour must be one of the palette")

	// ErrDuplicateTagName is returned when a tag is created under a name the
	// organization already uses.
	ErrDuplicateTagName = errutil.New(http.StatusConflict, "tag.duplicate_name", "tag name is already in use")

	// ErrEmptyTagUpdate is returned when a tag update sets neither a name nor
	// a colour.
	ErrEmptyTagUpdate = errutil.New(http.StatusBadRequest, "tag.empty_update", "tag update needs a name or a colour")
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

	// Color is the tag's colour: a palette name, such as "green".
	Color string `json:"color" db:"color"`

	// SortIndex is the position of the tag among its organization's tags.
	SortIndex int `json:"sortIndex" db:"sort_index"`

	// CreatedAt is the moment the tag was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// CreatedBy is the user who created the tag.
	CreatedBy null.String `json:"createdBy" db:"created_by"`
}

// NewTag creates a fresh instance of Tag from the given input. The name is
// kept without the white space around it. It fails on an input Validate
// refuses.
func NewTag(inp CreateInput, organizationID, userID string) (Tag, error) {
	inp = inp.sanitize()

	if err := inp.Validate(); err != nil {
		return Tag{}, err
	}

	return Tag{
		ID:             xid.New(),
		OrganizationID: organizationID,
		TagName:        inp.TagName,
		Color:          inp.Color,
		CreatedAt:      timeutil.Now(),
		CreatedBy:      null.StringFrom(userID),
	}, nil
}

// Summary returns the tag on its own: the identity, name and colour,
// without the per-user hidden flag or the documents carrying it.
func (t Tag) Summary() Summary {
	return Summary{ID: t.ID, TagName: t.TagName, Color: t.Color}
}

// ApplyUpdate returns the tag renamed and/or recoloured as the input
// describes. A new name is kept without the white space around it. It
// fails on an input Validate refuses. The receiver is left untouched.
func (t Tag) ApplyUpdate(inp UpdateInput) (Tag, error) {
	inp = inp.sanitize()

	if err := inp.Validate(); err != nil {
		return Tag{}, err
	}

	if inp.TagName.Valid {
		t.TagName = inp.TagName.String
	}

	if inp.Color.Valid {
		t.Color = inp.Color.String
	}

	return t, nil
}

// CreateInput is the input structure for creating a tag.
type CreateInput struct {
	// TagName is the display name of the tag.
	TagName string `json:"tagName"`

	// Color is the tag's colour: a palette name, such as "green".
	Color string `json:"color"`
}

// Validate reports whether the input, once sanitized, can become a tag.
func (ci CreateInput) Validate() error {
	sanitized := ci.sanitize()

	if err := validateTagName(sanitized.TagName); err != nil {
		return err
	}

	if !ValidColor(sanitized.Color) {
		return ErrInvalidTagColor
	}

	return nil
}

// sanitize returns the input with the white space around its name
// removed.
func (ci CreateInput) sanitize() CreateInput {
	ci.TagName = strings.TrimSpace(ci.TagName)

	return ci
}

// UpdateInput is the input for renaming or recolouring a tag. A field left
// unset keeps its current value.
type UpdateInput struct {
	// TagName is the new display name of the tag.
	TagName null.String `json:"tagName"`

	// Color is the new colour of the tag: a palette name.
	Color null.String `json:"color"`
}

// Validate reports whether the input, once sanitized, changes a tag: at
// least one field has to be set, and a set field is held to the same rules
// as on creation.
func (ui UpdateInput) Validate() error {
	sanitized := ui.sanitize()

	if !sanitized.TagName.Valid && !sanitized.Color.Valid {
		return ErrEmptyTagUpdate
	}

	if sanitized.TagName.Valid {
		if err := validateTagName(sanitized.TagName.String); err != nil {
			return err
		}
	}

	if sanitized.Color.Valid && !ValidColor(sanitized.Color.String) {
		return ErrInvalidTagColor
	}

	return nil
}

// sanitize returns the input with the white space around its name
// removed.
func (ui UpdateInput) sanitize() UpdateInput {
	if ui.TagName.Valid {
		ui.TagName = null.StringFrom(strings.TrimSpace(ui.TagName.String))
	}

	return ui
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

// AssignInput is the input for attaching a tag to a document branch.
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

	// Color is the tag's colour: a palette name, such as "green".
	Color string `json:"color" db:"color"`

	// Hidden reports whether the user who asked for the tree keeps this tag
	// out of their sidebar. It is a per-user preference, so two members of
	// the same organization see different values.
	Hidden bool `json:"hidden" db:"hidden"`

	// Documents are the documents carrying this tag, each with its own
	// subtree. A document carrying several tags appears under each of them.
	Documents document.Summaries `json:"documents,omitempty" db:"documents"`
}

// validateTagName reports whether a trimmed tag name is neither empty nor
// too long.
func validateTagName(name string) error {
	return strutil.ValidateName(name, _maxTagNameLength, ErrInvalidTagName, ErrTagNameTooLong)
}

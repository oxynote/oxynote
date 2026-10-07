package search

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// All available ViewFrom constants.
const (
	// ViewFromAll records every opening of a branch, whatever led to it.
	ViewFromAll ViewFrom = "all"

	// ViewFromSearch records an opening of a branch from search results.
	ViewFromSearch ViewFrom = "search"
)

// ErrInvalidViewFrom is returned when a view names an unknown origin.
var ErrInvalidViewFrom = errutil.New(http.StatusBadRequest, "document.invalid_view_from", "invalid view origin")

// ViewFrom names what led the user to open a branch.
type ViewFrom string

// Validate reports whether the origin is a known one.
func (vf ViewFrom) Validate() error {
	switch vf {
	case ViewFromAll, ViewFromSearch:
		return nil
	default:
		return ErrInvalidViewFrom
	}
}

// MarshalText returns the origin as text.
func (vf ViewFrom) MarshalText() ([]byte, error) {
	return []byte(vf), nil
}

// UnmarshalText parses an origin, refusing an unknown one so a bad value
// is reported where it is decoded.
func (vf *ViewFrom) UnmarshalText(text []byte) error {
	v := ViewFrom(strings.ToLower(strings.TrimSpace(string(text))))
	if err := v.Validate(); err != nil {
		return fmt.Errorf("view origin must be one of %s, %s, got %q", ViewFromAll, ViewFromSearch, text)
	}

	*vf = v

	return nil
}

// RecentDocument is a document branch the user viewed, with the time of
// the latest view.
type RecentDocument struct {
	document.Document

	// ViewedAt specifies the time of the latest view.
	ViewedAt time.Time `json:"viewedAt" db:"viewed_at"`
}

// RecentResponse is the recent documents endpoint's answer.
type RecentResponse struct {
	// Results specifies the document branches, most recently viewed
	// first.
	Results []RecentResult `json:"results"`
}

// NewRecentResponse describes the recently viewed document branches.
func NewRecentResponse(docs []RecentDocument) RecentResponse {
	res := RecentResponse{
		Results: make([]RecentResult, 0, len(docs)),
	}

	for _, doc := range docs {
		res.Results = append(res.Results, RecentResult{
			ResultDocument: newResultDocument(doc.Document),
			ViewedAt:       doc.ViewedAt,
		})
	}

	return res
}

// RecentResult is one document branch the user viewed.
type RecentResult struct {
	ResultDocument

	// ViewedAt specifies the time of the latest view.
	ViewedAt time.Time `json:"viewedAt"`
}

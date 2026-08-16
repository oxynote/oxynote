package document

import (
	"net/http"
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errcode"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sliceutil"
	"github.com/rs/xid"
)

// Summaries is a collection of Summary objects.
type Summaries []Summary

// Swap returns a copy of the Summaries slice with the summary moved to the
// given sort index. The receiver is left untouched.
func (ss Summaries) Swap(id xid.ID, sortIndex int) (Summaries, error) {
	if sortIndex < 0 || sortIndex >= len(ss) {
		return nil, errutil.New(
			http.StatusBadRequest,
			errcode.DocumentSummaryInvalidSortIndex,
			"sort index is out of range",
		)
	}

	index := slices.IndexFunc(ss, func(d Summary) bool {
		return d.ID == id
	})
	if index == -1 {
		return nil, errutil.ErrNotFound
	}

	nss := slices.Clone(ss)
	sliceutil.Move(nss, index, sortIndex)

	return nss, nil
}

// Remove returns a copy of the Summaries slice without the summary carrying
// the given id. The receiver is left untouched.
func (ss Summaries) Remove(id xid.ID) (Summaries, error) {
	index := slices.IndexFunc(ss, func(d Summary) bool {
		return d.ID == id
	})
	if index == -1 {
		return nil, errutil.ErrNotFound
	}

	return slices.Delete(slices.Clone(ss), index, index+1), nil
}

// Summary represents summary of the document.
type Summary struct {
	// ID is the unique identifier for the document.
	ID xid.ID `json:"id" db:"id"`

	// DocumentName is the name of the document.
	DocumentName string `json:"documentName" db:"document_name"`

	// Icon is the icon associated with the document.
	Icon string `json:"icon" db:"icon"`

	// Protected indicates whether the document is protected.
	Protected bool `json:"protected" db:"protected"`

	// Children are the child elements of this document.
	Children Summaries `json:"children,omitempty" db:"children"`
}

// SwapInput is the input tree for swapping documents.
type SwapInput struct {
	// ID is the identifier of the document to swap.
	ID xid.ID `json:"id"`

	// ParentID is the identifier of the expected parent document.
	ParentID null.Value[xid.ID] `json:"parentId"`

	// SortIndex is the index used to sort documents within the parent.
	SortIndex int `json:"sortIndex"`
}

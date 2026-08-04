package httpserver

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// _filterPrefix is a prefix that is used to group HTTP query
	// filters together.
	_filterPrefix = "filter-"

	// _sortPrefix is a prefix that is used to group HTTP query
	// sort keys together.
	_sortPrefix = "sort-"
)

var (
	// ErrInvalidFilterKey is returned when filter key is
	// determined to be invalid.
	ErrInvalidFilterKey = errutil.New(http.StatusBadRequest, "request.invalid_filter_key", "invalid filter key")

	// ErrInvalidFilterValue is returned when filter value is
	// determined to be invalid.
	ErrInvalidFilterValue = errutil.New(http.StatusBadRequest, "request.invalid_filter_value", "invalid filter value")

	// ErrInvalidSortKey is returned when sort key is
	// determined to be invalid.
	ErrInvalidSortKey = errutil.New(http.StatusBadRequest, "request.invalid_sort_key", "invalid sort key")
)

// Query is used to filter and retrieve bulk data from
// data stores.
type Query struct {
	// Limit specifies the total amount of data elements per page.
	Limit uint64 `schema:"limit" json:"limit"`

	// Page specifies data batch number.
	Page uint64 `schema:"page" json:"page"`

	// Filters specifies a map of fields and their expected values (can
	// be partial).
	// Filter's key must start with the "filter-" prefix in the request's
	// query parameters to be put in this map.
	Filters map[string]string `schema:"-" json:"filters,omitempty"`

	// SortKeys specifies a slice of fields that should be used
	// for sorting.
	// Sorting key must start with the "sort-" prefix in the request's
	// query parameters to be put in this slice.
	// NOTE: the order of keys is important.
	SortKeys []SortKey `schema:"-" json:"sortKeys,omitempty"`

	// Omissions specifies a list of keys that should be
	// omitted from the response.
	Omissions map[string]struct{} `schema:"-" json:"omissions,omitempty"`
}

// SortKey is used to sort data by a specific key.
type SortKey struct {
	// Key specifies the name of the field by which elements should
	// be sorted.
	Key string `json:"key"`

	// Asc specifies whether ascending sorting order should be used.
	Asc bool `json:"asc"`
}

// Validate checks whether query data is valid or not.
// Since filters and sort keys may be dynamic, they should be validated
// on the outside.
func (q Query) Validate() error {
	if q.Limit < 1 {
		return errutil.New(http.StatusBadRequest, "request.invalid_query_limit", "invalid limit")
	}

	if q.Page < 1 {
		return errutil.New(http.StatusBadRequest, "request.invalid_query_page", "invalid page")
	}

	return nil
}

// ParseQuery parses request's query parameters into query structure.
//
//nolint:gocognit // sorting key parsing requires additional checks.
func ParseQuery(r *http.Request) (Query, error) {
	var qr Query

	if err := DecodeForm(r, &qr); err != nil {
		return Query{}, err
	}

	// to avoid duplicates, we first need to put sort keys into a map
	var (
		sortKeys     map[uint64]SortKey
		sortPriority []uint64 // to maintain proper order
	)

	for k := range r.Form {
		switch {
		case strings.HasPrefix(k, _filterPrefix):
			nk := strings.TrimPrefix(k, _filterPrefix)
			if nk == "" {
				continue
			}

			v := r.Form.Get(k)
			if v == "" {
				return Query{}, ErrInvalidFilterValue
			}

			if qr.Filters == nil {
				qr.Filters = make(map[string]string)
			}

			qr.Filters[nk] = v
		case strings.HasPrefix(k, _sortPrefix):
			// some sort keys might contain more than three
			// dashes, but we care only about the first two
			ss := strings.SplitN(k, "-", 3)
			if len(ss) != 3 {
				continue
			}

			priority, err := strconv.ParseUint(ss[1], 10, 64)
			if err != nil {
				return Query{}, errutil.New(
					http.StatusBadRequest,
					"request.invalid_sort_index",
					"invalid sort index",
				)
			}

			var asc bool

			switch r.Form.Get(k) {
			case "asc":
				asc = true
			case "desc":
				// asc is false already
			default:
				return Query{}, errutil.New(
					http.StatusBadRequest,
					"request.invalid_sort_direction",
					"invalid sort direction",
				)
			}

			// check for duplicates
			for ski, sk := range sortKeys {
				switch {
				case ski == priority:
					return Query{}, errutil.New(
						http.StatusBadRequest,
						"request.duplicate_sort_priority",
						"duplicate sort priority",
					)
				case sk.Key == ss[2]:
					return Query{}, errutil.New(
						http.StatusBadRequest,
						"request.duplicate_sort_key",
						"duplicate sort key",
					)
				}
			}

			if sortKeys == nil {
				sortKeys = make(map[uint64]SortKey)
			}

			sortKeys[priority] = SortKey{
				Key: ss[2],
				Asc: asc,
			}

			sortPriority = append(sortPriority, priority)
		case k == "omissions":
			vv := r.Form[k]
			for i := range vv {
				if qr.Omissions == nil {
					qr.Omissions = make(map[string]struct{}, len(vv))
				}

				qr.Omissions[vv[i]] = struct{}{}
			}
		}
	}

	sort.Slice(sortPriority, func(i, j int) bool {
		return sortPriority[i] < sortPriority[j]
	})

	if sortKeys != nil {
		qr.SortKeys = make([]SortKey, 0, len(sortKeys))
	}

	for _, priority := range sortPriority {
		qr.SortKeys = append(qr.SortKeys, sortKeys[priority])
	}

	return qr, nil
}

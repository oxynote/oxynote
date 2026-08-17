// Package errcode centralizes machine-readable error codes.
package errcode

const (
	// AccountNotAuthenticated is returned when the request carries no
	// usable session.
	AccountNotAuthenticated = "account.not_authenticated"
)

const (
	// DataSourceQueryRequired is returned when the query parameter carrying
	// the data source query is missing.
	DataSourceQueryRequired = "query.required"
)

const (
	// DocumentSummaryInvalidSortIndex is returned when the sort index is invalid.
	DocumentSummaryInvalidSortIndex = "document_summary.invalid_sort_index"
)

package datasource

import (
	"context"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// ErrTypeNotSupported is returned when a data source speaks something
// the generic query cannot run.
var ErrTypeNotSupported = errutil.New(http.StatusBadRequest, "data_source.type_not_supported", "Generic query is not supported for this data source type.")

// Query runs q against the data source the runner operates and returns
// the result in the unified shape, whatever the source speaks. A query
// that answered nothing is normalised to QueryStatusNoData here, so no
// caller has to tell a nil result from an empty one.
func Query(
	ctx context.Context,
	r Runner,
	q string,
	tr processor.TimeRange,
	ct processor.ChartType,
) (*processor.QueryResult, error) {
	switch r.Type() {
	case TypePrometheus:
		client, err := r.Prometheus(ctx)
		if err != nil {
			return nil, err
		}

		res, err := client.QueryRange(ctx, q, tr)
		if err != nil {
			return nil, err
		}

		if res == nil {
			return noData(), nil
		}

		return res.Transform(ct), nil
	case TypePostgreSQL, TypeSupabase:
		client, err := r.PostgreSQL(ctx)
		if err != nil {
			return nil, err
		}

		res, err := client.Query(ctx, q, tr)
		if err != nil {
			return nil, err
		}

		if res == nil {
			return noData(), nil
		}

		return res.Transform(ct), nil
	case TypeMariaDB, TypeMySQL:
		client, err := r.MySQL(ctx)
		if err != nil {
			return nil, err
		}

		res, err := client.Query(ctx, q, tr)
		if err != nil {
			return nil, err
		}

		if res == nil {
			return noData(), nil
		}

		return res.Transform(ct), nil
	default:
		return nil, ErrTypeNotSupported
	}
}

// noData is the result of a query the source answered nothing to.
func noData() *processor.QueryResult {
	return &processor.QueryResult{Status: processor.QueryStatusNoData}
}

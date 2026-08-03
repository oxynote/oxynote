package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oxynote/heimdall/internal/datasource"
	"github.com/oxynote/heimdall/internal/datasource/processor"
)

// PostgreSQLQuery executes a SQL query via the connector.
func (c *Client) PostgreSQLQuery(ctx context.Context, ds datasource.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PostgreSQLQueryResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/postgresql/query",
		NewPostgreSQLQueryRequest(
			datasource.NewRunner(ds),
			query,
			&tr,
		),
	)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Result) == 0 {
		return resp.Status, nil, nil
	}

	var result processor.PostgreSQLQueryResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding postgresql query result: %w", err)
	}

	return resp.Status, &result, nil
}

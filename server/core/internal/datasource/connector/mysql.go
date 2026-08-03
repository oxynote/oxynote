package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oxynote/heimdall/internal/datasource"
	"github.com/oxynote/heimdall/internal/datasource/processor"
)

// MySQLQuery executes a SQL query against a MySQL data source via the connector.
func (c *Client) MySQLQuery(ctx context.Context, ds datasource.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.MySQLQueryResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/mysql/query",
		NewMySQLQueryRequest(
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

	var result processor.MySQLQueryResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding mysql query result: %w", err)
	}

	return resp.Status, &result, nil
}

package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
)

// SQLQueryLabels executes a query with LIMIT 1 and returns string column names with example values.
func (c *Client) SQLQueryLabels(ctx context.Context, ds datasource.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, map[string]string, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/sql/query-labels",
		NewSQLQueryLabelsRequest(
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

	var result map[string]string

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding sql query labels result: %w", err)
	}

	return resp.Status, result, nil
}

// SQLMetadata fetches the metadata (tables and columns) from a SQL data source via the connector.
func (c *Client) SQLMetadata(ctx context.Context, ds datasource.DataSource) (processor.ConnectionStatus, *processor.SQLMetadataResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/sql/metadata",
		NewSQLMetadataRequest(
			datasource.NewRunner(ds),
		),
	)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Result) == 0 {
		return resp.Status, nil, nil
	}

	var result processor.SQLMetadataResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding sql metadata result: %w", err)
	}

	return resp.Status, &result, nil
}

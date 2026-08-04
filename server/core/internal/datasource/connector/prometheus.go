package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
)

// PrometheusQuery executes a Prometheus query via the connector.
func (c *Client) PrometheusQuery(ctx context.Context, ds datasource.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusQueryResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/prometheus/query",
		NewPrometheusQueryRequest(
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

	var result processor.PrometheusQueryResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding prometheus query result: %w", err)
	}

	return resp.Status, &result, nil
}

// PrometheusMetadata retrieves Prometheus metadata via the connector.
func (c *Client) PrometheusMetadata(ctx context.Context, ds datasource.DataSource) (processor.ConnectionStatus, *processor.PrometheusMetadataResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/prometheus/metadata",
		NewPrometheusMetadataRequest(
			datasource.NewRunner(ds),
		),
	)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Result) == 0 {
		return resp.Status, nil, nil
	}

	var result processor.PrometheusMetadataResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding prometheus metadata result: %w", err)
	}

	return resp.Status, &result, nil
}

// PrometheusLabelNames retrieves Prometheus label names via the connector.
func (c *Client) PrometheusLabelNames(ctx context.Context, ds datasource.DataSource, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelNamesResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/prometheus/labels",
		NewPrometheusLabelNamesRequest(
			datasource.NewRunner(ds),
			matchers,
			&tr,
		),
	)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Result) == 0 {
		return resp.Status, nil, nil
	}

	var result processor.PrometheusLabelNamesResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding prometheus label names result: %w", err)
	}

	return resp.Status, &result, nil
}

// PrometheusLabelValues retrieves Prometheus label values via the connector.
func (c *Client) PrometheusLabelValues(ctx context.Context, ds datasource.DataSource, label string, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelValuesResult, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/prometheus/labels/values",
		NewPrometheusLabelValuesRequest(
			datasource.NewRunner(ds),
			label,
			matchers,
			&tr,
		),
	)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Result) == 0 {
		return resp.Status, nil, nil
	}

	var result processor.PrometheusLabelValuesResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding prometheus label values result: %w", err)
	}

	return resp.Status, &result, nil
}

// PrometheusSeries retrieves Prometheus series via the connector.
func (c *Client) PrometheusSeries(ctx context.Context, ds datasource.DataSource, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusSeriesResult, error) {
	resp, err := c.sendRequest(
		ctx,
		"/api/data-sources/prometheus/series",
		NewPrometheusSeriesRequest(
			datasource.NewRunner(ds),
			matchers,
			&tr,
		),
	)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Result) == 0 {
		return resp.Status, nil, nil
	}

	var result processor.PrometheusSeriesResult

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", nil, fmt.Errorf("error decoding prometheus series result: %w", err)
	}

	return resp.Status, &result, nil
}

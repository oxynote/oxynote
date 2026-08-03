package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/guregu/null/v5"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

const (
	// _queryLimit defines the maximum number of data points to return in a query.
	_queryLimit = 1000

	// _metadataLimit defines the maximum number of metadata entries to return.
	_metadataLimit = "1000"

	// _labelLimit defines the maximum number of labels to return.
	_labelLimit = 100
)

// Prometheus represents a Prometheus data source processor.
type Prometheus struct {
	inp Input
}

// NewPrometheus creates a new Prometheus data source processor.
func NewPrometheus(inp Input) *Prometheus {
	return &Prometheus{
		inp: inp,
	}
}

// TestConnection tests the connection to the Prometheus data source.
func (p *Prometheus) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	client, err := createPrometheusClient(p.inp.URL(), p.inp.Credentials())
	if err != nil {
		return "", fmt.Errorf("error creating prometheus client: %w", err)
	}

	res, err := client.Buildinfo(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			return ConnectionStatusUnauthorized, nil
		}

		return ConnectionStatusUnreachable, nil
	}

	// TODO: Add a supported list of versions.
	if res.Version == "" {
		return ConnectionStatusVersionNotSupported, nil
	}

	return ConnectionStatusSuccess, nil
}

// Metadata retrieves metadata from the Prometheus data source.
func (p *Prometheus) Metadata(ctx context.Context) (*PrometheusMetadataResult, error) {
	client, err := createPrometheusClient(p.inp.URL(), p.inp.Credentials())
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	v, err := client.Metadata(ctx, "", _metadataLimit)
	if err != nil {
		return nil, fmt.Errorf("error fetching metadata: %w", err)
	}

	return &PrometheusMetadataResult{
		Result: v,
	}, nil
}

// QueryRange performs a query against the Prometheus data source.
func (p *Prometheus) QueryRange(ctx context.Context, q string, tr TimeRange) (*PrometheusQueryResult, error) {
	client, err := createPrometheusClient(p.inp.URL(), p.inp.Credentials())
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	ntr := tr.Normalize()

	v, warns, err := client.QueryRange(ctx, ntr.ProcessPrometheusQuery(q), v1.Range{
		Start: ntr.From,
		End:   ntr.To,
		Step:  ntr.QueryStep(),
	}, v1.WithLimit(_queryLimit))
	if err != nil {
		var perr *v1.Error

		if ok := errors.As(err, &perr); ok {
			return nil, NewInvalidQueryError(perr.Msg)
		}

		return nil, fmt.Errorf("error performing query: %w", err)
	}

	return &PrometheusQueryResult{
		Type:     v.Type(),
		Result:   v,
		Warnings: warns,
	}, nil
}

// LabelNames retrieves all label names from the Prometheus data source.
// The matchers parameter can be used to filter label names by series selectors.
func (p *Prometheus) LabelNames(ctx context.Context, matchers []string, tr TimeRange) (*PrometheusLabelNamesResult, error) {
	client, err := createPrometheusClient(p.inp.URL(), p.inp.Credentials())
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	ntr := tr.Normalize()

	names, warns, err := client.LabelNames(
		ctx,
		matchers,
		ntr.From,
		ntr.To,
		v1.WithLimit(_labelLimit),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching label names: %w", err)
	}

	return &PrometheusLabelNamesResult{
		Warnings: warns,
		Result:   names,
	}, nil
}

// LabelValues retrieves all values for a specific label from the Prometheus data source.
// The matchers parameter can be used to filter label values by series selectors.
func (p *Prometheus) LabelValues(ctx context.Context, label string, matchers []string, tr TimeRange) (*PrometheusLabelValuesResult, error) {
	client, err := createPrometheusClient(p.inp.URL(), p.inp.Credentials())
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	ntr := tr.Normalize()

	values, warns, err := client.LabelValues(
		ctx,
		label,
		matchers,
		ntr.From,
		ntr.To,
		v1.WithLimit(_labelLimit),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching label values: %w", err)
	}

	result := make([]string, len(values))
	for i, v := range values {
		result[i] = string(v)
	}

	return &PrometheusLabelValuesResult{
		Warnings: warns,
		Result:   result,
	}, nil
}

// Series retrieves series from the Prometheus data source matching the given selectors.
// The matchers parameter specifies series selectors to filter the results.
func (p *Prometheus) Series(ctx context.Context, matchers []string, tr TimeRange) (*PrometheusSeriesResult, error) {
	client, err := createPrometheusClient(p.inp.URL(), p.inp.Credentials())
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	ntr := tr.Normalize()

	series, warns, err := client.Series(
		ctx,
		matchers,
		ntr.From,
		ntr.To,
		v1.WithLimit(_labelLimit),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching series: %w", err)
	}

	return &PrometheusSeriesResult{
		Warnings: warns,
		Result:   series,
	}, nil
}

// UpdatePrometheusCredentials updates the credentials for the Prometheus data source.
func UpdatePrometheusCredentials(rawCreds Credentials, inp CredentialsUpdateInput) (Credentials, error) {
	var creds PrometheusCredentials

	if rawCreds != nil {
		if err := json.Unmarshal(rawCreds, &creds); err != nil {
			return nil, fmt.Errorf("error unmarshaling credentials: %w", err)
		}
	}

	var update PrometheusCredentialsUpdate

	if err := json.Unmarshal(inp, &update); err != nil {
		return nil, fmt.Errorf("error unmarshaling credentials update input: %w", err)
	}

	if update.Username.Valid {
		creds.Username = update.Username.String
	}

	if update.Password.Valid {
		creds.Password = update.Password.String
	}

	if creds.Username == "" && creds.Password == "" {
		return nil, nil
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("error marshaling updated credentials: %w", err)
	}

	return data, nil
}

// createPrometheusClient creates a Prometheus API client using the provided URL and credentials.
func createPrometheusClient(url string, creds Credentials) (v1.API, error) {
	var roundTripper http.RoundTripper

	if creds != nil {
		var promCreds PrometheusCredentials

		if err := json.Unmarshal(creds, &promCreds); err != nil {
			return nil, fmt.Errorf("error unmarshaling credentials: %w", err)
		}

		roundTripper = config.NewBasicAuthRoundTripper(
			config.NewInlineSecret(promCreds.Username),
			config.NewInlineSecret(promCreds.Password),
			api.DefaultRoundTripper,
		)
	}

	client, err := api.NewClient(api.Config{
		Address:      url,
		RoundTripper: roundTripper,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	return v1.NewAPI(client), nil
}

// PrometheusCredentials represents the credentials for a Prometheus data source.
type PrometheusCredentials struct {
	// Username is the username for the Prometheus data source.
	Username string `json:"username"`

	// Password is the password for the Prometheus data source.
	Password string `json:"password"`
}

// PrometheusCredentialsUpdate represents the input for updating Prometheus credentials.
type PrometheusCredentialsUpdate struct {
	// Username is the username for the Prometheus data source.
	Username null.String `json:"username"`

	// Password is the password for the Prometheus data source.
	Password null.String `json:"password"`
}

// PrometheusMetadataResult represents the result of a metadata query.
type PrometheusMetadataResult struct {
	// Result contains the query result data.
	Result any `json:"result,omitempty"`
}

// PrometheusQueryResult represents the result of a data source query.
type PrometheusQueryResult struct {
	// Warnings contains any warnings generated during the query.
	Warnings []string `json:"warnings,omitempty"`

	// Type indicates the type of the query result.
	Type model.ValueType `json:"type"`

	// Result contains the query result data.
	Result any `json:"result,omitempty"`
}

// Transform transforms a PrometheusQueryResult into a unified QueryResult
// based on the requested chart type.
func (pqr *PrometheusQueryResult) Transform(ct ChartType) *QueryResult {
	if ct == "" {
		return &QueryResult{Status: QueryStatusTypeNotSelected}
	}

	switch pqr.Type {
	case model.ValString:
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	case model.ValScalar:
		return pqr.transformScalar(ct)
	case model.ValVector:
		return pqr.transformVector()
	case model.ValMatrix:
		return pqr.transformMatrix(ct)
	default:
		return &QueryResult{Status: QueryStatusNoData}
	}
}

// transformScalar transforms a scalar Prometheus result.
func (pqr *PrometheusQueryResult) transformScalar(ct ChartType) *QueryResult {
	if ct == ChartTypeLine {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	}

	rawJSON, err := json.Marshal(pqr.Result)
	if err != nil {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	var scalar model.Scalar
	if err := json.Unmarshal(rawJSON, &scalar); err != nil {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	v := float64(scalar.Value)
	if !isValidValue(v) {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data: []QueryResultSeries{
			{
				Labels: map[string]string{},
				Metrics: [][2]any{
					{
						scalar.Timestamp.Unix(),
						v,
					},
				},
			},
		},
	}
}

// transformVector transforms a vector Prometheus result.
func (pqr *PrometheusQueryResult) transformVector() *QueryResult {
	rawJSON, err := json.Marshal(pqr.Result)
	if err != nil {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	var vector model.Vector
	if err := json.Unmarshal(rawJSON, &vector); err != nil {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	if len(vector) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	// Check if all samples are histograms.
	allHistograms := true
	for _, sample := range vector {
		if sample.Histogram == nil {
			allHistograms = false
			break
		}
	}

	if allHistograms {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	}

	var (
		series        []QueryResultSeries
		hasValidValue bool
	)

	for _, sample := range vector {
		if sample.Histogram != nil {
			continue
		}

		v := float64(sample.Value)
		if !isValidValue(v) {
			continue
		}

		hasValidValue = true
		series = append(series, QueryResultSeries{
			Labels: promConvertPromMetricToLabels(sample.Metric),
			Metrics: [][2]any{
				{
					sample.Timestamp.Unix(),
					v,
				},
			},
		})
	}

	if !hasValidValue {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data:   series,
	}
}

// transformMatrix transforms a matrix Prometheus result.
func (pqr *PrometheusQueryResult) transformMatrix(ct ChartType) *QueryResult {
	rawJSON, err := json.Marshal(pqr.Result)
	if err != nil {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	var matrix model.Matrix
	if err := json.Unmarshal(rawJSON, &matrix); err != nil {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	if len(matrix) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	// Check if all streams are histogram-only.
	allHistograms := true
	for _, stream := range matrix {
		if len(stream.Values) > 0 {
			allHistograms = false
			break
		}
	}

	if allHistograms {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	}

	var (
		series        []QueryResultSeries
		hasValidValue bool
	)

	for _, stream := range matrix {
		if len(stream.Values) == 0 {
			continue
		}

		var metrics [][2]any

		if ct == ChartTypeGauge {
			// For gauge, only keep the last valid value.
			for i := len(stream.Values) - 1; i >= 0; i-- {
				v := float64(stream.Values[i].Value)
				if isValidValue(v) {
					hasValidValue = true
					metrics = [][2]any{
						{
							stream.Values[i].Timestamp.Unix(),
							v,
						},
					}

					break
				}
			}
		} else {
			for _, sp := range stream.Values {
				v := float64(sp.Value)
				if !isValidValue(v) {
					continue
				}

				hasValidValue = true
				metrics = append(metrics, [2]any{
					sp.Timestamp.Unix(),
					v,
				})
			}
		}

		if len(metrics) > 0 {
			series = append(series, QueryResultSeries{
				Labels:  promConvertPromMetricToLabels(stream.Metric),
				Metrics: metrics,
			})
		}
	}

	if !hasValidValue {
		return &QueryResult{Status: QueryStatusInvalid}
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data:   series,
	}
}

// promConvertPromMetricToLabels converts a Prometheus Metric (which is a set of label
// key-value pairs) into a map[string]string for easier use in our QueryResultSeries.
func promConvertPromMetricToLabels(m model.Metric) map[string]string {
	res := make(map[string]string, len(m))

	for k, v := range m {
		res[string(k)] = string(v)
	}

	return res
}

// PrometheusLabelNamesResult represents the result of a label names query.
type PrometheusLabelNamesResult struct {
	// Warnings contains any warnings generated during the query.
	Warnings []string `json:"warnings,omitempty"`

	// Result contains the label names.
	Result []string `json:"result,omitempty"`
}

// PrometheusLabelValuesResult represents the result of a label values query.
type PrometheusLabelValuesResult struct {
	// Warnings contains any warnings generated during the query.
	Warnings []string `json:"warnings,omitempty"`

	// Result contains the label values.
	Result []string `json:"result,omitempty"`
}

// PrometheusSeriesResult represents the result of a series query.
type PrometheusSeriesResult struct {
	// Warnings contains any warnings generated during the query.
	Warnings []string `json:"warnings,omitempty"`

	// Result contains the matching series label sets.
	Result []model.LabelSet `json:"result,omitempty"`
}

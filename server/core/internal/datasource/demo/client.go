package demo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

// _queryTimeout bounds one evaluation, matching what the data source
// processors give a real server.
var _queryTimeout = 10 * time.Second

// _maxSamples caps how many samples one evaluation may load. The demo's
// own series are far below it, so it only ever stops a query whose
// expansion — a subquery over a long window — would otherwise grow
// without bound.
const _maxSamples = 5_000_000

// newEngine creates the engine demo queries evaluate on.
func newEngine() *promql.Engine {
	return promql.NewEngine(promql.EngineOpts{
		MaxSamples:           _maxSamples,
		Timeout:              _queryTimeout,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		NoStepSubqueryIntervalFn: func(int64) int64 {
			return _tickMillis
		},
	})
}

// Client answers Prometheus operations out of the demo registry.
//
// One client serves every query a process answers. The registry it holds
// caches the history its walks replay, and a data source runner is built
// per request, so a client built per request would throw that cache away
// every time.
type Client struct {
	// engine evaluates the queries asked of the registry.
	engine *promql.Engine

	// parser reads the selectors the label and series endpoints are
	// narrowed by.
	parser parser.Parser

	// registry is everything this client publishes.
	registry registry
}

// NewClient creates a fresh instance of Client.
func NewClient() *Client {
	return &Client{
		engine:   newEngine(),
		parser:   parser.NewParser(parser.Options{}),
		registry: newRegistry(),
	}
}

// TestConnection tests the connection to the demo data source, which is
// this process and so is always reachable.
func (c *Client) TestConnection(context.Context) (processor.ConnectionStatus, error) {
	return processor.ConnectionStatusSuccess, nil
}

// Metadata retrieves metadata about the demo data source.
func (c *Client) Metadata(context.Context) (*processor.PrometheusMetadataResult, error) {
	md := make(map[string][]v1.Metadata, len(c.registry))

	for _, f := range c.registry {
		md[f.name] = []v1.Metadata{
			{
				Type: v1.MetricTypeGauge,
				Help: f.help,
			},
		}
	}

	return &processor.PrometheusMetadataResult{Result: md}, nil
}

// QueryRange performs a query against the demo data source over the
// specified time range.
func (c *Client) QueryRange(
	ctx context.Context,
	q string,
	tr processor.TimeRange,
) (*processor.PrometheusQueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	ntr := tr.Normalize()
	step := ntr.QueryStep()

	// the samples are spaced a step apart, so the lookback window has to
	// span one to reach the sample behind every point of the grid.
	query, err := c.engine.NewRangeQuery(
		ctx,
		queryable{registry: c.registry},
		promql.NewPrometheusQueryOpts(false, step+_tick),
		ntr.ProcessPrometheusQuery(q),
		ntr.From,
		ntr.To,
		step,
	)
	if err != nil {
		return nil, processor.NewInvalidQueryError(err.Error())
	}

	defer query.Close()

	res := query.Exec(ctx)
	if res.Err != nil {
		// an evaluation that ran out of time or was abandoned is not a
		// query the reader can fix, so it is not reported as one. What
		// the engine rejects on its own terms — a query too expensive to
		// run, a type it cannot evaluate — is.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("evaluating demo query: %w", res.Err)
		}

		return nil, processor.NewInvalidQueryError(res.Err.Error())
	}

	matrix, ok := res.Value.(promql.Matrix)
	if !ok {
		// NOCOV: a range query only ever evaluates to a matrix.
		return nil, errors.New("unexpected demo query result type")
	}

	warns, _ := res.Warnings.AsStrings(q, 0, 0)

	return &processor.PrometheusQueryResult{
		Type:     model.ValMatrix,
		Result:   toModelMatrix(matrix),
		Warnings: warns,
	}, nil
}

// LabelNames retrieves the label names the demo's series carry, narrowed
// to the ones the matchers select.
func (c *Client) LabelNames(
	_ context.Context,
	matchers []string,
	_ processor.TimeRange,
) (*processor.PrometheusLabelNamesResult, error) {
	sets, err := c.parseMatchers(matchers)
	if err != nil {
		return nil, err
	}

	return &processor.PrometheusLabelNamesResult{Result: c.registry.labelNames(sets)}, nil
}

// LabelValues retrieves the values the demo's series carry for the named
// label, narrowed to the ones the matchers select.
func (c *Client) LabelValues(
	_ context.Context,
	label string,
	matchers []string,
	_ processor.TimeRange,
) (*processor.PrometheusLabelValuesResult, error) {
	sets, err := c.parseMatchers(matchers)
	if err != nil {
		return nil, err
	}

	return &processor.PrometheusLabelValuesResult{Result: c.registry.labelValues(label, sets)}, nil
}

// Series retrieves the demo's series matching the given selectors.
func (c *Client) Series(
	_ context.Context,
	matchers []string,
	_ processor.TimeRange,
) (*processor.PrometheusSeriesResult, error) {
	sets, err := c.parseMatchers(matchers)
	if err != nil {
		return nil, err
	}

	var (
		res  []model.LabelSet
		seen = map[string]struct{}{}
	)

	for _, set := range sets {
		c.registry.forEach(set, func(lbls labels.Labels, _ func(tick int64) float64) {
			key := lbls.String()

			if _, ok := seen[key]; ok {
				return
			}

			seen[key] = struct{}{}

			res = append(res, toModelLabelSet(lbls))
		})
	}

	return &processor.PrometheusSeriesResult{Result: res}, nil
}

// parseMatchers turns the API's series selectors into matcher sets. No
// selector at all selects everything, which is what the endpoints answer
// when the caller narrows nothing.
func (c *Client) parseMatchers(mm []string) ([][]*labels.Matcher, error) {
	if len(mm) == 0 {
		return [][]*labels.Matcher{nil}, nil
	}

	sets, err := c.parser.ParseMetricSelectors(mm)
	if err != nil {
		return nil, processor.NewInvalidQueryError(err.Error())
	}

	return sets, nil
}

// toModelMatrix converts an evaluated matrix into the shape the real
// Prometheus client returns, so a chart cannot tell the two apart.
func toModelMatrix(m promql.Matrix) model.Matrix {
	res := make(model.Matrix, 0, len(m))

	for _, s := range m {
		values := make([]model.SamplePair, 0, len(s.Floats))

		for _, p := range s.Floats {
			values = append(values, model.SamplePair{
				Timestamp: model.Time(p.T),
				Value:     model.SampleValue(p.F),
			})
		}

		res = append(res, &model.SampleStream{
			Metric: model.Metric(toModelLabelSet(s.Metric)),
			Values: values,
		})
	}

	return res
}

// toModelLabelSet converts an evaluated label set into the shape the real
// Prometheus client returns.
func toModelLabelSet(lbls labels.Labels) model.LabelSet {
	ls := make(model.LabelSet, lbls.Len())

	lbls.Range(func(l labels.Label) {
		ls[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	})

	return ls
}

package tools

import (
	"context"
	"maps"
	"strconv"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/common/model"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testDataSourceID is the id every data-source test addresses.
var _testDataSourceID = xid.New()

// dataSourceDeps builds session wiring whose data-source lookups answer
// with one data source of the given type, and whose runner is the given
// mock.
func dataSourceDeps(t *testing.T, typ datasource.Type, runner *datasourceMock.Runner) *Deps {
	t.Helper()

	if runner == nil {
		runner = &datasourceMock.Runner{}
	}

	ds := datasource.DataSource{
		ID:     _testDataSourceID,
		Name:   "prod",
		Type:   typ,
		URL:    "http://prometheus:9090",
		Status: processor.ConnectionStatusSuccess,
	}

	db := &DBMock{
		FetchDataSourceFunc: func(_ context.Context, id xid.ID, orgID string) (*datasource.DataSource, error) {
			if id != _testDataSourceID || orgID != "org" {
				return nil, errutil.ErrNotFound
			}

			out := ds

			return &out, nil
		},
		FetchDataSourcesFunc: func(_ context.Context, orgID string) ([]datasource.DataSource, error) {
			if orgID != "org" {
				return nil, nil
			}

			return []datasource.DataSource{ds}, nil
		},
	}

	d := testDeps(db, nil, nil)
	d.runners = &DataSourceRunnersMock{
		RunnerFunc: func(datasource.DataSource) datasource.Runner { return runner },
	}

	return d
}

// prometheusRunner builds a runner handing out the given Prometheus
// client. A nil client is a data source that cannot serve one, which is
// what the real runner reports for the wrong type or a failed
// connection.
func prometheusRunner(client *datasourceMock.Prometheus) *datasourceMock.Runner {
	return &datasourceMock.Runner{
		TypeFunc: func() datasource.Type { return datasource.TypePrometheus },
		PrometheusFunc: func(context.Context) (datasource.Prometheus, error) {
			if client == nil {
				return nil, assert.AnError
			}

			return client, nil
		},
	}
}

// sqlRunner builds a runner handing out the given dialect-agnostic SQL
// client.
func sqlRunner(client *datasourceMock.SQL) *datasourceMock.Runner {
	return &datasourceMock.Runner{
		TypeFunc: func() datasource.Type { return datasource.TypePostgreSQL },
		SQLFunc: func(context.Context) (datasource.SQL, error) {
			if client == nil {
				return nil, assert.AnError
			}

			return client, nil
		},
	}
}

// dialectRunner builds a runner of the given type handing out both
// dialect clients, so query_data_source's own dispatch is what decides which
// one is reached.
func dialectRunner(typ datasource.Type, pg *datasourceMock.PostgreSQL, my *datasourceMock.MySQL) *datasourceMock.Runner {
	return &datasourceMock.Runner{
		TypeFunc: func() datasource.Type { return typ },
		PostgreSQLFunc: func(context.Context) (datasource.PostgreSQL, error) {
			if pg == nil {
				return nil, assert.AnError
			}

			return pg, nil
		},
		MySQLFunc: func(context.Context) (datasource.MySQL, error) {
			if my == nil {
				return nil, assert.AnError
			}

			return my, nil
		},
	}
}

// dataSourceCase is one data-source tool call: the wiring it runs
// against, the arguments the model supplied, and what it should produce.
type dataSourceCase struct {
	// Type is the data source's type, which decides what the lookup
	// reports and, for query_data_source, which dialect is reached.
	Type datasource.Type

	// Runner is the runner the call reads through. Nil is a don't-care
	// stub whose accessors hand out nothing.
	Runner *datasourceMock.Runner

	// Deps overrides the wiring entirely, for the cases that need the
	// lookup itself to fail.
	Deps *Deps

	// Args is the raw JSON the model supplied.
	Args string

	// Contains is a fragment the result has to carry.
	Contains string

	// Err is the expected failure, if any.
	Err error
}

// runDataSourceCase executes a data-source tool and asserts the outcome.
func runDataSourceCase(t *testing.T, tl Tool, name Name, c dataSourceCase) {
	t.Helper()

	d := c.Deps
	if d == nil {
		d = dataSourceDeps(t, c.Type, c.Runner)
	}

	got, err := tl.Execute(testInput(d, name, c.Args))

	testutil.AssertEqualError(t, c.Err, err)

	if c.Err != nil {
		return
	}

	assert.Contains(t, got, c.Contains)
}

// _badIDCases are the ways the model can name a data source that cannot
// be resolved. Every data-source tool but list_data_sources shares them.
func badIDCases(argsFor func(id string) string) map[string]dataSourceCase {
	return map[string]dataSourceCase{
		"No data source id": {
			Type: datasource.TypePrometheus,
			Args: argsFor(""),
			Err:  assert.AnError,
		},
		"An id that is not an xid": {
			Type: datasource.TypePrometheus,
			Args: argsFor("wibble"),
			Err:  assert.AnError,
		},
		"An id from another organisation": {
			Type: datasource.TypePrometheus,
			Args: argsFor(xid.New().String()),
			Err:  assert.AnError,
		},
	}
}

func Test_listDataSources_Info(t *testing.T) {
	t.Parallel()

	info := listDataSources{}.Info()

	assert.Equal(t, Traits{DataSource: true}, info.Traits)

	assert.Equal(t, NameListDataSources, info.Name)
	assert.Empty(t, info.Required)
	assert.Empty(t, info.Properties)
}

func Test_listDataSources_Execute(t *testing.T) {
	t.Parallel()

	failing := testDeps(&DBMock{
		FetchDataSourcesFunc: func(context.Context, string) ([]datasource.DataSource, error) {
			return nil, assert.AnError
		},
	}, nil, nil)

	cc := map[string]dataSourceCase{
		"The organisation's data sources": {
			Type:     datasource.TypePrometheus,
			Contains: `"name":"prod"`,
		},
		"A failing lookup": {
			Deps: failing,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, listDataSources{}, NameListDataSources, c)
		})
	}

	// the id, the name, the type and the status are what a tool needs to
	// be addressed; the URL and the credentials are the organisation's
	// secret and never reach the model.
	got, err := listDataSources{}.Execute(testInput(
		dataSourceDeps(t, datasource.TypePrometheus, nil),
		NameListDataSources,
		"",
	))
	require.NoError(t, err)
	assert.Contains(t, got, _testDataSourceID.String())
	assert.Contains(t, got, `"type":"prometheus"`)
	assert.Contains(t, got, `"status":"success"`)
	assert.NotContains(t, got, "prometheus:9090")
	assert.NotContains(t, got, "credentials")
}

func Test_dataSourceArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, dataSourceArgs{DataSourceID: _testDataSourceID}, map[string]Args{
		"data_source_id": dataSourceArgs{},
	})
}

func Test_getDataSourceMetadata_Info(t *testing.T) {
	t.Parallel()

	info := getDataSourceMetadata{}.Info()

	assert.Equal(t, Traits{DataSource: true}, info.Traits)

	assert.Equal(t, NameGetDataSourceMetadata, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
}

func Test_getDataSourceMetadata_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	cc := map[string]struct {
		Args     string
		Expected string
	}{
		"A data source that resolves is named": {
			Args:     `{"data_source_id":"` + _testDataSourceID.String() + `"}`,
			Expected: `Reading what "prod" holds`,
		},
		"One that does not is an error": {
			Args: `{"data_source_id":"` + xid.New().String() + `"}`,
		},
		"No id at all": {
			Args: `{}`,
		},
		"Unreadable arguments": {
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, getDataSourceMetadata{}.Title(testInput(d, NameGetDataSourceMetadata, c.Args)))
		})
	}
}

func Test_getDataSourceMetadata_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	prom := &datasourceMock.Prometheus{
		MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) {
			return &processor.PrometheusMetadataResult{Result: map[string]any{"up": "gauge"}}, nil
		},
	}

	sql := &datasourceMock.SQL{
		MetadataFunc: func(context.Context) (*processor.SQLMetadataResult, error) {
			return &processor.SQLMetadataResult{
				Tables: map[string]processor.SQLTable{
					"public.orders": {Columns: []processor.SQLColumn{{Name: "id"}, {Name: "total"}}},
				},
				DefaultSchema: "public",
			}, nil
		},
	}

	cc := map[string]dataSourceCase{
		"A Prometheus data source's metrics": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(prom),
			Args:     `{"data_source_id":"` + id + `"}`,
			Contains: "gauge",
		},
		"A SQL data source's tables, each with its column names": {
			Type:     datasource.TypePostgreSQL,
			Runner:   sqlRunner(sql),
			Args:     `{"data_source_id":"` + id + `"}`,
			Contains: `{"default_schema":"public","tables":{"public.orders":["id","total"]}}`,
		},
		"Unreadable arguments": {
			Type: datasource.TypePrometheus,
			Args: `{`,
			Err:  assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(nil),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no SQL client": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(nil),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"A failing Prometheus read": {
			Type: datasource.TypePrometheus,
			Runner: prometheusRunner(&datasourceMock.Prometheus{
				MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) {
					return nil, assert.AnError
				},
			}),
			Args: `{"data_source_id":"` + id + `"}`,
			Err:  assert.AnError,
		},
		"A failing SQL read": {
			Type: datasource.TypePostgreSQL,
			Runner: sqlRunner(&datasourceMock.SQL{
				MetadataFunc: func(context.Context) (*processor.SQLMetadataResult, error) {
					return nil, assert.AnError
				},
			}),
			Args: `{"data_source_id":"` + id + `"}`,
			Err:  assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, getDataSourceMetadata{}, NameGetDataSourceMetadata, c)
		})
	}
}

func Test_prometheusLabelsArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, prometheusLabelsArgs{DataSourceID: _testDataSourceID}, map[string]Args{
		"data_source_id": prometheusLabelsArgs{Label: "job"},
	})
}

func Test_listPrometheusLabels_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusLabels{}.Info()

	assert.Equal(t, Traits{DataSource: true}, info.Traits)

	assert.Equal(t, NameListPrometheusLabels, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
	assert.Contains(t, info.Properties, "label")
	assert.Contains(t, info.Properties, "matchers")
	assert.Contains(t, info.Properties, "from")
}

func Test_listPrometheusLabels_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)
	id := _testDataSourceID.String()

	cc := map[string]struct {
		Args     string
		Expected string
	}{
		"The label names": {
			Args:     `{"data_source_id":"` + id + `"}`,
			Expected: `Listing labels of "prod"`,
		},
		"One label's values": {
			Args:     `{"data_source_id":"` + id + `","label":"job"}`,
			Expected: `Listing values of label "job" in "prod"`,
		},
		// a data source the organisation does not own is an error, not a
		// label: the model is told what it named does not exist.
		"A data source of another organisation": {
			Args: `{"data_source_id":"` + xid.New().String() + `"}`,
		},
		"Unreadable arguments": {
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, listPrometheusLabels{}.Title(testInput(d, NameListPrometheusLabels, c.Args)))
		})
	}
}

func Test_listPrometheusLabels_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := &datasourceMock.Prometheus{
		LabelNamesFunc: func(_ context.Context, matchers []string, tr processor.TimeRange) (*processor.PrometheusLabelNamesResult, error) {
			if len(matchers) > 0 && matchers[0] == "boom" {
				return nil, assert.AnError
			}

			assert.Equal(t, time.Hour, tr.To.Sub(tr.From))

			return &processor.PrometheusLabelNamesResult{Result: []string{"job"}}, nil
		},
		LabelValuesFunc: func(_ context.Context, label string, _ []string, _ processor.TimeRange) (*processor.PrometheusLabelValuesResult, error) {
			if label == "boom" {
				return nil, assert.AnError
			}

			return &processor.PrometheusLabelValuesResult{Result: []string{"api"}}, nil
		},
	}

	cc := map[string]dataSourceCase{
		"The label names on the selected series": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(client),
			Args:     `{"data_source_id":"` + id + `","matchers":["up"]}`,
			Contains: `["job"]`,
		},
		"The values a label takes": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(client),
			Args:     `{"data_source_id":"` + id + `","label":"job"}`,
			Contains: `["api"]`,
		},
		"Unreadable arguments": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{`,
			Err:    assert.AnError,
		},
		"An unparseable range start": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","from":"yesterday"}`,
			Err:    assert.AnError,
		},
		"An inverted range": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePostgreSQL,
			Runner: prometheusRunner(nil),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"A failing names read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","matchers":["boom"]}`,
			Err:    assert.AnError,
		},
		"A failing values read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","label":"boom"}`,
			Err:    assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, listPrometheusLabels{}, NameListPrometheusLabels, c)
		})
	}
}

func Test_prometheusSeriesArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, prometheusSeriesArgs{DataSourceID: _testDataSourceID, Matchers: []string{"up"}}, map[string]Args{
		"data_source_id": prometheusSeriesArgs{Matchers: []string{"up"}},
		"matchers":       prometheusSeriesArgs{DataSourceID: _testDataSourceID},
	})
}

func Test_listPrometheusSeries_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusSeries{}.Info()

	assert.Equal(t, Traits{DataSource: true}, info.Traits)

	assert.Equal(t, NameListPrometheusSeries, info.Name)
	assert.Equal(t, []string{"data_source_id", "matchers"}, info.Required)
}

func Test_listPrometheusSeries_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	assert.Equal(t, `Listing series of "prod"`, listPrometheusSeries{}.Title(testInput(
		d,
		NameListPrometheusSeries,
		`{"data_source_id":"`+_testDataSourceID.String()+`","matchers":["up"]}`,
	)))

	// arguments that name nothing it can describe make no line.
	assert.Empty(t, listPrometheusSeries{}.Title(testInput(d, NameListPrometheusSeries, `{`)))
	assert.Empty(t, listPrometheusSeries{}.Title(testInput(d, NameListPrometheusSeries, `{"data_source_id":"`+xid.New().String()+`","matchers":["up"]}`)))
}

func Test_listPrometheusSeries_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := &datasourceMock.Prometheus{
		SeriesFunc: func(_ context.Context, matchers []string, _ processor.TimeRange) (*processor.PrometheusSeriesResult, error) {
			if matchers[0] == "boom" {
				return nil, assert.AnError
			}

			return &processor.PrometheusSeriesResult{
				Result: []model.LabelSet{
					{"job": "api"},
				},
			}, nil
		},
	}

	cc := map[string]dataSourceCase{
		"The series the matchers select": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(client),
			Args:     `{"data_source_id":"` + id + `","matchers":["{job=\"api\"}"]}`,
			Contains: "api",
		},
		"Unreadable arguments": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{`,
			Err:    assert.AnError,
		},
		// Prometheus rejects a series query with no selector, so the model
		// is told what is missing rather than handed the upstream error.
		"No matcher at all": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"An empty matcher list": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","matchers":[]}`,
			Err:    assert.AnError,
		},
		"An unparseable range": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","matchers":["up"],"from":"then"}`,
			Err:    assert.AnError,
		},
		"An inverted range": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","matchers":["up"],"from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePostgreSQL,
			Runner: prometheusRunner(nil),
			Args:   `{"data_source_id":"` + id + `","matchers":["up"]}`,
			Err:    assert.AnError,
		},
		"A failing read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","matchers":["boom"]}`,
			Err:    assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `","matchers":["up"]}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, listPrometheusSeries{}, NameListPrometheusSeries, c)
		})
	}
}

func Test_queryDataSourceArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, queryDataSourceArgs{DataSourceID: _testDataSourceID, Query: "up"}, map[string]Args{
		"data_source_id": queryDataSourceArgs{Query: "up"},
		"query":          queryDataSourceArgs{DataSourceID: _testDataSourceID},
	})
}

func Test_queryDataSource_Info(t *testing.T) {
	t.Parallel()

	info := queryDataSource{}.Info()

	assert.Equal(t, Traits{DataSource: true}, info.Traits)

	assert.Equal(t, NameQueryDataSource, info.Name)
	assert.Equal(t, []string{"data_source_id", "query"}, info.Required)

	chartType, ok := info.Properties["chart_type"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, []processor.ChartType{processor.ChartTypeLine, processor.ChartTypeBar, processor.ChartTypeGauge}, chartType["enum"])
}

func Test_queryDataSource_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	cc := map[string]struct {
		Args     string
		Expected string
	}{
		"A data source that resolves is named": {
			Args:     `{"data_source_id":"` + _testDataSourceID.String() + `","query":"up"}`,
			Expected: `Querying "prod"`,
		},
		// a data source the organisation does not own is an error, not a
		// label: the model is told what it named does not exist.
		"A data source of another organisation": {
			Args: `{"data_source_id":"` + xid.New().String() + `","query":"up"}`,
		},
		"Unreadable arguments": {
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, queryDataSource{}.Title(testInput(d, NameQueryDataSource, c.Args)))
		})
	}
}

func Test_queryDataSource_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	prom := func(res *processor.PrometheusQueryResult) *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				if q == "boom" {
					return nil, assert.AnError
				}

				return res, nil
			},
		}
	}

	rows := [][]any{
		{float64(1), float64(2)},
	}

	pg := &datasourceMock.PostgreSQL{
		QueryFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
			switch q {
			case "boom":
				return nil, assert.AnError
			case "empty":
				//nolint:nilnil // an absent result is what the executor reports here
				return nil, nil
			default:
				return &processor.PostgreSQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}, nil
			}
		},
	}

	my := &datasourceMock.MySQL{
		QueryFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.MySQLQueryResult, error) {
			switch q {
			case "boom":
				return nil, assert.AnError
			case "empty":
				//nolint:nilnil // an absent result is what the executor reports here
				return nil, nil
			default:
				return &processor.MySQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}, nil
			}
		},
	}

	cc := map[string]dataSourceCase{
		"Prometheus raw": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(prom(&processor.PrometheusQueryResult{Type: model.ValMatrix, Warnings: []string{"slow"}})),
			Args:     `{"data_source_id":"` + id + `","query":"up"}`,
			Contains: `"warnings":["slow"]`,
		},
		"Prometheus charted": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(prom(&processor.PrometheusQueryResult{Type: model.ValMatrix})),
			Args:     `{"data_source_id":"` + id + `","query":"up","chart_type":"line_chart"}`,
			Contains: `"status"`,
		},
		// a query that returned nothing has no result to transform, and the
		// metric block renders it as no-data.
		"A Prometheus query with no result at all is no-data": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(prom(nil)),
			Args:     `{"data_source_id":"` + id + `","query":"up","chart_type":"gauge_chart"}`,
			Contains: `"status":"no-data"`,
		},
		"PostgreSQL raw": {
			Type:     datasource.TypePostgreSQL,
			Runner:   dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"select 1"}`,
			Contains: `"columns":["time","value"]`,
		},
		"PostgreSQL charted": {
			Type:     datasource.TypePostgreSQL,
			Runner:   dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"select 1","chart_type":"line_chart"}`,
			Contains: `"status"`,
		},
		"A PostgreSQL query with no rows at all is no-data": {
			Type:     datasource.TypePostgreSQL,
			Runner:   dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"empty","chart_type":"line_chart"}`,
			Contains: `"status":"no-data"`,
		},
		"MySQL raw": {
			Type:     datasource.TypeMySQL,
			Runner:   dialectRunner(datasource.TypeMySQL, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"select 1"}`,
			Contains: `"columns":["time","value"]`,
		},
		"MariaDB charted": {
			Type:     datasource.TypeMariaDB,
			Runner:   dialectRunner(datasource.TypeMariaDB, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"select 1","chart_type":"bar_chart"}`,
			Contains: `"status"`,
		},
		"A MySQL query with no rows at all is no-data": {
			Type:     datasource.TypeMySQL,
			Runner:   dialectRunner(datasource.TypeMySQL, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"empty","chart_type":"line_chart"}`,
			Contains: `"status":"no-data"`,
		},
		"Unreadable arguments": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:   `{`,
			Err:    assert.AnError,
		},
		"No query": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"An unknown chart type": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:   `{"data_source_id":"` + id + `","query":"select 1","chart_type":"donut"}`,
			Err:    assert.AnError,
		},
		"An unparseable range": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:   `{"data_source_id":"` + id + `","query":"select 1","from":"epoch"}`,
			Err:    assert.AnError,
		},
		"An inverted range": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:   `{"data_source_id":"` + id + `","query":"select 1","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(nil),
			Args:   `{"data_source_id":"` + id + `","query":"up"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no PostgreSQL client": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, nil, my),
			Args:   `{"data_source_id":"` + id + `","query":"select 1"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no MySQL client": {
			Type:   datasource.TypeMySQL,
			Runner: dialectRunner(datasource.TypeMySQL, pg, nil),
			Args:   `{"data_source_id":"` + id + `","query":"select 1"}`,
			Err:    assert.AnError,
		},
		"A failing Prometheus read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(prom(nil)),
			Args:   `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:    assert.AnError,
		},
		"A failing PostgreSQL read": {
			Type:   datasource.TypePostgreSQL,
			Runner: dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:   `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:    assert.AnError,
		},
		"A failing MySQL read": {
			Type:   datasource.TypeMySQL,
			Runner: dialectRunner(datasource.TypeMySQL, pg, my),
			Args:   `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:    assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `","query":"select 1"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, queryDataSource{}, NameQueryDataSource, c)
		})
	}
}

func Test_newRawSeries(t *testing.T) {
	t.Parallel()

	stream := func(points int) *model.SampleStream {
		s := &model.SampleStream{Metric: model.Metric{"job": "api"}}

		for i := range points {
			s.Values = append(s.Values, model.SamplePair{Timestamp: model.Time(i), Value: model.SampleValue(i)})
		}

		return s
	}

	matrix := func(series, points int) model.Matrix {
		out := make(model.Matrix, 0, series)

		for range series {
			out = append(out, stream(points))
		}

		return out
	}

	cc := map[string]struct {
		Input     *processor.PrometheusQueryResult
		Series    int
		Points    int
		Truncated string
	}{
		"An answer within the caps is as it was": {
			Input:  &processor.PrometheusQueryResult{Type: model.ValMatrix, Result: matrix(2, 3)},
			Series: 2,
			Points: 3,
		},
		"Too many series keeps the first ones": {
			Input:     &processor.PrometheusQueryResult{Type: model.ValMatrix, Result: matrix(_maxRawSeries+5, 3)},
			Series:    _maxRawSeries,
			Points:    3,
			Truncated: "20 of 25 series, each its latest 100 points at most; set chart_type to describe them all",
		},
		"Too many points keeps the latest ones": {
			Input:     &processor.PrometheusQueryResult{Type: model.ValMatrix, Result: matrix(1, _maxRawPoints+50)},
			Series:    1,
			Points:    _maxRawPoints,
			Truncated: "1 of 1 series, each its latest 100 points at most; set chart_type to describe them all",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := newRawSeries(c.Input)
			assert.Equal(t, c.Truncated, got.Truncated)

			m, ok := got.Result.(model.Matrix)
			require.True(t, ok)
			require.Len(t, m, c.Series)
			assert.Len(t, m[0].Values, c.Points)
			// the points kept are the latest ones.
			assert.InDelta(t, float64(len(c.Input.Result.(model.Matrix)[0].Values)-1), float64(m[0].Values[len(m[0].Values)-1].Value), 0)
		})
	}

	// an answer that is not a range of series has nothing to cap.
	scalar := &processor.PrometheusQueryResult{Type: model.ValScalar, Result: &model.Scalar{Value: 1}}
	assert.Equal(t, rawSeries{Type: model.ValScalar, Result: scalar.Result}, newRawSeries(scalar))
}

func Test_newRawRows(t *testing.T) {
	t.Parallel()

	rows := func(n int) [][]any {
		out := make([][]any, 0, n)

		for i := range n {
			out = append(out, []any{i})
		}

		return out
	}

	assert.Equal(t, rawRows{Columns: []string{"a"}, Rows: rows(2)}, newRawRows([]string{"a"}, rows(2)))

	got := newRawRows([]string{"a"}, rows(_maxRawRows+50))
	assert.Len(t, got.Rows, _maxRawRows)
	assert.Equal(t, _maxRawRows+50, got.TotalRows)
}

func Test_timeRangeArgs_resolve(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	cc := map[string]struct {
		Args  timeRangeArgs
		Check func(t *testing.T, tr processor.TimeRange)
		Err   error
	}{
		"An inverted range": {
			Args: timeRangeArgs{From: to, To: from},
			Err:  errInvertedTimeRange,
		},
		"A future start with no end given": {
			Args: timeRangeArgs{From: timeutil.Now().Add(time.Hour)},
			Err:  errInvertedTimeRange,
		},
		"Both ends given": {
			Args: timeRangeArgs{From: from, To: to},
			Check: func(t *testing.T, tr processor.TimeRange) {
				t.Helper()

				assert.Equal(t, from, tr.From)
				assert.Equal(t, to, tr.To)
			},
		},
		"Neither end given defaults to the last hour": {
			Check: func(t *testing.T, tr processor.TimeRange) {
				t.Helper()

				assert.Equal(t, time.Hour, tr.To.Sub(tr.From))
				assert.WithinDuration(t, timeutil.Now(), tr.To, time.Minute)
			},
		},
		"Only the end given backs the start off an hour": {
			Args: timeRangeArgs{To: to},
			Check: func(t *testing.T, tr processor.TimeRange) {
				t.Helper()

				assert.Equal(t, to.Add(-time.Hour), tr.From)
			},
		},
		"Only the start given ends at now": {
			Args: timeRangeArgs{From: from},
			Check: func(t *testing.T, tr processor.TimeRange) {
				t.Helper()

				assert.Equal(t, from, tr.From)
				assert.WithinDuration(t, timeutil.Now(), tr.To, time.Minute)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tr, err := c.Args.resolve()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			c.Check(t, tr)
		})
	}
}

func Test_newChartPreview(t *testing.T) {
	t.Parallel()

	series := func(n int) []processor.QueryResultSeries {
		out := make([]processor.QueryResultSeries, 0, n)

		for i := range n {
			out = append(out, processor.QueryResultSeries{
				Labels:  map[string]string{"i": strconv.Itoa(i)},
				Metrics: [][2]any{{1, 1.5}, {2, 2.5}, {3, 3.5}},
			})
		}

		return out
	}

	cc := map[string]struct {
		Input  *processor.QueryResult
		Result chartPreview
	}{
		"Status without data carries no series": {
			Input:  &processor.QueryResult{Status: processor.QueryStatusNoData},
			Result: chartPreview{Status: processor.QueryStatusNoData},
		},
		"Series report their labels and endpoints, never their points": {
			Input: &processor.QueryResult{
				Status: processor.QueryStatusOK,
				Data:   series(1),
			},
			Result: chartPreview{
				Status:      processor.QueryStatusOK,
				SeriesCount: 1,
				Series: []chartPreviewSeries{{
					Labels:     map[string]string{"i": "0"},
					PointCount: 3,
					First:      [2]any{1, 1.5},
					Last:       [2]any{3, 3.5},
				}},
			},
		},
		"Empty series reports its count without endpoints": {
			Input: &processor.QueryResult{
				Status: processor.QueryStatusOK,
				Data:   []processor.QueryResultSeries{{Labels: map[string]string{"i": "0"}}},
			},
			Result: chartPreview{
				Status:      processor.QueryStatusOK,
				SeriesCount: 1,
				Series:      []chartPreviewSeries{{Labels: map[string]string{"i": "0"}}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newChartPreview(c.Input))
		})
	}

	// the cap bounds the answer without hiding how much there was.
	t.Run("More series than the cap are counted but not listed", func(t *testing.T) {
		t.Parallel()

		got := newChartPreview(&processor.QueryResult{
			Status: processor.QueryStatusOK,
			Data:   series(_maxPreviewSeries + 5),
		})

		assert.Equal(t, _maxPreviewSeries+5, got.SeriesCount)
		assert.Len(t, got.Series, _maxPreviewSeries)
	})
}

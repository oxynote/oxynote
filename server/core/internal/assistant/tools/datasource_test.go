package tools

import (
	"context"
	"maps"
	"strconv"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
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
// dialect clients, so query_sql's own dispatch is what decides which
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
	// reports and, for query_sql, which dialect is reached.
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

	assert.Equal(t, NameListDataSources, info.Name)
	assert.Empty(t, info.Required)
	assert.Empty(t, info.Properties)
}

func Test_listDataSources_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, listDataSources{}.Traits())
}

func Test_listDataSources_Title(t *testing.T) {
	t.Parallel()

	got, err := listDataSources{}.Title(testInput(
		dataSourceDeps(t, datasource.TypePrometheus, nil),
		NameListDataSources,
		"",
	))
	require.NoError(t, err)
	assert.Empty(t, got)
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

func Test_getPrometheusMetadataArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, getPrometheusMetadataArgs{DataSourceID: _testDataSourceID}, map[string]Args{
		"data_source_id": getPrometheusMetadataArgs{},
	})
}

func Test_getPrometheusMetadata_Info(t *testing.T) {
	t.Parallel()

	info := getPrometheusMetadata{}.Info()

	assert.Equal(t, NameGetPrometheusMetadata, info.Name)
	assert.Equal(t, []string{_keyDataSourceID}, info.Required)
}

func Test_getPrometheusMetadata_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, getPrometheusMetadata{}.Traits())
}

func Test_getPrometheusMetadata_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)
	other := xid.New().String()

	cc := map[string]struct {
		Args     string
		Expected string
		Err      error
	}{
		"A data source that resolves is named": {
			Args:     `{"data_source_id":"` + _testDataSourceID.String() + `"}`,
			Expected: `Reading metric metadata of "prod"`,
		},
		"One that does not is an error": {
			Args: `{"data_source_id":"` + other + `"}`,
			Err:  assert.AnError,
		},
		"No id at all": {
			Args: `{}`,
			Err:  assert.AnError,
		},
		"Unreadable arguments": {
			Args: `{`,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := getPrometheusMetadata{}.Title(testInput(d, NameGetPrometheusMetadata, c.Args))

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Expected, got)
		})
	}
}

func Test_getPrometheusMetadata_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	cc := map[string]dataSourceCase{
		"The data source's metric metadata": {
			Type: datasource.TypePrometheus,
			Args: `{"data_source_id":"` + id + `"}`,
			Runner: prometheusRunner(&datasourceMock.Prometheus{
				MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) {
					return &processor.PrometheusMetadataResult{
						Result: map[string]any{
							"up": "gauge",
						},
					}, nil
				},
			}),
			Contains: "gauge",
		},
		"Unreadable arguments": {
			Type: datasource.TypePrometheus,
			Args: `{`,
			Err:  assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePostgreSQL,
			Args:   `{"data_source_id":"` + id + `"}`,
			Runner: prometheusRunner(nil),
			Err:    assert.AnError,
		},
		"A failing read": {
			Type: datasource.TypePrometheus,
			Args: `{"data_source_id":"` + id + `"}`,
			Runner: prometheusRunner(&datasourceMock.Prometheus{
				MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) {
					return nil, assert.AnError
				},
			}),
			Err: assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, getPrometheusMetadata{}, NameGetPrometheusMetadata, c)
		})
	}
}

func Test_prometheusLabelNamesArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, prometheusLabelNamesArgs{DataSourceID: _testDataSourceID}, map[string]Args{
		"data_source_id": prometheusLabelNamesArgs{},
	})
}

func Test_listPrometheusLabelNames_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusLabelNames{}.Info()

	assert.Equal(t, NameListPrometheusLabelNames, info.Name)
	assert.Equal(t, []string{_keyDataSourceID}, info.Required)
	assert.Contains(t, info.Properties, _keyMatchers)
	assert.Contains(t, info.Properties, _keyFrom)
}

func Test_listPrometheusLabelNames_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, listPrometheusLabelNames{}.Traits())
}

func Test_listPrometheusLabelNames_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	got, err := listPrometheusLabelNames{}.Title(testInput(
		d,
		NameListPrometheusLabelNames,
		`{"data_source_id":"`+_testDataSourceID.String()+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, `Listing label names of "prod"`, got)

	_, err = listPrometheusLabelNames{}.Title(testInput(d, NameListPrometheusLabelNames, `{`))
	require.Error(t, err)

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err = listPrometheusLabelNames{}.Title(testInput(d, NameListPrometheusLabelNames, `{"data_source_id":"`+xid.New().String()+`"}`))
	require.Error(t, err)
}

func Test_listPrometheusLabelNames_Execute(t *testing.T) {
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
	}

	cc := map[string]dataSourceCase{
		"The label names on the selected series": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(client),
			Args:     `{"data_source_id":"` + id + `","matchers":["up"]}`,
			Contains: "job",
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
		"A failing read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","matchers":["boom"]}`,
			Err:    assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, listPrometheusLabelNames{}, NameListPrometheusLabelNames, c)
		})
	}
}

func Test_prometheusLabelValuesArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, prometheusLabelValuesArgs{DataSourceID: _testDataSourceID, Label: "job"}, map[string]Args{
		"data_source_id": prometheusLabelValuesArgs{Label: "job"},
		"label":          prometheusLabelValuesArgs{DataSourceID: _testDataSourceID},
	})
}

func Test_listPrometheusLabelValues_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusLabelValues{}.Info()

	assert.Equal(t, NameListPrometheusLabelValues, info.Name)
	assert.Equal(t, []string{_keyDataSourceID, _keyLabel}, info.Required)
}

func Test_listPrometheusLabelValues_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, listPrometheusLabelValues{}.Traits())
}

func Test_listPrometheusLabelValues_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)
	id := _testDataSourceID.String()

	cc := map[string]struct {
		Args     string
		Expected string
		Err      error
	}{
		"The label and the data source": {
			Args:     `{"data_source_id":"` + id + `","label":"job"}`,
			Expected: `Listing values of label "job" in "prod"`,
		},
		"No label yet": {
			Args: `{"data_source_id":"` + id + `"}`,
			Err:  assert.AnError,
		},
		"Unreadable arguments": {
			Args: `{`,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := listPrometheusLabelValues{}.Title(testInput(d, NameListPrometheusLabelValues, c.Args))

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Expected, got)
		})
	}

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err := listPrometheusLabelValues{}.Title(testInput(d, NameListPrometheusLabelValues, `{"data_source_id":"`+xid.New().String()+`","label":"job"}`))
	require.Error(t, err)
}

func Test_listPrometheusLabelValues_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := &datasourceMock.Prometheus{
		LabelValuesFunc: func(_ context.Context, label string, _ []string, _ processor.TimeRange) (*processor.PrometheusLabelValuesResult, error) {
			if label == "boom" {
				return nil, assert.AnError
			}

			return &processor.PrometheusLabelValuesResult{Result: []string{"api"}}, nil
		},
	}

	cc := map[string]dataSourceCase{
		"The values the label takes": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(client),
			Args:     `{"data_source_id":"` + id + `","label":"job"}`,
			Contains: "api",
		},
		"Unreadable arguments": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{`,
			Err:    assert.AnError,
		},
		"No label": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"An unparseable range end": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","label":"job","to":"soon"}`,
			Err:    assert.AnError,
		},
		"An inverted range": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","label":"job","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePostgreSQL,
			Runner: prometheusRunner(nil),
			Args:   `{"data_source_id":"` + id + `","label":"job"}`,
			Err:    assert.AnError,
		},
		"A failing read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client),
			Args:   `{"data_source_id":"` + id + `","label":"boom"}`,
			Err:    assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `","label":"job"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, listPrometheusLabelValues{}, NameListPrometheusLabelValues, c)
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

	assert.Equal(t, NameListPrometheusSeries, info.Name)
	assert.Equal(t, []string{_keyDataSourceID, _keyMatchers}, info.Required)
}

func Test_listPrometheusSeries_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, listPrometheusSeries{}.Traits())
}

func Test_listPrometheusSeries_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	got, err := listPrometheusSeries{}.Title(testInput(
		d,
		NameListPrometheusSeries,
		`{"data_source_id":"`+_testDataSourceID.String()+`","matchers":["up"]}`,
	))
	require.NoError(t, err)
	assert.Equal(t, `Listing series of "prod"`, got)

	_, err = listPrometheusSeries{}.Title(testInput(d, NameListPrometheusSeries, `{`))
	require.Error(t, err)

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err = listPrometheusSeries{}.Title(testInput(d, NameListPrometheusSeries, `{"data_source_id":"`+xid.New().String()+`","matchers":["up"]}`))
	require.Error(t, err)
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

func Test_queryPrometheusArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, queryPrometheusArgs{DataSourceID: _testDataSourceID, Query: "up"}, map[string]Args{
		"data_source_id": queryPrometheusArgs{Query: "up"},
		"query":          queryPrometheusArgs{DataSourceID: _testDataSourceID},
	})
}

func Test_queryPrometheus_Info(t *testing.T) {
	t.Parallel()

	info := queryPrometheus{}.Info()

	assert.Equal(t, NameQueryPrometheus, info.Name)
	assert.Equal(t, []string{_keyDataSourceID, _keyQuery}, info.Required)
	assert.Contains(t, info.Properties, _keyChartType)

	// the chart types the metric block schema declares are exactly the
	// ones chart_type accepts. The block package cannot import the
	// processor — that would drag the Prometheus client and pgx into a
	// leaf package — so the two lists are only kept equal by this check.
	for _, v := range block.MetricEnums()[document.AttrVisualizationType] {
		assert.True(t, processor.ChartType(v).IsValid(), "%q is in the block schema but not a chart type", v)
		assert.Contains(t, _descChartType, v)
	}

	for _, ct := range []processor.ChartType{
		processor.ChartTypeLine,
		processor.ChartTypeBar,
		processor.ChartTypeGauge,
	} {
		assert.Contains(t, block.MetricEnums()[document.AttrVisualizationType], string(ct))
	}
}

func Test_queryPrometheus_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, queryPrometheus{}.Traits())
}

func Test_queryPrometheus_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	got, err := queryPrometheus{}.Title(testInput(
		d,
		NameQueryPrometheus,
		`{"data_source_id":"`+_testDataSourceID.String()+`","query":"up"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, `Querying "prod"`, got)

	_, err = queryPrometheus{}.Title(testInput(d, NameQueryPrometheus, `{`))
	require.Error(t, err)

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err = queryPrometheus{}.Title(testInput(d, NameQueryPrometheus, `{"data_source_id":"`+xid.New().String()+`","query":"up"}`))
	require.Error(t, err)
}

func Test_queryPrometheus_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := func(res *processor.PrometheusQueryResult) *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				if q == "boom" {
					return nil, assert.AnError
				}

				return res, nil
			},
		}
	}

	cc := map[string]dataSourceCase{
		"A raw query returns the raw result": {
			Type: datasource.TypePrometheus,
			Runner: prometheusRunner(client(&processor.PrometheusQueryResult{
				Type:     model.ValMatrix,
				Warnings: []string{"slow"},
			})),
			Args:     `{"data_source_id":"` + id + `","query":"up"}`,
			Contains: `"warnings":["slow"]`,
		},
		"A chart query returns the transformed series": {
			Type: datasource.TypePrometheus,
			Runner: prometheusRunner(client(&processor.PrometheusQueryResult{
				Type: model.ValMatrix,
			})),
			Args:     `{"data_source_id":"` + id + `","query":"up","chart_type":"line_chart"}`,
			Contains: `"status"`,
		},
		// a query that returned nothing has no result to transform, and the
		// metric block renders it as no-data — which is the answer the model
		// asked for by naming a chart type.
		"A chart query with no result at all is no-data": {
			Type:     datasource.TypePrometheus,
			Runner:   prometheusRunner(client(nil)),
			Args:     `{"data_source_id":"` + id + `","query":"up","chart_type":"gauge_chart"}`,
			Contains: `"status":"no-data"`,
		},
		"Unreadable arguments": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client(nil)),
			Args:   `{`,
			Err:    assert.AnError,
		},
		"No query": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client(nil)),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"An unknown chart type": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client(nil)),
			Args:   `{"data_source_id":"` + id + `","query":"up","chart_type":"pie_chart"}`,
			Err:    assert.AnError,
		},
		"An unparseable range": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client(nil)),
			Args:   `{"data_source_id":"` + id + `","query":"up","from":"now"}`,
			Err:    assert.AnError,
		},
		"An inverted range": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client(nil)),
			Args:   `{"data_source_id":"` + id + `","query":"up","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no Prometheus client": {
			Type:   datasource.TypePostgreSQL,
			Runner: prometheusRunner(nil),
			Args:   `{"data_source_id":"` + id + `","query":"up"}`,
			Err:    assert.AnError,
		},
		"A failing read": {
			Type:   datasource.TypePrometheus,
			Runner: prometheusRunner(client(nil)),
			Args:   `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:    assert.AnError,
		},
	}

	maps.Copy(cc, badIDCases(func(id string) string {
		return `{"data_source_id":"` + id + `","query":"up"}`
	}))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, queryPrometheus{}, NameQueryPrometheus, c)
		})
	}
}

func Test_getSQLMetadataArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, getSQLMetadataArgs{DataSourceID: _testDataSourceID}, map[string]Args{
		"data_source_id": getSQLMetadataArgs{},
	})
}

func Test_getSQLMetadata_Info(t *testing.T) {
	t.Parallel()

	info := getSQLMetadata{}.Info()

	assert.Equal(t, NameGetSQLMetadata, info.Name)
	assert.Equal(t, []string{_keyDataSourceID}, info.Required)
}

func Test_getSQLMetadata_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, getSQLMetadata{}.Traits())
}

func Test_getSQLMetadata_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePostgreSQL, nil)

	got, err := getSQLMetadata{}.Title(testInput(
		d,
		NameGetSQLMetadata,
		`{"data_source_id":"`+_testDataSourceID.String()+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, `Reading tables of "prod"`, got)

	_, err = getSQLMetadata{}.Title(testInput(d, NameGetSQLMetadata, `{`))
	require.Error(t, err)

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err = getSQLMetadata{}.Title(testInput(d, NameGetSQLMetadata, `{"data_source_id":"`+xid.New().String()+`"}`))
	require.Error(t, err)
}

func Test_getSQLMetadata_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := &datasourceMock.SQL{
		MetadataFunc: func(context.Context) (*processor.SQLMetadataResult, error) {
			return &processor.SQLMetadataResult{
				Tables: map[string]processor.SQLTable{
					"public.orders": {
						Columns: []processor.SQLColumn{
							{Name: "id"},
						},
					},
				},
				DefaultSchema: "public",
			}, nil
		},
	}

	cc := map[string]dataSourceCase{
		"The data source's tables and columns": {
			Type:     datasource.TypePostgreSQL,
			Runner:   sqlRunner(client),
			Args:     `{"data_source_id":"` + id + `"}`,
			Contains: "public.orders",
		},
		"Unreadable arguments": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(client),
			Args:   `{`,
			Err:    assert.AnError,
		},
		"A data source that hands out no SQL client": {
			Type:   datasource.TypePrometheus,
			Runner: sqlRunner(nil),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"A failing read": {
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

			runDataSourceCase(t, getSQLMetadata{}, NameGetSQLMetadata, c)
		})
	}
}

func Test_sqlQueryLabelsArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, sqlQueryLabelsArgs{DataSourceID: _testDataSourceID, Query: "select 1"}, map[string]Args{
		"data_source_id": sqlQueryLabelsArgs{Query: "select 1"},
		"query":          sqlQueryLabelsArgs{DataSourceID: _testDataSourceID},
	})
}

func Test_getSQLQueryLabels_Info(t *testing.T) {
	t.Parallel()

	info := getSQLQueryLabels{}.Info()

	assert.Equal(t, NameGetSQLQueryLabels, info.Name)
	assert.Equal(t, []string{_keyDataSourceID, _keyQuery}, info.Required)
}

func Test_getSQLQueryLabels_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, getSQLQueryLabels{}.Traits())
}

func Test_getSQLQueryLabels_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypeMySQL, nil)

	got, err := getSQLQueryLabels{}.Title(testInput(
		d,
		NameGetSQLQueryLabels,
		`{"data_source_id":"`+_testDataSourceID.String()+`","query":"select 1"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, `Probing query labels of "prod"`, got)

	_, err = getSQLQueryLabels{}.Title(testInput(d, NameGetSQLQueryLabels, `{`))
	require.Error(t, err)

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err = getSQLQueryLabels{}.Title(testInput(d, NameGetSQLQueryLabels, `{"data_source_id":"`+xid.New().String()+`","query":"select 1"}`))
	require.Error(t, err)
}

func Test_getSQLQueryLabels_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := &datasourceMock.SQL{
		QueryLabelsFunc: func(_ context.Context, q string, _ processor.TimeRange) (map[string]string, error) {
			if q == "boom" {
				return nil, assert.AnError
			}

			return map[string]string{"region": "eu"}, nil
		},
	}

	cc := map[string]dataSourceCase{
		"The query's string columns": {
			Type:     datasource.TypeMariaDB,
			Runner:   sqlRunner(client),
			Args:     `{"data_source_id":"` + id + `","query":"select id from orders"}`,
			Contains: `"region":"eu"`,
		},
		"Unreadable arguments": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(client),
			Args:   `{`,
			Err:    assert.AnError,
		},
		"No query": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(client),
			Args:   `{"data_source_id":"` + id + `"}`,
			Err:    assert.AnError,
		},
		"An unparseable range": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(client),
			Args:   `{"data_source_id":"` + id + `","query":"select 1","to":"later"}`,
			Err:    assert.AnError,
		},
		"An inverted range": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(client),
			Args:   `{"data_source_id":"` + id + `","query":"select 1","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:    assert.AnError,
		},
		"A data source that hands out no SQL client": {
			Type:   datasource.TypePrometheus,
			Runner: sqlRunner(nil),
			Args:   `{"data_source_id":"` + id + `","query":"select 1"}`,
			Err:    assert.AnError,
		},
		"A failing read": {
			Type:   datasource.TypePostgreSQL,
			Runner: sqlRunner(client),
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

			runDataSourceCase(t, getSQLQueryLabels{}, NameGetSQLQueryLabels, c)
		})
	}
}

func Test_querySQLArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, querySQLArgs{DataSourceID: _testDataSourceID, Query: "select 1"}, map[string]Args{
		"data_source_id": querySQLArgs{Query: "select 1"},
		"query":          querySQLArgs{DataSourceID: _testDataSourceID},
	})
}

func Test_querySQL_Info(t *testing.T) {
	t.Parallel()

	info := querySQL{}.Info()

	assert.Equal(t, NameQuerySQL, info.Name)
	assert.Equal(t, []string{_keyDataSourceID, _keyQuery}, info.Required)
	assert.Contains(t, info.Properties, _keyChartType)
}

func Test_querySQL_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{DataSource: true}, querySQL{}.Traits())
}

func Test_querySQL_Title(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePostgreSQL, nil)

	got, err := querySQL{}.Title(testInput(
		d,
		NameQuerySQL,
		`{"data_source_id":"`+_testDataSourceID.String()+`","query":"select 1"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, `Querying "prod"`, got)

	_, err = querySQL{}.Title(testInput(d, NameQuerySQL, `{`))
	require.Error(t, err)

	// a data source the organisation does not own is an error, not a
	// label: the model is told what it named does not exist.
	_, err = querySQL{}.Title(testInput(d, NameQuerySQL, `{"data_source_id":"`+xid.New().String()+`","query":"select 1"}`))
	require.Error(t, err)
}

func Test_querySQL_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	rows := [][]any{
		{float64(1), float64(2)},
	}

	pg := &datasourceMock.PostgreSQL{
		QueryFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
			if q == "boom" {
				return nil, assert.AnError
			}

			// a query the data source answered with nothing at all: the
			// result is absent, not an error.
			if q == "empty" {
				//nolint:nilnil // an absent result is what the executor reports here
				return nil, nil
			}

			return &processor.PostgreSQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}, nil
		},
	}

	my := &datasourceMock.MySQL{
		QueryFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.MySQLQueryResult, error) {
			if q == "boom" {
				return nil, assert.AnError
			}

			if q == "empty" {
				//nolint:nilnil // an absent result is what the executor reports here
				return nil, nil
			}

			return &processor.MySQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}, nil
		},
	}

	cc := map[string]dataSourceCase{
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
		"A PostgreSQL chart query with no rows at all is no-data": {
			Type:     datasource.TypePostgreSQL,
			Runner:   dialectRunner(datasource.TypePostgreSQL, pg, my),
			Args:     `{"data_source_id":"` + id + `","query":"empty","chart_type":"line_chart"}`,
			Contains: `"status":"no-data"`,
		},
		"A MySQL chart query with no rows at all is no-data": {
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

			runDataSourceCase(t, querySQL{}, NameQuerySQL, c)
		})
	}
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

func Test_dataSourceProps(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]any{_keyDataSourceID: stringProp(_descDataSourceID)}, dataSourceProps(nil))

	props := dataSourceProps(map[string]any{
		_keyQuery: stringProp("q"),
	})
	assert.Contains(t, props, _keyDataSourceID)
	assert.Contains(t, props, _keyQuery)
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
		"Nil result reads as no data": {
			Input:  nil,
			Result: chartPreview{Status: processor.QueryStatusNoData},
		},
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

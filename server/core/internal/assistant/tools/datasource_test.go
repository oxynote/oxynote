package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/variable"
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

// dataSourceDB builds a database whose data-source lookups answer with
// one data source of the given type.
func dataSourceDB(typ datasource.Type) *DBMock {
	ds := datasource.DataSource{
		ID:     _testDataSourceID,
		Name:   "prod",
		Type:   typ,
		URL:    "http://prometheus:9090",
		Status: processor.ConnectionStatusSuccess,
	}

	return &DBMock{
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
}

// dataSourceDeps builds session wiring whose data-source lookups answer
// with one data source of the given type, and whose runner is the given
// mock.
func dataSourceDeps(t *testing.T, typ datasource.Type, runner *datasourceMock.Runner) *Deps {
	t.Helper()

	if runner == nil {
		runner = &datasourceMock.Runner{}
	}

	d := testDeps(dataSourceDB(typ), nil, nil)
	d.runners = &DataSourceRunnersMock{
		RunnerFunc: func(datasource.DataSource) datasource.Runner { return runner },
	}

	return d
}

// dataSourceCase is one data-source tool call: the wiring it runs
// against, the arguments the model supplied, and what it should produce.
type dataSourceCase struct {
	// Type is the data source's type. The lookup reports it, and the
	// runner says it is what it reads.
	Type datasource.Type

	// DB answers the data-source lookups. Nil holds one data source of
	// Type.
	DB *DBMock

	// Variables answers the variable reads. Nil holds the stubbed tree.
	Variables *VariableManagerMock

	// Prometheus, SQL, PostgreSQL and MySQL are the clients the runner
	// hands out. Nil is a don't-care client.
	Prometheus *datasourceMock.Prometheus
	SQL        *datasourceMock.SQL
	PostgreSQL *datasourceMock.PostgreSQL
	MySQL      *datasourceMock.MySQL

	// NoClient indicates the runner hands out no client at all, which is
	// what the real runner reports for the wrong type or a failed
	// connection.
	NoClient bool

	// Args is the raw JSON the model supplied.
	Args string

	// Contains is a fragment the result has to carry.
	Contains string

	// Omits are fragments the result must not carry.
	Omits []string

	// Err is the expected failure, if any.
	Err error

	// Calls is how often the call reaches each mocked method.
	Calls dataSourceCalls
}

// dataSourceCalls counts the calls a data-source tool makes to each
// mocked method it can reach. Each field is named after its method. The
// runner's accessors take the name of the client they hand out, and a
// method two clients share is prefixed with its client.
type dataSourceCalls struct {
	FetchDataSources   int
	FetchDataSource    int
	Runner             int
	FetchTree          int
	Type               int
	Prometheus         int
	SQL                int
	PostgreSQL         int
	MySQL              int
	PrometheusMetadata int
	LabelNames         int
	LabelValues        int
	Series             int
	QueryRange         int
	SQLMetadata        int
	PostgreSQLQuery    int
	MySQLQuery         int
}

// runDataSourceCase executes a data-source tool and asserts the outcome
// and the calls it made.
func runDataSourceCase(t *testing.T, tl Tool, name Name, c dataSourceCase) {
	t.Helper()

	variables := cmp.Or(c.Variables, stubVariableManager())
	prom := cmp.Or(c.Prometheus, &datasourceMock.Prometheus{})
	sql := cmp.Or(c.SQL, &datasourceMock.SQL{})
	pg := cmp.Or(c.PostgreSQL, &datasourceMock.PostgreSQL{})
	my := cmp.Or(c.MySQL, &datasourceMock.MySQL{})

	runner := &datasourceMock.Runner{
		TypeFunc: func() datasource.Type { return c.Type },
		PrometheusFunc: func(context.Context) (datasource.Prometheus, error) {
			if c.NoClient {
				return nil, assert.AnError
			}

			return prom, nil
		},
		SQLFunc: func(context.Context) (datasource.SQL, error) {
			if c.NoClient {
				return nil, assert.AnError
			}

			return sql, nil
		},
		PostgreSQLFunc: func(context.Context) (datasource.PostgreSQL, error) {
			if c.NoClient {
				return nil, assert.AnError
			}

			return pg, nil
		},
		MySQLFunc: func(context.Context) (datasource.MySQL, error) {
			if c.NoClient {
				return nil, assert.AnError
			}

			return my, nil
		},
	}

	d := dataSourceDeps(t, c.Type, runner)
	d.variableMan = variables

	if c.DB != nil {
		d.db = c.DB
	}

	db, ok := d.db.(*DBMock)
	require.True(t, ok)

	runners, ok := d.runners.(*DataSourceRunnersMock)
	require.True(t, ok)

	got, err := tl.Execute(d.newInput(context.Background(), name, json.RawMessage(c.Args)))

	testutil.AssertEqualError(t, c.Err, err)

	calls := dataSourceCalls{
		FetchDataSources:   len(db.FetchDataSourcesCalls()),
		FetchDataSource:    len(db.FetchDataSourceCalls()),
		Runner:             len(runners.RunnerCalls()),
		FetchTree:          len(variables.FetchTreeCalls()),
		Type:               len(runner.TypeCalls()),
		Prometheus:         len(runner.PrometheusCalls()),
		SQL:                len(runner.SQLCalls()),
		PostgreSQL:         len(runner.PostgreSQLCalls()),
		MySQL:              len(runner.MySQLCalls()),
		PrometheusMetadata: len(prom.MetadataCalls()),
		LabelNames:         len(prom.LabelNamesCalls()),
		LabelValues:        len(prom.LabelValuesCalls()),
		Series:             len(prom.SeriesCalls()),
		QueryRange:         len(prom.QueryRangeCalls()),
		SQLMetadata:        len(sql.MetadataCalls()),
		PostgreSQLQuery:    len(pg.QueryCalls()),
		MySQLQuery:         len(my.QueryCalls()),
	}

	assert.Equal(t, c.Calls, calls)

	if c.Err != nil {
		return
	}

	assert.Contains(t, got, c.Contains)

	for _, absent := range c.Omits {
		assert.NotContains(t, got, absent)
	}
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
			Type:  datasource.TypePrometheus,
			Args:  argsFor(xid.New().String()),
			Err:   assert.AnError,
			Calls: dataSourceCalls{FetchDataSource: 1},
		},
	}
}

func Test_listDataSources_Info(t *testing.T) {
	t.Parallel()

	info := listDataSources{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources}, info.Traits)

	assert.Equal(t, NameListDataSources, info.Name)
	assert.Empty(t, info.Required)
	assert.Empty(t, info.Properties)
}

func Test_listDataSources_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]dataSourceCase{
		// the id, the name, the type and the status are what a tool needs to
		// be addressed; the URL and the credentials are the organisation's
		// secret and never reach the model.
		"Data sources of the organisation": {
			Type:     datasource.TypePrometheus,
			Contains: `{"id":"` + _testDataSourceID.String() + `","name":"prod","type":"prometheus","status":"success"}`,
			Omits:    []string{"prometheus:9090", "credentials"},
			Calls:    dataSourceCalls{FetchDataSources: 1},
		},
		"Error returned by DB.FetchDataSources": {
			DB: &DBMock{
				FetchDataSourcesFunc: func(context.Context, string) ([]datasource.DataSource, error) {
					return nil, assert.AnError
				},
			},
			Err:   assert.AnError,
			Calls: dataSourceCalls{FetchDataSources: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, listDataSources{}, NameListDataSources, c)
		})
	}
}

func Test_dataSourceArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args dataSourceArgs
		Err  error
	}{
		"Data source id is required": {
			Args: dataSourceArgs{},
			Err:  errors.New("data_source_id is required"),
		},
		"Valid arguments": {
			Args: dataSourceArgs{DataSourceID: _testDataSourceID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_getDataSourceMetadata_Info(t *testing.T) {
	t.Parallel()

	info := getDataSourceMetadata{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources}, info.Traits)

	assert.Equal(t, NameGetDataSourceMetadata, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
}

func Test_getDataSourceMetadata_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args            string
		Expected        string
		FetchDataSource int
	}{
		"A data source that resolves is named": {
			Args:            `{"data_source_id":"` + _testDataSourceID.String() + `"}`,
			Expected:        `Reading what "prod" holds`,
			FetchDataSource: 1,
		},
		"One that does not is an error": {
			Args:            `{"data_source_id":"` + xid.New().String() + `"}`,
			FetchDataSource: 1,
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

			db := dataSourceDB(datasource.TypePrometheus)

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, getDataSourceMetadata{}.Title(testDeps(db, nil, nil).newInput(context.Background(), NameGetDataSourceMetadata, json.RawMessage(c.Args))))
			assert.Len(t, db.FetchDataSourceCalls(), c.FetchDataSource)
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
		"Metrics of a Prometheus data source": {
			Type:       datasource.TypePrometheus,
			Prometheus: prom,
			Args:       `{"data_source_id":"` + id + `"}`,
			Contains:   "gauge",
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1, PrometheusMetadata: 1},
		},
		"Tables of a SQL data source each with its column names": {
			Type:     datasource.TypePostgreSQL,
			SQL:      sql,
			Args:     `{"data_source_id":"` + id + `"}`,
			Contains: `{"default_schema":"public","tables":{"public.orders":["id","total"]}}`,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, SQL: 1, SQLMetadata: 1},
		},
		"Unreadable arguments": {
			Type: datasource.TypePrometheus,
			Args: `{`,
			Err:  assert.AnError,
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePrometheus,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `"}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1},
		},
		"Error returned by Runner.SQL": {
			Type:     datasource.TypePostgreSQL,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `"}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, SQL: 1},
		},
		"Error returned by Prometheus.Metadata": {
			Type: datasource.TypePrometheus,
			Prometheus: &datasourceMock.Prometheus{
				MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) {
					return nil, assert.AnError
				},
			},
			Args:  `{"data_source_id":"` + id + `"}`,
			Err:   assert.AnError,
			Calls: dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1, PrometheusMetadata: 1},
		},
		"Error returned by SQL.Metadata": {
			Type: datasource.TypePostgreSQL,
			SQL: &datasourceMock.SQL{
				MetadataFunc: func(context.Context) (*processor.SQLMetadataResult, error) {
					return nil, assert.AnError
				},
			},
			Args:  `{"data_source_id":"` + id + `"}`,
			Err:   assert.AnError,
			Calls: dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, SQL: 1, SQLMetadata: 1},
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

	cc := map[string]struct {
		Args prometheusLabelsArgs
		Err  error
	}{
		"Data source id is required": {
			Args: prometheusLabelsArgs{Label: "job"},
			Err:  errors.New("data_source_id is required"),
		},
		"Valid arguments": {
			Args: prometheusLabelsArgs{DataSourceID: _testDataSourceID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_listPrometheusLabels_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusLabels{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources}, info.Traits)

	assert.Equal(t, NameListPrometheusLabels, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
	assert.Contains(t, info.Properties, "label")
	assert.Contains(t, info.Properties, "matchers")
	assert.Contains(t, info.Properties, "from")
}

func Test_listPrometheusLabels_Title(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	cc := map[string]struct {
		Args            string
		Expected        string
		FetchDataSource int
	}{
		"The label names": {
			Args:            `{"data_source_id":"` + id + `"}`,
			Expected:        `Listing labels of "prod"`,
			FetchDataSource: 1,
		},
		"Values of one label": {
			Args:            `{"data_source_id":"` + id + `","label":"job"}`,
			Expected:        `Listing values of label "job" in "prod"`,
			FetchDataSource: 1,
		},
		// a data source the organisation does not own is an error, not a
		// label: the model is told what it named does not exist.
		"A data source of another organisation": {
			Args:            `{"data_source_id":"` + xid.New().String() + `"}`,
			FetchDataSource: 1,
		},
		"Unreadable arguments": {
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := dataSourceDB(datasource.TypePrometheus)

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, listPrometheusLabels{}.Title(testDeps(db, nil, nil).newInput(context.Background(), NameListPrometheusLabels, json.RawMessage(c.Args))))
			assert.Len(t, db.FetchDataSourceCalls(), c.FetchDataSource)
		})
	}
}

func Test_listPrometheusLabels_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			LabelNamesFunc: func(_ context.Context, matchers []string, tr processor.TimeRange) (*processor.PrometheusLabelNamesResult, error) {
				if len(matchers) > 0 && (matchers[0] == "boom" || strings.Contains(matchers[0], "$")) {
					return nil, assert.AnError
				}

				assert.Equal(t, time.Hour, tr.To.Sub(tr.From))

				return &processor.PrometheusLabelNamesResult{Result: []string{"job"}}, nil
			},
			LabelValuesFunc: func(_ context.Context, label string, matchers []string, _ processor.TimeRange) (*processor.PrometheusLabelValuesResult, error) {
				if label == "boom" || (len(matchers) > 0 && strings.Contains(matchers[0], "$")) {
					return nil, assert.AnError
				}

				return &processor.PrometheusLabelValuesResult{Result: []string{"api"}}, nil
			},
		}
	}

	// a $ left in a matcher fails the read, so the substitution cases pass
	// only when every reference was replaced from stubVariableTree.
	cc := map[string]dataSourceCase{
		"The label names with variables substituted": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["up{region=\"$region\"}"]}`,
			Contains:   `["job"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Prometheus: 1, LabelNames: 1},
		},
		"The label values with variables substituted": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","label":"job","matchers":["up{region=\"$region\"}"]}`,
			Contains:   `["api"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Prometheus: 1, LabelValues: 1},
		},
		"Error returned by VariableManager.FetchTree": {
			Type:       datasource.TypePrometheus,
			Variables:  failingTreeManager(),
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["up{region=\"$region\"}"]}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1},
		},
		"The label names on the selected series": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["up"]}`,
			Contains:   `["job"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1, LabelNames: 1},
		},
		"The values a label takes": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","label":"job"}`,
			Contains:   `["api"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1, LabelValues: 1},
		},
		"Unreadable arguments": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{`,
			Err:        assert.AnError,
		},
		"An unparseable range start": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","from":"yesterday"}`,
			Err:        assert.AnError,
		},
		"An inverted range": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:        assert.AnError,
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePostgreSQL,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `"}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1},
		},
		"Error returned by Prometheus.LabelNames": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["boom"]}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1, LabelNames: 1},
		},
		"Error returned by Prometheus.LabelValues": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","label":"boom"}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1, LabelValues: 1},
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

	cc := map[string]struct {
		Args prometheusSeriesArgs
		Err  error
	}{
		"Data source id is required": {
			Args: prometheusSeriesArgs{Matchers: []string{"up"}},
			Err:  errors.New("data_source_id is required"),
		},
		"Matchers are required": {
			Args: prometheusSeriesArgs{DataSourceID: _testDataSourceID},
			Err:  errors.New("matchers is required"),
		},
		"Valid arguments": {
			Args: prometheusSeriesArgs{DataSourceID: _testDataSourceID, Matchers: []string{"up"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_listPrometheusSeries_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusSeries{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources}, info.Traits)

	assert.Equal(t, NameListPrometheusSeries, info.Name)
	assert.Equal(t, []string{"data_source_id", "matchers"}, info.Required)
}

func Test_listPrometheusSeries_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args            string
		Expected        string
		FetchDataSource int
	}{
		"A data source that resolves is named": {
			Args:            `{"data_source_id":"` + _testDataSourceID.String() + `","matchers":["up"]}`,
			Expected:        `Listing series of "prod"`,
			FetchDataSource: 1,
		},
		"Unreadable arguments": {
			Args: `{`,
		},
		"A data source of another organisation": {
			Args:            `{"data_source_id":"` + xid.New().String() + `","matchers":["up"]}`,
			FetchDataSource: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := dataSourceDB(datasource.TypePrometheus)

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, listPrometheusSeries{}.Title(testDeps(db, nil, nil).newInput(context.Background(), NameListPrometheusSeries, json.RawMessage(c.Args))))
			assert.Len(t, db.FetchDataSourceCalls(), c.FetchDataSource)
		})
	}
}

func Test_listPrometheusSeries_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	client := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			SeriesFunc: func(_ context.Context, matchers []string, _ processor.TimeRange) (*processor.PrometheusSeriesResult, error) {
				if matchers[0] == "boom" || strings.Contains(matchers[0], "$") {
					return nil, assert.AnError
				}

				return &processor.PrometheusSeriesResult{
					Result: []model.LabelSet{
						{"job": "api"},
					},
				}, nil
			},
		}
	}

	// a $ left in a matcher fails the read, so the substitution case passes
	// only when every reference was replaced from stubVariableTree.
	cc := map[string]dataSourceCase{
		"The series with variables substituted": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["{region=\"$region\"}"]}`,
			Contains:   "api",
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Prometheus: 1, Series: 1},
		},
		"Error returned by VariableManager.FetchTree": {
			Type:       datasource.TypePrometheus,
			Variables:  failingTreeManager(),
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["{region=\"$region\"}"]}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1},
		},
		"The series the matchers select": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["{job=\"api\"}"]}`,
			Contains:   "api",
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1, Series: 1},
		},
		"Unreadable arguments": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{`,
			Err:        assert.AnError,
		},
		// Prometheus rejects a series query with no selector, so the model
		// is told what is missing rather than handed the upstream error.
		"No matcher at all": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `"}`,
			Err:        assert.AnError,
		},
		"An empty matcher list": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":[]}`,
			Err:        assert.AnError,
		},
		"An unparseable range": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["up"],"from":"then"}`,
			Err:        assert.AnError,
		},
		"An inverted range": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["up"],"from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:        assert.AnError,
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePostgreSQL,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `","matchers":["up"]}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1},
		},
		"Error returned by Prometheus.Series": {
			Type:       datasource.TypePrometheus,
			Prometheus: client(),
			Args:       `{"data_source_id":"` + id + `","matchers":["boom"]}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Prometheus: 1, Series: 1},
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

	cc := map[string]struct {
		Args queryDataSourceArgs
		Err  error
	}{
		"Data source id is required": {
			Args: queryDataSourceArgs{Query: "up"},
			Err:  errors.New("data_source_id is required"),
		},
		"Query is required": {
			Args: queryDataSourceArgs{DataSourceID: _testDataSourceID},
			Err:  errors.New("query is required"),
		},
		"Valid arguments": {
			Args: queryDataSourceArgs{DataSourceID: _testDataSourceID, Query: "up"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_queryDataSource_Info(t *testing.T) {
	t.Parallel()

	info := queryDataSource{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources}, info.Traits)

	assert.Equal(t, NameQueryDataSource, info.Name)
	assert.Equal(t, []string{"data_source_id", "query"}, info.Required)

	chartType, ok := info.Properties["chart_type"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, []processor.ChartType{processor.ChartTypeLine, processor.ChartTypeBar, processor.ChartTypeGauge}, chartType["enum"])
}

func Test_queryDataSource_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args            string
		Expected        string
		FetchDataSource int
	}{
		"A data source that resolves is named": {
			Args:            `{"data_source_id":"` + _testDataSourceID.String() + `","query":"up"}`,
			Expected:        `Querying "prod"`,
			FetchDataSource: 1,
		},
		// a data source the organisation does not own is an error, not a
		// label: the model is told what it named does not exist.
		"A data source of another organisation": {
			Args:            `{"data_source_id":"` + xid.New().String() + `","query":"up"}`,
			FetchDataSource: 1,
		},
		"Unreadable arguments": {
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := dataSourceDB(datasource.TypePrometheus)

			// arguments that name nothing it can describe make no line.
			assert.Equal(t, c.Expected, queryDataSource{}.Title(testDeps(db, nil, nil).newInput(context.Background(), NameQueryDataSource, json.RawMessage(c.Args))))
			assert.Len(t, db.FetchDataSourceCalls(), c.FetchDataSource)
		})
	}
}

func Test_queryDataSource_Execute(t *testing.T) {
	t.Parallel()

	id := _testDataSourceID.String()

	prom := func(res *processor.PrometheusQueryResult) *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				if q == "boom" || strings.Contains(q, "$") {
					return nil, assert.AnError
				}

				return res, nil
			},
		}
	}

	rows := [][]any{
		{float64(1), float64(2)},
	}

	pg := func() *datasourceMock.PostgreSQL {
		return &datasourceMock.PostgreSQL{
			QueryFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
				switch {
				case q == "boom", strings.Contains(q, "$"):
					return nil, assert.AnError
				case q == "empty":
					// an absent result is what the executor reports here.
					return nil, nil
				default:
					return &processor.PostgreSQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}, nil
				}
			},
		}
	}

	my := func() *datasourceMock.MySQL {
		return &datasourceMock.MySQL{
			QueryFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.MySQLQueryResult, error) {
				switch {
				case q == "boom", strings.Contains(q, "$"):
					return nil, assert.AnError
				case q == "empty":
					// an absent result is what the executor reports here.
					return nil, nil
				default:
					return &processor.MySQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}, nil
				}
			},
		}
	}

	// a $ left in a query fails it, so these pass only when every
	// reference was substituted from stubVariableTree. Both dialect
	// clients are handed out, so the tool's own dispatch decides which
	// one is reached.
	cc := map[string]dataSourceCase{
		"Prometheus substituted variables": {
			Type:       datasource.TypePrometheus,
			Prometheus: prom(&processor.PrometheusQueryResult{Type: model.ValMatrix, Warnings: []string{"slow"}}),
			Args:       `{"data_source_id":"` + id + `","query":"up{region=\"$region\",range=\"$metric_range\"}"}`,
			Contains:   `"warnings":["slow"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"PostgreSQL substituted variables": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select '$region'"}`,
			Contains:   `"columns":["time","value"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"MySQL substituted variables": {
			Type:       datasource.TypeMySQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select '$region'"}`,
			Contains:   `"columns":["time","value"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"Variables of the acting organization and user only": {
			Type: datasource.TypePostgreSQL,
			Variables: &VariableManagerMock{
				FetchTreeFunc: func(_ context.Context, orgID, userID string) (variable.Tree, error) {
					if orgID != "org" || userID != "user" {
						return variable.Tree{}, nil
					}

					return stubVariableTree(), nil
				},
			},
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select '$region'"}`,
			Contains:   `"columns":["time","value"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"Error returned by VariableManager.FetchTree": {
			Type:       datasource.TypePostgreSQL,
			Variables:  failingTreeManager(),
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select '$region'"}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, FetchTree: 1},
		},
		"Prometheus raw": {
			Type:       datasource.TypePrometheus,
			Prometheus: prom(&processor.PrometheusQueryResult{Type: model.ValMatrix, Warnings: []string{"slow"}}),
			Args:       `{"data_source_id":"` + id + `","query":"up"}`,
			Contains:   `"warnings":["slow"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"Prometheus charted": {
			Type:       datasource.TypePrometheus,
			Prometheus: prom(&processor.PrometheusQueryResult{Type: model.ValMatrix}),
			Args:       `{"data_source_id":"` + id + `","query":"up","chart_type":"line_chart"}`,
			Contains:   `"status"`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		// a query that returned nothing has no result to transform, and the
		// metric block renders it as no-data.
		"A Prometheus query with no result at all is no-data": {
			Type:       datasource.TypePrometheus,
			Prometheus: prom(nil),
			Args:       `{"data_source_id":"` + id + `","query":"up","chart_type":"gauge_chart"}`,
			Contains:   `"status":"no-data"`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"PostgreSQL raw": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1"}`,
			Contains:   `"columns":["time","value"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"PostgreSQL charted": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1","chart_type":"line_chart"}`,
			Contains:   `"status"`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"A PostgreSQL query with no rows at all is no-data": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"empty","chart_type":"line_chart"}`,
			Contains:   `"status":"no-data"`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"MySQL raw": {
			Type:       datasource.TypeMySQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1"}`,
			Contains:   `"columns":["time","value"]`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"MariaDB charted": {
			Type:       datasource.TypeMariaDB,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1","chart_type":"bar_chart"}`,
			Contains:   `"status"`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"A MySQL query with no rows at all is no-data": {
			Type:       datasource.TypeMySQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"empty","chart_type":"line_chart"}`,
			Contains:   `"status":"no-data"`,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"Unreadable arguments": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{`,
			Err:        assert.AnError,
		},
		"No query": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `"}`,
			Err:        assert.AnError,
		},
		"An unknown chart type": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1","chart_type":"donut"}`,
			Err:        assert.AnError,
		},
		"An unparseable range": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1","from":"epoch"}`,
			Err:        assert.AnError,
		},
		"An inverted range": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"select 1","from":"2026-08-01T12:00:00Z","to":"2026-08-01T10:00:00Z"}`,
			Err:        assert.AnError,
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePrometheus,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `","query":"up"}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1},
		},
		"Error returned by Runner.PostgreSQL": {
			Type:     datasource.TypePostgreSQL,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `","query":"select 1"}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, PostgreSQL: 1},
		},
		"Error returned by Runner.MySQL": {
			Type:     datasource.TypeMySQL,
			NoClient: true,
			Args:     `{"data_source_id":"` + id + `","query":"select 1"}`,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, MySQL: 1},
		},
		"Error returned by Prometheus.QueryRange": {
			Type:       datasource.TypePrometheus,
			Prometheus: prom(nil),
			Args:       `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"Error returned by PostgreSQL.Query": {
			Type:       datasource.TypePostgreSQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"Error returned by MySQL.Query": {
			Type:       datasource.TypeMySQL,
			PostgreSQL: pg(),
			MySQL:      my(),
			Args:       `{"data_source_id":"` + id + `","query":"boom"}`,
			Err:        assert.AnError,
			Calls:      dataSourceCalls{FetchDataSource: 1, Runner: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
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

	matrix := func(series, points int) model.Matrix {
		out := make(model.Matrix, 0, series)

		for range series {
			s := &model.SampleStream{Metric: model.Metric{"job": "api"}}

			for i := range points {
				s.Values = append(s.Values, model.SamplePair{Timestamp: model.Time(i), Value: model.SampleValue(i)})
			}

			out = append(out, s)
		}

		return out
	}

	cc := map[string]struct {
		Input     *processor.PrometheusQueryResult
		Series    int
		Points    int
		Truncated string
		Result    *rawSeries
	}{
		// an answer that is not a range of series has nothing to cap.
		"An answer that is not a range of series is as it was": {
			Input:  &processor.PrometheusQueryResult{Type: model.ValScalar, Result: &model.Scalar{Value: 1}},
			Result: &rawSeries{Type: model.ValScalar, Result: &model.Scalar{Value: 1}},
		},
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

			if c.Result != nil {
				assert.Equal(t, *c.Result, got)
				return
			}

			assert.Equal(t, c.Truncated, got.Truncated)

			m, ok := got.Result.(model.Matrix)
			require.True(t, ok)
			require.Len(t, m, c.Series)
			assert.Len(t, m[0].Values, c.Points)
			// the points kept are the latest ones.
			assert.InDelta(t, float64(len(c.Input.Result.(model.Matrix)[0].Values)-1), float64(m[0].Values[len(m[0].Values)-1].Value), 0)
		})
	}
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

	cc := map[string]struct {
		Rows   int
		Result rawRows
	}{
		"An answer within the cap is as it was": {
			Rows:   2,
			Result: rawRows{Columns: []string{"a"}, Rows: rows(2)},
		},
		"Too many rows keeps the first ones": {
			Rows:   _maxRawRows + 50,
			Result: rawRows{Columns: []string{"a"}, Rows: rows(_maxRawRows), TotalRows: _maxRawRows + 50},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newRawRows([]string{"a"}, rows(c.Rows)))
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
		Check  func(t *testing.T, got chartPreview)
	}{
		"Status without data carries no series": {
			Input:  &processor.QueryResult{Status: processor.QueryStatusNoData},
			Result: chartPreview{Status: processor.QueryStatusNoData},
		},
		"Series report their labels and endpoints but never their points": {
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
		// the cap bounds the answer without hiding how much there was.
		"More series than the cap are counted but not listed": {
			Input: &processor.QueryResult{
				Status: processor.QueryStatusOK,
				Data:   series(_maxPreviewSeries + 5),
			},
			Check: func(t *testing.T, got chartPreview) {
				t.Helper()

				assert.Equal(t, _maxPreviewSeries+5, got.SeriesCount)
				assert.Len(t, got.Series, _maxPreviewSeries)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := newChartPreview(c.Input)

			if c.Check != nil {
				c.Check(t, got)
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

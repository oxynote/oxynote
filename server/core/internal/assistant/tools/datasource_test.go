package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/org"
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
// with one Prometheus data source, and whose runner is the given mock.
func dataSourceDeps(t *testing.T, runner *datasourceMock.Runner) *Deps {
	t.Helper()

	if runner == nil {
		runner = &datasourceMock.Runner{}
	}

	d := testDeps(dataSourceDB(datasource.TypePrometheus), nil, nil)
	d.runners = &DataSourceRunnersMock{
		RunnerFunc: func(datasource.DataSource) datasource.Runner { return runner },
	}

	return d
}

// stubDataSource is the data source of the given type every
// data-source stub holds.
func stubDataSource(typ datasource.Type) *datasource.DataSource {
	return &datasource.DataSource{
		ID:     _testDataSourceID,
		Name:   "prod",
		Type:   typ,
		URL:    "http://prometheus:9090",
		Status: processor.ConnectionStatusSuccess,
	}
}

// invertedRange is a time range that ends before it starts.
func invertedRange() timeRange {
	from := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	return timeRange{From: from, To: from.Add(-2 * time.Hour)}
}

// dataSourceCase is one data-source tool call: the stubs it runs
// against, the tool with its arguments, and what it should produce.
type dataSourceCase struct {
	// Type is the data source's type, which the runner says it reads.
	Type datasource.Type

	// Tool is the call.
	Tool Tool

	// RunnerErr is what Input.DataSourceRunner fails with.
	RunnerErr error

	// VariablesErr is what the variable reads fail with. Otherwise they
	// answer from stubVariableTree.
	VariablesErr error

	// Prometheus, SQL, PostgreSQL, MySQL and Logs are the clients the
	// runner hands out. Nil is a don't-care client.
	Prometheus *datasourceMock.Prometheus
	SQL        *datasourceMock.SQL
	PostgreSQL *datasourceMock.PostgreSQL
	MySQL      *datasourceMock.MySQL
	Logs       *datasourceMock.Logs

	// NoClient indicates the runner hands out no client at all, which is
	// what the real runner reports for the wrong type or a failed
	// connection.
	NoClient bool

	// Result is the expected result.
	Result any

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
	Runner             int
	Values             int
	Substitute         int
	Type               int
	Prometheus         int
	SQL                int
	PostgreSQL         int
	MySQL              int
	Logs               int
	PrometheusMetadata int
	LabelNames         int
	LabelValues        int
	Series             int
	QueryRange         int
	SQLMetadata        int
	PostgreSQLQuery    int
	MySQLQuery         int
	LogFields          int
	LogFieldValues     int
	LogsLogs           int
}

// runDataSourceCase runs a data-source tool over a stub input and
// asserts the outcome and the calls it made.
func runDataSourceCase(t *testing.T, c dataSourceCase) {
	t.Helper()

	prom := cmp.Or(c.Prometheus, &datasourceMock.Prometheus{})
	sql := cmp.Or(c.SQL, &datasourceMock.SQL{})
	pg := cmp.Or(c.PostgreSQL, &datasourceMock.PostgreSQL{})
	my := cmp.Or(c.MySQL, &datasourceMock.MySQL{})
	logs := cmp.Or(c.Logs, &datasourceMock.Logs{})

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
		LogsFunc: func(context.Context) (datasource.Logs, error) {
			if c.NoClient {
				return nil, assert.AnError
			}

			return logs, nil
		},
	}

	values := func() (map[string]string, error) {
		if c.VariablesErr != nil {
			return nil, c.VariablesErr
		}

		return stubVariableTree().Values(nil)
	}

	inp := &InputMock{
		ContextFunc: context.Background,
		DataSourceRunnerFunc: func(xid.ID) (datasource.Runner, error) {
			if c.RunnerErr != nil {
				return nil, c.RunnerErr
			}

			return runner, nil
		},
		VariableValuesFunc: func(...string) (map[string]string, error) {
			return values()
		},
		SubstituteVariablesFunc: func(texts ...string) ([]string, error) {
			vv, err := values()
			if err != nil {
				return nil, err
			}

			return variable.Substitute(vv, texts...), nil
		},
	}

	got, err := c.Tool.Exec(inp)
	testutil.AssertEqualError(t, c.Err, err)

	calls := dataSourceCalls{
		Runner:             len(inp.DataSourceRunnerCalls()),
		Values:             len(inp.VariableValuesCalls()),
		Substitute:         len(inp.SubstituteVariablesCalls()),
		Type:               len(runner.TypeCalls()),
		Prometheus:         len(runner.PrometheusCalls()),
		SQL:                len(runner.SQLCalls()),
		PostgreSQL:         len(runner.PostgreSQLCalls()),
		MySQL:              len(runner.MySQLCalls()),
		Logs:               len(runner.LogsCalls()),
		PrometheusMetadata: len(prom.MetadataCalls()),
		LabelNames:         len(prom.LabelNamesCalls()),
		LabelValues:        len(prom.LabelValuesCalls()),
		Series:             len(prom.SeriesCalls()),
		QueryRange:         len(prom.QueryRangeCalls()),
		SQLMetadata:        len(sql.MetadataCalls()),
		PostgreSQLQuery:    len(pg.QueryCalls()),
		MySQLQuery:         len(my.QueryCalls()),
		LogFields:          len(logs.LogFieldsCalls()),
		LogFieldValues:     len(logs.LogFieldValuesCalls()),
		LogsLogs:           len(logs.LogsCalls()),
	}

	assert.Equal(t, c.Calls, calls)

	if err != nil {
		return
	}

	assert.Equal(t, c.Result, got)
}

// stubDataSourceInput answers the data-source lookup with the test
// data source, or fails with err.
func stubDataSourceInput(err error) *InputMock {
	return &InputMock{
		FetchDataSourceFunc: func(xid.ID) (*datasource.DataSource, error) {
			if err != nil {
				return nil, err
			}

			return stubDataSource(datasource.TypePrometheus), nil
		},
	}
}

func Test_listDataSources_Info(t *testing.T) {
	t.Parallel()

	info := listDataSources{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameListDataSources, info.Name)
	assert.Empty(t, info.Required)
	assert.Empty(t, info.Properties)
	assert.Contains(t, info.Description, "prometheus, loki, postgresql")
	assert.Contains(t, info.Description, "loki holds logs")
}

func Test_listDataSources_Validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, listDataSources{}.Validate())
}

func Test_listDataSources_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		// the id, the name, the type and the status are what a tool needs
		// to be addressed. The URL and the credentials are the
		// organisation's secret and never reach the model.
		"Data sources of the organisation": {
			Input: &InputMock{
				FetchDataSourcesFunc: func() ([]datasource.DataSource, error) {
					return []datasource.DataSource{*stubDataSource(datasource.TypePrometheus)}, nil
				},
			},
			Result: listDataSourcesResult{DataSources: []dataSourceInfo{{
				ID:     _testDataSourceID,
				Name:   "prod",
				Type:   datasource.TypePrometheus,
				Status: processor.ConnectionStatusSuccess,
			}}},
		},
		"Error returned by Input.FetchDataSources": {
			Input: &InputMock{
				FetchDataSourcesFunc: func() ([]datasource.DataSource, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listDataSources{}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchDataSourcesCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_getDataSourceMetadata_Info(t *testing.T) {
	t.Parallel()

	info := getDataSourceMetadata{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameGetDataSourceMetadata, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
}

func Test_getDataSourceMetadata_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool getDataSourceMetadata
		Err  error
	}{
		"Data source id is required": {
			Tool: getDataSourceMetadata{},
			Err:  errors.New("data_source_id is required"),
		},
		"Valid arguments": {
			Tool: getDataSourceMetadata{DataSourceID: _testDataSourceID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_getDataSourceMetadata_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   getDataSourceMetadata
		Result string
	}{
		"Data source that resolves is named": {
			Input:  stubDataSourceInput(nil),
			Tool:   getDataSourceMetadata{DataSourceID: _testDataSourceID},
			Result: `Reading what "prod" holds`,
		},
		"Error returned by Input.FetchDataSource": {
			Input:  stubDataSourceInput(assert.AnError),
			Tool:   getDataSourceMetadata{DataSourceID: _testDataSourceID},
			Result: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.Title(c.Input))

			ff := c.Input.FetchDataSourceCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testDataSourceID, ff[0].DataSourceID)
		})
	}
}

func Test_getDataSourceMetadata_Exec(t *testing.T) {
	t.Parallel()

	tool := getDataSourceMetadata{DataSourceID: _testDataSourceID}
	promResult := &processor.PrometheusMetadataResult{Result: map[string]any{"up": "gauge"}}
	logsResult := &processor.LogFieldsResult{Fields: []processor.LogField{{Name: "app", Kind: processor.LogFieldKindLabel}}}

	cc := map[string]dataSourceCase{
		"Metrics of a Prometheus data source": {
			Type: datasource.TypePrometheus,
			Tool: tool,
			Prometheus: &datasourceMock.Prometheus{
				MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) { return promResult, nil },
			},
			Result: promResult,
			Calls:  dataSourceCalls{Runner: 1, Type: 1, Prometheus: 1, PrometheusMetadata: 1},
		},
		"Tables of a SQL data source each with its column names": {
			Type: datasource.TypePostgreSQL,
			Tool: tool,
			SQL: &datasourceMock.SQL{
				MetadataFunc: func(context.Context) (*processor.SQLMetadataResult, error) {
					return &processor.SQLMetadataResult{
						Tables: map[string]processor.SQLTable{
							"public.orders": {Columns: []processor.SQLColumn{{Name: "id"}, {Name: "total"}}},
						},
						DefaultSchema: "public",
					}, nil
				},
			},
			Result: sqlMetadataResult{DefaultSchema: "public", Tables: map[string][]string{"public.orders": {"id", "total"}}},
			Calls:  dataSourceCalls{Runner: 1, Type: 1, SQL: 1, SQLMetadata: 1},
		},
		// the fields of the last hour, of every log, so no query narrows
		// them.
		"Fields of a Loki data source": {
			Type: datasource.TypeLoki,
			Tool: tool,
			Logs: &datasourceMock.Logs{
				LogFieldsFunc: func(_ context.Context, q string, tr processor.TimeRange) (*processor.LogFieldsResult, error) {
					assert.Empty(t, q)
					assert.Equal(t, time.Hour, tr.To.Sub(tr.From))

					return logsResult, nil
				},
			},
			Result: logsResult,
			Calls:  dataSourceCalls{Runner: 1, Type: 1, Logs: 1, LogFields: 1},
		},
		"Error returned by Input.DataSourceRunner": {
			Type:      datasource.TypePrometheus,
			Tool:      tool,
			RunnerErr: assert.AnError,
			Err:       assert.AnError,
			Calls:     dataSourceCalls{Runner: 1},
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePrometheus,
			Tool:     tool,
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Type: 1, Prometheus: 1},
		},
		"Error returned by Runner.Logs": {
			Type:     datasource.TypeLoki,
			Tool:     tool,
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Type: 1, Logs: 1},
		},
		"Error returned by Logs.LogFields": {
			Type: datasource.TypeLoki,
			Tool: tool,
			Logs: &datasourceMock.Logs{
				LogFieldsFunc: func(context.Context, string, processor.TimeRange) (*processor.LogFieldsResult, error) {
					return nil, assert.AnError
				},
			},
			Err:   assert.AnError,
			Calls: dataSourceCalls{Runner: 1, Type: 1, Logs: 1, LogFields: 1},
		},
		"Error returned by Runner.SQL": {
			Type:     datasource.TypePostgreSQL,
			Tool:     tool,
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Type: 1, SQL: 1},
		},
		"Error returned by Prometheus.Metadata": {
			Type: datasource.TypePrometheus,
			Tool: tool,
			Prometheus: &datasourceMock.Prometheus{
				MetadataFunc: func(context.Context) (*processor.PrometheusMetadataResult, error) { return nil, assert.AnError },
			},
			Err:   assert.AnError,
			Calls: dataSourceCalls{Runner: 1, Type: 1, Prometheus: 1, PrometheusMetadata: 1},
		},
		"Error returned by SQL.Metadata": {
			Type: datasource.TypePostgreSQL,
			Tool: tool,
			SQL: &datasourceMock.SQL{
				MetadataFunc: func(context.Context) (*processor.SQLMetadataResult, error) { return nil, assert.AnError },
			},
			Err:   assert.AnError,
			Calls: dataSourceCalls{Runner: 1, Type: 1, SQL: 1, SQLMetadata: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, c)
		})
	}
}

func Test_listPrometheusLabels_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusLabels{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameListPrometheusLabels, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
	assert.Contains(t, info.Properties, "label")
	assert.Contains(t, info.Properties, "matchers")
	assert.Contains(t, info.Properties, "from")
}

func Test_listPrometheusLabels_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool listPrometheusLabels
		Err  error
	}{
		"Data source id is required": {
			Tool: listPrometheusLabels{Label: "job"},
			Err:  errors.New("data_source_id is required"),
		},
		"Valid arguments": {
			Tool: listPrometheusLabels{DataSourceID: _testDataSourceID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_listPrometheusLabels_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   listPrometheusLabels
		Result string
	}{
		"Label names": {
			Input:  stubDataSourceInput(nil),
			Tool:   listPrometheusLabels{DataSourceID: _testDataSourceID},
			Result: `Listing labels of "prod"`,
		},
		"Values of one label": {
			Input:  stubDataSourceInput(nil),
			Tool:   listPrometheusLabels{DataSourceID: _testDataSourceID, Label: "job"},
			Result: `Listing values of label "job" in "prod"`,
		},
		"Error returned by Input.FetchDataSource": {
			Input:  stubDataSourceInput(assert.AnError),
			Tool:   listPrometheusLabels{DataSourceID: _testDataSourceID},
			Result: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.Title(c.Input))

			ff := c.Input.FetchDataSourceCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testDataSourceID, ff[0].DataSourceID)
		})
	}
}

func Test_listPrometheusLabels_Exec(t *testing.T) {
	t.Parallel()

	names := &processor.PrometheusLabelNamesResult{Result: []string{"job"}}
	values := &processor.PrometheusLabelValuesResult{Result: []string{"api"}}

	client := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			LabelNamesFunc: func(_ context.Context, matchers []string, tr processor.TimeRange) (*processor.PrometheusLabelNamesResult, error) {
				if len(matchers) > 0 && (matchers[0] == "boom" || strings.Contains(matchers[0], "$")) {
					return nil, assert.AnError
				}

				assert.Equal(t, time.Hour, tr.To.Sub(tr.From))

				return names, nil
			},
			LabelValuesFunc: func(_ context.Context, label string, matchers []string, _ processor.TimeRange) (*processor.PrometheusLabelValuesResult, error) {
				if label == "boom" || (len(matchers) > 0 && strings.Contains(matchers[0], "$")) {
					return nil, assert.AnError
				}

				return values, nil
			},
		}
	}

	tool := func(label string, matchers ...string) listPrometheusLabels {
		return listPrometheusLabels{DataSourceID: _testDataSourceID, Label: label, Matchers: matchers}
	}

	// a $ left in a matcher fails the read, so the substitution cases pass
	// only when every reference was replaced.
	cc := map[string]dataSourceCase{
		"The label names with variables substituted": {
			Type:       datasource.TypePrometheus,
			Tool:       tool("", `up{region="$region"}`),
			Prometheus: client(),
			Result:     names,
			Calls:      dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1, LabelNames: 1},
		},
		"The label values with variables substituted": {
			Type:       datasource.TypePrometheus,
			Tool:       tool("job", `up{region="$region"}`),
			Prometheus: client(),
			Result:     values,
			Calls:      dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1, LabelValues: 1},
		},
		"Error returned by Input.SubstituteVariables": {
			Type:         datasource.TypePrometheus,
			Tool:         tool("", `up{region="$region"}`),
			VariablesErr: assert.AnError,
			Prometheus:   client(),
			Err:          assert.AnError,
			Calls:        dataSourceCalls{Runner: 1, Substitute: 1},
		},
		"An inverted range": {
			Type: datasource.TypePrometheus,
			Tool: func() listPrometheusLabels {
				tl := tool("")
				tl.timeRange = invertedRange()

				return tl
			}(),
			Err: _errInvertedTimeRange,
		},
		"Error returned by Input.DataSourceRunner": {
			Type:      datasource.TypePrometheus,
			Tool:      tool(""),
			RunnerErr: assert.AnError,
			Err:       assert.AnError,
			Calls:     dataSourceCalls{Runner: 1},
		},
		// the runner refuses the wrong type, naming it, so the tool adds
		// nothing of its own.
		"A Loki data source is refused": {
			Type:     datasource.TypeLoki,
			Tool:     tool(""),
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1},
		},
		"Error returned by Prometheus.LabelNames": {
			Type:       datasource.TypePrometheus,
			Tool:       tool("", "boom"),
			Prometheus: client(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1, LabelNames: 1},
		},
		"Error returned by Prometheus.LabelValues": {
			Type:       datasource.TypePrometheus,
			Tool:       tool("boom"),
			Prometheus: client(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1, LabelValues: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, c)
		})
	}
}

func Test_listPrometheusSeries_Info(t *testing.T) {
	t.Parallel()

	info := listPrometheusSeries{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameListPrometheusSeries, info.Name)
	assert.Equal(t, []string{"data_source_id", "matchers"}, info.Required)
}

func Test_listPrometheusSeries_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool listPrometheusSeries
		Err  error
	}{
		"Data source id is required": {
			Tool: listPrometheusSeries{Matchers: []string{"up"}},
			Err:  errors.New("data_source_id is required"),
		},
		"Matchers are required": {
			Tool: listPrometheusSeries{DataSourceID: _testDataSourceID},
			Err:  errors.New("matchers is required"),
		},
		"Valid arguments": {
			Tool: listPrometheusSeries{DataSourceID: _testDataSourceID, Matchers: []string{"up"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_listPrometheusSeries_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   listPrometheusSeries
		Result string
	}{
		"Data source that resolves is named": {
			Input:  stubDataSourceInput(nil),
			Tool:   listPrometheusSeries{DataSourceID: _testDataSourceID, Matchers: []string{"up"}},
			Result: `Listing series of "prod"`,
		},
		"Error returned by Input.FetchDataSource": {
			Input:  stubDataSourceInput(assert.AnError),
			Tool:   listPrometheusSeries{DataSourceID: _testDataSourceID, Matchers: []string{"up"}},
			Result: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.Title(c.Input))

			ff := c.Input.FetchDataSourceCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testDataSourceID, ff[0].DataSourceID)
		})
	}
}

func Test_listPrometheusSeries_Exec(t *testing.T) {
	t.Parallel()

	series := &processor.PrometheusSeriesResult{Result: []model.LabelSet{{"job": "api"}}}

	client := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			SeriesFunc: func(_ context.Context, matchers []string, _ processor.TimeRange) (*processor.PrometheusSeriesResult, error) {
				if matchers[0] == "boom" || strings.Contains(matchers[0], "$") {
					return nil, assert.AnError
				}

				return series, nil
			},
		}
	}

	tool := func(matchers ...string) listPrometheusSeries {
		return listPrometheusSeries{DataSourceID: _testDataSourceID, Matchers: matchers}
	}

	// a $ left in a matcher fails the read, so the substitution case
	// passes only when every reference was replaced.
	cc := map[string]dataSourceCase{
		"The series with variables substituted": {
			Type:       datasource.TypePrometheus,
			Tool:       tool(`{region="$region"}`),
			Prometheus: client(),
			Result:     series,
			Calls:      dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1, Series: 1},
		},
		"Error returned by Input.SubstituteVariables": {
			Type:         datasource.TypePrometheus,
			Tool:         tool(`{region="$region"}`),
			VariablesErr: assert.AnError,
			Prometheus:   client(),
			Err:          assert.AnError,
			Calls:        dataSourceCalls{Runner: 1, Substitute: 1},
		},
		"An inverted range": {
			Type: datasource.TypePrometheus,
			Tool: func() listPrometheusSeries {
				tl := tool("up")
				tl.timeRange = invertedRange()

				return tl
			}(),
			Err: _errInvertedTimeRange,
		},
		"Error returned by Input.DataSourceRunner": {
			Type:      datasource.TypePrometheus,
			Tool:      tool("up"),
			RunnerErr: assert.AnError,
			Err:       assert.AnError,
			Calls:     dataSourceCalls{Runner: 1},
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePostgreSQL,
			Tool:     tool("up"),
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1},
		},
		"Error returned by Prometheus.Series": {
			Type:       datasource.TypePrometheus,
			Tool:       tool("boom"),
			Prometheus: client(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Substitute: 1, Prometheus: 1, Series: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, c)
		})
	}
}

func Test_listLogFields_Info(t *testing.T) {
	t.Parallel()

	info := listLogFields{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameListLogFields, info.Name)
	assert.Equal(t, []string{"data_source_id"}, info.Required)
	assert.Contains(t, info.Properties, "field")
	assert.Contains(t, info.Properties, "query")
	assert.Contains(t, info.Properties, "from")
	assert.Contains(t, info.Properties, "to")
	assert.NotContains(t, info.Description, "lines")
}

func Test_listLogFields_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool listLogFields
		Err  error
	}{
		"Data source id is required": {
			Tool: listLogFields{Field: "app"},
			Err:  errors.New("data_source_id is required"),
		},
		"Valid arguments": {
			Tool: listLogFields{DataSourceID: _testDataSourceID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_listLogFields_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   listLogFields
		Result string
	}{
		"Field names": {
			Input:  stubDataSourceInput(nil),
			Tool:   listLogFields{DataSourceID: _testDataSourceID},
			Result: `Listing fields of "prod"`,
		},
		"Values of one field": {
			Input:  stubDataSourceInput(nil),
			Tool:   listLogFields{DataSourceID: _testDataSourceID, Field: "app"},
			Result: `Listing values of field "app" in "prod"`,
		},
		"Error returned by Input.FetchDataSource": {
			Input:  stubDataSourceInput(assert.AnError),
			Tool:   listLogFields{DataSourceID: _testDataSourceID},
			Result: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.Title(c.Input))

			ff := c.Input.FetchDataSourceCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testDataSourceID, ff[0].DataSourceID)
		})
	}
}

func Test_listLogFields_Exec(t *testing.T) {
	t.Parallel()

	fields := func(name string) *processor.LogFieldsResult {
		return &processor.LogFieldsResult{Fields: []processor.LogField{{Name: name, Kind: processor.LogFieldKindLabel}}}
	}

	values := func(value string) *processor.LogFieldValuesResult {
		return &processor.LogFieldValuesResult{Values: []processor.LogFieldValue{{Value: value, Count: null.IntFrom(3)}}}
	}

	client := func() *datasourceMock.Logs {
		return &datasourceMock.Logs{
			LogFieldsFunc: func(_ context.Context, q string, tr processor.TimeRange) (*processor.LogFieldsResult, error) {
				if q == "boom" || strings.Contains(q, "$") {
					return nil, assert.AnError
				}

				assert.Equal(t, time.Hour, tr.To.Sub(tr.From))

				if q != "" {
					return fields("narrowed"), nil
				}

				return fields("app"), nil
			},
			LogFieldValuesFunc: func(_ context.Context, field, q string, _ processor.TimeRange) (*processor.LogFieldValuesResult, error) {
				if field == "boom" || strings.Contains(q, "$") {
					return nil, assert.AnError
				}

				if q != "" {
					return values("narrowed"), nil
				}

				return values("api"), nil
			},
		}
	}

	tool := func(field, query string) listLogFields {
		return listLogFields{DataSourceID: _testDataSourceID, Field: field, Query: query}
	}

	// a $ left in a query fails the read, so the substitution cases pass
	// only when every reference was replaced.
	cc := map[string]dataSourceCase{
		"The field names": {
			Type:   datasource.TypeLoki,
			Tool:   tool("", ""),
			Logs:   client(),
			Result: fields("app"),
			Calls:  dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1, LogFields: 1},
		},
		"The field names of the logs a query matches": {
			Type:   datasource.TypeLoki,
			Tool:   tool("", `{region="$region"}`),
			Logs:   client(),
			Result: fields("narrowed"),
			Calls:  dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1, LogFields: 1},
		},
		"The values a field takes with their counts": {
			Type:   datasource.TypeLoki,
			Tool:   tool("app", ""),
			Logs:   client(),
			Result: values("api"),
			Calls:  dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1, LogFieldValues: 1},
		},
		"The values a field takes in the logs a query matches": {
			Type:   datasource.TypeLoki,
			Tool:   tool("app", `{region="$region"}`),
			Logs:   client(),
			Result: values("narrowed"),
			Calls:  dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1, LogFieldValues: 1},
		},
		"Error returned by Input.SubstituteVariables": {
			Type:         datasource.TypeLoki,
			Tool:         tool("", `{region="$region"}`),
			VariablesErr: assert.AnError,
			Logs:         client(),
			Err:          assert.AnError,
			Calls:        dataSourceCalls{Runner: 1, Substitute: 1},
		},
		"An inverted range": {
			Type: datasource.TypeLoki,
			Tool: func() listLogFields {
				tl := tool("", "")
				tl.timeRange = invertedRange()

				return tl
			}(),
			Err: _errInvertedTimeRange,
		},
		"Error returned by Input.DataSourceRunner": {
			Type:      datasource.TypeLoki,
			Tool:      tool("", ""),
			RunnerErr: assert.AnError,
			Err:       assert.AnError,
			Calls:     dataSourceCalls{Runner: 1},
		},
		// the runner refuses the wrong type, naming it, so the tool adds
		// nothing of its own.
		"A Prometheus data source is refused": {
			Type:     datasource.TypePrometheus,
			Tool:     tool("", ""),
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1},
		},
		"Error returned by Logs.LogFields": {
			Type:  datasource.TypeLoki,
			Tool:  tool("", "boom"),
			Logs:  client(),
			Err:   assert.AnError,
			Calls: dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1, LogFields: 1},
		},
		"Error returned by Logs.LogFieldValues": {
			Type:  datasource.TypeLoki,
			Tool:  tool("boom", ""),
			Logs:  client(),
			Err:   assert.AnError,
			Calls: dataSourceCalls{Runner: 1, Substitute: 1, Logs: 1, LogFieldValues: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, c)
		})
	}
}

func Test_queryDataSource_Info(t *testing.T) {
	t.Parallel()

	info := queryDataSource{}.Info()

	assert.Equal(t, Traits{DataSource: true, Group: GroupDataSources, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameQueryDataSource, info.Name)
	assert.Equal(t, []string{"data_source_id", "query"}, info.Required)

	chartType, ok := info.Properties["chart_type"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, []processor.ChartType{processor.ChartTypeLine, processor.ChartTypeBar, processor.ChartTypeGauge}, chartType["enum"])
}

func Test_queryDataSource_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool queryDataSource
		Err  error
	}{
		"Data source id is required": {
			Tool: queryDataSource{Query: "up"},
			Err:  errors.New("data_source_id is required"),
		},
		"Query is required": {
			Tool: queryDataSource{DataSourceID: _testDataSourceID},
			Err:  errors.New("query is required"),
		},
		"Valid arguments": {
			Tool: queryDataSource{DataSourceID: _testDataSourceID, Query: "up"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_queryDataSource_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   queryDataSource
		Result string
	}{
		"Data source that resolves is named": {
			Input:  stubDataSourceInput(nil),
			Tool:   queryDataSource{DataSourceID: _testDataSourceID, Query: "up"},
			Result: `Querying "prod"`,
		},
		"Error returned by Input.FetchDataSource": {
			Input:  stubDataSourceInput(assert.AnError),
			Tool:   queryDataSource{DataSourceID: _testDataSourceID, Query: "up"},
			Result: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.Title(c.Input))

			ff := c.Input.FetchDataSourceCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testDataSourceID, ff[0].DataSourceID)
		})
	}
}

func Test_queryDataSource_Exec(t *testing.T) {
	t.Parallel()

	prom := func(res *processor.PrometheusQueryResult) *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				switch {
				case q == "boom", strings.Contains(q, "$"):
					return nil, assert.AnError
				case q == "rejected":
					return nil, processor.NewInvalidQueryError("parse error")
				case q == "down":
					return nil, processor.ConnectionStatusUnreachable.Error()
				default:
					return res, nil
				}
			},
		}
	}

	rows := [][]any{
		{float64(1), float64(2)},
	}

	// a fill only comes from a query, so the over-filled results are
	// decoded.
	const overfilled = `{"columns": ["time", "value"], "rows": [[1, 2]], "fill": {"interval": 1, "from": 0, "to": 300000}}`

	var (
		overfilledPG processor.PostgreSQLQueryResult
		overfilledMy processor.MySQLQueryResult
	)

	require.NoError(t, json.Unmarshal([]byte(overfilled), &overfilledPG))
	require.NoError(t, json.Unmarshal([]byte(overfilled), &overfilledMy))

	pgResult := &processor.PostgreSQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}
	myResult := &processor.MySQLQueryResult{Columns: []string{"time", "value"}, Rows: rows}

	pg := func() *datasourceMock.PostgreSQL {
		return &datasourceMock.PostgreSQL{
			QueryFunc: func(_ context.Context, q string, _ processor.TimeRange, vars map[string]string) (*processor.PostgreSQLQueryResult, error) {
				q = variable.Substitute(vars, q)[0]

				switch {
				case q == "boom", strings.Contains(q, "$"):
					return nil, assert.AnError
				case q == "empty":
					// an absent result is what the executor reports here.
					return nil, nil
				case q == "overfilled":
					return &overfilledPG, nil
				default:
					return pgResult, nil
				}
			},
		}
	}

	my := func() *datasourceMock.MySQL {
		return &datasourceMock.MySQL{
			QueryFunc: func(_ context.Context, q string, _ processor.TimeRange, vars map[string]string) (*processor.MySQLQueryResult, error) {
				q = variable.Substitute(vars, q)[0]

				switch {
				case q == "boom", strings.Contains(q, "$"):
					return nil, assert.AnError
				case q == "empty":
					// an absent result is what the executor reports here.
					return nil, nil
				case q == "overfilled":
					return &overfilledMy, nil
				default:
					return myResult, nil
				}
			},
		}
	}

	logs := func() *datasourceMock.Logs {
		return &datasourceMock.Logs{
			LogsFunc: func(_ context.Context, q string, _ processor.TimeRange, _ processor.LogsPage) (*processor.LogsResult, error) {
				if strings.Contains(q, "$") {
					return nil, assert.AnError
				}

				return &processor.LogsResult{
					Status: processor.QueryStatusOK,
					Lines:  []processor.LogLine{{Level: processor.LogLevelError, Line: q}},
				}, nil
			},
		}
	}

	query := func(q string, chart processor.ChartType) queryDataSource {
		return queryDataSource{DataSourceID: _testDataSourceID, Query: q, ChartType: chart}
	}

	matrix := &processor.PrometheusQueryResult{Type: model.ValMatrix, Warnings: []string{"slow"}}
	noData := &processor.QueryResult{Status: processor.QueryStatusNoData}

	pgChart, err := pgResult.Transform(processor.ChartTypeLine)
	require.NoError(t, err)

	myChart, err := myResult.Transform(processor.ChartTypeBar)
	require.NoError(t, err)

	// a $ left in a query fails it, so these pass only when every
	// reference resolves. Every dialect client is handed out, so the
	// tool's own dispatch decides which one is reached.
	cc := map[string]dataSourceCase{
		"Prometheus substituted variables": {
			Type:       datasource.TypePrometheus,
			Tool:       query(`up{region="$region",range="$metric_range"}`, ""),
			Prometheus: prom(matrix),
			Result:     newRawSeries(matrix),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"PostgreSQL substituted variables": {
			Type:       datasource.TypePostgreSQL,
			Tool:       query("select '$region'", ""),
			PostgreSQL: pg(),
			MySQL:      my(),
			Result:     newRawRows(pgResult.Columns, pgResult.Rows),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"MySQL substituted variables": {
			Type:       datasource.TypeMySQL,
			Tool:       query("select '$region'", ""),
			PostgreSQL: pg(),
			MySQL:      my(),
			Result:     newRawRows(myResult.Columns, myResult.Rows),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"Error returned by Input.VariableValues": {
			Type:         datasource.TypePostgreSQL,
			Tool:         query("select '$region'", ""),
			VariablesErr: assert.AnError,
			PostgreSQL:   pg(),
			Err:          assert.AnError,
			Calls:        dataSourceCalls{Runner: 1, Values: 1},
		},
		"Prometheus charted": {
			Type:       datasource.TypePrometheus,
			Tool:       query("up", processor.ChartTypeLine),
			Prometheus: prom(matrix),
			Result:     newChartPreview(matrix.Transform(processor.ChartTypeLine)),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		// a query that returned nothing has no result to transform, and
		// the metric block renders it as no-data.
		"A Prometheus query with no result at all is no-data": {
			Type:       datasource.TypePrometheus,
			Tool:       query("up", processor.ChartTypeGauge),
			Prometheus: prom(nil),
			Result:     noData,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"Supabase charted": {
			Type:       datasource.TypeSupabase,
			Tool:       query("select 1", processor.ChartTypeLine),
			PostgreSQL: pg(),
			MySQL:      my(),
			Result:     newChartPreview(pgChart),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"A PostgreSQL query with no rows at all is no-data": {
			Type:       datasource.TypePostgreSQL,
			Tool:       query("empty", processor.ChartTypeLine),
			PostgreSQL: pg(),
			MySQL:      my(),
			Result:     noData,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"MariaDB charted": {
			Type:       datasource.TypeMariaDB,
			Tool:       query("select 1", processor.ChartTypeBar),
			PostgreSQL: pg(),
			MySQL:      my(),
			Result:     newChartPreview(myChart),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"A MySQL query with no rows at all is no-data": {
			Type:       datasource.TypeMySQL,
			Tool:       query("empty", processor.ChartTypeLine),
			PostgreSQL: pg(),
			MySQL:      my(),
			Result:     noData,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"Loki substituted variables": {
			Type: datasource.TypeLoki,
			Tool: query(`{region="$region"}`, ""),
			Logs: logs(),
			Result: rawLogs{
				Status: processor.QueryStatusOK,
				Logs:   []rawLog{{Level: processor.LogLevelError, Message: `{region="eu-west-1"}`}},
			},
			Calls: dataSourceCalls{Runner: 1, Values: 1, Type: 1, Logs: 1, LogsLogs: 1},
		},
		// no chart draws logs, so the check is refused before any read.
		"A Loki chart check is refused": {
			Type:  datasource.TypeLoki,
			Tool:  query(`{app="api"}`, processor.ChartTypeLine),
			Logs:  logs(),
			Err:   _errLogsChartType,
			Calls: dataSourceCalls{Runner: 1, Values: 1, Type: 1},
		},
		"An inverted range": {
			Type: datasource.TypePostgreSQL,
			Tool: func() queryDataSource {
				tl := query("select 1", "")
				tl.timeRange = invertedRange()

				return tl
			}(),
			Err: _errInvertedTimeRange,
		},
		"Error returned by Input.DataSourceRunner": {
			Type:      datasource.TypePostgreSQL,
			Tool:      query("select 1", ""),
			RunnerErr: assert.AnError,
			Err:       assert.AnError,
			Calls:     dataSourceCalls{Runner: 1},
		},
		"Error returned by Runner.Prometheus": {
			Type:     datasource.TypePrometheus,
			Tool:     query("up", ""),
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1},
		},
		"Error returned by Runner.PostgreSQL": {
			Type:     datasource.TypePostgreSQL,
			Tool:     query("select 1", ""),
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Values: 1, Type: 1, PostgreSQL: 1},
		},
		"Error returned by Runner.MySQL": {
			Type:     datasource.TypeMySQL,
			Tool:     query("select 1", ""),
			NoClient: true,
			Err:      assert.AnError,
			Calls:    dataSourceCalls{Runner: 1, Values: 1, Type: 1, MySQL: 1},
		},
		"Error returned by Prometheus.QueryRange": {
			Type:       datasource.TypePrometheus,
			Tool:       query("boom", ""),
			Prometheus: prom(nil),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"Error returned by PostgreSQLQueryResult.Transform": {
			Type:       datasource.TypePostgreSQL,
			Tool:       query("overfilled", processor.ChartTypeLine),
			PostgreSQL: pg(),
			MySQL:      my(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"Error returned by MySQLQueryResult.Transform": {
			Type:       datasource.TypeMySQL,
			Tool:       query("overfilled", processor.ChartTypeLine),
			PostgreSQL: pg(),
			MySQL:      my(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"A Prometheus query the data source rejected": {
			Type:       datasource.TypePrometheus,
			Tool:       query("rejected", ""),
			Prometheus: prom(nil),
			Err:        fmt.Errorf("%w; check the query against get_data_source_metadata", processor.NewInvalidQueryError("parse error")),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"A Prometheus query the data source could not serve": {
			Type:       datasource.TypePrometheus,
			Tool:       query("down", ""),
			Prometheus: prom(nil),
			Err:        processor.ConnectionStatusUnreachable.Error(),
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"Error returned by PostgreSQL.Query": {
			Type:       datasource.TypePostgreSQL,
			Tool:       query("boom", ""),
			PostgreSQL: pg(),
			MySQL:      my(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"Error returned by MySQL.Query": {
			Type:       datasource.TypeMySQL,
			Tool:       query("boom", ""),
			PostgreSQL: pg(),
			MySQL:      my(),
			Err:        assert.AnError,
			Calls:      dataSourceCalls{Runner: 1, Values: 1, Type: 1, MySQL: 1, MySQLQuery: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runDataSourceCase(t, c)
		})
	}
}

func Test_queryLogs(t *testing.T) {
	t.Parallel()

	logs := &datasourceMock.Logs{
		LogsFunc: func(_ context.Context, q string, _ processor.TimeRange, page processor.LogsPage) (*processor.LogsResult, error) {
			// the newest logs, at most the cap of them.
			assert.Equal(t, processor.LogsPage{Limit: _maxRawLogs, Order: processor.LogsOrderDesc}, page)

			switch q {
			case "boom":
				return nil, assert.AnError
			case "down":
				return nil, processor.ConnectionStatusUnreachable.Error()
			case "empty":
				return &processor.LogsResult{Status: processor.QueryStatusNoData}, nil
			default:
				return &processor.LogsResult{
					Status: processor.QueryStatusOK,
					Lines:  []processor.LogLine{{Level: processor.LogLevelError, Line: q}},
				}, nil
			}
		},
	}

	cc := map[string]struct {
		NoClient bool
		Query    string
		Result   any
		Err      error
	}{
		"The newest logs": {
			Query: `{app="api"}`,
			Result: rawLogs{
				Status: processor.QueryStatusOK,
				Logs:   []rawLog{{Level: processor.LogLevelError, Message: `{app="api"}`}},
			},
		},
		"A query that matches no log is no-data": {
			Query:  "empty",
			Result: &processor.QueryResult{Status: processor.QueryStatusNoData},
		},
		"Error returned by Runner.Logs": {
			NoClient: true,
			Query:    `{app="api"}`,
			Err:      assert.AnError,
		},
		// a metric query fails in the client, so the hint names what the
		// tool runs.
		"Error returned by Logs.Logs": {
			Query: "boom",
			Err:   fmt.Errorf("%w; check the fields with list_log_fields, and use a log query, since metric queries such as rate() are not supported", assert.AnError),
		},
		"Logs.Logs that could not serve the query": {
			Query: "down",
			Err:   processor.ConnectionStatusUnreachable.Error(),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runner := &datasourceMock.Runner{
				LogsFunc: func(context.Context) (datasource.Logs, error) {
					if c.NoClient {
						return nil, assert.AnError
					}

					return logs, nil
				},
			}

			got, err := queryLogs(&InputMock{ContextFunc: context.Background}, runner, c.Query, processor.TimeRange{})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_queryError(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Err    error
		Result error
	}{
		"Query the data source rejected": {
			Err:    processor.NewInvalidQueryError("parse error"),
			Result: fmt.Errorf("%w; check the query against get_data_source_metadata", processor.NewInvalidQueryError("parse error")),
		},
		"Data source that is down": {
			Err:    processor.ConnectionStatusUnreachable.Error(),
			Result: processor.ConnectionStatusUnreachable.Error(),
		},
		"Error without a code": {
			Err:    assert.AnError,
			Result: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, queryError(c.Err))
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

func Test_newRawLogs(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

	line := processor.LogLine{
		ID:     "1",
		Time:   at,
		Level:  processor.LogLevelWarning,
		Line:   "slow request",
		Labels: map[string]string{"app": "api"},
		Fields: map[string]string{"trace_id": "abc"},
		Parsed: map[string]string{"duration": "2s"},
	}

	cc := map[string]struct {
		Input  *processor.LogsResult
		Result rawLogs
	}{
		"Every log is kept with its detail": {
			Input: &processor.LogsResult{Status: processor.QueryStatusOK, Lines: []processor.LogLine{line}},
			Result: rawLogs{
				Status: processor.QueryStatusOK,
				Logs: []rawLog{{
					Time:    at,
					Level:   processor.LogLevelWarning,
					Message: "slow request",
					Labels:  map[string]string{"app": "api"},
					Fields:  map[string]string{"trace_id": "abc"},
					Parsed:  map[string]string{"duration": "2s"},
				}},
			},
		},
		"More logs past the page are flagged": {
			Input: &processor.LogsResult{
				Status:     processor.QueryStatusOK,
				Lines:      []processor.LogLine{line},
				NextCursor: null.ValueFrom(processor.Cursor{}),
			},
			Result: rawLogs{
				Status: processor.QueryStatusOK,
				Logs: []rawLog{{
					Time:    at,
					Level:   processor.LogLevelWarning,
					Message: "slow request",
					Labels:  map[string]string{"app": "api"},
					Fields:  map[string]string{"trace_id": "abc"},
					Parsed:  map[string]string{"duration": "2s"},
				}},
				HasMore: true,
			},
		},
		"A timestamp group past the cap is cut": {
			// a page whose logs share one timestamp holds the whole group,
			// however long it is.
			Input: &processor.LogsResult{
				Status: processor.QueryStatusOK,
				Lines:  slices.Repeat([]processor.LogLine{line}, _maxRawLogs+1),
			},
			Result: rawLogs{
				Status: processor.QueryStatusOK,
				Logs: slices.Repeat([]rawLog{{
					Time:    at,
					Level:   processor.LogLevelWarning,
					Message: "slow request",
					Labels:  map[string]string{"app": "api"},
					Fields:  map[string]string{"trace_id": "abc"},
					Parsed:  map[string]string{"duration": "2s"},
				}}, _maxRawLogs),
				HasMore: true,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newRawLogs(c.Input))
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

func Test_timeRange_resolve(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	cc := map[string]struct {
		Args  timeRange
		Check func(t *testing.T, tr processor.TimeRange)
		Err   error
	}{
		"An inverted range": {
			Args: timeRange{From: to, To: from},
			Err:  _errInvertedTimeRange,
		},
		"A future start with no end given": {
			Args: timeRange{From: timeutil.Now().Add(time.Hour)},
			Err:  _errInvertedTimeRange,
		},
		"Both ends given": {
			Args: timeRange{From: from, To: to},
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
			Args: timeRange{To: to},
			Check: func(t *testing.T, tr processor.TimeRange) {
				t.Helper()

				assert.Equal(t, to.Add(-time.Hour), tr.From)
			},
		},
		"Only the start given ends at now": {
			Args: timeRange{From: from},
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

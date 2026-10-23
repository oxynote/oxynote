package datasource

import (
	"context"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

// stubRunner is a runner of one type whose clients answer the query with
// fixed values, or that hands out no client at all when clientErr is set.
// The clients embed their interface, so only the query method exists; any
// other call is a test bug. Every call is counted in calls.
type stubRunner struct {
	Runner

	typ        Type
	clientErr  error
	err        error
	prometheus *processor.PrometheusQueryResult
	postgreSQL *processor.PostgreSQLQueryResult
	mySQL      *processor.MySQLQueryResult
	calls      *stubCalls
}

// stubCalls counts the calls made to a stubRunner and its clients.
type stubCalls struct {
	Type            int
	Prometheus      int
	PostgreSQL      int
	MySQL           int
	QueryRange      int
	PostgreSQLQuery int
	MySQLQuery      int
}

func (s stubRunner) Type() Type {
	s.calls.Type++

	return s.typ
}

func (s stubRunner) Prometheus(context.Context) (Prometheus, error) {
	s.calls.Prometheus++

	if s.clientErr != nil {
		return nil, s.clientErr
	}

	return stubPrometheus{res: s.prometheus, err: s.err, calls: s.calls}, nil
}

func (s stubRunner) PostgreSQL(context.Context) (PostgreSQL, error) {
	s.calls.PostgreSQL++

	if s.clientErr != nil {
		return nil, s.clientErr
	}

	return stubPostgreSQL{res: s.postgreSQL, err: s.err, calls: s.calls}, nil
}

func (s stubRunner) MySQL(context.Context) (MySQL, error) {
	s.calls.MySQL++

	if s.clientErr != nil {
		return nil, s.clientErr
	}

	return stubMySQL{res: s.mySQL, err: s.err, calls: s.calls}, nil
}

type stubPrometheus struct {
	Prometheus

	res   *processor.PrometheusQueryResult
	err   error
	calls *stubCalls
}

func (s stubPrometheus) QueryRange(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
	s.calls.QueryRange++

	return s.res, s.err
}

type stubPostgreSQL struct {
	PostgreSQL

	res   *processor.PostgreSQLQueryResult
	err   error
	calls *stubCalls
}

func (s stubPostgreSQL) Query(context.Context, string, processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
	s.calls.PostgreSQLQuery++

	return s.res, s.err
}

type stubMySQL struct {
	MySQL

	res   *processor.MySQLQueryResult
	err   error
	calls *stubCalls
}

func (s stubMySQL) Query(context.Context, string, processor.TimeRange) (*processor.MySQLQueryResult, error) {
	s.calls.MySQLQuery++

	return s.res, s.err
}

func Test_Query(t *testing.T) {
	rows := &processor.PostgreSQLQueryResult{
		Columns: []string{"time", "value"},
		Rows:    [][]any{{1700000000.0, 10.0}},
	}

	cc := map[string]struct {
		Runner stubRunner
		Result *processor.QueryResult
		Err    error
		Calls  stubCalls
	}{
		"Unsupported type": {
			Runner: stubRunner{typ: Type("bogus")},
			Err:    ErrTypeNotSupported,
			Calls:  stubCalls{Type: 1},
		},
		"Error returned by Runner.Prometheus": {
			Runner: stubRunner{typ: TypePrometheus, clientErr: assert.AnError},
			Err:    assert.AnError,
			Calls:  stubCalls{Type: 1, Prometheus: 1},
		},
		"Error returned by Runner.PostgreSQL": {
			Runner: stubRunner{typ: TypePostgreSQL, clientErr: assert.AnError},
			Err:    assert.AnError,
			Calls:  stubCalls{Type: 1, PostgreSQL: 1},
		},
		"Error returned by Runner.MySQL": {
			Runner: stubRunner{typ: TypeMySQL, clientErr: assert.AnError},
			Err:    assert.AnError,
			Calls:  stubCalls{Type: 1, MySQL: 1},
		},
		"Error returned by Prometheus.QueryRange": {
			Runner: stubRunner{typ: TypePrometheus, err: assert.AnError},
			Err:    assert.AnError,
			Calls:  stubCalls{Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"Error returned by PostgreSQL.Query": {
			Runner: stubRunner{typ: TypePostgreSQL, err: assert.AnError},
			Err:    assert.AnError,
			Calls:  stubCalls{Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"Error returned by MySQL.Query": {
			Runner: stubRunner{typ: TypeMariaDB, err: assert.AnError},
			Err:    assert.AnError,
			Calls:  stubCalls{Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"Prometheus answered nothing": {
			Runner: stubRunner{typ: TypePrometheus},
			Result: &processor.QueryResult{Status: processor.QueryStatusNoData},
			Calls:  stubCalls{Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"PostgreSQL answered nothing": {
			Runner: stubRunner{typ: TypePostgreSQL},
			Result: &processor.QueryResult{Status: processor.QueryStatusNoData},
			Calls:  stubCalls{Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"MySQL answered nothing": {
			Runner: stubRunner{typ: TypeMySQL},
			Result: &processor.QueryResult{Status: processor.QueryStatusNoData},
			Calls:  stubCalls{Type: 1, MySQL: 1, MySQLQuery: 1},
		},
		"Prometheus result transformed": {
			Runner: stubRunner{typ: TypePrometheus, prometheus: &processor.PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"job": "a"},
						Values: []model.SamplePair{{Timestamp: model.Time(1700000000000), Value: 10}},
					},
				},
			}},
			Result: &processor.QueryResult{
				Status: processor.QueryStatusOK,
				Data: []processor.QueryResultSeries{
					{Labels: map[string]string{"job": "a"}, Metrics: [][2]any{{int64(1700000000), 10.0}}},
				},
			},
			Calls: stubCalls{Type: 1, Prometheus: 1, QueryRange: 1},
		},
		"PostgreSQL result transformed": {
			Runner: stubRunner{typ: TypePostgreSQL, postgreSQL: rows},
			Result: (&processor.PostgreSQLQueryResult{Columns: rows.Columns, Rows: rows.Rows}).Transform(processor.ChartTypeLine),
			Calls:  stubCalls{Type: 1, PostgreSQL: 1, PostgreSQLQuery: 1},
		},
		"MySQL result transformed": {
			Runner: stubRunner{typ: TypeMySQL, mySQL: &processor.MySQLQueryResult{Columns: rows.Columns, Rows: rows.Rows}},
			Result: (&processor.MySQLQueryResult{Columns: rows.Columns, Rows: rows.Rows}).Transform(processor.ChartTypeLine),
			Calls:  stubCalls{Type: 1, MySQL: 1, MySQLQuery: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tr := processor.TimeRange{From: time.Unix(1700000000, 0), To: time.Unix(1700003600, 0)}

			r := c.Runner
			r.calls = &stubCalls{}

			res, err := Query(context.Background(), r, "up", tr, processor.ChartTypeLine)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Calls, *r.calls)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// _cacheTTL is how long a query's answer serves the calls that ask
	// for it again.
	_cacheTTL = 10 * time.Second

	// _cacheMaxBytes caps the bytes of answers the in-process cache holds.
	_cacheMaxBytes = 64 << 20

	// _cacheGrid is what SQL and log ranges are aligned to before they are
	// keyed and run. It is the smallest chart step, so a window widens by
	// less than that at each edge however long it is.
	_cacheGrid = 15 * time.Second

	// _cacheKeyPrefix namespaces the cache's keys in a store other
	// components share.
	_cacheKeyPrefix = "datasource:query:"
)

// cachedPrometheus answers range queries out of the manager's query cache.
type cachedPrometheus struct {
	Prometheus

	m  *Manager
	ds DataSource
}

// QueryRange performs the range query unless the same one was answered a
// moment ago. The client aligns the range to the query step itself, so the
// key does the same and the client is given the range as asked: aligning
// twice can widen the window.
func (cp cachedPrometheus) QueryRange(ctx context.Context, q string, tr processor.TimeRange) (*processor.PrometheusQueryResult, error) {
	return cachedQuery(ctx, cp.m, cacheKey(cp.ds, "query_range", tr.Normalize(), q), func(ctx context.Context) (*processor.PrometheusQueryResult, error) {
		return cp.Prometheus.QueryRange(ctx, q, tr)
	})
}

// cachedPostgreSQL answers queries out of the manager's query cache.
type cachedPostgreSQL struct {
	PostgreSQL

	m  *Manager
	ds DataSource
}

// Query executes the query unless the same one was answered a moment ago.
func (cp cachedPostgreSQL) Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
	gtr := tr.AlignTo(_cacheGrid)

	return cachedQuery(ctx, cp.m, cacheKey(cp.ds, "postgresql_query", gtr, q), func(ctx context.Context) (*processor.PostgreSQLQueryResult, error) {
		return cp.PostgreSQL.Query(ctx, q, gtr)
	})
}

// cachedMySQL answers queries out of the manager's query cache.
type cachedMySQL struct {
	MySQL

	m  *Manager
	ds DataSource
}

// Query executes the query unless the same one was answered a moment ago.
func (cm cachedMySQL) Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.MySQLQueryResult, error) {
	gtr := tr.AlignTo(_cacheGrid)

	return cachedQuery(ctx, cm.m, cacheKey(cm.ds, "mysql_query", gtr, q), func(ctx context.Context) (*processor.MySQLQueryResult, error) {
		return cm.MySQL.Query(ctx, q, gtr)
	})
}

// cachedLogs answers log reads out of the manager's query cache. The
// client is a named field because the interface shares its name with one of
// its own methods.
type cachedLogs struct {
	client Logs

	m  *Manager
	ds DataSource
}

// TestConnection tests the connection through the client.
func (cl cachedLogs) TestConnection(ctx context.Context) (processor.ConnectionStatus, error) {
	return cl.client.TestConnection(ctx)
}

// Logs reads the page unless the same one was read a moment ago.
func (cl cachedLogs) Logs(
	ctx context.Context,
	q string,
	tr processor.TimeRange,
	page processor.LogsPage,
) (*processor.LogsResult, error) {
	var cursor []byte

	if page.Cursor.Valid {
		var err error

		cursor, err = page.Cursor.V.MarshalText()
		if err != nil {
			return nil, err
		}
	}

	gtr := tr.AlignTo(_cacheGrid)
	key := cacheKey(cl.ds, "logs", gtr, q, strconv.Itoa(page.Limit), string(cursor))

	return cachedQuery(ctx, cl.m, key, func(ctx context.Context) (*processor.LogsResult, error) {
		return cl.client.Logs(ctx, q, gtr, page)
	})
}

// LogsVolume counts the lines unless the same count was made a moment ago.
func (cl cachedLogs) LogsVolume(ctx context.Context, q string, tr processor.TimeRange) (*processor.QueryResult, error) {
	gtr := tr.AlignTo(_cacheGrid)

	return cachedQuery(ctx, cl.m, cacheKey(cl.ds, "logs_volume", gtr, q), func(ctx context.Context) (*processor.QueryResult, error) {
		return cl.client.LogsVolume(ctx, q, gtr)
	})
}

// LogFields lists the field names unless the same list was made a moment
// ago.
func (cl cachedLogs) LogFields(ctx context.Context, q string, tr processor.TimeRange) (*processor.LogFieldsResult, error) {
	gtr := tr.AlignTo(_cacheGrid)

	return cachedQuery(ctx, cl.m, cacheKey(cl.ds, "log_fields", gtr, q), func(ctx context.Context) (*processor.LogFieldsResult, error) {
		return cl.client.LogFields(ctx, q, gtr)
	})
}

// LogFieldValues lists the field's values unless the same list was made a
// moment ago.
func (cl cachedLogs) LogFieldValues(ctx context.Context, field, q string, tr processor.TimeRange) (*processor.LogFieldValuesResult, error) {
	gtr := tr.AlignTo(_cacheGrid)

	return cachedQuery(ctx, cl.m, cacheKey(cl.ds, "log_field_values", gtr, field, q), func(ctx context.Context) (*processor.LogFieldValuesResult, error) {
		return cl.client.LogFieldValues(ctx, field, q, gtr)
	})
}

// cachedQuery answers a query out of the manager's cache, or runs call and
// keeps its answer for _cacheTTL. Identical calls arriving while one runs
// wait for it and share its answer. A cache that cannot be read or written
// is logged and treated as empty: the query still runs.
func cachedQuery[T cacheable](
	ctx context.Context,
	m *Manager,
	key string,
	call func(context.Context) (T, error),
) (T, error) {
	var zero T

	data, err := m.cache.Get(ctx, key)
	if err != nil && !errors.Is(err, errutil.ErrNotFound) {
		m.log.WarnContext(ctx, "cannot read the query cache", slog.String("error", err.Error()))
	}

	if err == nil {
		var kept T

		uerr := json.Unmarshal(data, &kept)
		if uerr == nil {
			m.metrics.recordLookup(_lookupHit)

			return kept, nil
		}

		m.log.WarnContext(ctx, "cannot decode a cached query answer", slog.String("error", uerr.Error()))
	}

	// singleflight marks every caller as sharing once there are two, so
	// the caller that ran the query is told apart by having run it.
	var led bool

	v, err, _ := m.flights.Do(key, func() (any, error) {
		led = true

		// the query outlives a caller who gives up on it, since the calls
		// waiting on it would get the cancellation too. Every processor
		// bounds a query with a timeout of its own.
		shared := context.WithoutCancel(ctx)

		ans, cerr := call(shared)
		if cerr != nil || !ans.HasData() {
			return ans, cerr
		}

		enc, merr := json.Marshal(ans)
		if merr != nil {
			m.log.WarnContext(shared, "cannot encode a query answer for the cache", slog.String("error", merr.Error()))

			return ans, nil
		}

		if serr := m.cache.Set(shared, key, enc); serr != nil {
			m.log.WarnContext(shared, "cannot write the query cache", slog.String("error", serr.Error()))
		}

		return ans, nil
	})

	if led {
		m.metrics.recordLookup(_lookupMiss)
	} else {
		m.metrics.recordLookup(_lookupShared)
	}

	if err != nil {
		return zero, err
	}

	res, ok := v.(T)
	if !ok {
		// NOCOV: the shared call only ever answers with T.
		return zero, errors.New("unexpected cached query answer")
	}

	return res, nil
}

// cacheKey names the cache entry of one query: the data source as it
// stands, the method asked, the aligned time range and every other
// argument the answer depends on. Calls a few seconds apart that align to
// the same range share an entry.
func cacheKey(ds DataSource, method string, tr processor.TimeRange, args ...string) string {
	var updatedAt string

	if ds.UpdatedAt.Valid {
		updatedAt = ds.UpdatedAt.Time.Format(time.RFC3339Nano)
	}

	parts := append([]string{
		ds.ID.String(),
		updatedAt,
		method,
		strconv.FormatInt(tr.From.Unix(), 10),
		strconv.FormatInt(tr.To.Unix(), 10),
	}, args...)

	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))

	return _cacheKeyPrefix + hex.EncodeToString(sum[:])
}

// cacheable is an answer the query cache can keep.
type cacheable interface {
	// HasData should report whether the answer holds anything; an empty
	// one is not kept, so a metric that starts reporting shows at once.
	HasData() bool
}

// queryCache keeps query answers for _cacheTTL.
//
//go:generate ../../scripts/codegen/mock -t internal queryCache query_cache
type queryCache interface {
	// Start should run the cache's upkeep until ctx is cancelled.
	Start(ctx context.Context)

	// Get should return the answer kept under key, or errutil.ErrNotFound.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set should keep the answer under key.
	Set(ctx context.Context, key string, value []byte) error
}

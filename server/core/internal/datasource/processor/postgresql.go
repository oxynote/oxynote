package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// _queryTimeout is the maximum duration for a data-source call.
	_queryTimeout = 10 * time.Second

	// _connectTimeout bounds the initial dial. TestConnection runs on the
	// request's own context, so without it a black-holed host holds the
	// create and test endpoints until the OS gives up on the TCP handshake.
	_connectTimeout = 10 * time.Second

	// _queryPayloadLimit is the maximum estimated payload size (in bytes) for a query result.
	_queryPayloadLimit = 5 * 1024 * 1024 // 5 MB

	// _pgMaxConns caps the connections one data source's pool holds.
	_pgMaxConns = 10
)

// _pgAllowedConnKeys lists the connection-string keys a data-source URL may
// carry. The URL is organisation-supplied, and keys outside this list can
// reach the local filesystem (sslkey, passfile, servicefile, ...) or override
// session behavior (options), so parsing fails closed on anything else.
var _pgAllowedConnKeys = []string{
	"host",
	"port",
	"dbname",
	"user",
	"password",
	"sslmode",
	"connect_timeout",
}

// PostgreSQL represents a PostgreSQL data source processor. It keeps a
// pool of connections to the data source until it is closed.
type PostgreSQL struct {
	inp Input

	mu     sync.Mutex
	pool   *pgxpool.Pool
	closed bool
}

// NewPostgreSQL creates a new PostgreSQL data source processor.
func NewPostgreSQL(inp Input) *PostgreSQL {
	return &PostgreSQL{
		inp: inp,
	}
}

// Close closes the pool, waiting for the queries it is running.
func (p *PostgreSQL) Close() error {
	p.mu.Lock()
	pool := p.pool
	p.closed = true
	p.mu.Unlock()

	if pool != nil {
		pool.Close()
	}

	return nil
}

// TestConnection tests the connection to the PostgreSQL data source. The
// data source passes only when its account is read-only: queries run on
// sessions nothing else makes read-only, so the account is what keeps them
// from writing.
func (p *PostgreSQL) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	pool, err := p.connect(ctx)
	if err != nil {
		return pgConnectionStatus(err), nil
	}

	if err = pool.Ping(ctx); err != nil {
		return pgConnectionStatus(err), nil
	}

	var readOnly bool

	// reset_val reports the value the data source itself defines through
	// the server, database or role, which a SET an earlier query left on the
	// pooled session cannot mask.
	err = pool.QueryRow(ctx,
		"SELECT COALESCE((SELECT reset_val FROM pg_settings "+
			"WHERE name = 'default_transaction_read_only') = 'on', false) "+
			"OR pg_is_in_recovery()",
	).Scan(&readOnly)
	if err != nil {
		return pgConnectionStatus(err), nil
	}

	if !readOnly {
		return ConnectionStatusNotReadOnly, nil
	}

	return ConnectionStatusSuccess, nil
}

// pgConnectionStatus classifies a failed connection attempt. A refused
// handshake is reported as unauthorized rather than unreachable: the two are
// indistinguishable to the user otherwise, and a typo in the password is the
// far more common of them.
func pgConnectionStatus(err error) ConnectionStatus {
	// class 28 is "invalid authorization specification".
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && strings.HasPrefix(pgErr.Code, "28") {
		return ConnectionStatusUnauthorized
	}

	return ConnectionStatusUnreachable
}

// Metadata retrieves all tables and their columns from the PostgreSQL data source.
func (p *PostgreSQL) Metadata(ctx context.Context) (*SQLMetadataResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	pool, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}

	rows, err := pool.Query(ctx,
		"SELECT table_schema, table_name, column_name FROM information_schema.columns WHERE table_schema NOT IN ('information_schema', 'pg_catalog') ORDER BY table_schema, table_name, ordinal_position",
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching metadata: %w", err)
	}
	defer rows.Close()

	tables, err := scanInformationSchema(rows)
	if err != nil {
		return nil, err
	}

	var defaultSchema string

	// a role with its own search_path, or a database without a public
	// schema, would otherwise be told its tables live somewhere they do not.
	if serr := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&defaultSchema); serr != nil {
		return nil, fmt.Errorf("error reading the default schema: %w", serr)
	}

	return &SQLMetadataResult{
		Tables:        tables,
		DefaultSchema: defaultSchema,
	}, nil
}

// QueryLabels executes a SQL query with LIMIT 1 against the PostgreSQL data source
// and returns the string (label) columns with their example values.
func (p *PostgreSQL) QueryLabels(ctx context.Context, q string, tr TimeRange) (map[string]string, error) {
	q = tr.ProcessPostgreSQLQuery(q)
	q = sqlSetLimit(q, 1)

	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	pool, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, pgQueryError(err)
	}
	defer rows.Close()

	columns := make([]string, len(rows.FieldDescriptions()))
	for i, fd := range rows.FieldDescriptions() {
		columns[i] = fd.Name
	}

	labels := make(map[string]string)

	if !rows.Next() {
		// pgx surfaces execution failures, including a query killed by the
		// timeout, through Err rather than the Query call.
		if rerr := rows.Err(); rerr != nil {
			return nil, pgQueryError(rerr)
		}

		return labels, nil
	}

	values, err := rows.Values()
	if err != nil {
		return nil, fmt.Errorf("error reading row values: %w", err)
	}

	for i, v := range values {
		if s, ok := v.(string); ok {
			labels[columns[i]] = s
		}
	}

	return labels, nil
}

// sqlSetLimit bounds the query to at most n rows.
func sqlSetLimit(q string, n int) string {
	// the query is wrapped rather than patched: appending or rewriting a
	// LIMIT has to understand everything that can end a statement — a
	// semicolon, an OFFSET, MySQL's "LIMIT a, b", a trailing comment — and
	// gets each of them wrong in a different way. A subquery bounds any
	// shape, including one that carries its own larger limit.
	// the newlines matter: a query ending in a line comment would otherwise
	// swallow the closing parenthesis.
	return fmt.Sprintf("SELECT * FROM (\n%s\n) AS oxynote_limited LIMIT %d", strings.TrimRight(q, "; \t\r\n"), n)
}

// Query executes a SQL query against the PostgreSQL data source.
func (p *PostgreSQL) Query(ctx context.Context, q string, tr TimeRange) (*PostgreSQLQueryResult, error) {
	q = tr.ProcessPostgreSQLQuery(q)

	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	pool, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, pgQueryError(err)
	}
	defer rows.Close()

	columns := make([]string, len(rows.FieldDescriptions()))
	for i, fd := range rows.FieldDescriptions() {
		columns[i] = fd.Name
	}

	var (
		resultRows  [][]any
		payloadSize int
	)

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("error reading row values: %w", err)
		}

		for i, v := range values {
			payloadSize += estimateValueSize(v)
			values[i] = pgNormalizeValue(v)
		}

		if payloadSize > _queryPayloadLimit {
			return nil, NewInvalidQueryError("query result exceeds the maximum payload size")
		}

		resultRows = append(resultRows, values)
	}

	// pgx defers execution, so a runtime failure raised by the query text
	// (a division by zero, a bad cast) surfaces here rather than at Query.
	if err := rows.Err(); err != nil {
		return nil, pgQueryError(err)
	}

	return &PostgreSQLQueryResult{
		Columns: columns,
		Rows:    resultRows,
	}, nil
}

// connect returns the pool of connections to the data source, building it
// on first use.
//
// The query execution mode is pinned to the extended protocol whatever the
// data source URL asks for, so a stored query runs one statement and cannot
// chain more behind it.
func (p *PostgreSQL) connect(ctx context.Context) (*pgxpool.Pool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, _errClosed
	}

	if p.pool != nil {
		return p.pool, nil
	}

	connStr, err := p.buildConnectionString()
	if err != nil {
		return nil, err
	}

	connCfg, err := pgx.ParseConfigWithOptions(connStr, pgx.ParseConfigOptions{
		ConnStringAllowedKeys: _pgAllowedConnKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("error parsing postgresql connection string: %w", err)
	}

	connCfg.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	connCfg.ConnectTimeout = _connectTimeout

	// pgxpool accepts only a config it parsed itself. What it parses here
	// is replaced by connCfg, so the key allowlist above still decides
	// what the URL may set.
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		// NOCOV: connCfg parsed the same string, and the allowlist leaves
		// pgxpool no key of its own to reject.
		return nil, fmt.Errorf("error parsing postgresql connection string: %w", err)
	}

	cfg.ConnConfig = connCfg
	cfg.MaxConns = _pgMaxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		// NOCOV: a pool without a minimum size dials nothing here, and the
		// config came from ParseConfig, so there is nothing left to fail.
		return nil, fmt.Errorf("error creating postgresql pool: %w", err)
	}

	p.pool = pool

	return pool, nil
}

// buildConnectionString builds a PostgreSQL connection string from the URL and credentials.
func (p *PostgreSQL) buildConnectionString() (string, error) {
	u, err := url.Parse(p.inp.URL())
	if err != nil {
		return "", fmt.Errorf("error parsing postgresql url: %w", err)
	}

	if inpCreds := p.inp.Credentials(); len(inpCreds.data) > 0 {
		var creds BasicCredentials

		if err := json.Unmarshal(inpCreds.data, &creds); err != nil {
			return "", fmt.Errorf("error unmarshaling credentials: %w", err)
		}

		if creds.Username != "" || creds.Password != "" {
			u.User = url.UserPassword(creds.Username, creds.Password)
		}
	}

	return u.String(), nil
}

// PostgreSQLQueryResult represents the result of a PostgreSQL query.
type PostgreSQLQueryResult struct {
	// Columns contains the column names of the result set.
	Columns []string `json:"columns"`

	// Rows contains the result rows, each as a slice of values.
	Rows [][]any `json:"rows"`
}

// HasData reports whether the result holds any row.
func (pqr *PostgreSQLQueryResult) HasData() bool {
	return pqr != nil && len(pqr.Rows) > 0
}

// sql returns the dialect-agnostic view of the result, which is where the
// transformation into series lives. pgx reports no column types, so the
// classification falls back to the values' own shape.
func (pqr *PostgreSQLQueryResult) sql() sqlQueryResult {
	return sqlQueryResult{
		Columns: pqr.Columns,
		Rows:    pqr.Rows,
	}
}

// Transform transforms a PostgreSQLQueryResult into a unified QueryResult
// based on the requested chart type.
func (pqr *PostgreSQLQueryResult) Transform(ct ChartType) *QueryResult {
	return pqr.sql().transform(ct, pgParseTimestamp, pgParseNumericValue)
}

// pgNormalizeValue converts native pgx row values into the JSON-shaped values
// the rest of the pipeline expects (Transform, HTTP responses). Timestamps
// become RFC3339 strings and all numeric types become float64, mirroring what
// JSON marshaling produces for these types. The unsigned ones pgx decodes
// (oid, xid, cid, xid8) are identifiers, so they become decimal strings and
// stay labels.
func pgNormalizeValue(v any) any {
	switch val := v.(type) {
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case int16:
		return float64(val)
	case float32:
		// pgx decodes real as float32; without this the column reads as a
		// label and the chart reports a data mismatch.
		return float64(val)
	case pgtype.Numeric:
		f, err := val.Float64Value()
		if err != nil || !f.Valid {
			return v
		}

		return f.Float64
	default:
		return v
	}
}

// pgParseTimestamp parses a timestamp value from a PostgreSQL result row.
// It handles float64 (unix seconds) and string (RFC3339) values.
func pgParseTimestamp(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case string:
		t, err := time.Parse(time.RFC3339Nano, val)
		if err != nil {
			t, err = time.Parse(time.RFC3339, val)
			if err != nil {
				return 0, false
			}
		}

		return t.Unix(), true
	default:
		return 0, false
	}
}

// pgParseNumericValue parses a numeric value from a PostgreSQL result row.
func pgParseNumericValue(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	default:
		return 0, false
	}
}

// pgQueryError maps a failure the query text itself caused to a user-facing
// invalid-query error, leaving everything else as an internal one. Class 42
// is a syntax or semantic mistake, class 22 a data exception raised by the
// query's own expressions (a division by zero, a bad cast), and 57014 the
// server cancelling the query — none of them are the deployment's fault.
func pgQueryError(err error) error {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if ok &&
		(strings.HasPrefix(pgErr.Code, "42") ||
			strings.HasPrefix(pgErr.Code, "22") ||
			pgErr.Code == "57014") {
		return NewInvalidQueryError(pgErr.Message)
	}

	return fmt.Errorf("error executing query: %w", err)
}

package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// _queryTimeout is the maximum duration for a PostgreSQL query.
	_queryTimeout = 10 * time.Second

	// _connectTimeout bounds the initial dial. TestConnection runs on the
	// request's own context, so without it a black-holed host holds the
	// create and test endpoints until the OS gives up on the TCP handshake.
	_connectTimeout = 10 * time.Second

	// _queryPayloadLimit is the maximum estimated payload size (in bytes) for a query result.
	_queryPayloadLimit = 5 * 1024 * 1024 // 5 MB
)

// PostgreSQL represents a PostgreSQL data source processor.
type PostgreSQL struct {
	inp Input
}

// NewPostgreSQL creates a new PostgreSQL data source processor.
func NewPostgreSQL(inp Input) *PostgreSQL {
	return &PostgreSQL{
		inp: inp,
	}
}

// TestConnection tests the connection to the PostgreSQL data source.
func (p *PostgreSQL) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	conn, err := p.connect(ctx)
	if err != nil {
		return pgConnectionStatus(err), nil
	}
	defer conn.Close(ctx) //nolint:errcheck // error provides no meaningful info

	if err = conn.Ping(ctx); err != nil {
		return pgConnectionStatus(err), nil
	}

	var readOnly bool

	// connect puts the session into read-only mode, which masks
	// current_setting; reset_val still reports the value the data source
	// itself defines through the server, database or role.
	err = conn.QueryRow(ctx,
		"SELECT COALESCE((SELECT reset_val FROM pg_settings "+
			"WHERE name = 'default_transaction_read_only') = 'on', false) "+
			"OR pg_is_in_recovery()",
	).Scan(&readOnly)
	if err != nil {
		return ConnectionStatusUnreachable, nil
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

	conn, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}
	defer conn.Close(ctx) //nolint:errcheck // error provides no meaningful info

	rows, err := conn.Query(ctx,
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
	if serr := conn.QueryRow(ctx, "SELECT current_schema()").Scan(&defaultSchema); serr != nil {
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

	conn, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}
	defer conn.Close(ctx) //nolint:errcheck // error provides no meaningful info

	rows, err := conn.Query(ctx, q)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code[0:2] == "42" {
			return nil, NewInvalidQueryError(pgErr.Message)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
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

	conn, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}
	defer conn.Close(ctx) //nolint:errcheck // error provides no meaningful info

	rows, err := conn.Query(ctx, q)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code[0:2] == "42" {
			return nil, NewInvalidQueryError(pgErr.Message)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &PostgreSQLQueryResult{
		Columns: columns,
		Rows:    resultRows,
	}, nil
}

// connect creates a pgx connection using the data source URL and credentials.
//
// Two things are locked down on every connection. The query execution mode is
// pinned to the extended protocol whatever the data source URL asks for: under
// the simple protocol a single stored query may carry several statements,
// which would let a leading "SET transaction_read_only = off" disarm the
// read-only gate. The session is then put into read-only mode, so the server
// itself rejects any write the query text attempts instead of the code
// trusting a status recorded earlier on a different connection.
func (p *PostgreSQL) connect(ctx context.Context) (*pgx.Conn, error) {
	connStr, err := p.buildConnectionString()
	if err != nil {
		return nil, err
	}

	cfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing postgresql connection string: %w", err)
	}

	cfg.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	cfg.ConnectTimeout = _connectTimeout

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}

	if _, err = conn.Exec(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY"); err != nil {
		conn.Close(ctx) //nolint:errcheck,gosec // error provides no meaningful info

		return nil, fmt.Errorf("error enforcing a read-only session: %w", err)
	}

	return conn, nil
}

// buildConnectionString builds a PostgreSQL connection string from the URL and credentials.
func (p *PostgreSQL) buildConnectionString() (string, error) {
	u, err := url.Parse(p.inp.URL())
	if err != nil {
		return "", fmt.Errorf("error parsing postgresql url: %w", err)
	}

	if p.inp.Credentials() != nil {
		var creds BasicCredentials

		if err := json.Unmarshal(p.inp.Credentials(), &creds); err != nil {
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
// JSON marshaling produces for these types.
func pgNormalizeValue(v any) any {
	switch val := v.(type) {
	case time.Time:
		return val.Format(time.RFC3339Nano)
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

// pgQueryError maps a syntax or semantic failure reported by PostgreSQL to a
// user-facing invalid-query error, leaving everything else as an internal one.
func pgQueryError(err error) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code[0:2] == "42" {
		return NewInvalidQueryError(pgErr.Message)
	}

	return fmt.Errorf("error executing query: %w", err)
}

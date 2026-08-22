// Package processor implements the per-type data-source clients used for connection tests and queries.
package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	// _mysqlConnMaxLifetime bounds how long a MySQL connection may be
	// reused.
	_mysqlConnMaxLifetime = 30 * time.Second

	// _mysqlConnectTimeout bounds the initial MySQL dial.
	_mysqlConnectTimeout = 10 * time.Second
)

// _mysqlReadOnlyPrivileges is the set of MySQL/MariaDB privileges considered read-only.
var _mysqlReadOnlyPrivileges = map[string]struct{}{
	"SELECT":             {},
	"SHOW VIEW":          {},
	"SHOW DATABASES":     {},
	"PROCESS":            {},
	"REPLICATION CLIENT": {},
	"REPLICATION SLAVE":  {},
	"USAGE":              {},
}

// MySQL represents a MySQL data source processor.
type MySQL struct {
	inp Input
}

// NewMySQL creates a new MySQL data source processor.
func NewMySQL(inp Input) *MySQL {
	return &MySQL{
		inp: inp,
	}
}

// TestConnection tests the connection to the MySQL data source.
func (m *MySQL) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	db, err := m.connect()
	if err != nil {
		return mysqlConnectionStatus(err), nil
	}
	defer db.Close() //nolint:errcheck // error provides no meaningful info

	if err = db.PingContext(ctx); err != nil {
		return mysqlConnectionStatus(err), nil
	}

	readOnly, err := mysqlCheckReadOnly(ctx, db)
	if err != nil {
		return mysqlConnectionStatus(err), nil
	}

	if !readOnly {
		return ConnectionStatusNotReadOnly, nil
	}

	return ConnectionStatusSuccess, nil
}

// _mysqlAuthErrors are the driver error numbers that mean the server refused
// the credentials rather than failing to answer.
var _mysqlAuthErrors = map[uint16]bool{
	1044: true, // access denied for user to database.
	1045: true, // access denied for user.
	1698: true, // access denied, authentication plugin refused.
}

// mysqlConnectionStatus classifies a failed connection attempt. A refused
// handshake is reported as unauthorized rather than unreachable: the two are
// indistinguishable to the user otherwise, and a typo in the password is the
// far more common of them.
func mysqlConnectionStatus(err error) ConnectionStatus {
	if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok && _mysqlAuthErrors[mysqlErr.Number] {
		return ConnectionStatusUnauthorized
	}

	return ConnectionStatusUnreachable
}

// Metadata retrieves all tables and their columns from the MySQL data source.
func (m *MySQL) Metadata(ctx context.Context) (*SQLMetadataResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	db, err := m.connect()
	if err != nil {
		return nil, fmt.Errorf("error connecting to mysql: %w", err)
	}
	defer db.Close() //nolint:errcheck // error provides no meaningful info

	rows, err := db.QueryContext(ctx, //nolint:rowserrcheck // scanInformationSchema checks rows.Err
		"SELECT table_schema, table_name, column_name FROM information_schema.columns WHERE table_schema NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys') ORDER BY table_schema, table_name, ordinal_position",
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching metadata: %w", err)
	}
	defer rows.Close() //nolint:errcheck // error provides no meaningful info

	tables, err := scanInformationSchema(rows)
	if err != nil {
		return nil, err
	}

	var defaultSchema string

	err = db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&defaultSchema)
	if err != nil {
		return nil, fmt.Errorf("error fetching default schema: %w", err)
	}

	return &SQLMetadataResult{
		Tables:        tables,
		DefaultSchema: defaultSchema,
	}, nil
}

// QueryLabels executes a SQL query with LIMIT 1 against the MySQL data source
// and returns the string (label) columns with their example values.
func (m *MySQL) QueryLabels(ctx context.Context, q string, tr TimeRange) (map[string]string, error) {
	q = tr.ProcessMySQLQuery(q)
	q = sqlSetLimit(q, 1)

	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	db, err := m.connect()
	if err != nil {
		return nil, fmt.Errorf("error connecting to mysql: %w", err)
	}
	defer db.Close() //nolint:errcheck // error provides no meaningful info

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok && mysqlErr.Number >= 1064 && mysqlErr.Number <= 1149 {
			return nil, NewInvalidQueryError(mysqlErr.Message)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close() //nolint:errcheck // error provides no meaningful info

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error reading columns: %w", err)
	}

	labels := make(map[string]string)

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating rows: %w", err)
		}

		return labels, nil
	}

	dest := make([]any, len(columns))
	ptrs := make([]any, len(columns))

	for i := range dest {
		ptrs[i] = &dest[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, fmt.Errorf("error scanning row: %w", err)
	}

	for i, v := range dest {
		switch s := v.(type) {
		case []byte:
			labels[columns[i]] = string(s)
		case string:
			labels[columns[i]] = s
		default:
		}
	}

	return labels, nil
}

// Query executes a SQL query against the MySQL data source.
func (m *MySQL) Query(ctx context.Context, q string, tr TimeRange) (*MySQLQueryResult, error) {
	q = tr.ProcessMySQLQuery(q)

	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	db, err := m.connect()
	if err != nil {
		return nil, fmt.Errorf("error connecting to mysql: %w", err)
	}
	defer db.Close() //nolint:errcheck // error provides no meaningful info

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok && mysqlErr.Number >= 1064 && mysqlErr.Number <= 1149 {
			return nil, NewInvalidQueryError(mysqlErr.Message)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close() //nolint:errcheck // error provides no meaningful info

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error reading columns: %w", err)
	}

	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("error reading column types: %w", err)
	}

	columnTypes := make([]string, len(types))
	for i, ct := range types {
		columnTypes[i] = ct.DatabaseTypeName()
	}

	var (
		resultRows  [][]any
		payloadSize int
	)

	for rows.Next() {
		dest := make([]any, len(columns))
		ptrs := make([]any, len(columns))

		for i := range dest {
			ptrs[i] = &dest[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		for i, v := range dest {
			payloadSize += estimateValueSize(v)

			// The MySQL driver returns string values as []byte.
			// Convert to string so they JSON-marshal as strings, not base64.
			if b, ok := v.([]byte); ok {
				dest[i] = string(b)
			}
		}

		if payloadSize > _queryPayloadLimit {
			return nil, NewInvalidQueryError("query result exceeds the maximum payload size")
		}

		resultRows = append(resultRows, dest)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &MySQLQueryResult{
		Columns:     columns,
		ColumnTypes: columnTypes,
		Rows:        resultRows,
	}, nil
}

// connect creates a database/sql connection using the data source URL and credentials.
func (m *MySQL) connect() (*sql.DB, error) {
	dsn, err := m.buildDSN()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("error connecting to mysql: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(_mysqlConnMaxLifetime)

	return db, nil
}

// buildDSN builds a MySQL DSN from the URL and credentials.
// Input URL format: mysql://user:pass@host:3306/db
// Output DSN format: user:pass@tcp(host:port)/db?parseTime=true&timeout=10s.
func (m *MySQL) buildDSN() (string, error) {
	u, err := url.Parse(m.inp.URL())
	if err != nil {
		return "", fmt.Errorf("error parsing mysql url: %w", err)
	}

	var (
		username string
		password string
	)

	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	if m.inp.Credentials() != nil {
		var creds BasicCredentials

		if err := json.Unmarshal(m.inp.Credentials(), &creds); err != nil {
			return "", fmt.Errorf("error unmarshaling credentials: %w", err)
		}

		if creds.Username != "" || creds.Password != "" {
			username = creds.Username
			password = creds.Password
		}
	}

	host := u.Hostname()
	port := u.Port()

	if port == "" {
		port = "3306"
	}

	dbName := strings.TrimPrefix(u.Path, "/")

	cfg := mysql.NewConfig()
	cfg.User = username
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = host + ":" + port
	cfg.DBName = dbName
	cfg.ParseTime = true
	cfg.Timeout = _mysqlConnectTimeout

	// carry the recognised connection parameters over from the URL; without
	// this a data source cannot ask for TLS and credentials cross the wire in
	// the clear.
	if tls := u.Query().Get("tls"); tls != "" {
		cfg.TLSConfig = tls
	}

	return cfg.FormatDSN(), nil
}

// MySQLQueryResult represents the result of a MySQL query.
type MySQLQueryResult struct {
	// Columns contains the column names of the result set.
	Columns []string `json:"columns"`

	// Rows contains the result rows, each as a slice of values.
	Rows [][]any `json:"rows"`

	// ColumnTypes contains the database type of each column, which is what
	// separates a numeric column from a label: the driver hands DECIMAL
	// over as bytes, exactly like a VARCHAR, so the value alone cannot
	// tell them apart. Kept out of the payload; it exists for
	// classification.
	ColumnTypes []string `json:"-"`
}

// sql returns the dialect-agnostic view of the result, which is where the
// transformation into series lives.
func (mqr *MySQLQueryResult) sql() sqlQueryResult {
	return sqlQueryResult{
		Columns:     mqr.Columns,
		Rows:        mqr.Rows,
		ColumnTypes: mqr.ColumnTypes,
	}
}

// Transform transforms a MySQLQueryResult into a unified QueryResult
// based on the requested chart type.
func (mqr *MySQLQueryResult) Transform(ct ChartType) *QueryResult {
	return mqr.sql().transform(ct, mysqlParseTimestamp, mysqlParseNumericValue)
}

// _sqlNumericTypes are the column types whose values carry a magnitude
// rather than a name. DECIMAL is the one that matters most: the driver
// returns it as bytes, so nothing about the value says it is a number.
var _sqlNumericTypes = map[string]bool{
	"TINYINT":            true,
	"SMALLINT":           true,
	"MEDIUMINT":          true,
	"INT":                true,
	"INTEGER":            true,
	"BIGINT":             true,
	"UNSIGNED TINYINT":   true,
	"UNSIGNED SMALLINT":  true,
	"UNSIGNED MEDIUMINT": true,
	"UNSIGNED INT":       true,
	"UNSIGNED BIGINT":    true,
	"DECIMAL":            true,
	"NUMERIC":            true,
	"FLOAT":              true,
	"DOUBLE":             true,
	"REAL":               true,
	"BIT":                true,
}

// mysqlCheckReadOnly checks whether the connected user has only read-only privileges
// by parsing the output of SHOW GRANTS FOR CURRENT_USER().
func mysqlCheckReadOnly(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, "SHOW GRANTS FOR CURRENT_USER()")
	if err != nil {
		return false, err
	}
	defer rows.Close() //nolint:errcheck // error provides no meaningful info

	for rows.Next() {
		var grant string
		if err := rows.Scan(&grant); err != nil {
			return false, err
		}

		grant = strings.ToUpper(grant)

		// Extract the privileges portion between "GRANT " and " ON ".
		grantIdx := strings.Index(grant, "GRANT ")
		onIdx := strings.Index(grant, " ON ")

		if grantIdx < 0 {
			continue
		}

		// a grant without " ON " is a role grant, whose privileges are not
		// listed here and can include writes; treat it as not read-only
		// rather than skipping it.
		if onIdx < 0 {
			return false, nil
		}

		privs := grant[grantIdx+len("GRANT ") : onIdx]

		if strings.Contains(privs, "ALL PRIVILEGES") {
			return false, nil
		}

		for p := range strings.SplitSeq(privs, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			if _, ok := _mysqlReadOnlyPrivileges[p]; !ok {
				return false, nil
			}
		}
	}

	if err := rows.Err(); err != nil {
		return false, err
	}

	return true, nil
}

// mysqlParseTimestamp parses a timestamp value from a MySQL result row.
// It handles float64 (unix seconds), int64 (unix seconds), time.Time, and string (RFC3339) values.
func mysqlParseTimestamp(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case int64:
		return val, true
	case time.Time:
		return val.Unix(), true
	case string:
		t, err := time.Parse(time.RFC3339Nano, val)
		if err != nil {
			t, err = time.Parse(time.RFC3339, val)
			if err != nil {
				return 0, false
			}
		}

		return t.Unix(), true
	case []byte:
		s := string(val)

		// Try parsing as a number first.
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(n), true
		}

		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return 0, false
			}
		}

		return t.Unix(), true
	default:
		return 0, false
	}
}

// mysqlParseNumericValue parses a numeric value from a MySQL result row.
// It handles float64, int64, and []byte (for DECIMAL columns).
func mysqlParseNumericValue(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case []byte:
		f, err := strconv.ParseFloat(string(val), 64)
		if err != nil {
			return 0, false
		}

		return f, true
	case string:
		// the driver hands DECIMAL over as []byte, which Query converts to
		// a string before the columns are classified; without this the
		// column reads as a label and the chart reports a data mismatch.
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, false
		}

		return f, true
	default:
		return 0, false
	}
}

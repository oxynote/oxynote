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
	"github.com/guregu/null/v5"
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
		return ConnectionStatusUnreachable, nil
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return ConnectionStatusUnreachable, nil
	}

	readOnly, err := mysqlCheckReadOnly(ctx, db)
	if err != nil {
		return ConnectionStatusUnreachable, nil
	}

	if !readOnly {
		return ConnectionStatusNotReadOnly, nil
	}

	return ConnectionStatusSuccess, nil
}

// Metadata retrieves all tables and their columns from the MySQL data source.
func (m *MySQL) Metadata(ctx context.Context) (*SQLMetadataResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	db, err := m.connect()
	if err != nil {
		return nil, fmt.Errorf("error connecting to mysql: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx,
		"SELECT table_schema, table_name, column_name FROM information_schema.columns WHERE table_schema NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys') ORDER BY table_schema, table_name, ordinal_position",
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching metadata: %w", err)
	}
	defer rows.Close()

	tables := make(map[string]SQLTable)

	for rows.Next() {
		var schema, tableName, columnName string

		if err := rows.Scan(&schema, &tableName, &columnName); err != nil {
			return nil, fmt.Errorf("error scanning metadata: %w", err)
		}

		key := schema + "." + tableName

		table := tables[key]
		table.Columns = append(table.Columns, SQLColumn{Name: columnName})
		tables[key] = table
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metadata: %w", err)
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
	defer db.Close()

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		var mysqlErr *mysql.MySQLError

		if errors.As(err, &mysqlErr) && mysqlErr.Number >= 1064 && mysqlErr.Number <= 1149 {
			return nil, NewInvalidQueryError(mysqlErr.Message)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error reading columns: %w", err)
	}

	labels := make(map[string]string)

	if !rows.Next() {
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
		if s, ok := v.([]byte); ok {
			labels[columns[i]] = string(s)
		} else if s, ok := v.(string); ok {
			labels[columns[i]] = s
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
	defer db.Close()

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		var mysqlErr *mysql.MySQLError

		if errors.As(err, &mysqlErr) && mysqlErr.Number >= 1064 && mysqlErr.Number <= 1149 {
			return nil, NewInvalidQueryError(mysqlErr.Message)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error reading columns: %w", err)
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
			payloadSize += mysqlEstimateValueSize(v)

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
		Columns: columns,
		Rows:    resultRows,
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
	db.SetConnMaxLifetime(30 * time.Second)

	return db, nil
}

// buildDSN builds a MySQL DSN from the URL and credentials.
// Input URL format: mysql://user:pass@host:3306/db
// Output DSN format: user:pass@tcp(host:port)/db?parseTime=true&timeout=10s
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
		var creds MySQLCredentials

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
	cfg.Timeout = 10 * time.Second

	return cfg.FormatDSN(), nil
}

// UpdateMySQLCredentials updates the credentials for a MySQL data source.
func UpdateMySQLCredentials(rawCreds Credentials, inp CredentialsUpdateInput) (Credentials, error) {
	var creds MySQLCredentials

	if rawCreds != nil {
		if err := json.Unmarshal(rawCreds, &creds); err != nil {
			return nil, fmt.Errorf("error unmarshaling credentials: %w", err)
		}
	}

	var update MySQLCredentialsUpdate

	if err := json.Unmarshal(inp, &update); err != nil {
		return nil, fmt.Errorf("error unmarshaling credentials update input: %w", err)
	}

	if update.Username.Valid {
		creds.Username = update.Username.String
	}

	if update.Password.Valid {
		creds.Password = update.Password.String
	}

	if creds.Username == "" && creds.Password == "" {
		return nil, nil
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("error marshaling updated credentials: %w", err)
	}

	return data, nil
}

// MySQLCredentials represents the credentials for a MySQL data source.
type MySQLCredentials struct {
	// Username is the username for the MySQL data source.
	Username string `json:"username"`

	// Password is the password for the MySQL data source.
	Password string `json:"password"`
}

// MySQLCredentialsUpdate represents the input for updating MySQL credentials.
type MySQLCredentialsUpdate struct {
	// Username is the username for the MySQL data source.
	Username null.String `json:"username"`

	// Password is the password for the MySQL data source.
	Password null.String `json:"password"`
}

// MySQLQueryResult represents the result of a MySQL query.
type MySQLQueryResult struct {
	// Columns contains the column names of the result set.
	Columns []string `json:"columns"`

	// Rows contains the result rows, each as a slice of values.
	Rows [][]any `json:"rows"`
}

// Transform transforms a MySQLQueryResult into a unified QueryResult
// based on the requested chart type.
func (mqr *MySQLQueryResult) Transform(ct ChartType) *QueryResult {
	if ct == "" {
		return &QueryResult{Status: QueryStatusTypeNotSelected}
	}

	if len(mqr.Columns) == 0 || len(mqr.Rows) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	timeIdx, valueIdxs, labelIdxs := mqr.identifyColumns()
	if timeIdx < 0 || len(valueIdxs) == 0 {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	}

	multipleValues := len(valueIdxs) > 1

	seriesMap := make(map[string]*QueryResultSeries)
	var seriesOrder []string

	for _, row := range mqr.Rows {
		if len(row) <= timeIdx {
			continue
		}

		ts, ok := mysqlParseTimestamp(row[timeIdx])
		if !ok {
			continue
		}

		labels := make(map[string]string, len(labelIdxs))
		var labelKeyParts []string

		for _, li := range labelIdxs {
			var lbl string
			if li < len(row) && row[li] != nil {
				lbl = fmt.Sprintf("%v", row[li])
			}

			labels[mqr.Columns[li]] = lbl
			labelKeyParts = append(labelKeyParts, mqr.Columns[li]+"="+lbl)
		}

		labelKey := strings.Join(labelKeyParts, "\x00")

		for _, vi := range valueIdxs {
			if vi >= len(row) {
				continue
			}

			val, ok := mysqlParseNumericValue(row[vi])
			if !ok || !isValidValue(val) {
				continue
			}

			key := mqr.Columns[vi] + "\x00" + labelKey

			if _, exists := seriesMap[key]; !exists {
				seriesLabels := make(map[string]string, len(labels)+1)
				for k, v := range labels {
					seriesLabels[k] = v
				}

				if multipleValues {
					seriesLabels["__name__"] = mqr.Columns[vi]
				}

				seriesMap[key] = &QueryResultSeries{
					Labels: seriesLabels,
				}
				seriesOrder = append(seriesOrder, key)
			}

			seriesMap[key].Metrics = append(seriesMap[key].Metrics, [2]any{ts, val})
		}
	}

	if len(seriesMap) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	series := make([]QueryResultSeries, 0, len(seriesOrder))

	for _, key := range seriesOrder {
		s := seriesMap[key]

		if ct == ChartTypeGauge && len(s.Metrics) > 0 {
			s.Metrics = s.Metrics[len(s.Metrics)-1:]
		}

		series = append(series, *s)
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data:   series,
	}
}

// identifyColumns detects which columns are time, value (numeric), and labels (non-numeric).
func (mqr *MySQLQueryResult) identifyColumns() (timeIdx int, valueIdxs []int, labelIdxs []int) {
	timeIdx = -1

	for i, col := range mqr.Columns {
		if strings.ToLower(col) == "time" {
			timeIdx = i
			break
		}
	}

	if len(mqr.Rows) == 0 {
		return
	}

	firstRow := mqr.Rows[0]

	for i := range mqr.Columns {
		if i == timeIdx {
			continue
		}

		if i < len(firstRow) {
			if _, ok := mysqlParseNumericValue(firstRow[i]); ok {
				valueIdxs = append(valueIdxs, i)
				continue
			}
		}

		labelIdxs = append(labelIdxs, i)
	}

	return
}

// mysqlCheckReadOnly checks whether the connected user has only read-only privileges
// by parsing the output of SHOW GRANTS FOR CURRENT_USER().
func mysqlCheckReadOnly(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, "SHOW GRANTS FOR CURRENT_USER()")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var grant string
		if err := rows.Scan(&grant); err != nil {
			return false, err
		}

		grant = strings.ToUpper(grant)

		// Extract the privileges portion between "GRANT " and " ON ".
		grantIdx := strings.Index(grant, "GRANT ")
		onIdx := strings.Index(grant, " ON ")

		if grantIdx < 0 || onIdx < 0 {
			continue
		}

		privs := grant[grantIdx+len("GRANT ") : onIdx]

		if strings.Contains(privs, "ALL PRIVILEGES") {
			return false, nil
		}

		for _, p := range strings.Split(privs, ",") {
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
	default:
		return 0, false
	}
}

// mysqlEstimateValueSize returns a rough byte-size estimate for a single value
// as it would appear in a JSON-encoded response.
func mysqlEstimateValueSize(v any) int {
	switch val := v.(type) {
	case nil:
		return 4 // "null"
	case bool:
		return 5 // "false"
	case string:
		return len(val) + 2
	case []byte:
		return len(val)
	default:
		return 8 // numeric types
	}
}

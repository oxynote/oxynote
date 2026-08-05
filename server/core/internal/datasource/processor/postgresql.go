package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// _queryTimeout is the maximum duration for a PostgreSQL query.
	_queryTimeout = 10 * time.Second

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
		return ConnectionStatusUnreachable, nil
	}
	defer conn.Close(ctx)

	if err := conn.Ping(ctx); err != nil {
		return ConnectionStatusUnreachable, nil
	}

	var readOnly bool

	err = conn.QueryRow(ctx,
		"SELECT current_setting('default_transaction_read_only')::bool OR pg_is_in_recovery()",
	).Scan(&readOnly)
	if err != nil {
		return ConnectionStatusUnreachable, nil
	}

	if !readOnly {
		return ConnectionStatusNotReadOnly, nil
	}

	return ConnectionStatusSuccess, nil
}

// Metadata retrieves all tables and their columns from the PostgreSQL data source.
func (p *PostgreSQL) Metadata(ctx context.Context) (*SQLMetadataResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	conn, err := p.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx,
		"SELECT table_schema, table_name, column_name FROM information_schema.columns WHERE table_schema NOT IN ('information_schema', 'pg_catalog') ORDER BY table_schema, table_name, ordinal_position",
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

	return &SQLMetadataResult{
		Tables:        tables,
		DefaultSchema: "public",
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
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, q)
	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code[0:2] == "42" {
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

// sqlSetLimit replaces or appends a LIMIT clause in the query to the given value.
func sqlSetLimit(q string, n int) string {
	re := regexp.MustCompile(`(?i)\bLIMIT\s+\d+\s*$`)

	limit := fmt.Sprintf("LIMIT %d", n)

	if re.MatchString(strings.TrimSpace(q)) {
		return re.ReplaceAllString(q, limit)
	}

	q = strings.TrimRight(q, "; \t\n")

	return q + " " + limit
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
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, q)
	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code[0:2] == "42" {
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
			payloadSize += pgEstimateValueSize(v)
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
func (p *PostgreSQL) connect(ctx context.Context) (*pgx.Conn, error) {
	connStr, err := p.buildConnectionString()
	if err != nil {
		return nil, err
	}

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to postgresql: %w", err)
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
		var creds PostgreSQLCredentials

		if err := json.Unmarshal(p.inp.Credentials(), &creds); err != nil {
			return "", fmt.Errorf("error unmarshaling credentials: %w", err)
		}

		if creds.Username != "" || creds.Password != "" {
			u.User = url.UserPassword(creds.Username, creds.Password)
		}
	}

	return u.String(), nil
}

// UpdatePostgreSQLCredentials updates the credentials for the PostgreSQL data source.
func UpdatePostgreSQLCredentials(rawCreds Credentials, inp CredentialsUpdateInput) (Credentials, error) {
	var creds PostgreSQLCredentials

	if rawCreds != nil {
		if err := json.Unmarshal(rawCreds, &creds); err != nil {
			return nil, fmt.Errorf("error unmarshaling credentials: %w", err)
		}
	}

	var update PostgreSQLCredentialsUpdate

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

// PostgreSQLCredentials represents the credentials for a PostgreSQL data source.
type PostgreSQLCredentials struct {
	// Username is the username for the PostgreSQL data source.
	Username string `json:"username"`

	// Password is the password for the PostgreSQL data source.
	Password string `json:"password"`
}

// PostgreSQLCredentialsUpdate represents the input for updating PostgreSQL credentials.
type PostgreSQLCredentialsUpdate struct {
	// Username is the username for the PostgreSQL data source.
	Username null.String `json:"username"`

	// Password is the password for the PostgreSQL data source.
	Password null.String `json:"password"`
}

// PostgreSQLQueryResult represents the result of a PostgreSQL query.
type PostgreSQLQueryResult struct {
	// Columns contains the column names of the result set.
	Columns []string `json:"columns"`

	// Rows contains the result rows, each as a slice of values.
	Rows [][]any `json:"rows"`
}

// Transform transforms a PostgreSQLQueryResult into a unified QueryResult
// based on the requested chart type.
//
// The SQL query is expected to return:
//   - A time column named "time" (as RFC3339 string or unix seconds)
//   - One or more numeric columns, each producing a separate series
//   - Any non-numeric, non-time columns are treated as series labels
//
// Each numeric column generates its own set of series. When there are multiple
// numeric columns, the column name is added as the "__name__" label to distinguish them.
func (pqr *PostgreSQLQueryResult) Transform(ct ChartType) *QueryResult {
	if ct == "" {
		return &QueryResult{Status: QueryStatusTypeNotSelected}
	}

	if len(pqr.Columns) == 0 || len(pqr.Rows) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	timeIdx, valueIdxs, labelIdxs := pqr.identifyColumns()
	if timeIdx < 0 || len(valueIdxs) == 0 {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	}

	multipleValues := len(valueIdxs) > 1

	// Group rows by (value column, label combination) into series, preserving insertion order.
	seriesMap := make(map[string]*QueryResultSeries)
	var seriesOrder []string

	for _, row := range pqr.Rows {
		if len(row) <= timeIdx {
			continue
		}

		ts, ok := pgParseTimestamp(row[timeIdx])
		if !ok {
			continue
		}

		// Build label key once per row (shared across value columns).
		labels := make(map[string]string, len(labelIdxs))
		var labelKeyParts []string

		for _, li := range labelIdxs {
			var lbl string
			if li < len(row) && row[li] != nil {
				lbl = fmt.Sprintf("%v", row[li])
			}

			labels[pqr.Columns[li]] = lbl
			labelKeyParts = append(labelKeyParts, pqr.Columns[li]+"="+lbl)
		}

		labelKey := strings.Join(labelKeyParts, "\x00")

		// Create a data point for each numeric value column.
		for _, vi := range valueIdxs {
			if vi >= len(row) {
				continue
			}

			val, ok := pgParseNumericValue(row[vi])
			if !ok || !isValidValue(val) {
				continue
			}

			// Build series key: value column name + label key.
			key := pqr.Columns[vi] + "\x00" + labelKey

			if _, exists := seriesMap[key]; !exists {
				seriesLabels := make(map[string]string, len(labels)+1)
				for k, v := range labels {
					seriesLabels[k] = v
				}

				if multipleValues {
					seriesLabels["__name__"] = pqr.Columns[vi]
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
			// For gauge, only keep the last value.
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
func (pqr *PostgreSQLQueryResult) identifyColumns() (timeIdx int, valueIdxs []int, labelIdxs []int) {
	timeIdx = -1

	// Identify time column by name.
	for i, col := range pqr.Columns {
		if strings.ToLower(col) == "time" {
			timeIdx = i
			break
		}
	}

	if len(pqr.Rows) == 0 {
		return
	}

	// Classify remaining columns by inspecting the first row's values.
	firstRow := pqr.Rows[0]

	for i := range pqr.Columns {
		if i == timeIdx {
			continue
		}

		if i < len(firstRow) {
			if _, ok := firstRow[i].(float64); ok {
				valueIdxs = append(valueIdxs, i)
				continue
			}
		}

		labelIdxs = append(labelIdxs, i)
	}

	return
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
	default:
		return 0, false
	}
}

// pgEstimateValueSize returns a rough byte-size estimate for a single value
// as it would appear in a JSON-encoded response.
func pgEstimateValueSize(v any) int {
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

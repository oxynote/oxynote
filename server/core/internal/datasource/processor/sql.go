package processor

import "fmt"

// SQLColumn represents a column in a SQL table.
type SQLColumn struct {
	// Name is the name of the column.
	Name string `json:"name"`
}

// SQLTable represents a table with its columns in a SQL database.
type SQLTable struct {
	// Columns contains the list of columns in the table.
	Columns []SQLColumn `json:"columns"`
}

// SQLMetadataResult represents the metadata of a SQL database.
type SQLMetadataResult struct {
	// Tables maps schema-qualified table names to their column definitions.
	Tables map[string]SQLTable `json:"tables"`

	// DefaultSchema is the default schema for the database (e.g., "public" for PostgreSQL).
	DefaultSchema string `json:"defaultSchema"`
}

// scanInformationSchema reads schema/table/column triples into the table map
// a metadata result carries.
func scanInformationSchema(rows metadataRows) (map[string]SQLTable, error) {
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

	return tables, nil
}

// metadataRows is the row cursor both SQL drivers expose, so that reading
// information_schema is written once.
type metadataRows interface {
	// Next should advance the cursor to the next row.
	Next() bool

	// Scan should read the current row into the given destinations.
	Scan(dest ...any) error

	// Err should report the error that ended the iteration, if any.
	Err() error
}

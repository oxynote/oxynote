package processor

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

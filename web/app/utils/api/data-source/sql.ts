export interface SQLMetadataResult {
	// the key (i.e., table name) includes the schema as well
	// Example: "public.users": ["id", "name", "email"]
	tables: Record<string, { columns: { name: string }[] }>
	defaultSchema: string
}

export interface SQLLabelsParams extends GenericQueryTimeRange {
	q: string
}

export interface SQLLabelsResult {
	// example: hostname: web-1
	labels: Record<string, string>
}

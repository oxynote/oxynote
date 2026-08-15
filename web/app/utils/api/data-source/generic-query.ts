export enum GenericQueryChartType {
	Line = "line_chart",
	Bar = "bar_chart",
	Gauge = "gauge_chart",
}

export interface GenericQueryTimeRange {
	from: Date | string
	to: Date | string
	timeRangeKey: string // key representing a preset time range (for caching; local)
}

export interface GenericQueryParams extends GenericQueryTimeRange {
	q: string
	chartType: GenericQueryChartType
}

export interface GenericMultipleQueriesParams extends GenericQueryTimeRange {
	queries: string[]
	chartType: GenericQueryChartType
}

export enum GenericQueryResultStatus {
	Ok = "ok",
	NoData = "no-data",
	TypeNotSelected = "type-not-selected", // legacy; should not be used anymore, as chart type is now required

	// This means that the returned data cannot be visualized with the
	// selected chart type.
	ChartAndDataMismatch = "chart-and-data-mismatch",

	// The data source returned data, but none of it can be turned into a
	// meaningful numeric value for the requested chart type — and it’s not
	// just a “wrong chart for this result type” situation.
	// More concretely, invalid is used only after the data was
	// structurally compatible, but all usable numeric samples were rejected.
	Invalid = "invalid",

	// This means that an error occurred during the query execution, and
	// the data source was not able to return any data.
	// NOTE: this is a local-only status, and should not be returned by the API.
	QueryError = "query-error",
}

export interface GenericQueryResult {
	status: GenericQueryResultStatus
	data: {
		labels: Record<string, string> // label name: label value
		metrics: [number, number][] // unix seconds (int) and value (int/float)
	}[]
	queryErrorMessage?: string // only set when status is QueryError (local-only)
}

export function formatQueryTimeRange(timeRange?: {
	from?: Date | string
	to?: Date | string
}): Record<string, string> {
	const params: Record<string, string> = {}

	if (timeRange?.from) {
		params.from =
			timeRange.from instanceof Date
				? timeRange.from.toISOString()
				: timeRange.from
	}

	if (timeRange?.to) {
		params.to =
			timeRange.to instanceof Date ? timeRange.to.toISOString() : timeRange.to
	}

	return params
}

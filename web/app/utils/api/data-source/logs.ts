export enum LogLevel {
	Critical = "critical",
	Error = "error",
	Warning = "warning",
	Info = "info",
	Debug = "debug",
	Trace = "trace",
	Unknown = "unknown",
}

// the order lines are read in, which is also a log block's order setting
export enum LogOrder {
	Desc = "desc",
	Asc = "asc",
}

export enum LogFieldKind {
	Label = "label",
	Field = "field",
}

// the block's queries, each read on its own, which the view merges
export interface LogQueriesParams extends GenericQueryTimeRange {
	queries: string[]
}

export interface LogLinesParams extends LogQueriesParams {
	limit?: number
	order?: LogOrder
}

export interface LogFieldsParams extends GenericQueryTimeRange {
	q?: string
}

export interface LogFieldValuesParams extends LogFieldsParams {
	field: string
}

export interface LogLine {
	id: string
	time: string // RFC 3339 with nanoseconds, which a Date would drop
	level: LogLevel
	line: string
	labels: Record<string, string>
	fields: Record<string, string>
}

export interface LogLinesResult {
	status: GenericQueryResultStatus.Ok | GenericQueryResultStatus.NoData
	lines: LogLine[]
	cursor: string | null // opaque; null when nothing older is left
	partial: boolean
}

// a page of every query's lines with the window it was read from, as
// from and to query parameters. Every older page reads the same window. A
// query's result is null on a page after it ran out of older lines.
export interface LogLinesPage {
	results: (LogLinesResult | null)[]
	window: Record<string, string>
}

export interface LogField {
	name: string
	kind: LogFieldKind
}

export interface LogFieldsResult {
	fields: LogField[]
}

export interface LogFieldValue {
	value: string
	count: number | null
}

export interface LogFieldValuesResult {
	values: LogFieldValue[]
}

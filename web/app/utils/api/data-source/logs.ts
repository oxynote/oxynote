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

export interface LogsParams extends LogQueriesParams {
	limit: number
	order?: LogOrder
}

export interface LogFieldsParams extends GenericQueryTimeRange {
	q?: string
}

export interface LogFieldValuesParams extends LogFieldsParams {
	field: string
}

export interface Log {
	id: string
	time: string // RFC 3339 with nanoseconds, which a Date would drop
	level: LogLevel
	line: string
	labels: Record<string, string>
	fields: Record<string, string>
}

// both cursors are opaque. nextCursor reads on past the result's last line,
// and previousCursor reads back from its first line in the other order.
// Either is null when the window holds nothing more that way.
export interface LogsResult {
	status: GenericQueryResultStatus.Ok | GenericQueryResultStatus.NoData
	lines: Log[]
	nextCursor: string | null
	previousCursor: string | null
	partial: boolean
}

// a page of every query's lines with the window it was read from, as
// from and to query parameters. Every page reads the same window. A
// query's result is null on a page that did not read it.
export interface LogsPage {
	results: (LogsResult | null)[]
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

export enum LogLevel {
	Critical = "critical",
	Error = "error",
	Warning = "warning",
	Info = "info",
	Debug = "debug",
	Trace = "trace",
	Unknown = "unknown",
}

export enum LogFieldKind {
	Label = "label",
	Field = "field",
}

export interface LogQueryParams extends GenericQueryTimeRange {
	q: string
}

export interface LogLinesParams extends LogQueryParams {
	limit?: number
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

// a page of lines with the window it was read from, as from and to query
// parameters. Every older page reads the same window.
export interface LogLinesPage extends LogLinesResult {
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

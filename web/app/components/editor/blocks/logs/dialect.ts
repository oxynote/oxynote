import type { KeyValue } from "./rows"

// the formats the view reads a line in, key by key
export enum LineFormat {
	JSON = "json",
	Logfmt = "logfmt",
}

// a filter a reader added from a line's fields. It narrows only the view
// it was added in and is never saved with the block.
export interface LogFilter {
	key: string
	value: string
	// a field the source answers from its index
	indexed: boolean
	exclude: boolean
	// the format of the line the key was read from. A source that reads
	// keys only when the query asks for them uses it to read the key.
	format?: LineFormat
}

// LogDialect holds what the view needs to know about a log source's query
// language and how the source names the keys it parses
export interface LogDialect {
	// withViewFilters returns the query narrowed by the view's filters and
	// search
	withViewFilters(query: string, filters: LogFilter[], search: string): string
	// canFilterLineKey tells whether a filter on a key read from a line of
	// the given format can run on the query
	canFilterLineKey(query: string, format: LineFormat): boolean
	// lineKeyName names a key read from the log's line the way the source
	// names it once parsed
	lineKeyName(key: string, log: Log): string
	// repeatsLineKey tells whether a key the source parsed repeats a key of
	// the line. A key the source made from a nested value counts only when
	// nested is true.
	repeatsLineKey(
		parsed: KeyValue,
		lineKey: string,
		lineValue: unknown,
		nested: boolean,
	): boolean
}

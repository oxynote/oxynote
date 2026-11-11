import { LineFormat, type LogDialect, type LogFilter } from "./dialect"

// the keys that repeat a line's level, which its pill already shows
const LEVEL_KEYS = new Set(["level", "lvl", "detected_level"])

// the keys a structured line names its message with, in order of preference
const MESSAGE_KEYS = ["msg", "message", "log"]

// one logfmt pair: a key, then a quoted or a bare value. A bare value ends
// at white space.
const LOGFMT_PAIR = /\s*([^\s="]+)=("(?:[^"\\]|\\.)*"|[^\s"]*)(?=\s|$)/y

export type KeyValue = [key: string, value: string]

// a key read from the line, or one the query's parser added
export interface ExtractedField {
	key: string
	value: string
	// how the row writes the value
	display: string
	// the format the key was read in, or null for a key the parser added
	format: LineFormat | null
	// an object or array, which a source may split into one key per entry
	nested: boolean
	// a JSON null, which a source may not parse as a value at all
	isNull: boolean
}

// one row of a details table. filter is what its buttons add, or null
// when they are off, and then disabledReason says why.
export interface FieldRow {
	key: string
	value: string
	filter: Omit<LogFilter, "exclude"> | null
	disabledReason?: string
}

// extracted holds the fields from the line, which a search can match.
// fields are carried beside the line, so a search cannot match them. Every
// value is ready to show.
export interface LogSummary {
	message: string
	extracted: KeyValue[]
	fields: KeyValue[]
	labels: KeyValue[]
}

// the logs one query loaded, and whether it has logs left on either side
export interface QueryLogs {
	logs: Log[]
	hasMore: boolean
	hasPrevious?: boolean
}

export interface TextPart {
	text: string
	match: boolean
}

interface ReadLine {
	record: Record<string, unknown>
	format: LineFormat
}

// summarizeLog splits a log into what its row shows. A JSON or logfmt line
// gives its message, and its other keys join the extracted fields. Any
// other line is its own message. A known level shows in the pill, so its
// keys stay out. An unknown one keeps them, so no part of the log goes
// missing.
export function summarizeLog(log: Log, dialect: LogDialect): LogSummary {
	const shown = (key: string) =>
		log.level === LogLevel.Unknown || !LEVEL_KEYS.has(key)
	const line = readLine(log.line)
	const messageKey = line
		? MESSAGE_KEYS.find((k) => typeof line.record[k] === "string")
		: undefined
	const message = line
		? messageKey
			? String(line.record[messageKey])
			: ""
		: log.line

	return {
		// a message of only white space is no message, so the row shows
		// nothing in its place
		message: message.trim() ? message : "",
		extracted: extract(log, line, dialect, true)
			.filter((f) => !(f.format && f.key === messageKey) && shown(f.key))
			.map((f): KeyValue => [f.key, f.display]),
		fields: sortedEntries(log.fields)
			.filter(([k]) => shown(k))
			.map(displayPair),
		labels: sortedEntries(log.labels).map(displayPair),
	}
}

// extractedFields lists the keys of a JSON or logfmt line in their order,
// then the keys the query's parser added. A parser key that repeats a line
// key stays out. A key the source made from a nested value stays in,
// because it can be filtered on and the nested value cannot.
export function extractedFields(
	log: Log,
	dialect: LogDialect,
): ExtractedField[] {
	return extract(log, readLine(log.line), dialect, false)
}

// displayValue quotes a value when it is empty or holds white space, so the
// pairs stay apart. It adds no escapes, so the text reads as written.
export function displayValue(value: string): string {
	return value === "" || /\s/.test(value) ? `"${value}"` : value
}

// highlightParts splits text around every case-insensitive occurrence of
// the search
export function highlightParts(text: string, search: string): TextPart[] {
	const term = search.trim().toLowerCase()
	if (!term) {
		return [{ text: text, match: false }]
	}

	const parts: TextPart[] = []
	const lower = text.toLowerCase()
	let start = 0
	let found = lower.indexOf(term)

	while (found >= 0) {
		if (found > start) {
			parts.push({ text: text.slice(start, found), match: false })
		}

		parts.push({ text: text.slice(found, found + term.length), match: true })
		start = found + term.length
		found = lower.indexOf(term, start)
	}

	if (start < text.length) {
		parts.push({ text: text.slice(start), match: false })
	}

	return parts
}

// mergeQueryLogs interleaves the logs each query loaded, in the order
// they were read: newest first or oldest first. A query with logs left
// past its loaded ones may still hold logs that belong before another
// query's last one, so the merge stops at the nearest point such a query
// has reached. A query with logs left before its loaded ones bounds the
// top of the merge the same way. A log two queries both matched shows
// once.
export function mergeQueryLogs(queries: QueryLogs[], order: LogOrder): Log[] {
	const past = pastIn(order)
	const { frontier, top } = mergeBounds(queries, order)
	const seen = new Set<string>()
	const merged: { log: Log; nanos: bigint }[] = []

	for (const query of queries) {
		for (const log of query.logs) {
			const nanos = timeNanos(log.time)

			if (
				seen.has(log.id) ||
				(frontier !== null && past(nanos, frontier)) ||
				(top !== null && past(top, nanos))
			) {
				continue
			}

			seen.add(log.id)
			merged.push({ log: log, nanos: nanos })
		}
	}

	// a stable sort keeps the source's order for logs sharing a time
	merged.sort((a, b) =>
		a.nanos === b.nanos ? 0 : past(a.nanos, b.nanos) ? 1 : -1,
	)

	return merged.map((m) => m.log)
}

// mergeBounds finds where the merge of the queries' logs stops. The
// frontier is the nearest point a query with logs left past its loaded
// ones has reached. The top is the same point for logs left before the
// loaded ones. Either is null when no query bounds that side.
export function mergeBounds(
	queries: QueryLogs[],
	order: LogOrder,
): { frontier: bigint | null; top: bigint | null } {
	const past = pastIn(order)
	let frontier: bigint | null = null
	let top: bigint | null = null

	for (const query of queries) {
		const last = query.logs.at(-1)
		const first = query.logs[0]

		if (query.hasMore && last) {
			const nanos = timeNanos(last.time)

			if (frontier === null || past(frontier, nanos)) {
				frontier = nanos
			}
		}

		if (query.hasPrevious && first) {
			const nanos = timeNanos(first.time)

			if (top === null || past(nanos, top)) {
				top = nanos
			}
		}
	}

	return { frontier: frontier, top: top }
}

// pastIn compares two times by reading order. It is true when a comes
// after b.
export function pastIn(order: LogOrder) {
	return (a: bigint, b: bigint) => (order === LogOrder.Asc ? a > b : a < b)
}

// timeNanos reads an RFC 3339 time as nanoseconds since the epoch. A Date
// would drop everything below a millisecond.
export function timeNanos(time: string): bigint {
	const fraction = /\.(\d+)/.exec(time)?.[1] ?? ""
	const millis = Date.parse(time.replace(/\.\d+/, ""))

	return (
		BigInt(Number.isNaN(millis) ? 0 : millis) * 1_000_000n +
		BigInt(fraction.padEnd(9, "0").slice(0, 9))
	)
}

// sortedEntries lists a record's pairs by key, so rows read the same way
// every time
export function sortedEntries(record: Record<string, string>): KeyValue[] {
	return Object.entries(record).sort(([a], [b]) => a.localeCompare(b))
}

function displayPair([key, value]: KeyValue): KeyValue {
	return [key, displayValue(value)]
}

// extract leaves out the keys the source made from a nested value only when
// nestedRepeats is true
function extract(
	log: Log,
	line: ReadLine | null,
	dialect: LogDialect,
	nestedRepeats: boolean,
): ExtractedField[] {
	const fromLine = line
		? Object.entries(line.record).map(([key, value]): ExtractedField => {
				const text = typeof value === "string" ? value : JSON.stringify(value)

				return {
					key: key,
					value: text,
					display: typeof value === "string" ? displayValue(value) : text,
					format: line.format,
					nested: value !== null && typeof value === "object",
					isNull: value === null,
				}
			})
		: []
	const fromParser = sortedEntries(log.parsed ?? {})
		.filter(
			(pair) =>
				!(
					line &&
					Object.entries(line.record).some(([k, v]) =>
						dialect.repeatsLineKey(pair, k, v, nestedRepeats),
					)
				),
		)
		.map(([key, value]): ExtractedField => ({
			key: key,
			value: value,
			display: displayValue(value),
			format: null,
			nested: false,
			isNull: false,
		}))

	return [...fromLine, ...fromParser]
}

// readLine reads a JSON or logfmt line key by key, or gives null for any
// other line
function readLine(line: string): ReadLine | null {
	const json = parseJSONObject(line)
	if (json) {
		return { record: json, format: LineFormat.JSON }
	}

	const logfmt = parseLogfmt(line)

	return logfmt ? { record: logfmt, format: LineFormat.Logfmt } : null
}

function parseJSONObject(text: string): Record<string, unknown> | null {
	if (!text.trimStart().startsWith("{")) {
		return null
	}

	try {
		const parsed: unknown = JSON.parse(text)

		return parsed && typeof parsed === "object" && !Array.isArray(parsed)
			? (parsed as Record<string, unknown>)
			: null
	} catch {
		return null
	}
}

// parseLogfmt reads a line made only of logfmt pairs. A line with any other
// word is plain text, so it gives null.
function parseLogfmt(line: string): Record<string, string> | null {
	const values = new Map<string, string>()
	let at = 0

	while (/\S/.test(line.slice(at))) {
			LOGFMT_PAIR.lastIndex = at
			const match = LOGFMT_PAIR.exec(line)
			if (!match) {
					return null
			}

			const [, key = "", value = ""] = match
			const quoted = value.startsWith('"')
			// Loki keeps the first value of a repeated key. But it skips an
			// empty value, and it drops a bare value that holds "=".
			if (!values.get(key) && (quoted || !value.includes("="))) {
					values.set(key, value.startsWith('"') ? unquote(value) : value)
			}

			at = LOGFMT_PAIR.lastIndex
	}

	return values.size ? Object.fromEntries(values) : null
}

// unquote reads a quoted logfmt value. Loki also reads \' as a quote mark,
// but JSON does not know it. Any other escape JSON does not know, such as
// Go's \x, leaves the text between the quotes as written.
function unquote(quoted: string): string {
	const json = quoted.replace(/\\(.)/g, (escape, char: string) =>
			char === "'" ? char : escape,
	)

	try {
		return String(JSON.parse(json))
	} catch {
		return quoted.slice(1, -1)
	}
}

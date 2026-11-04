// the keys that repeat a line's level, which its pill already shows
const LEVEL_KEYS = new Set(["level", "lvl", "detected_level"])

// the keys a JSON line names its message with, in order of preference
const MESSAGE_KEYS = ["msg", "message", "log"]

export type KeyValue = [key: string, value: string]

// fieldsInLine is true when the fields are written in the line itself.
// Only the line is searched, so only such fields can match a search.
export interface LogSummary {
	message: string
	fields: KeyValue[]
	fieldsInLine: boolean
	labels: KeyValue[]
}

export interface TextPart {
	text: string
	match: boolean
}

// summarizeLog splits a log into what its row shows: the message, the
// log's own fields and the stream labels. A JSON line gives its message
// and fields key by key. Any other line is its own message, and its fields
// are the ones the source parsed from it.
export function summarizeLog(log: Log): LogSummary {
	const labels = sortedEntries(log.labels)
	const json = parseJSONObject(log.line)

	if (!json) {
		return {
			message: log.line,
			fields: sortedEntries(log.fields).filter(([k]) => !LEVEL_KEYS.has(k)),
			fieldsInLine: false,
			labels: labels,
		}
	}

	const messageKey = MESSAGE_KEYS.find((k) => typeof json[k] === "string")
	const fields = Object.entries(json)
		.filter(([k]) => k !== messageKey && !LEVEL_KEYS.has(k))
		.map(([k, v]): KeyValue => [
			k,
			typeof v === "string" ? v : JSON.stringify(v),
		])

	return {
		message: messageKey ? String(json[messageKey]) : "",
		fields: fields,
		fieldsInLine: true,
		labels: labels,
	}
}

// displayValue writes a value as logfmt does: quoted only when it is empty
// or holds white space or a quote
export function displayValue(value: string): string {
	return value === "" || /[\s"]/.test(value) ? JSON.stringify(value) : value
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
export function mergeQueryLogs(
	queries: { logs: Log[]; hasMore: boolean; hasPrevious?: boolean }[],
	order: LogOrder,
): Log[] {
	// past compares two times by reading order. It is true when a comes
	// after b.
	const past = (a: bigint, b: bigint) =>
		order === LogOrder.Asc ? a > b : a < b
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

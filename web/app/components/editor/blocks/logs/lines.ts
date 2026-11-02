// the keys that repeat a line's level, which its pill already shows
const LEVEL_KEYS = new Set(["level", "lvl", "detected_level"])

// the keys a JSON line names its message with, in order of preference
const MESSAGE_KEYS = ["msg", "message", "log"]

export type KeyValue = [key: string, value: string]

export interface LineSummary {
	message: string
	fields: KeyValue[]
	labels: KeyValue[]
}

export interface TextPart {
	text: string
	match: boolean
}

// summarizeLine splits a line into what its row shows: the message, the
// line's own fields and the stream labels. A JSON line gives its message
// and fields key by key. Any other line is its own message, and its fields
// are the ones the source parsed from it.
export function summarizeLine(line: LogLine): LineSummary {
	const labels = sortedEntries(line.labels)
	const json = parseJSONObject(line.line)

	if (!json) {
		return {
			message: line.line,
			fields: sortedEntries(line.fields).filter(([k]) => !LEVEL_KEYS.has(k)),
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

// mergeQueryLines interleaves the lines each query loaded, newest first.
// A query with older pages left may still hold lines newer than another
// query's oldest, so the merge stops at the newest point a query with
// more to load has reached. A line two queries both matched shows once.
export function mergeQueryLines(
	queries: { lines: LogLine[]; hasMore: boolean }[],
): LogLine[] {
	let frontier: bigint | null = null

	for (const query of queries) {
		const oldest = query.lines.at(-1)

		if (query.hasMore && oldest) {
			const nanos = timeNanos(oldest.time)

			if (frontier === null || nanos > frontier) {
				frontier = nanos
			}
		}
	}

	const seen = new Set<string>()
	const merged: { line: LogLine; nanos: bigint }[] = []

	for (const query of queries) {
		for (const line of query.lines) {
			const nanos = timeNanos(line.time)

			if (seen.has(line.id) || (frontier !== null && nanos < frontier)) {
				continue
			}

			seen.add(line.id)
			merged.push({ line: line, nanos: nanos })
		}
	}

	// a stable sort keeps the source's order for lines sharing a time
	merged.sort((a, b) => (a.nanos === b.nanos ? 0 : a.nanos > b.nanos ? -1 : 1))

	return merged.map((m) => m.line)
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

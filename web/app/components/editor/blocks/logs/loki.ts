import { parser } from "@oxynote/lezer-logql"
import { LineFormat, type LogDialect, type LogFilter } from "./dialect"
import type { KeyValue } from "./rows"

// RE2 metacharacters, which a search has to match literally
const REGEX_SPECIAL = /[\\.+*?()|[\]{}^$]/g

// Loki names a parsed key with an underscore in place of any other
// character. It joins nested JSON keys with underscores too, and adds a
// suffix when the key clashes with a label or field.
const KEY_CHAR = /[^A-Za-z0-9_]/g
const CLASH_SUFFIX = "_extracted"

// the LogQL parser that reads each line format
const PARSERS: Record<LineFormat, string> = {
	[LineFormat.JSON]: "json",
	[LineFormat.Logfmt]: "logfmt",
}

// the stages that can rewrite the line or remove or rename a key. The row
// reads the line as it comes out of the query, so after such a stage its
// keys may not match the keys Loki parsed.
const LOG_CHANGING_STAGES = new Set([
	"DropLabelsExpr",
	"KeepLabelsExpr",
	"LabelFormatExpr",
	"LineFormatExpr",
	"DecolorizeExpr",
])

// lokiDialect reads LogQL queries and names keys as Loki's parsers do
export const lokiDialect: LogDialect = {
	withViewFilters,
	canFilterLineKey,
	lineKeyName,
	repeatsLineKey,
}

// withViewFilters returns the query narrowed by the view's filters and
// search. An indexed filter joins the stream selector. Any other filter,
// and an indexed one when the query has no selector to join, follows the
// query as a label filter. A filter on a key read from the line brings the
// parser for its format. The parser goes right after the stream selector,
// but only when the query has no parser of its own. The search follows as
// a case-insensitive line filter.
function withViewFilters(
	query: string,
	filters: LogFilter[],
	search: string,
): string {
	const selector = streamSelector(query)
	const joined = selector ? filters.filter((f) => f.indexed) : []
	let result = query

	if (selector && joined.length) {
		const inner = query.slice(selector.from + 1, selector.to).trim()
		const matchers = [inner, ...joined.map(matcher)].filter(Boolean)

		result = `${query.slice(0, selector.from)}{${matchers.join(", ")}}${query.slice(selector.to + 1)}`
	}

	const parsers = new Set(
		filters.flatMap((f) => (f.format ? PARSERS[f.format] : [])),
	)
	const end = streamSelector(result)?.to
	if (parsers.size && end !== undefined && !queryStages(query).parsers.length) {
		const stages = [...parsers].map((p) => ` | ${p}`).join("")

		result = `${result.slice(0, end + 1)}${stages}${result.slice(end + 1)}`
	}

	for (const filter of filters) {
		if (!joined.includes(filter)) {
			result += ` | ${matcher(filter)}`
		}
	}

	const term = search.trim()
	if (term) {
		result += ` |~ ${quote(`(?i)${term.replace(REGEX_SPECIAL, "\\$&")}`)}`
	}

	return result
}

// canFilterLineKey tells whether a filter on a key read from a line of the
// given format works on the query. It does when the query parses the line
// in that format, or parses it in no way so the filter can add the parser.
// It does not when the query parses the line some other way, or can change
// the line or its keys, because the key may then not reach the filter.
function canFilterLineKey(query: string, format: LineFormat): boolean {
	const stages = queryStages(query)

	return (
		!stages.unreadable &&
		!stages.changesLog &&
		(!stages.parsers.length || stages.parsers.includes(format))
	)
}

// lineKeyName names a key read from the line the way Loki's parser does. A
// name that clashes with a label or field gets Loki's suffix.
function lineKeyName(key: string, log: Log): string {
	const name = key.replace(KEY_CHAR, "_")
	const safe = /^[0-9]/.test(name) ? `_${name}` : name

	return Object.hasOwn(log.labels, safe) || Object.hasOwn(log.fields, safe)
		? `${safe}${CLASH_SUFFIX}`
		: safe
}

// repeatsLineKey tells whether a key Loki parsed is a line key under Loki's
// name for it, with the same value
function repeatsLineKey(
	[key, value]: KeyValue,
	lineKey: string,
	lineValue: unknown,
	nested: boolean,
): boolean {
	const name = key.endsWith(CLASH_SUFFIX)
		? key.slice(0, -CLASH_SUFFIX.length)
		: key
	const lineName = lineKey.replace(KEY_CHAR, "_")

	if (lineName === name) {
		return sameValue(value, lineValue)
	}

	return (
		nested &&
		lineValue !== null &&
		typeof lineValue === "object" &&
		name.startsWith(`${lineName}_`)
	)
}

// sameValue tells whether Loki's text for a value is the line's value. Loki
// keeps a JSON number as it is written in the line, which can differ from
// how JavaScript writes it. So numbers compare by value.
function sameValue(text: string, lineValue: unknown): boolean {
	if (typeof lineValue === "string") {
		return text === lineValue
	}

	if (typeof lineValue === "number") {
		return text.trim() !== "" && Number(text) === lineValue
	}

	return text === JSON.stringify(lineValue)
}

function matcher(filter: LogFilter): string {
	return `${filter.key}${filter.exclude ? "!=" : "="}${quote(filter.value)}`
}

// quote writes a double-quoted LogQL string. Loki reads it the way Go's
// strconv.Unquote does, which refuses a raw newline or other control
// character. JSON escapes those, and every JSON escape is one Go accepts.
function quote(value: string): string {
	return JSON.stringify(value)
}

// streamSelector finds the braces of the query's stream selector, the
// first brace outside a string. Braces inside quoted values do not count.
function streamSelector(query: string): { from: number; to: number } | null {
	let from = -1
	let quoteChar: string | null = null

	for (let i = 0; i < query.length; i++) {
		const char = query.charAt(i)

		if (quoteChar) {
			if (char === "\\" && quoteChar === '"') {
				i++
			} else if (char === quoteChar) {
				quoteChar = null
			}

			continue
		}

		if (char === '"' || char === "`") {
			quoteChar = char
		} else if (char === "{" && from < 0) {
			from = i
		} else if (char === "}" && from >= 0) {
			return { from, to: i }
		}
	}

	return null
}

// queryStages lists the parsers of the query's pipeline, as a line format
// or "other". A json or logfmt parser that names its keys counts as other,
// because it takes only those keys. unreadable is true when the query does
// not parse, so its stages are unknown.
function queryStages(query: string): {
	parsers: (LineFormat | "other")[]
	changesLog: boolean
	unreadable: boolean
} {
	const stages = {
		parsers: [] as (LineFormat | "other")[],
		changesLog: false,
		unreadable: false,
	}

	parser.parse(query).iterate({
		enter: (node) => {
			if (node.type.isError) {
				stages.unreadable = true
			} else if (LOG_CHANGING_STAGES.has(node.name)) {
				stages.changesLog = true
			} else if (node.name === "LogfmtParser") {
				stages.parsers.push(LineFormat.Logfmt)
			} else if (node.name === "LabelParser") {
				const json =
					node.node.firstChild?.name === "Json" &&
					!node.node.firstChild.nextSibling
				stages.parsers.push(json ? LineFormat.JSON : "other")
			} else if (
				node.name === "JsonExpressionParser" ||
				node.name === "LogfmtExpressionParser"
			) {
				stages.parsers.push("other")
			}
		},
	})

	return stages
}

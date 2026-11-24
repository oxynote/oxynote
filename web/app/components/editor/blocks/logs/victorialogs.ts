import { LineFormat, type LogDialect, type LogFilter } from "./dialect"
import { isNumberText, sameValue, type KeyValue } from "./rows"

// RE2 metacharacters, which a search has to match literally
const REGEX_SPECIAL = /[\\.+*?()|[\]{}^$]/g

// the escapes Go reads as one fixed character. A quote is escaped only
// inside its own kind of quotes, so it is handled apart.
const SIMPLE_ESCAPES: Record<string, string | undefined> = {
	a: "\x07",
	b: "\b",
	f: "\f",
	n: "\n",
	r: "\r",
	t: "\t",
	v: "\v",
	"\\": "\\",
}

// the escapes that write a number: its base and how many digits follow
const NUMBER_ESCAPES: Record<
	string,
	{ base: number; length: number } | undefined
> = {
	x: { base: 16, length: 2 },
	u: { base: 16, length: 4 },
	U: { base: 16, length: 8 },
}

// one logfmt pair: a key, then a value in double quotes, single quotes or
// backticks, or a bare value that ends at white space
const LOGFMT_PAIR =
	/\s*([^\s="]+)=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`[^`]*`|[^\s"]*)(?=\s|$)/y

const ENCODER = new TextEncoder()

// the pipe that reads each line format
const UNPACK_PIPES: Record<LineFormat, string> = {
	[LineFormat.JSON]: "unpack_json",
	[LineFormat.Logfmt]: "unpack_logfmt",
}

// a temporary name for a key that a filter reads out of the line.
// VictoriaLogs filters only on fields, so the key is unpacked into a field
// first. But unpacking under the key's own name would overwrite a field of
// the log that has the same name. So the key is unpacked under this prefix
// instead, then filtered on, then deleted again. The rows still show the
// log's own fields.
const LINE_KEY_PREFIX = "__oxynote_line."

// victoriaLogsDialect writes LogsQL and names keys as VictoriaLogs' unpack
// pipes do
export const victoriaLogsDialect: LogDialect = {
	withViewFilters,
	canFilterLineKey: () => true,
	lineKeyName: (key) => key,
	logfmtParseFailed: () => false,
	decodeLogfmtValue,
	repeatsLineKey,
	logfmtPair: LOGFMT_PAIR,
	logfmtKeepsLastValue: true,
	keepsLineKeyNames: true,
	indexesAllFields: true,
}

// withViewFilters returns the query narrowed by the view's filters and
// search. They follow the query as pipes, so they read the logs as the
// query returns them, which is how the rows show them. Each pipe starts
// a new line, because a comment at the end of the query would hide it.
//
// An indexed filter is a stream filter and any other one an exact match.
// The search is a case-insensitive match on the message. A filter on a
// key read from the line unpacks that key first, and removes it after
// the match so the rows keep their fields.
function withViewFilters(
	query: string,
	filters: LogFilter[],
	search: string,
): string {
	const matchers = filters
		.filter((f) => !f.format)
		.map((f) => (f.indexed ? streamMatcher(f) : fieldMatcher(f.key, f)))
	const term = search.trim()

	if (term) {
		matchers.push(
			`_msg:~${quote(`(?i)${term.replace(REGEX_SPECIAL, "\\$&")}`)}`,
		)
	}

	const pipes = matchers.length ? [`filter ${matchers.join(" ")}`] : []

	for (const format of Object.values(LineFormat)) {
		const lineFilters = filters.filter((f) => f.format === format)
		if (!lineFilters.length) {
			continue
		}

		pipes.push(
			`${UNPACK_PIPES[format]} fields (${lineFilters.map((f) => quote(f.key)).join(", ")}) result_prefix ${quote(LINE_KEY_PREFIX)}`,
			`filter ${lineFilters.map((f) => fieldMatcher(unpackedName(f), f)).join(" ")}`,
			`delete ${lineFilters.map((f) => quote(unpackedName(f))).join(", ")}`,
		)
	}

	return pipes.reduce((result, pipe) => `${result}\n| ${pipe}`, query)
}

// decodeLogfmtValue reads a value as VictoriaLogs' logfmt parser does. It
// unquotes a value in double quotes, single quotes or backticks. A quoted
// value it cannot unquote is kept as written.
function decodeLogfmtValue(raw: string): string {
	const quote = raw.charAt(0)
	if (quote === "`") {
		return raw.length > 1 && raw.endsWith("`")
			? raw.slice(1, -1).replaceAll("\r", "")
			: raw
	}

	if (quote !== '"' && quote !== "'") {
		return raw
	}

	return unquote(raw, quote) ?? raw
}

// unquote reads a quoted value the way Go's strconv.Unquote does, which is
// how VictoriaLogs reads it. An escape such as \x41 writes a single byte,
// so the bytes are collected first and read as UTF-8 at the end. It gives
// null when Go would refuse the value.
function unquote(raw: string, quote: string): string | null {
	if (raw.length < 2 || !raw.endsWith(quote)) {
		return null
	}

	// Go reads the value code point by code point
	const chars = Array.from(raw.slice(1, -1))
	const bytes: number[] = []
	let i = 0

	while (i < chars.length) {
		const char = chars[i] ?? ""
		if (char === quote) {
			return null
		}

		if (char !== "\\") {
			bytes.push(...ENCODER.encode(char))
			i++

			continue
		}

		const escape = readEscape(chars, i + 1, quote)
		if (!escape) {
			return null
		}

		bytes.push(...escape.bytes)
		i = escape.end
	}

	return new TextDecoder().decode(new Uint8Array(bytes))
}

// readEscape reads the escape that starts at chars[at], just after its
// backslash. It gives the bytes the escape writes and where the next
// character starts, or null when Go would refuse the escape.
function readEscape(
	chars: string[],
	at: number,
	quote: string,
): { bytes: number[]; end: number } | null {
	const escape = chars[at] ?? ""
	const simple = escape === quote ? quote : SIMPLE_ESCAPES[escape]

	if (simple !== undefined) {
		return { bytes: [...ENCODER.encode(simple)], end: at + 1 }
	}

	// an octal escape has no letter, so its first digit is the escape itself
	const octal = /[0-7]/.test(escape)
	const kind = octal ? { base: 8, length: 3 } : NUMBER_ESCAPES[escape]
	if (!kind) {
		return null
	}

	const start = octal ? at : at + 1
	const digits = chars.slice(start, start + kind.length).join("")
	const pattern = kind.base === 8 ? /^[0-7]+$/ : /^[0-9a-fA-F]+$/
	if (digits.length !== kind.length || !pattern.test(digits)) {
		return null
	}

	const value = parseInt(digits, kind.base)
	const end = start + kind.length

	// \x and octal escapes write one byte. \u and \U write a code point.
	if (escape !== "u" && escape !== "U") {
		return value > 255 ? null : { bytes: [value], end: end }
	}

	if (value > 0x10ffff || (value >= 0xd800 && value <= 0xdfff)) {
		return null
	}

	return { bytes: [...ENCODER.encode(String.fromCodePoint(value))], end: end }
}

// repeatsLineKey tells whether a field is a key of the line as an unpack
// pipe writes it. The pipe keeps the key's name and writes a list without
// spaces. It splits a nested value into one field per inner key, named
// with the outer key and a dot before it.
function repeatsLineKey(
	[key, value]: KeyValue,
	lineKey: string,
	lineValue: unknown,
	nested: boolean,
): boolean {
	if (key === lineKey) {
		return sameValue(value, lineValue)
	}

	return (
		nested &&
		key.startsWith(`${lineKey}.`) &&
		holdsPath(lineValue, key.slice(lineKey.length + 1), value)
	)
}

// holdsPath tells whether a nested value holds the text at a dotted path.
// An inner key can hold dots too, so each key is tried as a prefix.
function holdsPath(node: unknown, path: string, text: string): boolean {
	if (
		node === null ||
		typeof node !== "object" ||
		Array.isArray(node) ||
		isNumberText(node)
	) {
		return false
	}

	return Object.entries(node).some(([key, value]) =>
		key === path
			? sameValue(text, value)
			: path.startsWith(`${key}.`) &&
				holdsPath(value, path.slice(key.length + 1), text),
	)
}

function unpackedName(filter: LogFilter): string {
	return `${LINE_KEY_PREFIX}${filter.key}`
}

function streamMatcher(filter: LogFilter): string {
	return `{${quote(filter.key)}${filter.exclude ? "!=" : "="}${quote(filter.value)}}`
}

function fieldMatcher(field: string, filter: LogFilter): string {
	return `${filter.exclude ? "!" : ""}${quote(field)}:=${quote(filter.value)}`
}

// quote writes a double-quoted LogsQL string. VictoriaLogs reads it the
// way Go's strconv.Unquote does, and every JSON escape is one Go accepts.
function quote(value: string): string {
	return JSON.stringify(value)
}

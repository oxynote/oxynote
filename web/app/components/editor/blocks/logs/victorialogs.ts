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
// backticks, or a bare value. VictoriaLogs ends a value only at a space, so
// a tab is part of a bare value.
const LOGFMT_PAIR =
	/\s*([^\s="]+)=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`[^`]*`|[^ "]*)(?= |$)/y

const ENCODER = new TextEncoder()

// white space at either end of a text, as Go's strings.TrimSpace removes
// it. Unlike JavaScript's trim, it keeps U+FEFF and removes U+0085.
const GO_SPACE = /^(?:[^\S\uFEFF]|\u0085)+|(?:[^\S\uFEFF]|\u0085)+$/g

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
	readsLineKey,
	logfmtParseFailed: () => false,
	decodeLogfmtValue: unquoteLogsQL,
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

// before v1.54.0 VictoriaLogs unpacks a JSON line only when the line
// starts with its brace. The view also reads a line with white space
// before the brace. A logfmt log is read again as VictoriaLogs reads it,
// because the view reads only its first line, without a carriage return
// at its end.
//
// A value with bytes that are not UTF-8, such as "\xff", reaches the view
// with U+FFFD in their place. So the view cannot write a filter that
// matches the stored bytes.
function readsLineKey(
	log: Log,
	format: LineFormat,
	key: string,
	value: string,
): boolean {
	if (value.includes("\uFFFD")) {
		return false
	}

	if (format === LineFormat.Logfmt) {
		return unpackLogfmt(log.line).get(key) === value
	}

	return log.line.startsWith("{")
}

// unpackLogfmt reads text as VictoriaLogs' unpack_logfmt pipe does. A key
// ends at = or a space, and a bare value ends only at a space. A later
// value of a key replaces the earlier one.
function unpackLogfmt(text: string): Map<string, string> {
	const fields = new Map<string, string>()
	let rest: string | null = text

	while (rest !== null) {
		const end: number = rest.search(/[= ]/)
		const name = (end < 0 ? rest : rest.slice(0, end)).replace(GO_SPACE, "")
		const tail: string = end < 0 ? "" : rest.slice(end + 1)
		let value = ""
		let next: string | null = null

		if (end >= 0 && rest.charAt(end) === " ") {
			next = tail
		} else if (tail) {
			const quoted = readQuotedValue(tail)
			const space = tail.indexOf(" ")

			if (quoted) {
				value = quoted.value
				next = tail.startsWith(" ", quoted.length)
					? tail.slice(quoted.length + 1)
					: null
			} else {
				value = space < 0 ? tail : tail.slice(0, space)
				next = space < 0 ? null : tail.slice(space + 1)
			}
		}

		if (name || value) {
			fields.set(name, value)
		}

		rest = next
	}

	return fields
}

// readQuotedValue reads the quoted value at the start of text, as
// VictoriaLogs does. It gives the value and the length of its quoted text,
// or null when text does not start with a value VictoriaLogs can unquote.
function readQuotedValue(
	text: string,
): { value: string; length: number } | null {
	const quote = text.charAt(0)

	if (quote === "`") {
		const end = text.indexOf("`", 1)

		return end < 0
			? null
			: { value: text.slice(1, end).replaceAll("\r", ""), length: end + 1 }
	}

	if (quote === '"') {
		const end = doubleQuotedEnd(text)
		const value = end < 0 ? null : unquote(text.slice(0, end), quote)

		return value === null ? null : { value, length: end }
	}

	return quote === "'" ? readSingleQuoted(text) : null
}

// doubleQuotedEnd gives where the double-quoted text at the start of text
// ends, or -1 when it does not end. A Go string cannot hold a newline.
function doubleQuotedEnd(text: string): number {
	for (let i = 1; i < text.length; i++) {
		const char = text.charAt(i)
		if (char === "\n") {
			return -1
		}

		if (char === "\\") {
			i++
		} else if (char === '"') {
			return i + 1
		}
	}

	return -1
}

// readSingleQuoted reads a single-quoted value as VictoriaLogs does. It
// reads one character or escape at a time and may hold a newline. A \x or
// octal escape writes the character with that code, not a single byte.
function readSingleQuoted(
	text: string,
): { value: string; length: number } | null {
	const chars = Array.from(text)
	let value = ""
	let i = 1

	while (i < chars.length && chars[i] !== "'") {
		const char = chars[i] ?? ""
		if (char !== "\\") {
			value += char
			i++

			continue
		}

		const escape = readEscape(chars, i + 1, "'")
		if (!escape) {
			return null
		}

		const code = chars[i + 1] ?? ""
		value += /^[x0-7]$/.test(code)
			? String.fromCodePoint(escape.bytes[0] ?? 0)
			: new TextDecoder().decode(new Uint8Array(escape.bytes))
		i = escape.end
	}

	if (i >= chars.length) {
		return null
	}

	return { value, length: chars.slice(0, i + 1).join("").length }
}

// unquoteLogsQL reads text as VictoriaLogs reads a string, in a query or
// in a logfmt value. It unquotes text in double quotes, single quotes or
// backticks. Any other text, and quoted text it cannot unquote, is kept as
// written.
export function unquoteLogsQL(raw: string): string {
	const quote = raw.charAt(0)
	if (quote === "`") {
		return raw.length > 1 && raw.endsWith("`")
			? raw.slice(1, -1).replaceAll("\r", "")
			: raw
	}

	if (quote === "'") {
		const read = readSingleQuoted(raw)

		return read?.length === raw.length ? read.value : raw
	}

	if (quote !== '"') {
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

import { describe, it } from "vitest"
import { LineFormat, type LogFilter } from "./dialect"
import { extractedFields } from "./rows"
import { victoriaLogsDialect } from "./victorialogs"

// 25.0 as the line writes it
const RAW_25 = (JSON as JSON & { rawJSON: (text: string) => unknown }).rawJSON(
	"25.0",
)

const LOG: Log = {
	id: "log-1",
	time: "2026-01-01T12:00:00Z",
	level: LogLevel.Info,
	line: "",
	labels: { status: "label" },
	fields: { status: "field" },
}

describe("victoriaLogsDialect.withViewFilters", () => {
	it.for([
		{
			name: "returns the query unchanged without filters or search",
			input: { query: "_time:5m error", filters: [], search: "" },
			expected: "_time:5m error",
		},
		{
			name: "adds an indexed filter as a stream filter",
			input: {
				query: "error",
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: 'error\n| filter {"env"="prod"}',
		},
		{
			name: "adds an excluding indexed filter as a negative stream filter",
			input: {
				query: "error",
				filters: [filter("env", "prod", { indexed: true, exclude: true })],
				search: "",
			},
			expected: 'error\n| filter {"env"!="prod"}',
		},
		{
			name: "adds any other filter as an exact match",
			input: {
				query: "error",
				filters: [filter("status", "500")],
				search: "",
			},
			expected: 'error\n| filter "status":="500"',
		},
		{
			name: "negates an excluding filter",
			input: {
				query: "error",
				filters: [filter("status", "500", { exclude: true })],
				search: "",
			},
			expected: 'error\n| filter !"status":="500"',
		},
		{
			name: "quotes a field name and a value that hold special characters",
			input: {
				query: "*",
				filters: [filter("k8s.pod name", 'a "b"\nc')],
				search: "",
			},
			expected: String.raw`*
| filter "k8s.pod name":="a \"b\"\nc"`,
		},
		{
			name: "adds the search as a case-insensitive match on the message",
			input: { query: "*", filters: [], search: "Timeout" },
			expected: '*\n| filter _msg:~"(?i)Timeout"',
		},
		{
			name: "matches the regex characters of a search literally",
			input: { query: "*", filters: [], search: "a.b*(c)" },
			expected: String.raw`*
| filter _msg:~"(?i)a\\.b\\*\\(c\\)"`,
		},
		{
			name: "ignores a search of only white space",
			input: { query: "*", filters: [], search: "   " },
			expected: "*",
		},
		{
			name: "joins the filters and the search in one pipe",
			input: {
				query: "*",
				filters: [
					filter("env", "prod", { indexed: true }),
					filter("status", "500"),
				],
				search: " timeout ",
			},
			expected:
				'*\n| filter {"env"="prod"} "status":="500" _msg:~"(?i)timeout"',
		},
		{
			name: "keeps the pipes of the query before its own",
			input: {
				query: "error | unpack_json",
				filters: [filter("status", "500")],
				search: "",
			},
			expected: 'error | unpack_json\n| filter "status":="500"',
		},
		{
			name: "starts its pipe on a new line after a comment",
			input: {
				query: "error # only errors",
				filters: [filter("status", "500")],
				search: "",
			},
			expected: 'error # only errors\n| filter "status":="500"',
		},
		{
			name: "unpacks a key read from a JSON line under a prefix and removes it",
			input: {
				query: "*",
				filters: [filter("status", "500", { format: LineFormat.JSON })],
				search: "",
			},
			expected: [
				"*",
				'| unpack_json fields ("status") result_prefix "__oxynote_line."',
				'| filter "__oxynote_line.status":="500"',
				'| delete "__oxynote_line.status"',
			].join("\n"),
		},
		{
			name: "unpacks a key read from a logfmt line with the logfmt pipe",
			input: {
				query: "*",
				filters: [
					filter("status", "500", {
						format: LineFormat.Logfmt,
						exclude: true,
					}),
				],
				search: "",
			},
			expected: [
				"*",
				'| unpack_logfmt fields ("status") result_prefix "__oxynote_line."',
				'| filter !"__oxynote_line.status":="500"',
				'| delete "__oxynote_line.status"',
			].join("\n"),
		},
		{
			name: "unpacks every key of one line format together",
			input: {
				query: "*",
				filters: [
					filter("status", "500", { format: LineFormat.JSON }),
					filter("user", "ann", { format: LineFormat.JSON }),
				],
				search: "",
			},
			expected: [
				"*",
				'| unpack_json fields ("status", "user") result_prefix "__oxynote_line."',
				'| filter "__oxynote_line.status":="500" "__oxynote_line.user":="ann"',
				'| delete "__oxynote_line.status", "__oxynote_line.user"',
			].join("\n"),
		},
		{
			name: "runs the filters on fields before the ones on line keys",
			input: {
				query: "*",
				filters: [
					filter("status", "500", { format: LineFormat.JSON }),
					filter("env", "prod", { indexed: true }),
				],
				search: "x",
			},
			expected: [
				"*",
				'| filter {"env"="prod"} _msg:~"(?i)x"',
				'| unpack_json fields ("status") result_prefix "__oxynote_line."',
				'| filter "__oxynote_line.status":="500"',
				'| delete "__oxynote_line.status"',
			].join("\n"),
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			victoriaLogsDialect.withViewFilters(
				input.query,
				input.filters,
				input.search,
			),
		).toBe(expected)
	})
})

describe("victoriaLogsDialect.canFilterLineKey", () => {
	it.for([LineFormat.JSON, LineFormat.Logfmt])(
		"lets a %s line key filter run on a query with pipes",
		(format, { expect }) => {
			expect(
				victoriaLogsDialect.canFilterLineKey(
					"error | unpack_json | format 'x' as _msg",
					format,
				),
			).toBe(true)
		},
	)
})

describe("victoriaLogsDialect.lineKeyName", () => {
	it("keeps the name of a key that a label and a field also have", ({
		expect,
	}) => {
		expect(victoriaLogsDialect.lineKeyName("status", LOG)).toBe("status")
	})
})

describe("victoriaLogsDialect.logfmtParseFailed", () => {
	it("never reports a failed parse", ({ expect }) => {
		expect(victoriaLogsDialect.logfmtParseFailed(LOG)).toBe(false)
	})
})

describe("victoriaLogsDialect.decodeLogfmtValue", () => {
	it.for([
		{ name: "keeps a bare value", input: "billing", expected: "billing" },
		{
			name: "keeps an equals sign in a bare value",
			input: "/a=b",
			expected: "/a=b",
		},
		{
			name: "unquotes a quoted value",
			input: String.raw`"a \"b\" c"`,
			expected: 'a "b" c',
		},
		{
			name: "keeps a quoted value with an invalid escape as written",
			input: String.raw`"bad\q"`,
			expected: String.raw`"bad\q"`,
		},
		{
			name: "keeps a slash escape as written, which Go refuses",
			input: String.raw`"a\/b"`,
			expected: String.raw`"a\/b"`,
		},
		{
			name: "keeps an escaped single quote in double quotes as written",
			input: String.raw`"it\'s"`,
			expected: String.raw`"it\'s"`,
		},
		{
			name: "keeps a lone surrogate escape as written",
			input: String.raw`"a\ud800b"`,
			expected: String.raw`"a\ud800b"`,
		},
		{
			name: "keeps a value without its closing quote as written",
			input: '"abc',
			expected: '"abc',
		},
		{
			name: "keeps a value with an unescaped quote inside as written",
			input: "'a'b'",
			expected: "'a'b'",
		},
		{
			name: "reads the escapes Go reads as one character",
			input: String.raw`"\a\b\f\n\r\t\v\\"`,
			expected: "\x07\b\f\n\r\t\v\\",
		},
		{
			name: "reads a literal tab",
			input: '"a\tb"',
			expected: "a\tb",
		},
		{
			name: "reads hex and octal byte escapes",
			input: String.raw`"\x41\101"`,
			expected: "AA",
		},
		{
			name: "reads byte escapes that make one UTF-8 character",
			input: String.raw`"\xc3\xa9"`,
			expected: "é",
		},
		{
			name: "replaces a byte that is not UTF-8",
			input: String.raw`"\xff"`,
			expected: "\uFFFD",
		},
		{
			name: "keeps an octal escape above one byte as written",
			input: String.raw`"\400"`,
			expected: String.raw`"\400"`,
		},
		{
			name: "keeps a short hex escape as written",
			input: String.raw`"\x4"`,
			expected: String.raw`"\x4"`,
		},
		{
			name: "reads short and long unicode escapes",
			input: String.raw`"\u00e9\U0001F600"`,
			expected: "é😀",
		},
		{
			name: "keeps a unicode escape past the last code point as written",
			input: String.raw`"\U00110000"`,
			expected: String.raw`"\U00110000"`,
		},
		{
			name: "unquotes a value in single quotes",
			input: String.raw`'it\'s'`,
			expected: "it's",
		},
		{
			name: "unquotes a value in backticks without reading escapes",
			input: "`a\\nb\r`",
			expected: "a\\nb",
		},
		{
			name: "keeps a lone backtick as written",
			input: "`",
			expected: "`",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(victoriaLogsDialect.decodeLogfmtValue(input)).toBe(expected)
	})
})

describe("victoriaLogsDialect.repeatsLineKey", () => {
	it.for<{
		name: string
		field: [string, string]
		lineKey: string
		lineValue: unknown
		nested: boolean
		expected: boolean
	}>([
		{
			name: "is true for a key with the line's text",
			field: ["status", "ok"],
			lineKey: "status",
			lineValue: "ok",
			nested: false,
			expected: true,
		},
		{
			name: "is false for a key with another value",
			field: ["status", "ok"],
			lineKey: "status",
			lineValue: "failed",
			nested: false,
			expected: false,
		},
		{
			name: "is false for another key with the same value",
			field: ["state", "ok"],
			lineKey: "status",
			lineValue: "ok",
			nested: true,
			expected: false,
		},
		{
			name: "is true for a number as the line writes it",
			field: ["ratio", "25.0"],
			lineKey: "ratio",
			lineValue: RAW_25,
			nested: false,
			expected: true,
		},
		{
			name: "is true for a number an engine without source text read",
			field: ["ratio", "25.0"],
			lineKey: "ratio",
			lineValue: 25,
			nested: false,
			expected: true,
		},
		{
			name: "is false for another number",
			field: ["ratio", "26"],
			lineKey: "ratio",
			lineValue: RAW_25,
			nested: false,
			expected: false,
		},
		{
			name: "is true for a boolean",
			field: ["ok", "true"],
			lineKey: "ok",
			lineValue: true,
			nested: false,
			expected: true,
		},
		{
			name: "is true for a list written without spaces",
			field: ["tags", '["a","b"]'],
			lineKey: "tags",
			lineValue: ["a", "b"],
			nested: false,
			expected: true,
		},
		{
			name: "is true for a key made from a nested value",
			field: ["user.id", "7"],
			lineKey: "user",
			lineValue: { id: 7 },
			nested: true,
			expected: true,
		},
		{
			name: "is false for a key made from a nested value when nested keys stay",
			field: ["user.id", "7"],
			lineKey: "user",
			lineValue: { id: 7 },
			nested: false,
			expected: false,
		},
		{
			name: "is false for a dotted key beside a value that is not nested",
			field: ["user.id", "7"],
			lineKey: "user",
			lineValue: "ann",
			nested: true,
			expected: false,
		},
		{
			name: "is false for a dotted key beside a number",
			field: ["ratio.x", "1"],
			lineKey: "ratio",
			lineValue: RAW_25,
			nested: true,
			expected: false,
		},
		{
			name: "is false for a dotted key the nested value does not hold",
			field: ["user.region", "eu"],
			lineKey: "user",
			lineValue: { id: 7 },
			nested: true,
			expected: false,
		},
		{
			name: "is false for a nested key with another value",
			field: ["user.id", "8"],
			lineKey: "user",
			lineValue: { id: 7 },
			nested: true,
			expected: false,
		},
		{
			name: "is true for a key two levels deep",
			field: ["user.geo.city", "Riga"],
			lineKey: "user",
			lineValue: { geo: { city: "Riga" } },
			nested: true,
			expected: true,
		},
		{
			name: "is true for an inner key that holds a dot",
			field: ["user.geo.city", "Riga"],
			lineKey: "user",
			lineValue: { "geo.city": "Riga" },
			nested: true,
			expected: true,
		},
		{
			name: "is false for a dotted key beside a null",
			field: ["user.id", "7"],
			lineKey: "user",
			lineValue: null,
			nested: true,
			expected: false,
		},
	])("$name", ({ field, lineKey, lineValue, nested, expected }, { expect }) => {
		expect(
			victoriaLogsDialect.repeatsLineKey(field, lineKey, lineValue, nested),
		).toBe(expected)
	})
})

describe("victoriaLogsDialect.logfmtPair", () => {
	it.for([
		{ name: "single quotes", line: "x='ab cd' y=1" },
		{ name: "backticks", line: "x=`ab cd` y=1" },
		{ name: "double quotes", line: 'x="ab cd" y=1' },
	])("reads a value in $name", ({ line }, { expect }) => {
		const fields = extractedFields({ ...LOG, line }, victoriaLogsDialect)

		expect(fields.map((f) => [f.key, f.value])).toEqual([
			["x", "ab cd"],
			["y", "1"],
		])
	})

	it("keeps a line with bare words as plain text", ({ expect }) => {
		expect(
			extractedFields({ ...LOG, line: "foo a=1" }, victoriaLogsDialect),
		).toEqual([])
	})
})

function filter(
	key: string,
	value: string,
	options: { indexed?: boolean; exclude?: boolean; format?: LineFormat } = {},
): LogFilter {
	return {
		key,
		value,
		indexed: options.indexed ?? false,
		exclude: options.exclude ?? false,
		...(options.format ? { format: options.format } : {}),
	}
}

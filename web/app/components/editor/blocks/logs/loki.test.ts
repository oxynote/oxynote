import { describe, it } from "vitest"
import { LineFormat, type LogFilter } from "./dialect"
import { lokiDialect } from "./loki"

describe("lokiDialect.withViewFilters", () => {
	it.for([
		{
			name: "returns the query unchanged without filters or search",
			input: { query: '{job="api"} |= "x"', filters: [], search: "" },
			expected: '{job="api"} |= "x"',
		},
		{
			name: "joins an indexed filter after the selector's matchers",
			input: {
				query: '{job="api"}',
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: '{job="api", env="prod"}',
		},
		{
			name: "joins an excluding indexed filter as a negative matcher",
			input: {
				query: '{job="api"}',
				filters: [filter("env", "prod", { indexed: true, exclude: true })],
				search: "",
			},
			expected: '{job="api", env!="prod"}',
		},
		{
			name: "fills an empty selector with only the indexed filters",
			input: {
				query: "{ }",
				filters: [
					filter("env", "prod", { indexed: true }),
					filter("job", "api", { indexed: true }),
				],
				search: "",
			},
			expected: '{env="prod", job="api"}',
		},
		{
			name: "keeps the pipeline after the selector",
			input: {
				query: 'sum(count_over_time({job="api"} |= "x" [5m]))',
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: 'sum(count_over_time({job="api", env="prod"} |= "x" [5m]))',
		},
		{
			name: "skips braces inside a double-quoted value",
			input: {
				query: '{job="a{b}"}',
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: '{job="a{b}", env="prod"}',
		},
		{
			name: "skips an escaped quote inside a double-quoted value",
			input: {
				query: '{job="a\\"}"}',
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: '{job="a\\"}", env="prod"}',
		},
		{
			name: "skips braces inside a backtick value",
			input: {
				query: "{job=`a}\\`}",
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: '{job=`a}\\`, env="prod"}',
		},
		{
			name: "appends a non-indexed filter as a label filter",
			input: {
				query: '{job="api"}',
				filters: [filter("status", "500")],
				search: "",
			},
			expected: '{job="api"} | status="500"',
		},
		{
			name: "appends an excluding non-indexed filter as a negative label filter",
			input: {
				query: '{job="api"}',
				filters: [filter("status", "500", { exclude: true })],
				search: "",
			},
			expected: '{job="api"} | status!="500"',
		},
		{
			name: "appends an indexed filter as a label filter when no selector closes",
			input: {
				query: '{job="api"',
				filters: [filter("env", "prod", { indexed: true })],
				search: "",
			},
			expected: '{job="api" | env="prod"',
		},
		{
			name: "escapes backslashes and quotes in a filter value",
			input: {
				query: "{}",
				filters: [
					filter("path", 'C:\\a "b"', { indexed: true }),
					filter("msg", 'say "hi"'),
				],
				search: "",
			},
			expected: '{path="C:\\\\a \\"b\\""} | msg="say \\"hi\\""',
		},
		{
			name: "escapes a newline and a tab in a filter value and the search",
			input: {
				query: "{}",
				filters: [filter("msg", "a\nb\tc")],
				search: "x\ny",
			},
			expected: '{} | msg="a\\nb\\tc" |~ "(?i)x\\ny"',
		},
		{
			name: "appends a trimmed search as a case-insensitive line filter",
			input: { query: '{job="api"}', filters: [], search: "  timeout " },
			expected: '{job="api"} |~ "(?i)timeout"',
		},
		{
			name: "escapes regex metacharacters and quotes in the search",
			input: { query: "{}", filters: [], search: 'a.b(c)*"d"' },
			expected: '{} |~ "(?i)a\\\\.b\\\\(c\\\\)\\\\*\\"d\\""',
		},
		{
			name: "ignores a blank search",
			input: { query: "{}", filters: [], search: " \t" },
			expected: "{}",
		},
		{
			name: "applies selector matchers, then label filters in order, then the search",
			input: {
				query: '{job="api"} | json',
				filters: [
					filter("status", "500"),
					filter("env", "prod", { indexed: true }),
					filter("route", "/health", { exclude: true }),
				],
				search: "slow",
			},
			expected:
				'{job="api", env="prod"} | json | status="500" | route!="/health" |~ "(?i)slow"',
		},
		{
			name: "adds the parser a line key needs after the selector",
			input: {
				query: '{job="api"} |= "x" | line_format "{{.msg}}"',
				filters: [filter("user", "bob", { format: LineFormat.Logfmt })],
				search: "",
			},
			expected:
				'{job="api"} | logfmt |= "x" | line_format "{{.msg}}" | user="bob"',
		},
		{
			name: "adds each parser once, after the joined indexed filters",
			input: {
				query: '{job="api"}',
				filters: [
					filter("env", "prod", { indexed: true }),
					filter("a", "1", { format: LineFormat.JSON }),
					filter("b", "2", { format: LineFormat.JSON }),
				],
				search: "",
			},
			expected: '{job="api", env="prod"} | json | a="1" | b="2"',
		},
		{
			name: "adds no parser when the query has one",
			input: {
				query: '{job="api"} | json',
				filters: [filter("user", "bob", { format: LineFormat.JSON })],
				search: "",
			},
			expected: '{job="api"} | json | user="bob"',
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			lokiDialect.withViewFilters(input.query, input.filters, input.search),
		).toBe(expected)
	})
})

describe("lokiDialect.canFilterLineKey", () => {
	it.for([
		{ query: '{job="api"}', format: LineFormat.JSON, expected: true },
		{ query: '{job="api"} | json', format: LineFormat.JSON, expected: true },
		{
			query: '{job="api"} | logfmt --strict',
			format: LineFormat.Logfmt,
			expected: true,
		},
		{
			query: '{job="api"} | line_format "{{.x}}"',
			format: LineFormat.JSON,
			expected: false,
		},
		{
			query: '{job="api"} | json | line_format "{{.x}}"',
			format: LineFormat.JSON,
			expected: false,
		},
		{
			query: '{job="api"} | decolorize',
			format: LineFormat.Logfmt,
			expected: false,
		},
		{ query: '{job="api"} | json', format: LineFormat.Logfmt, expected: false },
		{
			query: '{job="api"} | pattern "<a>"',
			format: LineFormat.JSON,
			expected: false,
		},
		{
			query: '{job="api"} | json user="u"',
			format: LineFormat.JSON,
			expected: false,
		},
		{
			query: '{job="api"} | drop user',
			format: LineFormat.JSON,
			expected: false,
		},
		{
			query: '{job="api"} | json | label_format a=b',
			format: LineFormat.JSON,
			expected: false,
		},
		{ query: '{job="api" |', format: LineFormat.JSON, expected: false },
	])(
		"is $expected for $format keys and $query",
		({ query, format, expected }, { expect }) => {
			expect(lokiDialect.canFilterLineKey(query, format)).toBe(expected)
		},
	)
})

describe("lokiDialect.lineKeyName", () => {
	it.for([
		{ input: "user", expected: "user" },
		{ input: "http.status", expected: "http_status" },
		{ input: "2xx", expected: "_2xx" },
		{ input: "app", expected: "app_extracted" },
		{ input: "trace-id", expected: "trace_id_extracted" },
		{ input: "constructor", expected: "constructor" },
	])("names $input as $expected", ({ input, expected }, { expect }) => {
		const log: Log = {
			id: "l1",
			time: "2026-01-01T12:00:00Z",
			level: LogLevel.Info,
			line: "",
			labels: { app: "api" },
			fields: { trace_id: "abc" },
		}

		expect(lokiDialect.lineKeyName(input, log)).toBe(expected)
	})
})

// 25.0 as the line writes it
const RAW_25 = (JSON as JSON & { rawJSON: (text: string) => unknown }).rawJSON(
	"25.0",
)

describe("lokiDialect.repeatsLineKey", () => {
	it.for([
		{
			name: "is true for the same key and text",
			input: {
				parsed: ["msg", "m"],
				lineKey: "msg",
				lineValue: "m",
				nested: true,
			},
			expected: true,
		},
		{
			name: "is false for the same key with other text",
			input: {
				parsed: ["msg", "x"],
				lineKey: "msg",
				lineValue: "m",
				nested: true,
			},
			expected: false,
		},
		{
			name: "is true for a key Loki renamed",
			input: {
				parsed: ["http_status", "200"],
				lineKey: "http.status",
				lineValue: "200",
				nested: true,
			},
			expected: true,
		},
		{
			name: "is true for a key Loki marked as clashing",
			input: {
				parsed: ["msg_extracted", "m"],
				lineKey: "msg",
				lineValue: "m",
				nested: true,
			},
			expected: true,
		},
		{
			name: "compares a number by value, however Loki writes it",
			input: {
				parsed: ["n", "1e300"],
				lineKey: "n",
				lineValue: 1e300,
				nested: true,
			},
			expected: true,
		},
		{
			name: "compares a number kept as written by value",
			input: {
				parsed: ["ms", "25"],
				lineKey: "ms",
				lineValue: RAW_25,
				nested: true,
			},
			expected: true,
		},
		{
			name: "is false for a key that only starts like a number key",
			input: {
				parsed: ["ms_x", "25"],
				lineKey: "ms",
				lineValue: RAW_25,
				nested: true,
			},
			expected: false,
		},
		{
			name: "is false for a number the parser gave another value",
			input: {
				parsed: ["count", "two"],
				lineKey: "count",
				lineValue: 1,
				nested: true,
			},
			expected: false,
		},
		{
			name: "compares a boolean by its JSON text",
			input: {
				parsed: ["ok", "true"],
				lineKey: "ok",
				lineValue: true,
				nested: true,
			},
			expected: true,
		},
		{
			name: "is true for a key Loki made from a nested value",
			input: {
				parsed: ["user_id", "1"],
				lineKey: "user",
				lineValue: { id: 1 },
				nested: true,
			},
			expected: true,
		},
		{
			name: "is false for a key Loki made from a nested value when nested keys stay",
			input: {
				parsed: ["user_id", "1"],
				lineKey: "user",
				lineValue: { id: 1 },
				nested: false,
			},
			expected: false,
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			lokiDialect.repeatsLineKey(
				input.parsed as [string, string],
				input.lineKey,
				input.lineValue,
				input.nested,
			),
		).toBe(expected)
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

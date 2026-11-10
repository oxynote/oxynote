import { describe, it } from "vitest"
import {
	displayValue,
	extractedFields,
	highlightParts,
	mergeBounds,
	mergeQueryLogs,
	pastIn,
	sortedEntries,
	summarizeLog,
	timeNanos,
	type KeyValue,
	type LogSummary,
} from "./rows"
import { LineFormat, type LogDialect } from "./dialect"
const NOON_MILLIS = BigInt(Date.UTC(2026, 0, 1, 12, 0, 0))
// a source that names a parsed key as the line does, so these tests read
// only the generic rules
const EXACT_DIALECT: LogDialect = {
	withViewFilters: (query) => query,
	canFilterLineKey: () => true,
	lineKeyName: (key) => key,
	repeatsLineKey: ([key, value], lineKey, lineValue) =>
		key === lineKey && (typeof lineValue !== "string" || lineValue === value),
}

describe("summarizeLog", () => {
	it.for<{
		name: string
		input: Pick<Log, "line" | "labels" | "fields"> &
			Partial<Pick<Log, "level" | "parsed">>
		expected: LogSummary
	}>([
		{
			name: "keeps a plain line as its message with its fields by key",
			input: {
				line: "GET /health 200",
				labels: { job: "api", env: "prod" },
				fields: { status: "200", method: "GET", level: "info" },
			},
			expected: {
				message: "GET /health 200",
				extracted: [],
				fields: [
					["method", "GET"],
					["status", "200"],
				],
				labels: [
					["env", "prod"],
					["job", "api"],
				],
			},
		},
		{
			name: "shows the keys a parser took from a plain line",
			input: {
				line: "GET /health 200",
				labels: {},
				fields: {},
				parsed: { status: "200", method: "GET" },
			},
			expected: {
				message: "GET /health 200",
				extracted: [
					["method", "GET"],
					["status", "200"],
				],
				fields: [],
				labels: [],
			},
		},
		{
			name: "splits a logfmt line and leaves out the parser keys it repeats",
			input: {
				level: LogLevel.Error,
				line: 'level=error msg="value out of range" value=+Inf',
				labels: {},
				fields: { trace_id: "abc" },
				parsed: { level: "error", msg: "value out of range", value: "+Inf" },
			},
			expected: {
				message: "value out of range",
				extracted: [["value", "+Inf"]],
				fields: [["trace_id", "abc"]],
				labels: [],
			},
		},
		{
			name: "keeps a parser key whose value differs from the line",
			input: {
				line: "status=200",
				labels: {},
				fields: {},
				parsed: { status: "500" },
			},
			expected: {
				message: "",
				extracted: [
					["status", "200"],
					["status", "500"],
				],
				fields: [],
				labels: [],
			},
		},
		{
			name: "reads quoted and empty logfmt values without escapes",
			input: {
				line: 'msg="say \\"hi\\"" empty= spaced="two words" go="\\x01"',
				labels: {},
				fields: {},
			},
			expected: {
				message: 'say "hi"',
				extracted: [
					["empty", '""'],
					["spaced", '"two words"'],
					["go", "\\x01"],
				],
				fields: [],
				labels: [],
			},
		},
		{
			name: "keeps the first value of a repeated logfmt key, as Loki does",
			input: {
				line: "component=billing component=stripe-client attempt=1 attempt=2",
				labels: {},
				fields: {},
			},
			expected: {
				message: "",
				extracted: [
					["component", "billing"],
					["attempt", "1"],
				],
				fields: [],
				labels: [],
			},
		},
		{
			name: "keeps a line with a word outside logfmt pairs as plain text",
			input: { line: "user=bob logged in", labels: {}, fields: {} },
			expected: {
				message: "user=bob logged in",
				extracted: [],
				fields: [],
				labels: [],
			},
		},
		{
			name: "keeps the level keys when the level is unknown",
			input: {
				level: LogLevel.Unknown,
				line: "level=notice msg=hi",
				labels: {},
				fields: { detected_level: "notice" },
				parsed: { level: "notice" },
			},
			expected: {
				message: "hi",
				extracted: [["level", "notice"]],
				fields: [["detected_level", "notice"]],
				labels: [],
			},
		},
		{
			name: "gives a line of only white space no message",
			input: { line: "   ", labels: { app: "api" }, fields: {} },
			expected: {
				message: "",
				extracted: [],
				fields: [],
				labels: [["app", "api"]],
			},
		},
		{
			name: "reads the message and fields of a JSON line in source order",
			input: {
				line: '{"status":"200","msg":"done","level":"info","lvl":"i","detected_level":"info","method":"GET"}',
				labels: { job: "api" },
				fields: {},
			},
			expected: {
				message: "done",
				extracted: [
					["status", "200"],
					["method", "GET"],
				],
				fields: [],
				labels: [["job", "api"]],
			},
		},
		{
			name: "keeps the fields carried beside a JSON line",
			input: {
				line: '{"msg":"login ok","user":"bob"}',
				labels: {},
				fields: { trace_id: "abc", detected_level: "info" },
			},
			expected: {
				message: "login ok",
				extracted: [["user", "bob"]],
				fields: [["trace_id", "abc"]],
				labels: [],
			},
		},
		{
			name: "writes non-string JSON values as JSON without escapes",
			input: {
				line: '{"message":"done","count":3,"ok":true,"none":null,"user":{"id":1,"name":"a b"},"tags":["a"],"quote":"say \\"hi\\""}',
				labels: {},
				fields: {},
			},
			expected: {
				message: "done",
				extracted: [
					["count", "3"],
					["ok", "true"],
					["none", "null"],
					["user", '{"id":1,"name":"a b"}'],
					["tags", '["a"]'],
					["quote", '"say "hi""'],
				],
				fields: [],
				labels: [],
			},
		},
		{
			name: "prefers msg over message and log",
			input: {
				line: '{"log":"c","message":"b","msg":"a"}',
				labels: {},
				fields: {},
			},
			expected: {
				message: "a",
				extracted: [
					["log", "c"],
					["message", "b"],
				],
				fields: [],
				labels: [],
			},
		},
		{
			name: "takes the log key when no other message key is a string",
			input: {
				line: '{"msg":5,"log":"started"}',
				labels: {},
				fields: {},
			},
			expected: {
				message: "started",
				extracted: [["msg", "5"]],
				fields: [],
				labels: [],
			},
		},
		{
			name: "leaves the message empty for a JSON line without one",
			input: { line: '  {"status":"200"}', labels: {}, fields: {} },
			expected: {
				message: "",
				extracted: [["status", "200"]],
				fields: [],
				labels: [],
			},
		},
		{
			name: "keeps a line that only starts like JSON as plain text",
			input: { line: "{not json", labels: {}, fields: { a: "1" } },
			expected: {
				message: "{not json",
				extracted: [],
				fields: [["a", "1"]],
				labels: [],
			},
		},
		{
			name: "keeps a JSON array line as plain text",
			input: { line: '["a"]', labels: {}, fields: {} },
			expected: {
				message: '["a"]',
				extracted: [],
				fields: [],
				labels: [],
			},
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			summarizeLog(makeLog({ id: "l1", ...input }), EXACT_DIALECT),
		).toEqual(expected)
	})
})

describe("extractedFields", () => {
	it("lists every line key, then the parser keys the line does not repeat, keeping the keys of a nested value", ({
		expect,
	}) => {
		expect(
			extractedFields(
				makeLog({
					id: "l1",
					line: '{"msg":"m","n":2,"user":{"id":1}}',
					parsed: { msg: "m", user_id: "1", region: "eu" },
				}),
				EXACT_DIALECT,
			),
		).toEqual([
			{
				key: "msg",
				value: "m",
				display: "m",
				format: LineFormat.JSON,
				nested: false,
				isNull: false,
			},
			{
				key: "n",
				value: "2",
				display: "2",
				format: LineFormat.JSON,
				nested: false,
				isNull: false,
			},
			{
				key: "user",
				value: '{"id":1}',
				display: '{"id":1}',
				format: LineFormat.JSON,
				nested: true,
				isNull: false,
			},
			{
				key: "region",
				value: "eu",
				display: "eu",
				format: null,
				nested: false,
				isNull: false,
			},
			{
				key: "user_id",
				value: "1",
				display: "1",
				format: null,
				nested: false,
				isNull: false,
			},
		])
	})

	it("reads a logfmt line in its own format", ({ expect }) => {
		expect(
			extractedFields(makeLog({ id: "l1", line: 'msg="a b"' }), EXACT_DIALECT),
		).toEqual([
			{
				key: "msg",
				value: "a b",
				display: '"a b"',
				format: LineFormat.Logfmt,
				nested: false,
				isNull: false,
			},
		])
	})
})

describe("displayValue", () => {
	it.for([
		{ input: "GET", expected: "GET" },
		{ input: "a=b", expected: "a=b" },
		{ input: "", expected: '""' },
		{ input: "two words", expected: '"two words"' },
		{ input: "tab\there", expected: '"tab\there"' },
		{ input: 'say "hi"', expected: '"say "hi""' },
		{ input: 'a"b', expected: 'a"b' },
	])("writes $input as $expected", ({ input, expected }, { expect }) => {
		expect(displayValue(input)).toBe(expected)
	})
})

describe("highlightParts", () => {
	it.for([
		{
			name: "returns the whole text unmarked for an empty search",
			input: { text: "Timeout", search: "" },
			expected: [{ text: "Timeout", match: false }],
		},
		{
			name: "returns the whole text unmarked for a blank search",
			input: { text: "Timeout", search: "  " },
			expected: [{ text: "Timeout", match: false }],
		},
		{
			name: "returns the whole text unmarked when nothing matches",
			input: { text: "Timeout", search: "error" },
			expected: [{ text: "Timeout", match: false }],
		},
		{
			name: "marks a match in the middle in any case",
			input: { text: "read TIMEOUT now", search: " timeout " },
			expected: [
				{ text: "read ", match: false },
				{ text: "TIMEOUT", match: true },
				{ text: " now", match: false },
			],
		},
		{
			name: "marks matches at both ends and repeated ones",
			input: { text: "abXabab", search: "ab" },
			expected: [
				{ text: "ab", match: true },
				{ text: "X", match: false },
				{ text: "ab", match: true },
				{ text: "ab", match: true },
			],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(highlightParts(input.text, input.search)).toEqual(expected)
	})
})

describe("mergeQueryLogs", () => {
	it.for([
		{
			name: "returns nothing without queries",
			input: () => [],
			expected: [],
		},
		{
			name: "orders one query's lines newest first",
			input: () => [
				{
					logs: [line("a", "12:00:00.1Z"), line("b", "12:00:00.3Z")],
					hasMore: false,
				},
			],
			expected: ["b", "a"],
		},
		{
			name: "interleaves queries by their nanosecond times",
			input: () => [
				{
					logs: [
						line("a3", "12:00:00.000000003Z"),
						line("a1", "12:00:00.000000001Z"),
					],
					hasMore: false,
				},
				{ logs: [line("b2", "12:00:00.000000002Z")], hasMore: false },
			],
			expected: ["a3", "b2", "a1"],
		},
		{
			name: "shows a line two queries matched once",
			input: () => [
				{
					logs: [line("a", "12:00:02Z"), line("s", "12:00:01Z")],
					hasMore: false,
				},
				{ logs: [line("s", "12:00:01Z")], hasMore: false },
			],
			expected: ["a", "s"],
		},
		{
			name: "hides lines older than where a query with more to load stopped",
			input: () => [
				{
					logs: [line("a1", "12:00:05Z"), line("a2", "12:00:01Z")],
					hasMore: false,
				},
				{
					logs: [line("b1", "12:00:04Z"), line("b2", "12:00:03Z")],
					hasMore: true,
				},
			],
			expected: ["a1", "b1", "b2"],
		},
		{
			name: "stops at the newest point among queries with more to load",
			input: () => [
				{
					logs: [line("a1", "12:00:05Z"), line("a2", "12:00:02Z")],
					hasMore: true,
				},
				{
					logs: [line("b1", "12:00:04Z"), line("b2", "12:00:01Z")],
					hasMore: true,
				},
			],
			expected: ["a1", "b1", "a2"],
		},
		{
			name: "hides lines newer than where a query with lines before its loaded ones starts",
			input: () => [
				{
					logs: [line("a1", "12:00:05Z"), line("a2", "12:00:01Z")],
					hasMore: false,
				},
				{
					logs: [line("b1", "12:00:04Z"), line("b2", "12:00:02Z")],
					hasMore: false,
					hasPrevious: true,
				},
			],
			expected: ["b1", "b2", "a2"],
		},
		{
			name: "starts at the oldest point among queries with lines before their loaded ones",
			input: () => [
				{
					logs: [line("a1", "12:00:05Z"), line("a2", "12:00:02Z")],
					hasMore: false,
					hasPrevious: true,
				},
				{
					logs: [line("b1", "12:00:04Z"), line("b2", "12:00:01Z")],
					hasMore: false,
					hasPrevious: true,
				},
			],
			expected: ["b1", "a2", "b2"],
		},
		{
			name: "ignores a query with more to load but no lines",
			input: () => [
				{
					logs: [line("a", "12:00:02Z"), line("b", "12:00:01Z")],
					hasMore: false,
				},
				{ logs: [], hasMore: true },
			],
			expected: ["a", "b"],
		},
		{
			name: "keeps the source order of lines sharing a time",
			input: () => [
				{
					logs: [line("a", "12:00:01Z"), line("b", "12:00:01Z")],
					hasMore: false,
				},
				{ logs: [line("c", "12:00:01Z")], hasMore: false },
			],
			expected: ["a", "b", "c"],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(mergeQueryLogs(input(), LogOrder.Desc).map((l) => l.id)).toEqual(
			expected,
		)
	})

	it.for([
		{
			name: "orders one oldest-first query's lines oldest first",
			input: () => [
				{
					logs: [line("b", "12:00:00.3Z"), line("a", "12:00:00.1Z")],
					hasMore: false,
				},
			],
			expected: ["a", "b"],
		},
		{
			name: "hides lines newer than where an oldest-first query with more to load stopped",
			input: () => [
				{
					logs: [line("a1", "12:00:01Z"), line("a2", "12:00:05Z")],
					hasMore: false,
				},
				{
					logs: [line("b1", "12:00:02Z"), line("b2", "12:00:03Z")],
					hasMore: true,
				},
			],
			expected: ["a1", "b1", "b2"],
		},
		{
			name: "hides lines older than where an oldest-first query with lines before its loaded ones starts",
			input: () => [
				{
					logs: [line("a1", "12:00:01Z"), line("a2", "12:00:05Z")],
					hasMore: false,
				},
				{
					logs: [line("b1", "12:00:02Z"), line("b2", "12:00:03Z")],
					hasMore: false,
					hasPrevious: true,
				},
			],
			expected: ["b1", "b2", "a2"],
		},
		{
			name: "stops at the oldest point among oldest-first queries with more to load",
			input: () => [
				{
					logs: [line("a1", "12:00:01Z"), line("a2", "12:00:04Z")],
					hasMore: true,
				},
				{
					logs: [line("b1", "12:00:02Z"), line("b2", "12:00:05Z")],
					hasMore: true,
				},
			],
			expected: ["a1", "b1", "a2"],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(mergeQueryLogs(input(), LogOrder.Asc).map((l) => l.id)).toEqual(
			expected,
		)
	})
})

describe("mergeBounds", () => {
	it.for([
		{
			name: "has no bounds when no query has lines left",
			order: LogOrder.Desc,
			input: () => [{ logs: [line("a", "12:00:01Z")], hasMore: false }],
			expected: { frontier: null, top: null },
		},
		{
			name: "stops at the newest last line of the queries with more, newest first",
			order: LogOrder.Desc,
			input: () => [
				{
					logs: [line("a2", "12:00:04Z"), line("a1", "12:00:01Z")],
					hasMore: true,
				},
				{
					logs: [line("b2", "12:00:05Z"), line("b1", "12:00:02Z")],
					hasMore: true,
				},
				{ logs: [line("c1", "12:00:03Z")], hasMore: false },
			],
			expected: {
				frontier: NOON_MILLIS * 1_000_000n + 2_000_000_000n,
				top: null,
			},
		},
		{
			name: "stops at the oldest last line of the queries with more, oldest first",
			order: LogOrder.Asc,
			input: () => [
				{
					logs: [line("a1", "12:00:01Z"), line("a2", "12:00:04Z")],
					hasMore: true,
				},
				{
					logs: [line("b1", "12:00:02Z"), line("b2", "12:00:05Z")],
					hasMore: true,
				},
			],
			expected: {
				frontier: NOON_MILLIS * 1_000_000n + 4_000_000_000n,
				top: null,
			},
		},
		{
			name: "starts at the oldest first line of the queries with lines before, newest first",
			order: LogOrder.Desc,
			input: () => [
				{
					logs: [line("a2", "12:00:04Z")],
					hasMore: false,
					hasPrevious: true,
				},
				{
					logs: [line("b2", "12:00:05Z")],
					hasMore: false,
					hasPrevious: true,
				},
			],
			expected: {
				frontier: null,
				top: NOON_MILLIS * 1_000_000n + 4_000_000_000n,
			},
		},
	])("$name", ({ order, input, expected }, { expect }) => {
		expect(mergeBounds(input(), order)).toEqual(expected)
	})
})

describe("pastIn", () => {
	it.for([
		{
			name: "newest first, an older time is past",
			order: LogOrder.Desc,
			input: [1n, 2n],
			expected: true,
		},
		{
			name: "newest first, a newer time is not past",
			order: LogOrder.Desc,
			input: [2n, 1n],
			expected: false,
		},
		{
			name: "oldest first, a newer time is past",
			order: LogOrder.Asc,
			input: [2n, 1n],
			expected: true,
		},
		{
			name: "oldest first, an equal time is not past",
			order: LogOrder.Asc,
			input: [1n, 1n],
			expected: false,
		},
	])("$name", ({ order, input, expected }, { expect }) => {
		const [a = 0n, b = 0n] = input

		expect(pastIn(order)(a, b)).toBe(expected)
	})
})

describe("timeNanos", () => {
	it.for([
		{
			input: "2026-01-01T12:00:00.825596924Z",
			expected: NOON_MILLIS * 1_000_000n + 825_596_924n,
		},
		{
			input: "2026-01-01T12:00:00.5Z",
			expected: NOON_MILLIS * 1_000_000n + 500_000_000n,
		},
		{
			input: "2026-01-01T12:00:00.1234567891Z",
			expected: NOON_MILLIS * 1_000_000n + 123_456_789n,
		},
		{ input: "2026-01-01T12:00:00Z", expected: NOON_MILLIS * 1_000_000n },
		{
			input: "2026-01-01T14:00:00.000000007+02:00",
			expected: NOON_MILLIS * 1_000_000n + 7n,
		},
		{ input: "not a time", expected: 0n },
	])("reads $input", ({ input, expected }, { expect }) => {
		expect(timeNanos(input)).toBe(expected)
	})
})

describe("sortedEntries", () => {
	it.for<{
		name: string
		input: Record<string, string>
		expected: KeyValue[]
	}>([
		{ name: "returns nothing for an empty record", input: {}, expected: [] },
		{
			name: "lists the pairs by key",
			input: { job: "api", env: "prod", app: "web" },
			expected: [
				["app", "web"],
				["env", "prod"],
				["job", "api"],
			],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(sortedEntries(input)).toEqual(expected)
	})
})

function makeLog(overrides: Partial<Log> & { id: string }): Log {
	return {
		time: "2026-01-01T12:00:00Z",
		level: LogLevel.Info,
		line: "",
		labels: {},
		fields: {},
		...overrides,
	}
}

function line(id: string, clock: string): Log {
	return makeLog({ id, time: `2026-01-01T${clock}` })
}

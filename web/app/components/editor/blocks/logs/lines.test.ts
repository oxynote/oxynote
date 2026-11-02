import { describe, it } from "vitest"
import {
	displayValue,
	highlightParts,
	mergeQueryLines,
	sortedEntries,
	summarizeLine,
	timeNanos,
	type KeyValue,
	type LineSummary,
} from "./lines"

const NOON_MILLIS = BigInt(Date.UTC(2026, 0, 1, 12, 0, 0))

describe("summarizeLine", () => {
	it.for<{
		name: string
		input: Pick<LogLine, "line" | "labels" | "fields">
		expected: LineSummary
	}>([
		{
			name: "keeps a plain line as its message with the parsed fields by key",
			input: {
				line: "GET /health 200",
				labels: { job: "api", env: "prod" },
				fields: { status: "200", method: "GET", level: "info" },
			},
			expected: {
				message: "GET /health 200",
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
			name: "reads the message and fields of a JSON line in source order",
			input: {
				line: '{"status":"200","msg":"done","level":"info","lvl":"i","detected_level":"info","method":"GET"}',
				labels: { job: "api" },
				fields: { ignored: "x" },
			},
			expected: {
				message: "done",
				fields: [
					["status", "200"],
					["method", "GET"],
				],
				labels: [["job", "api"]],
			},
		},
		{
			name: "writes non-string JSON values as JSON",
			input: {
				line: '{"message":"done","count":3,"ok":true,"none":null,"user":{"id":1},"tags":["a"]}',
				labels: {},
				fields: {},
			},
			expected: {
				message: "done",
				fields: [
					["count", "3"],
					["ok", "true"],
					["none", "null"],
					["user", '{"id":1}'],
					["tags", '["a"]'],
				],
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
				fields: [
					["log", "c"],
					["message", "b"],
				],
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
				fields: [["msg", "5"]],
				labels: [],
			},
		},
		{
			name: "leaves the message empty for a JSON line without one",
			input: { line: '  {"status":"200"}', labels: {}, fields: {} },
			expected: { message: "", fields: [["status", "200"]], labels: [] },
		},
		{
			name: "keeps a line that only starts like JSON as plain text",
			input: { line: "{not json", labels: {}, fields: { a: "1" } },
			expected: { message: "{not json", fields: [["a", "1"]], labels: [] },
		},
		{
			name: "keeps a JSON array line as plain text",
			input: { line: '["a"]', labels: {}, fields: {} },
			expected: { message: '["a"]', fields: [], labels: [] },
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(summarizeLine(makeLine({ id: "l1", ...input }))).toEqual(expected)
	})
})

describe("displayValue", () => {
	it.for([
		{ input: "GET", expected: "GET" },
		{ input: "a=b", expected: "a=b" },
		{ input: "", expected: '""' },
		{ input: "two words", expected: '"two words"' },
		{ input: "tab\there", expected: '"tab\\there"' },
		{ input: 'say "hi"', expected: '"say \\"hi\\""' },
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

describe("mergeQueryLines", () => {
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
					lines: [line("a", "12:00:00.1Z"), line("b", "12:00:00.3Z")],
					hasMore: false,
				},
			],
			expected: ["b", "a"],
		},
		{
			name: "interleaves queries by their nanosecond times",
			input: () => [
				{
					lines: [
						line("a3", "12:00:00.000000003Z"),
						line("a1", "12:00:00.000000001Z"),
					],
					hasMore: false,
				},
				{ lines: [line("b2", "12:00:00.000000002Z")], hasMore: false },
			],
			expected: ["a3", "b2", "a1"],
		},
		{
			name: "shows a line two queries matched once",
			input: () => [
				{
					lines: [line("a", "12:00:02Z"), line("s", "12:00:01Z")],
					hasMore: false,
				},
				{ lines: [line("s", "12:00:01Z")], hasMore: false },
			],
			expected: ["a", "s"],
		},
		{
			name: "hides lines older than where a query with more to load stopped",
			input: () => [
				{
					lines: [line("a1", "12:00:05Z"), line("a2", "12:00:01Z")],
					hasMore: false,
				},
				{
					lines: [line("b1", "12:00:04Z"), line("b2", "12:00:03Z")],
					hasMore: true,
				},
			],
			expected: ["a1", "b1", "b2"],
		},
		{
			name: "stops at the newest point among queries with more to load",
			input: () => [
				{
					lines: [line("a1", "12:00:05Z"), line("a2", "12:00:02Z")],
					hasMore: true,
				},
				{
					lines: [line("b1", "12:00:04Z"), line("b2", "12:00:01Z")],
					hasMore: true,
				},
			],
			expected: ["a1", "b1", "a2"],
		},
		{
			name: "ignores a query with more to load but no lines",
			input: () => [
				{
					lines: [line("a", "12:00:02Z"), line("b", "12:00:01Z")],
					hasMore: false,
				},
				{ lines: [], hasMore: true },
			],
			expected: ["a", "b"],
		},
		{
			name: "keeps the source order of lines sharing a time",
			input: () => [
				{
					lines: [line("a", "12:00:01Z"), line("b", "12:00:01Z")],
					hasMore: false,
				},
				{ lines: [line("c", "12:00:01Z")], hasMore: false },
			],
			expected: ["a", "b", "c"],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(mergeQueryLines(input()).map((l) => l.id)).toEqual(expected)
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

function makeLine(overrides: Partial<LogLine> & { id: string }): LogLine {
	return {
		time: "2026-01-01T12:00:00Z",
		level: LogLevel.Info,
		line: "",
		labels: {},
		fields: {},
		...overrides,
	}
}

function line(id: string, clock: string): LogLine {
	return makeLine({ id, time: `2026-01-01T${clock}` })
}

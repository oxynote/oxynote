import { describe, it } from "vitest"
import { withViewFilters, type LogFilter } from "./logql"

describe("withViewFilters", () => {
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
	])("$name", ({ input, expected }, { expect }) => {
		expect(withViewFilters(input.query, input.filters, input.search)).toBe(
			expected,
		)
	})
})

function filter(
	key: string,
	value: string,
	options: { indexed?: boolean; exclude?: boolean } = {},
): LogFilter {
	return {
		key,
		value,
		indexed: options.indexed ?? false,
		exclude: options.exclude ?? false,
	}
}

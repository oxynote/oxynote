import { describe, it } from "vitest"

import { collectHeaderEntries, toAxiosHeaders, toHeaders } from "./headers.js"

const nonHeaderShapedCases: { name: string; input: unknown }[] = [
	{ name: "null", input: null },
	{ name: "undefined", input: undefined },
	{ name: "a string", input: "content-type: text/plain" },
	{ name: "a number", input: 7 },
]

const collectedValueCases: {
	name: string
	input: unknown
	expected: string
}[] = [
	{ name: "a string", input: "text/plain", expected: "text/plain" },
	{ name: "a number", input: 42, expected: "42" },
	{ name: "a boolean", input: false, expected: "false" },
	{
		name: "a bigint",
		input: 9007199254740993n,
		expected: "9007199254740993",
	},
	{ name: "an array of strings", input: ["a", "b"], expected: "a, b" },
	{
		name: "an array mixing primitives and dropped values",
		input: ["a", 1, null, undefined],
		expected: "a, 1",
	},
	{
		name: "a nested array",
		input: [["a", "b"], "c"],
		expected: "a, b, c",
	},
]

const droppedValueCases: { name: string; input: unknown }[] = [
	{ name: "null", input: null },
	{ name: "undefined", input: undefined },
	{ name: "a plain object", input: { nested: "value" } },
	{ name: "a symbol", input: Symbol("x-a") },
	{ name: "an empty array", input: [] },
	{ name: "an array of only dropped values", input: [null, undefined] },
]

describe("headers", () => {
	describe("collectHeaderEntries", () => {
		it.for(nonHeaderShapedCases)(
			"returns null for $name",
			({ input }, { expect }) => {
				expect(collectHeaderEntries(input)).toBeNull()
			},
		)

		it.for(collectedValueCases)(
			"collects $name as a single header value",
			({ input, expected }, { expect }) => {
				expect(
					collectHeaderEntries({ "x-a": input }),
				).toEqual([["x-a", expected]])
			},
		)

		it.for(droppedValueCases)(
			"drops a header whose value is $name",
			({ input }, { expect }) => {
				expect(
					collectHeaderEntries({ "x-a": input }),
				).toEqual([])
			},
		)

		it("collects the entries of a WHATWG Headers instance", ({
			expect,
		}) => {
			const headers = new Headers({
				"X-A": "1",
				"Content-Type": "text/plain",
			})

			expect(collectHeaderEntries(headers)).toEqual([
				["content-type", "text/plain"],
				["x-a", "1"],
			])
		})

		it("collects a repeated Headers entry as one comma-separated value", ({
			expect,
		}) => {
			const headers = new Headers()
			headers.append("X-A", "1")
			headers.append("X-A", "2")

			expect(collectHeaderEntries(headers)).toEqual([
				["x-a", "1, 2"],
			])
		})

		it("collects the entries of a Map", ({ expect }) => {
			const headers = new Map<string, unknown>([
				["x-a", "1"],
				["x-b", ["2", "3"]],
				["x-c", null],
			])

			expect(collectHeaderEntries(headers)).toEqual([
				["x-a", "1"],
				["x-b", "2, 3"],
			])
		})

		it("collects the entries of an iterable that has no forEach", ({
			expect,
		}) => {
			const headers = {
				*[Symbol.iterator](): Generator<
					[string, unknown]
				> {
					yield ["x-a", "1"]
					yield ["x-b", ["2", "3"]]
					yield ["x-c", undefined]
				},
			}

			expect(collectHeaderEntries(headers)).toEqual([
				["x-a", "1"],
				["x-b", "2, 3"],
			])
		})

		it("collects the entries of a plain object map", ({
			expect,
		}) => {
			expect(
				collectHeaderEntries({
					"X-A": "1",
					"content-type": "text/plain",
				}),
			).toEqual([
				["X-A", "1"],
				["content-type", "text/plain"],
			])
		})

		it("collects the entries of a null-prototype object map", ({
			expect,
		}) => {
			const headers = Object.assign(
				Object.create(null) as object,
				{
					"x-a": "1",
				},
			)

			expect(collectHeaderEntries(headers)).toEqual([
				["x-a", "1"],
			])
		})

		it("returns an empty list for an object with no own keys", ({
			expect,
		}) => {
			expect(collectHeaderEntries({})).toEqual([])
		})

		// a symbol key stringifies to "Symbol(x-a)", which
		// Headers.append rejects as an invalid header name
		it("drops a symbol key of a plain object map", ({ expect }) => {
			const key = Symbol("x-a")

			expect(
				collectHeaderEntries({
					[key]: "1",
					"x-b": "2",
				}),
			).toEqual([["x-b", "2"]])
		})
	})

	describe("toHeaders", () => {
		it("prefers the header-shaped headers argument over the request", ({
			expect,
		}) => {
			const res = toHeaders(
				{
					rawHeaders: ["x-raw", "raw"],
					headers: { "x-req": "req" },
				},
				{ "X-A": "1" },
			)

			expect([...res.entries()]).toEqual([["x-a", "1"]])
		})

		it("lowercases the collected header names", ({ expect }) => {
			const res = toHeaders(null, {
				"Content-Type": "text/plain",
			})

			expect(res.get("content-type")).toBe("text/plain")
		})

		it("combines repeated entries of an iterable into one value", ({
			expect,
		}) => {
			const res = toHeaders(
				null,
				new Map<string, unknown>([["x-a", "1"]]),
			)
			res.append("x-a", "2")

			expect(res.get("x-a")).toBe("1, 2")
		})

		it("takes an empty headers argument as header-shaped and ignores rawHeaders", ({
			expect,
		}) => {
			const res = toHeaders({ rawHeaders: ["x-a", "1"] }, {})

			expect([...res.entries()]).toEqual([])
		})

		it("falls back to rawHeaders when the headers argument is not header-shaped", ({
			expect,
		}) => {
			const res = toHeaders(
				{
					rawHeaders: [
						"X-A",
						"1",
						"Content-Type",
						"text/plain",
					],
					headers: { "x-req": "req" },
				},
				null,
			)

			expect([...res.entries()]).toEqual([
				["content-type", "text/plain"],
				["x-a", "1"],
			])
		})

		it("combines a repeated rawHeaders key into one value", ({
			expect,
		}) => {
			const res = toHeaders(
				{ rawHeaders: ["X-A", "1", "x-a", "2"] },
				undefined,
			)

			expect(res.get("x-a")).toBe("1, 2")
		})

		it("skips a trailing rawHeaders key that has no value", ({
			expect,
		}) => {
			const res = toHeaders(
				{ rawHeaders: ["x-a", "1", "x-b"] },
				null,
			)

			expect([...res.entries()]).toEqual([["x-a", "1"]])
		})

		it("skips a rawHeaders pair whose key or value is not a primitive", ({
			expect,
		}) => {
			const res = toHeaders(
				{
					rawHeaders: [
						{},
						"1",
						"x-b",
						{},
						"x-c",
						"3",
					],
				},
				null,
			)

			expect([...res.entries()]).toEqual([["x-c", "3"]])
		})

		it("falls back to req.headers when the request has no rawHeaders", ({
			expect,
		}) => {
			const res = toHeaders(
				{ headers: { "X-A": "1", "x-b": null } },
				null,
			)

			expect([...res.entries()]).toEqual([["x-a", "1"]])
		})

		it("falls back to req.headers when rawHeaders is not an array", ({
			expect,
		}) => {
			const res = toHeaders(
				{
					rawHeaders: "x-a: 1",
					headers: { "x-b": "2" },
				},
				null,
			)

			expect([...res.entries()]).toEqual([["x-b", "2"]])
		})

		it("returns an empty Headers when the request is null", ({
			expect,
		}) => {
			expect([...toHeaders(null, null).entries()]).toEqual([])
		})

		it("returns an empty Headers when the request is undefined", ({
			expect,
		}) => {
			expect([
				...toHeaders(undefined, undefined).entries(),
			]).toEqual([])
		})

		it("returns an empty Headers when neither argument is header-shaped", ({
			expect,
		}) => {
			expect([
				...toHeaders(
					"not a request",
					"not headers",
				).entries(),
			]).toEqual([])
		})
	})

	describe("toAxiosHeaders", () => {
		it("sets each collected entry on the returned AxiosHeaders", ({
			expect,
		}) => {
			const res = toAxiosHeaders({
				"X-A": "1",
				"content-type": "text/plain",
				"x-count": 3,
			})

			expect(res.get("x-a")).toBe("1")
			expect(res.get("Content-Type")).toBe("text/plain")
			expect(res.get("x-count")).toBe("3")
		})

		it("sets the entries of a WHATWG Headers instance", ({
			expect,
		}) => {
			const res = toAxiosHeaders(new Headers({ "X-A": "1" }))

			expect(res.get("x-a")).toBe("1")
		})

		it("omits a header whose value is dropped", ({ expect }) => {
			const res = toAxiosHeaders({ "x-a": "1", "x-b": null })

			expect(res.get("x-b")).toBeUndefined()
			expect(res.get("x-a")).toBe("1")
		})

		it.for(nonHeaderShapedCases)(
			"returns empty AxiosHeaders for $name",
			({ input }, { expect }) => {
				expect(toAxiosHeaders(input).toJSON()).toEqual(
					{},
				)
			},
		)
	})
})

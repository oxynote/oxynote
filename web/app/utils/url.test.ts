import { describe, it } from "vitest"
import {
	addDeletionSuccessStatusToUrl,
	createNameSlug,
	createNameSlugWithId,
	ensureHttps,
	equalNameSlugs,
	extractDocInfoFromSlug,
	extractDomain,
	isDeletionSuccessInUrl,
	isOptimisticInsertId,
	isXid,
} from "./url"

// default length of https://github.com/rs/xid
const XID = "c9m4hqk2v70qadhkl33g"

describe("isXid", () => {
	it.for([
		{ name: "accepts a 20-character id", input: XID, expected: true },
		{ name: "rejects a shorter id", input: "abc", expected: false },
		{ name: "rejects an empty id", input: "", expected: false },
	])("$name", ({ input, expected }, { expect }) => {
		expect(isXid(input)).toBe(expected)
	})
})

describe("isOptimisticInsertId", () => {
	it.for([
		{ name: "accepts an optimistic id", input: "optimistic-1", expected: true },
		{ name: "rejects a regular id", input: XID, expected: false },
	])("$name", ({ input, expected }, { expect }) => {
		expect(isOptimisticInsertId(input)).toBe(expected)
	})
})

describe("createNameSlug", () => {
	it.for([
		{
			name: "replaces spaces with dashes",
			input: "Hello World",
			expected: "Hello-World",
		},
		{
			name: "strips special characters",
			input: "Hello, World!",
			expected: "Hello-World",
		},
		{
			name: "truncates to 20 characters without a trailing dash",
			input: "a very long document name",
			expected: "a-very-long-document",
		},
		{
			name: "returns an empty string for an empty name",
			input: "",
			expected: "",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(createNameSlug(input)).toBe(expected)
	})
})

describe("createNameSlugWithId", () => {
	it("appends the id to the slugged name", ({ expect }) => {
		expect(createNameSlugWithId("Hello World", XID)).toBe(`Hello-World-${XID}`)
	})

	it("returns the id alone for an empty name", ({ expect }) => {
		expect(createNameSlugWithId("", XID)).toBe(XID)
	})

	it("returns the id alone when the name slugs to nothing", ({ expect }) => {
		expect(createNameSlugWithId("!!!", XID)).toBe(XID)
	})

	it("throws for an id that is not 20 characters long", ({ expect }) => {
		expect(() => createNameSlugWithId("Hello", "short-id")).toThrow(
			"the ID must have exactly 20 characters to be included in a slug",
		)
	})
})

describe("equalNameSlugs", () => {
	it.for([
		{
			name: "accepts identical strings",
			a: "My Doc",
			b: "My Doc",
			expected: true,
		},
		{
			name: "accepts a name and its slug",
			a: "Hello World",
			b: "Hello-World",
			expected: true,
		},
		{
			name: "accepts names that only differ beyond the slug cut-off",
			a: "a very long document name",
			b: "a very long document title",
			expected: true,
		},
		{
			name: "rejects different names",
			a: "Hello",
			b: "Goodbye",
			expected: false,
		},
	])("$name", ({ a, b, expected }, { expect }) => {
		expect(equalNameSlugs(a, b)).toBe(expected)
	})
})

describe("extractDocInfoFromSlug", () => {
	it.for([
		{
			name: "extracts the id from a bare id slug",
			input: XID,
			expected: { id: XID },
		},
		{
			name: "extracts the name and id from a combined slug",
			input: `my-doc-${XID}`,
			expected: { id: XID, name: "my-doc" },
		},
		{
			name: "rejects a slug whose last part is not an id",
			input: "my-doc-123",
			expected: undefined,
		},
		{
			name: "rejects a bare non-id slug",
			input: "my",
			expected: undefined,
		},
		{ name: "rejects an empty slug", input: "", expected: undefined },
	])("$name", ({ input, expected }, { expect }) => {
		expect(extractDocInfoFromSlug(input)).toEqual(expected)
	})
})

describe("ensureHttps", () => {
	it.for([
		{
			name: "prefixes a bare domain",
			input: "example.com",
			expected: "https://example.com",
		},
		{
			name: "keeps an existing https prefix",
			input: "https://example.com",
			expected: "https://example.com",
		},
		{
			name: "keeps an existing http prefix",
			input: "http://example.com",
			expected: "http://example.com",
		},
		{
			name: "matches the prefix case-insensitively",
			input: "HTTPS://example.com",
			expected: "HTTPS://example.com",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(ensureHttps(input)).toBe(expected)
	})
})

describe("extractDomain", () => {
	it.for([
		{
			name: "strips subdomains down to domain and tld",
			input: "https://docs.sub.example.com/path?x=1",
			expected: "example.com",
		},
		{
			name: "handles a bare domain without protocol",
			input: "example.com",
			expected: "example.com",
		},
		{
			name: "falls back to the hostname for localhost",
			input: "http://localhost:8080",
			expected: "localhost",
		},
		{ name: "returns unparseable input unchanged", input: "", expected: "" },
	])("$name", ({ input, expected }, { expect }) => {
		expect(extractDomain(input)).toBe(expected)
	})
})

describe("addDeletionSuccessStatusToUrl", () => {
	it("appends the deletion status parameter", ({ expect }) => {
		expect(addDeletionSuccessStatusToUrl("https://app.test/docs")).toBe(
			"https://app.test/docs?deletion=success",
		)
	})

	it("preserves existing query parameters", ({ expect }) => {
		expect(addDeletionSuccessStatusToUrl("https://app.test/docs?x=1")).toBe(
			"https://app.test/docs?x=1&deletion=success",
		)
	})
})

describe("isDeletionSuccessInUrl", () => {
	it.for([
		{
			name: "detects the deletion success parameter",
			input: "https://app.test/docs?deletion=success",
			expected: true,
		},
		{
			name: "rejects other deletion values",
			input: "https://app.test/docs?deletion=failed",
			expected: false,
		},
		{
			name: "rejects urls without the parameter",
			input: "https://app.test/docs",
			expected: false,
		},
		{ name: "rejects unparseable urls", input: "not a url", expected: false },
	])("$name", ({ input, expected }, { expect }) => {
		expect(isDeletionSuccessInUrl(input)).toBe(expected)
	})
})

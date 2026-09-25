import { describe, it } from "vitest"
import {
	docNameByIdInDocumentTree,
	isIdInDocumentTree,
	makeWsDocumentMaintainersChangeTopic,
	makeWsDocumentMetadataChangeTopic,
	makeWsDocumentReviewersChangeTopic,
	makeWsHooksChangeTopic,
	type DocumentTreeElement,
} from "./document"

function element(
	id: string,
	children?: DocumentTreeElement[] | null,
): DocumentTreeElement {
	return {
		id,
		documentName: `doc-${id}`,
		icon: "icon",
		protected: false,
		children,
	}
}

const tree = [element("a", [element("b", [element("c")]), element("d", null)])]

describe("makeWsDocumentMetadataChangeTopic", () => {
	it("builds the metadata change topic for the document", ({ expect }) => {
		expect(makeWsDocumentMetadataChangeTopic("d1")).toBe(
			"change@documents.d1.metadata",
		)
	})
})

describe("makeWsDocumentMaintainersChangeTopic", () => {
	it("builds the maintainers change topic for the document", ({ expect }) => {
		expect(makeWsDocumentMaintainersChangeTopic("d1")).toBe(
			"change@documents.d1.maintainers",
		)
	})
})

describe("makeWsDocumentReviewersChangeTopic", () => {
	it("builds the reviewers change topic for the document", ({ expect }) => {
		expect(makeWsDocumentReviewersChangeTopic("d1")).toBe(
			"change@documents.d1.reviewers",
		)
	})
})

describe("makeWsHooksChangeTopic", () => {
	it("builds the hooks change topic for the document", ({ expect }) => {
		expect(makeWsHooksChangeTopic("d1")).toBe("change@documents.d1.hooks")
	})
})

describe("isIdInDocumentTree", () => {
	it.for([
		{ name: "finds a top-level element", id: "a", expected: true },
		{ name: "finds a deeply nested element", id: "c", expected: true },
		{ name: "handles null children", id: "d", expected: true },
		{ name: "rejects an unknown id", id: "x", expected: false },
	])("$name", ({ id, expected }, { expect }) => {
		expect(isIdInDocumentTree(tree, id)).toBe(expected)
	})

	it("returns false for an empty tree", ({ expect }) => {
		expect(isIdInDocumentTree([], "a")).toBe(false)
	})
})

describe("docNameByIdInDocumentTree", () => {
	it.for([
		{
			name: "returns the name of a top-level element",
			id: "a",
			expected: "doc-a",
		},
		{
			name: "returns the name of a nested element",
			id: "c",
			expected: "doc-c",
		},
		{ name: "returns null for an unknown id", id: "x", expected: null },
	])("$name", ({ id, expected }, { expect }) => {
		expect(docNameByIdInDocumentTree(tree, id)).toBe(expected)
	})
})

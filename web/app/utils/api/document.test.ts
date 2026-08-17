import { describe, it, vi } from "vitest"
import {
	DocumentHookType,
	docNameByIdInDocumentTree,
	defaultDocumentHookState,
	isIdInDocumentTree,
	makeWsDocumentMaintainersChangeTopic,
	makeWsDocumentMetadataChangeTopic,
	makeWsDocumentReviewersChangeTopic,
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

describe("defaultDocumentHookState", () => {
	it("stamps the scheduled reminder state with the current time", ({
		expect,
	}) => {
		vi.useFakeTimers()
		vi.setSystemTime(new Date("2024-06-15T12:00:00Z"))

		try {
			expect(
				defaultDocumentHookState(DocumentHookType.ScheduledReminder),
			).toEqual({ lastActiveAt: new Date("2024-06-15T12:00:00Z") })
		} finally {
			vi.useRealTimers()
		}
	})

	it("stamps the url watcher state with the current time", ({ expect }) => {
		vi.useFakeTimers()
		vi.setSystemTime(new Date("2024-06-15T12:00:00Z"))

		try {
			expect(defaultDocumentHookState(DocumentHookType.URLWatcher)).toEqual({
				lastCheckedAt: new Date("2024-06-15T12:00:00Z"),
				status: "active",
			})
		} finally {
			vi.useRealTimers()
		}
	})

	it("returns an empty checksum state for github tracking", ({ expect }) => {
		expect(defaultDocumentHookState(DocumentHookType.GitHubTracking)).toEqual({
			pathsChecksums: {},
			status: "active",
		})
	})

	it("returns an empty digest state for the container image watcher", ({
		expect,
	}) => {
		expect(
			defaultDocumentHookState(DocumentHookType.ContainerImageWatcher),
		).toEqual({
			status: "active",
			digest: "",
		})
	})
})

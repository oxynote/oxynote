import { describe, it } from "vitest"
import {
	SIDEBAR_ITEM_PLACEHOLDER_ID,
	documentTreeBreadcrumbs,
	extractDocumentTreeElement,
	processDocumentTree,
} from "./sidebar"

// isXid() only checks for a 20-character length, and
// createNameSlugWithId() rejects anything shorter
const ID_A = "a".padEnd(20, "0")
const ID_B = "b".padEnd(20, "0")
const ID_C = "c".padEnd(20, "0")

function treeElement(
	id: string,
	documentName: string,
	children?: DocumentTreeElement[] | null,
): DocumentTreeElement {
	return {
		id: id,
		documentName: documentName,
		icon: "lucide:file",
		protected: false,
		children: children,
	}
}

describe("processDocumentTree", () => {
	it("maps a flat tree onto sidebar items linking into the organization", ({
		expect,
	}) => {
		const items = processDocumentTree(
			[treeElement(ID_A, "Runbook", [])],
			null,
			"placeholder",
			"Acme Corp",
		)

		expect(items).toEqual([
			{
				id: ID_A,
				name: "Runbook",
				icon: "lucide:file",
				partOfDocumentTree: true,
				active: false,
				draggable: true,
				url: `/Acme-Corp/Runbook-${ID_A}`,
				prefetchUrlOnInteraction: true,
				localOptimisticInsert: undefined,
				children: [
					expect.objectContaining({ id: SIDEBAR_ITEM_PLACEHOLDER_ID }),
				],
			},
		])
	})

	it("marks the item whose id matches the active document", ({ expect }) => {
		const items = processDocumentTree(
			[treeElement(ID_A, "First", []), treeElement(ID_B, "Second", [])],
			ID_B,
			"placeholder",
			"Acme",
		)

		expect(items.map((item) => item.active)).toEqual([false, true])
	})

	it("points an optimistically inserted item at '#' instead of its slug", ({
		expect,
	}) => {
		const [item] = processDocumentTree(
			[{ ...treeElement(ID_A, "Draft", []), localOptimisticInsert: true }],
			null,
			"placeholder",
			"Acme",
		)

		expect(item?.url).toBe("#")
		expect(item?.localOptimisticInsert).toBe(true)
	})

	it("recurses into children", ({ expect }) => {
		const items = processDocumentTree(
			[treeElement(ID_A, "Parent", [treeElement(ID_B, "Child", [])])],
			null,
			"placeholder",
			"Acme",
		)

		expect(items[0]?.children).toEqual([
			expect.objectContaining({ id: ID_B, name: "Child" }),
		])
	})

	it("gives an item with no children a single placeholder child", ({
		expect,
	}) => {
		const items = processDocumentTree(
			[treeElement(ID_A, "Leaf", null)],
			null,
			"Add a page",
			"Acme",
		)

		expect(items[0]?.children).toEqual([
			{
				id: SIDEBAR_ITEM_PLACEHOLDER_ID,
				name: "Add a page",
				url: null,
				partOfDocumentTree: true,
				icon: null,
				active: false,
				draggable: false,
				children: [],
			},
		])
	})

	it("gives an item with an empty child list a placeholder child too", ({
		expect,
	}) => {
		const items = processDocumentTree(
			[treeElement(ID_A, "Leaf", [])],
			null,
			"Add a page",
			"Acme",
		)

		expect(items[0]?.children).toEqual([
			expect.objectContaining({ id: SIDEBAR_ITEM_PLACEHOLDER_ID }),
		])
	})

	it("returns a lone placeholder for an empty tree", ({ expect }) => {
		const items = processDocumentTree([], null, "Add a page", "Acme")

		expect(items).toEqual([
			expect.objectContaining({
				id: SIDEBAR_ITEM_PLACEHOLDER_ID,
				name: "Add a page",
			}),
		])
	})
})

describe("extractDocumentTreeElement", () => {
	it("returns the matching top-level element", ({ expect }) => {
		const target = treeElement(ID_B, "Second", [])

		expect(
			extractDocumentTreeElement(
				[treeElement(ID_A, "First", []), target],
				ID_B,
			),
		).toBe(target)
	})

	it("finds an element nested several levels deep", ({ expect }) => {
		const target = treeElement(ID_C, "Deep", [])
		const tree = [
			treeElement(ID_A, "Root", [treeElement(ID_B, "Middle", [target])]),
		]

		expect(extractDocumentTreeElement(tree, ID_C)).toBe(target)
	})

	it("returns null when no element matches", ({ expect }) => {
		expect(
			extractDocumentTreeElement([treeElement(ID_A, "First", [])], ID_C),
		).toBeNull()
	})

	it("returns null for an empty tree", ({ expect }) => {
		expect(extractDocumentTreeElement([], ID_A)).toBeNull()
	})
})

describe("documentTreeBreadcrumbs", () => {
	it("returns a single crumb when the target is at the root", ({ expect }) => {
		expect(
			documentTreeBreadcrumbs([treeElement(ID_A, "Runbook", [])], ID_A),
		).toEqual([
			{
				id: ID_A,
				name: "Runbook",
				icon: "lucide:file",
				href: `/Runbook-${ID_A}`,
			},
		])
	})

	it("returns the path from the root down to the target", ({ expect }) => {
		const tree = [
			treeElement(ID_A, "Root", [
				treeElement(ID_B, "Middle", [treeElement(ID_C, "Leaf", [])]),
			]),
		]

		expect(documentTreeBreadcrumbs(tree, ID_C).map((c) => c.name)).toEqual([
			"Root",
			"Middle",
			"Leaf",
		])
	})

	it("returns an empty array when the target is absent", ({ expect }) => {
		expect(
			documentTreeBreadcrumbs([treeElement(ID_A, "Root", [])], ID_C),
		).toEqual([])
	})

	it("returns an empty array for an empty tree", ({ expect }) => {
		expect(documentTreeBreadcrumbs([], ID_A)).toEqual([])
	})
})

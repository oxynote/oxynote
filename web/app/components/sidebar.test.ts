import { describe, it } from "vitest"
import {
	SIDEBAR_ITEM_PLACEHOLDER_ID,
	documentTreeBreadcrumbs,
	extractDocumentTreeElement,
	processDocumentTree,
	processTagTree,
	type SidebarItemAction,
} from "./sidebar"

// the tree builders take their menus from the caller, so the tests hand
// them a recognisable one instead of the app's real handlers. One object
// per id: a fresh closure per call would never compare equal to itself.
const _actions = new Map<string, SidebarItemAction>()

function action(id: string): SidebarItemAction {
	const existing = _actions.get(id)
	if (existing) {
		return existing
	}

	const made: SidebarItemAction = {
		id: id,
		name: id,
		icon: `lucide:${id}`,
		fn: () => undefined,
	}
	_actions.set(id, made)

	return made
}

const noActions = () => []

// isXid() only checks for a 20-character length, and
// createNameSlugWithId() rejects anything shorter
const ID_A = "a".padEnd(20, "0")
const ID_B = "b".padEnd(20, "0")
const ID_C = "c".padEnd(20, "0")
const BRANCH_A = "branch-a".padEnd(20, "0")

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
		defaultBranchId: BRANCH_A,
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
			noActions,
		)

		expect(items).toEqual([
			{
				id: ID_A,
				name: "Runbook",
				icon: "lucide:file",
				acceptsChildren: true,
				active: false,
				draggable: true,
				url: `/Acme-Corp/Runbook-${ID_A}`,
				prefetchUrlOnInteraction: true,
				localOptimisticInsert: undefined,
				actions: [],
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
			noActions,
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
			noActions,
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
			noActions,
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
			noActions,
		)

		expect(items[0]?.children).toEqual([
			{
				id: SIDEBAR_ITEM_PLACEHOLDER_ID,
				name: "Add a page",
				url: null,
				acceptsChildren: false,
				icon: null,
				active: false,
				draggable: false,
				actions: [],
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
			noActions,
		)

		expect(items[0]?.children).toEqual([
			expect.objectContaining({ id: SIDEBAR_ITEM_PLACEHOLDER_ID }),
		])
	})

	it("returns a lone placeholder for an empty tree", ({ expect }) => {
		const items = processDocumentTree([], null, "Add a page", "Acme", noActions)

		expect(items).toEqual([
			expect.objectContaining({
				id: SIDEBAR_ITEM_PLACEHOLDER_ID,
				name: "Add a page",
			}),
		])
	})
})

function tagElement(
	id: string,
	tagName: string,
	documents?: DocumentTreeElement[] | null,
): TagTreeElement {
	return {
		id: id,
		tagName: tagName,
		color: "#22c55e",
		hidden: false,
		documents: documents,
	}
}

describe("processTagTree", () => {
	it("maps a tag onto a draggable row carrying its colour", ({ expect }) => {
		const items = processTagTree(
			[tagElement(ID_A, "Production")],
			null,
			"Acme",
			() => [action("hide-tag")],
			noActions,
		)

		expect(items).toEqual([
			{
				id: ID_A,
				name: "Production",
				dotColor: "#22c55e",
				acceptsChildren: false,
				active: false,
				draggable: true,
				dragGroup: "tags",
				actions: [action("hide-tag")],
				children: null,
			},
		])
	})

	it("lists a tag's documents as links into the organization", ({ expect }) => {
		const items = processTagTree(
			[tagElement(ID_A, "Production", [treeElement(ID_B, "Runbook")])],
			null,
			"Acme Corp",
			noActions,
			(documentId, branchId, tagId) => [
				action(`remove-${documentId}@${branchId}-from-${tagId}`),
			],
		)

		expect(items[0]?.children).toEqual([
			{
				id: ID_B,
				name: "Runbook",
				icon: "lucide:file",
				acceptsChildren: true,
				active: false,
				draggable: false,
				dragGroup: "tag-documents",
				url: `/Acme-Corp/Runbook-${ID_B}`,
				prefetchUrlOnInteraction: true,
				actions: [action(`remove-${ID_B}@${BRANCH_A}-from-${ID_A}`)],
				children: null,
			},
		])
	})

	it("offers no detach action to a row still waiting for its branch", ({
		expect,
	}) => {
		const optimistic = treeElement(ID_B, "Runbook")
		delete optimistic.defaultBranchId

		const items = processTagTree(
			[tagElement(ID_A, "Production", [optimistic])],
			null,
			"Acme",
			noActions,
			() => [action("remove-tag")],
		)

		expect(items[0]?.children?.[0]?.actions).toEqual([])
	})

	it("recurses into a document's own children", ({ expect }) => {
		const items = processTagTree(
			[
				tagElement(ID_A, "Production", [
					treeElement(ID_B, "Parent", [treeElement(ID_C, "Child")]),
				]),
			],
			null,
			"Acme",
			noActions,
			noActions,
		)

		expect(items[0]?.children?.[0]?.children).toEqual([
			expect.objectContaining({ id: ID_C, name: "Child", draggable: false }),
		])
	})

	it("marks the document that is currently open", ({ expect }) => {
		const items = processTagTree(
			[
				tagElement(ID_A, "Production", [
					treeElement(ID_B, "First"),
					treeElement(ID_C, "Second"),
				]),
			],
			ID_C,
			"Acme",
			noActions,
			noActions,
		)

		expect(items[0]?.children?.map((child) => child.active)).toEqual([
			false,
			true,
		])
	})

	it("offers the detach action only to the documents carrying the tag", ({
		expect,
	}) => {
		const items = processTagTree(
			[
				tagElement(ID_A, "Production", [
					treeElement(ID_B, "Parent", [treeElement(ID_C, "Child")]),
				]),
			],
			null,
			"Acme",
			noActions,
			(documentId) => [action(`remove-${documentId}`)],
		)

		const parent = items[0]?.children?.[0]

		expect(parent?.actions).toEqual([action(`remove-${ID_B}`)])
		expect(parent?.children?.[0]?.actions).toEqual([])
	})

	it("keeps a hidden tag out of the sidebar", ({ expect }) => {
		const items = processTagTree(
			[
				{ ...tagElement(ID_A, "Production"), hidden: true },
				tagElement(ID_B, "Staging"),
			],
			null,
			"Acme",
			noActions,
			noActions,
		)

		expect(items.map((item) => item.name)).toEqual(["Staging"])
	})

	it("leaves a tag holding no documents without a child list", ({ expect }) => {
		const items = processTagTree(
			[tagElement(ID_A, "Empty", [])],
			null,
			"Acme",
			noActions,
			noActions,
		)

		expect(items[0]?.children).toBeNull()
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

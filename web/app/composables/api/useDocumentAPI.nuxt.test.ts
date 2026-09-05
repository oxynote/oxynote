import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import type { DocumentTreeElement } from "~/utils"
import {
	ANY_DATE,
	ANY_STRING,
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	matchingString,
	mockDeferredEndpoint,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedAuthOrganization,
	seedAuthSession,
	seedQueryData,
} from "./test-helpers"
import useDocumentAPI from "./useDocumentAPI"
import useDocumentHookAPI from "./useDocumentHookAPI"
import useTagAPI from "./useTagAPI"

const DOC_ID = makeXid("doc1")
const DOC_ID_2 = makeXid("doc2")
const CHILD_ID = makeXid("chld")
const BRANCH_ID = makeXid("brn1")
const BRANCH_ID_2 = makeXid("brn2")
const USER_ID = makeXid("usr1")
const USER_ID_2 = makeXid("usr2")
const ORG_ID = makeXid("org1")
const SHORT_ID = "nanoid-1"

const TREE_KEY = ["documents", "tree"] as const
const BRANCHES_KEY = ["documents", DOC_ID, "branches"] as const
const REVIEWERS_KEY = [
	"documents",
	DOC_ID,
	"branches",
	BRANCH_ID,
	"reviewers",
] as const

const TREE_URL = "/api/documents/tree"
const MERGE_URL = `http://test.local/auth-realtime/api/documents/${DOC_ID}/merge`

function makeDocumentAPI() {
	return runInApp(() => useDocumentAPI())
}

function seedAuth() {
	seedAuthSession(USER_ID)
	seedAuthOrganization(ORG_ID)
}

function makeElem(
	id: string,
	name: string,
	overrides: Partial<DocumentTreeElement> = {},
): DocumentTreeElement {
	return {
		id,
		documentName: name,
		icon: "icon-a",
		protected: false,
		children: null,
		...overrides,
	}
}

function makeBranch(branchId: string, branchName: string) {
	return {
		branchId,
		branchName,
		documentName: "Doc A",
		icon: "icon-a",
		protected: false,
		default: branchName === "main",
		createdAt: "2026-01-01T00:00:00.000Z",
		updatedAt: "2026-01-01T00:00:00.000Z",
	}
}

function makeReviewer(userId: string, currentlyApproved: boolean) {
	return {
		branchId: BRANCH_ID,
		userId,
		organizationId: ORG_ID,
		currentlyApproved,
		previouslyApproved: false,
	}
}

// creating the composable eagerly loads the tree query once when its cache
// entry is empty; the tests either seed the tree first (the seed is fresh,
// so the eager load is skipped) or use refresh() as the act, which joins
// the in-flight eager load — both keep the call accounting deterministic
describe("useDocumentAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchDocumentTree", () => {
		it("fetches the document tree", async ({ expect }) => {
			const tree = [makeElem(DOC_ID, "Doc A")]
			const treeCalls = mockEndpoint("GET", TREE_URL, () => tree)
			const api = makeDocumentAPI()

			const result = await api.fetchDocumentTree.refresh()

			expect(result.data).toEqual(tree)
			expect(treeCalls).toHaveLength(1)
		})
	})

	describe("updateDocumentTree", () => {
		it("bails out for a non-xid document id without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await api.updateDocumentTree.mutateAsync({
				id: SHORT_ID,
				parentId: null,
				insertBeforeId: null,
			})

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(putCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("rejects when the document is not in the tree", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await expect(
				api.updateDocumentTree.mutateAsync({
					id: DOC_ID_2,
					parentId: null,
					insertBeforeId: null,
				}),
			).rejects.toThrow("invalid document tree update data")

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(putCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("moves a root document before a sibling and refreshes the tree", async ({
			expect,
		}) => {
			const serverTree = [
				makeElem(DOC_ID_2, "Doc B"),
				makeElem(DOC_ID, "Doc A"),
			]
			const treeCalls = mockEndpoint("GET", TREE_URL, () => serverTree)
			const put = mockDeferredEndpoint("PUT", TREE_URL)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A"),
				makeElem(DOC_ID_2, "Doc B"),
			])
			const api = makeDocumentAPI()

			const pending = api.updateDocumentTree.mutateAsync({
				id: DOC_ID_2,
				parentId: null,
				insertBeforeId: DOC_ID,
			})
			await put.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID_2, "Doc B"),
				makeElem(DOC_ID, "Doc A"),
			])
			put.resolve(null)

			await pending

			expect(put.calls).toHaveLength(1)
			expect(put.calls[0]?.body).toEqual({
				id: DOC_ID_2,
				parentId: null,
				sortIndex: 0,
			})
			// the success invalidation refetches the active tree query
			expect(treeCalls).toHaveLength(1)
			expect(api.fetchDocumentTree.data.value).toEqual(serverTree)
		})

		it("moves a root document into a parent", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const put = mockDeferredEndpoint("PUT", TREE_URL)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A"),
				makeElem(DOC_ID_2, "Doc B", {
					children: [makeElem(CHILD_ID, "Child")],
				}),
			])
			const api = makeDocumentAPI()

			const pending = api.updateDocumentTree.mutateAsync({
				id: DOC_ID,
				parentId: DOC_ID_2,
				insertBeforeId: null,
			})
			await put.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID_2, "Doc B", {
					children: [makeElem(DOC_ID, "Doc A"), makeElem(CHILD_ID, "Child")],
				}),
			])
			put.resolve(null)

			await pending

			expect(put.calls[0]?.body).toEqual({
				id: DOC_ID,
				parentId: DOC_ID_2,
				sortIndex: 0,
			})
			expect(treeCalls).toHaveLength(1)
		})

		it("moves the only child of a parent to the root", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const put = mockDeferredEndpoint("PUT", TREE_URL)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID_2, "Doc B", {
					children: [makeElem(CHILD_ID, "Child")],
				}),
			])
			const api = makeDocumentAPI()

			const pending = api.updateDocumentTree.mutateAsync({
				id: CHILD_ID,
				parentId: null,
				insertBeforeId: null,
			})
			await put.reached

			// the emptied children key is removed from the old parent
			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(CHILD_ID, "Child"),
				makeElem(DOC_ID_2, "Doc B", { children: undefined }),
			])
			put.resolve(null)

			await pending

			expect(put.calls[0]?.body).toEqual({
				id: CHILD_ID,
				parentId: null,
				sortIndex: 0,
			})
			expect(treeCalls).toHaveLength(1)
		})

		it("rolls back the tree when the request fails", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			mockEndpoint("PUT", TREE_URL, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A"),
				makeElem(DOC_ID_2, "Doc B"),
			])
			const api = makeDocumentAPI()

			await expect(
				api.updateDocumentTree.mutateAsync({
					id: DOC_ID_2,
					parentId: null,
					insertBeforeId: DOC_ID,
				}),
			).rejects.toThrow()

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID, "Doc A"),
				makeElem(DOC_ID_2, "Doc B"),
			])
			expect(treeCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic move", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const put = mockDeferredEndpoint("PUT", TREE_URL)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A"),
				makeElem(DOC_ID_2, "Doc B"),
			])
			const api = makeDocumentAPI()

			const pending = api.updateDocumentTree.mutateAsync({
				id: DOC_ID_2,
				parentId: null,
				insertBeforeId: DOC_ID,
			})
			await put.reached

			// divergent data written after the optimistic move must survive
			// the failure
			seedQueryData(TREE_KEY, [makeElem(CHILD_ID, "Divergent")])
			put.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(TREE_KEY)).toEqual([makeElem(CHILD_ID, "Divergent")])
		})
	})

	describe("createDocument", () => {
		it("skips the local optimistic insert when asked", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint("POST", "/api/documents")
			seedQueryData(TREE_KEY, [])
			const api = makeDocumentAPI()

			const pending = api.createDocument.mutateAsync({
				name: "Doc A",
				icon: "icon-a",
				parentId: null,
				skipLocalOptimisticInsert: true,
			})
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([])
			post.resolve({ id: DOC_ID })

			await pending

			// the local-only flag is stripped from the request body
			expect(post.calls[0]?.body).toEqual({
				name: "Doc A",
				icon: "icon-a",
				parentId: null,
			})
			expect(treeCalls).toHaveLength(1)
		})

		it("bails out for a non-xid parent without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const postCalls = mockEndpoint("POST", "/api/documents", () => ({
				id: DOC_ID,
			}))
			seedQueryData(TREE_KEY, [])
			const api = makeDocumentAPI()

			await api.createDocument.mutateAsync({
				name: "Doc A",
				icon: "icon-a",
				parentId: SHORT_ID,
			})

			expect(readQueryData(TREE_KEY)).toEqual([])
			expect(postCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("inserts the document into an empty tree", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint("POST", "/api/documents")
			seedQueryData(TREE_KEY, [])
			const api = makeDocumentAPI()

			const pending = api.createDocument.mutateAsync({
				name: "Doc A",
				icon: "icon-a",
				parentId: null,
			})
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				{
					id: ANY_STRING,
					documentName: "Doc A",
					icon: "icon-a",
					children: null,
					protected: false,
					localOptimisticInsert: true,
				},
			])
			post.resolve({ id: DOC_ID })

			await pending

			expect(post.calls[0]?.body).toEqual({
				name: "Doc A",
				icon: "icon-a",
				parentId: null,
			})
			expect(treeCalls).toHaveLength(1)
		})

		it("prepends the document at the root of a non-empty tree", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint("POST", "/api/documents")
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			const pending = api.createDocument.mutateAsync({
				name: "Doc B",
				icon: "icon-b",
				parentId: null,
			})
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				expect.objectContaining({
					documentName: "Doc B",
					localOptimisticInsert: true,
				}),
				makeElem(DOC_ID, "Doc A"),
			])
			post.resolve({ id: DOC_ID_2 })

			await pending
		})

		it("prepends the document under its parent", async ({ expect }) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint("POST", "/api/documents")
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A", { children: [makeElem(CHILD_ID, "Child")] }),
			])
			const api = makeDocumentAPI()

			const pending = api.createDocument.mutateAsync({
				name: "Doc B",
				icon: "icon-b",
				parentId: DOC_ID,
			})
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID, "Doc A", {
					children: [
						expect.objectContaining({
							documentName: "Doc B",
							localOptimisticInsert: true,
						}),
						makeElem(CHILD_ID, "Child"),
					],
				}),
			])
			post.resolve({ id: DOC_ID_2 })

			await pending
		})

		it("rolls back the tree when the request fails", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			mockEndpoint("POST", "/api/documents", () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await expect(
				api.createDocument.mutateAsync({
					name: "Doc B",
					icon: "icon-b",
					parentId: null,
				}),
			).rejects.toThrow()

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(treeCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic insert", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint("POST", "/api/documents")
			seedQueryData(TREE_KEY, [])
			const api = makeDocumentAPI()

			const pending = api.createDocument.mutateAsync({
				name: "Doc A",
				icon: "icon-a",
				parentId: null,
			})
			await post.reached

			seedQueryData(TREE_KEY, [makeElem(CHILD_ID, "Divergent")])
			post.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(TREE_KEY)).toEqual([makeElem(CHILD_ID, "Divergent")])
		})
	})

	describe("deleteDocument", () => {
		it("bails out for a non-xid id without a request", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeElem(DOC_ID, "Doc A"),
			])
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${SHORT_ID}`,
				() => null,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await api.deleteDocument.mutateAsync(SHORT_ID)

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(deleteCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("removes a root document optimistically", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const tagTreeCalls = mockEndpoint("GET", "/api/tags/tree", () => [])
			const del = mockDeferredEndpoint("DELETE", `/api/documents/${DOC_ID}`)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A"),
				makeElem(DOC_ID_2, "Doc B"),
			])
			const api = makeDocumentAPI()
			const tagAPI = runInApp(() => useTagAPI())
			await tagAPI.fetchTagTree.refresh()

			const pending = api.deleteDocument.mutateAsync(DOC_ID)
			await del.reached

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID_2, "Doc B")])
			del.resolve(null)

			await pending
			await flushPromises()

			expect(del.calls).toHaveLength(1)
			expect(treeCalls).toHaveLength(1)
			// the document leaves every tag it was listed under
			expect(tagTreeCalls).toHaveLength(2)
		})

		it("removes a nested document and drops the emptied children key", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const del = mockDeferredEndpoint("DELETE", `/api/documents/${CHILD_ID}`)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A", { children: [makeElem(CHILD_ID, "Child")] }),
			])
			const api = makeDocumentAPI()

			const pending = api.deleteDocument.mutateAsync(CHILD_ID)
			await del.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID, "Doc A", { children: undefined }),
			])
			del.resolve(null)

			await pending
		})

		it("rolls back the tree when the request fails", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			mockEndpoint("DELETE", `/api/documents/${DOC_ID}`, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await expect(api.deleteDocument.mutateAsync(DOC_ID)).rejects.toThrow()

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(treeCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const del = mockDeferredEndpoint("DELETE", `/api/documents/${DOC_ID}`)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			const pending = api.deleteDocument.mutateAsync(DOC_ID)
			await del.reached

			seedQueryData(TREE_KEY, [makeElem(CHILD_ID, "Divergent")])
			del.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(TREE_KEY)).toEqual([makeElem(CHILD_ID, "Divergent")])
		})
	})

	describe("updateDocumentTreeElementCache", () => {
		it("does nothing without a cached tree", ({ expect }) => {
			const api = makeDocumentAPI()

			api.updateDocumentTreeElementCache(DOC_ID, {
				name: "Renamed",
				icon: "icon-b",
				protected: true,
			})

			expect(readQueryData(TREE_KEY)).toBeUndefined()
		})

		it("updates the matching nested element", ({ expect }) => {
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A", { children: [makeElem(CHILD_ID, "Child")] }),
			])
			const api = makeDocumentAPI()

			api.updateDocumentTreeElementCache(CHILD_ID, {
				name: "Renamed",
				icon: "icon-b",
				protected: true,
			})

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID, "Doc A", {
					children: [
						makeElem(CHILD_ID, "Renamed", { icon: "icon-b", protected: true }),
					],
				}),
			])
		})
	})

	describe("duplicateDocument", () => {
		it("bails out for a non-xid id without a request", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeElem(DOC_ID, "Doc A"),
			])
			const postCalls = mockEndpoint(
				"POST",
				`/api/documents/${SHORT_ID}/duplicate`,
				() => ({ id: DOC_ID }),
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await api.duplicateDocument.mutateAsync(SHORT_ID)

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(postCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("inserts a timestamped copy next to the source document", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const tagTreeCalls = mockEndpoint("GET", "/api/tags/tree", () => [])
			const post = mockDeferredEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/duplicate`,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A", { icon: "icon-src" })])
			const api = makeDocumentAPI()
			const tagAPI = runInApp(() => useTagAPI())
			await tagAPI.fetchTagTree.refresh()

			const pending = api.duplicateDocument.mutateAsync(DOC_ID)
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				{
					id: ANY_STRING,
					documentName: matchingString(
						/^Doc A \(\d{4} \w{3}\. \d{2} \d{2}:\d{2}\)$/,
					),
					icon: "icon-src",
					children: null,
					protected: false,
					localOptimisticInsert: true,
				},
				makeElem(DOC_ID, "Doc A", { icon: "icon-src" }),
			])
			post.resolve({ id: DOC_ID_2 })

			await pending
			await flushPromises()

			expect(post.calls).toHaveLength(1)
			expect(treeCalls).toHaveLength(1)
			// the copy carries the source's tags, so the tag tree lists it too
			expect(tagTreeCalls).toHaveLength(2)
		})

		it("inserts the copy under the source document's parent", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint(
				"POST",
				`/api/documents/${CHILD_ID}/duplicate`,
			)
			seedQueryData(TREE_KEY, [
				makeElem(DOC_ID, "Doc A", { children: [makeElem(CHILD_ID, "Child")] }),
			])
			const api = makeDocumentAPI()

			const pending = api.duplicateDocument.mutateAsync(CHILD_ID)
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID, "Doc A", {
					children: [
						expect.objectContaining({
							documentName: matchingString(/^Child \(/),
							localOptimisticInsert: true,
						}),
						makeElem(CHILD_ID, "Child"),
					],
				}),
			])
			post.resolve({ id: DOC_ID_2 })

			await pending
		})

		it("falls back to a default name when the source is not cached", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint(
				"POST",
				`/api/documents/${DOC_ID_2}/duplicate`,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			const pending = api.duplicateDocument.mutateAsync(DOC_ID_2)
			await post.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				expect.objectContaining({
					documentName: matchingString(/^New Document \(/),
					icon: "mingcute:document-2-fill",
					localOptimisticInsert: true,
				}),
				makeElem(DOC_ID, "Doc A"),
			])
			post.resolve({ id: DOC_ID_2 })

			await pending
		})

		it("rolls back the tree when the request fails", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			mockEndpoint("POST", `/api/documents/${DOC_ID}/duplicate`, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await expect(api.duplicateDocument.mutateAsync(DOC_ID)).rejects.toThrow()

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(treeCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic insert", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/duplicate`,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			const pending = api.duplicateDocument.mutateAsync(DOC_ID)
			await post.reached

			seedQueryData(TREE_KEY, [makeElem(CHILD_ID, "Divergent")])
			post.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(TREE_KEY)).toEqual([makeElem(CHILD_ID, "Divergent")])
		})
	})

	describe("searchDocuments", () => {
		it("searches with the encoded query", async ({ expect }) => {
			const results = [{ id: "b1", documentId: DOC_ID }]
			const searchCalls = mockEndpoint(
				"GET",
				"/api/documents/search",
				() => results,
			)
			const api = makeDocumentAPI()

			const result = await api.searchDocuments("a b/c")

			expect(result).toEqual(results)
			expect(searchCalls).toHaveLength(1)
			expect(searchCalls[0]?.query).toEqual({ q: "a b/c" })
		})
	})

	describe("updateDocumentBranch", () => {
		it("bails out for a non-xid document id without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeElem(SHORT_ID, "Doc A"),
			])
			const putCalls = mockEndpoint(
				"PUT",
				`http://test.local/auth-realtime/api/documents/${SHORT_ID}/branches/${BRANCH_ID}`,
				() => null,
			)
			seedQueryData(TREE_KEY, [makeElem(SHORT_ID, "Doc A")])
			const api = makeDocumentAPI()

			await api.updateDocumentBranch.mutateAsync({
				id: SHORT_ID,
				branchId: BRANCH_ID,
				protectedMode: true,
			})

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(SHORT_ID, "Doc A")])
			expect(putCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("bails out for a non-xid branch id without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeElem(DOC_ID, "Doc A"),
			])
			const putCalls = mockEndpoint(
				"PUT",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches/${SHORT_ID}`,
				() => null,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await api.updateDocumentBranch.mutateAsync({
				id: DOC_ID,
				branchId: SHORT_ID,
				protectedMode: true,
			})

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
			expect(putCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("flips the protected flag optimistically and refreshes the tree", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const put = mockDeferredEndpoint(
				"PUT",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches/${BRANCH_ID}`,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			const pending = api.updateDocumentBranch.mutateAsync({
				id: DOC_ID,
				branchId: BRANCH_ID,
				protectedMode: true,
			})
			await put.reached

			expect(readQueryData(TREE_KEY)).toEqual([
				makeElem(DOC_ID, "Doc A", { protected: true }),
			])
			put.resolve(null)

			await pending

			expect(put.calls).toHaveLength(1)
			expect(put.calls[0]?.body).toEqual({ protected: true })
			expect(treeCalls).toHaveLength(1)
		})

		it("rolls back the tree when the request fails", async ({ expect }) => {
			mockEndpoint("GET", TREE_URL, () => [])
			mockEndpoint(
				"PUT",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches/${BRANCH_ID}`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			await expect(
				api.updateDocumentBranch.mutateAsync({
					id: DOC_ID,
					branchId: BRANCH_ID,
					protectedMode: true,
				}),
			).rejects.toThrow()

			expect(readQueryData(TREE_KEY)).toEqual([makeElem(DOC_ID, "Doc A")])
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const put = mockDeferredEndpoint(
				"PUT",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches/${BRANCH_ID}`,
			)
			seedQueryData(TREE_KEY, [makeElem(DOC_ID, "Doc A")])
			const api = makeDocumentAPI()

			const pending = api.updateDocumentBranch.mutateAsync({
				id: DOC_ID,
				branchId: BRANCH_ID,
				protectedMode: true,
			})
			await put.reached

			seedQueryData(TREE_KEY, [makeElem(CHILD_ID, "Divergent")])
			put.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(TREE_KEY)).toEqual([makeElem(CHILD_ID, "Divergent")])
		})
	})

	describe("createDocumentBranch", () => {
		it("bails out for a non-xid source branch", async ({ expect }) => {
			const postCalls = mockEndpoint(
				"POST",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
				() => ({}),
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			await api.createDocumentBranch.mutateAsync({
				docId: DOC_ID,
				req: { branch: "draft", sourceBranchId: SHORT_ID },
			})

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
			])
			expect(postCalls).toHaveLength(0)
		})

		it("appends an optimistic branch copying the source metadata", async ({
			expect,
		}) => {
			const postCalls = mockEndpoint(
				"POST",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
				() => ({ id: DOC_ID }),
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			await api.createDocumentBranch.mutateAsync({
				docId: DOC_ID,
				req: { branch: "draft", sourceBranchId: BRANCH_ID },
			})

			// the branches entry has no active query, so the invalidation
			// leaves the optimistic append in place
			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
				{
					branchId: ANY_STRING,
					branchName: "draft",
					documentName: "Doc A",
					icon: "icon-a",
					protected: false,
					default: false,
					createdAt: ANY_DATE,
					updatedAt: ANY_DATE,
				},
			])
			expect(postCalls).toHaveLength(1)
			expect(postCalls[0]?.body).toEqual({
				branch: "draft",
				sourceBranchId: BRANCH_ID,
			})
		})

		it("falls back to empty metadata when the source branch is not cached", async ({
			expect,
		}) => {
			mockEndpoint(
				"POST",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
				() => ({
					id: DOC_ID,
				}),
			)
			seedQueryData(BRANCHES_KEY, [])
			const api = makeDocumentAPI()

			await api.createDocumentBranch.mutateAsync({
				docId: DOC_ID,
				req: { branch: "draft", sourceBranchId: BRANCH_ID },
			})

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				expect.objectContaining({
					branchName: "draft",
					documentName: "",
					icon: "",
				}),
			])
		})

		it("rolls back the branches when the request fails", async ({ expect }) => {
			mockEndpoint(
				"POST",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			await expect(
				api.createDocumentBranch.mutateAsync({
					docId: DOC_ID,
					req: { branch: "draft", sourceBranchId: BRANCH_ID },
				}),
			).rejects.toThrow()

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
			])
		})

		it("skips the rollback when the cache changed after the optimistic append", async ({
			expect,
		}) => {
			const post = mockDeferredEndpoint(
				"POST",
				`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			const pending = api.createDocumentBranch.mutateAsync({
				docId: DOC_ID,
				req: { branch: "draft", sourceBranchId: BRANCH_ID },
			})
			await post.reached

			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID_2, "divergent")])
			post.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID_2, "divergent"),
			])
		})
	})

	describe("deleteDocumentBranch", () => {
		it("bails out for a non-xid branch id", async ({ expect }) => {
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${SHORT_ID}`,
				() => null,
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			await api.deleteDocumentBranch.mutateAsync({
				docId: DOC_ID,
				branchId: SHORT_ID,
			})

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
			])
			expect(deleteCalls).toHaveLength(0)
		})

		it("removes the branch optimistically", async ({ expect }) => {
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID_2}`,
				() => null,
			)
			seedQueryData(BRANCHES_KEY, [
				makeBranch(BRANCH_ID, "main"),
				makeBranch(BRANCH_ID_2, "draft"),
			])
			const api = makeDocumentAPI()

			await api.deleteDocumentBranch.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID_2,
			})

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
			])
			expect(deleteCalls).toHaveLength(1)
		})

		it("leaves the branches unchanged for an unknown branch id", async ({
			expect,
		}) => {
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID_2}`,
				() => null,
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			await api.deleteDocumentBranch.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID_2,
			})

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
			])
			expect(deleteCalls).toHaveLength(1)
		})

		it("rolls back the branches when the request fails", async ({ expect }) => {
			mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			await expect(
				api.deleteDocumentBranch.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
				}),
			).rejects.toThrow()

			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID, "main"),
			])
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			const del = mockDeferredEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}`,
			)
			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID, "main")])
			const api = makeDocumentAPI()

			const pending = api.deleteDocumentBranch.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
			})
			await del.reached

			seedQueryData(BRANCHES_KEY, [makeBranch(BRANCH_ID_2, "divergent")])
			del.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(BRANCHES_KEY)).toEqual([
				makeBranch(BRANCH_ID_2, "divergent"),
			])
		})
	})

	describe("mergeDocumentBranches", () => {
		it("bails out for a non-xid branch id", async ({ expect }) => {
			const mergeCalls = mockEndpoint("PUT", MERGE_URL, () => null)
			const api = makeDocumentAPI()

			await api.mergeDocumentBranches.mutateAsync({
				docId: DOC_ID,
				fromBranchId: SHORT_ID,
				toBranchId: BRANCH_ID,
			})

			expect(mergeCalls).toHaveLength(0)
		})

		it("merges through the auth-realtime api", async ({ expect }) => {
			const mergeCalls = mockEndpoint("PUT", MERGE_URL, () => null)
			const api = makeDocumentAPI()

			await api.mergeDocumentBranches.mutateAsync({
				docId: DOC_ID,
				fromBranchId: BRANCH_ID,
				toBranchId: BRANCH_ID_2,
			})

			expect(mergeCalls).toHaveLength(1)
			expect(mergeCalls[0]?.body).toEqual({
				fromBranchId: BRANCH_ID,
				toBranchId: BRANCH_ID_2,
			})
		})

		it("refreshes the hooks and tags the target branch took from the source", async ({
			expect,
		}) => {
			seedAuthSession(USER_ID)
			seedAuthOrganization(ORG_ID)
			mockEndpoint("PUT", MERGE_URL, () => null)
			const hookCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/hooks`,
				() => [],
			)
			const tagCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID_2}/tags`,
				() => [],
			)
			const treeCalls = mockEndpoint("GET", "/api/tags/tree", () => [])
			const api = makeDocumentAPI()
			const hooks = runInApp(() =>
				useDocumentHookAPI().useFetchDocumentHooksByDocID(DOC_ID, BRANCH_ID_2),
			)
			const tagAPI = runInApp(() => useTagAPI())
			const tags = runInApp(() =>
				tagAPI.useFetchBranchTags(DOC_ID, BRANCH_ID_2),
			)
			await hooks.refresh()
			await tags.refresh()
			await tagAPI.fetchTagTree.refresh()

			await api.mergeDocumentBranches.mutateAsync({
				docId: DOC_ID,
				fromBranchId: BRANCH_ID,
				toBranchId: BRANCH_ID_2,
			})
			await flushPromises()

			expect(hookCalls).toHaveLength(2)
			expect(tagCalls).toHaveLength(2)
			expect(treeCalls).toHaveLength(2)
		})
	})

	describe("useFetchDocumentMaintainersByDocId", () => {
		it("returns no maintainers without a document id", async ({ expect }) => {
			const maintainerCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/maintainers`,
				() => [USER_ID],
			)
			const api = makeDocumentAPI()
			const maintainers = runInApp(() =>
				api.useFetchDocumentMaintainersByDocId(null),
			)

			const result = await maintainers.refresh()

			expect(result.data).toEqual([])
			expect(maintainerCalls).toHaveLength(0)
		})

		it("fetches the maintainers of the document", async ({ expect }) => {
			const maintainerCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/maintainers`,
				() => [USER_ID],
			)
			const api = makeDocumentAPI()
			const maintainers = runInApp(() =>
				api.useFetchDocumentMaintainersByDocId(DOC_ID),
			)

			const result = await maintainers.refresh()

			expect(result.data).toEqual([USER_ID])
			expect(maintainerCalls).toHaveLength(1)
		})
	})

	describe("useFetchDocumentBranchesByDocId", () => {
		it("returns no branches without a document id", async ({ expect }) => {
			const branchCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches`,
				() => [makeBranch(BRANCH_ID, "main")],
			)
			const api = makeDocumentAPI()
			const branches = runInApp(() => api.useFetchDocumentBranchesByDocId(null))

			const result = await branches.refresh()

			expect(result.data).toEqual([])
			expect(branchCalls).toHaveLength(0)
		})

		it("fetches the branches of the document", async ({ expect }) => {
			const branchCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches`,
				() => [makeBranch(BRANCH_ID, "main")],
			)
			const api = makeDocumentAPI()
			const branches = runInApp(() =>
				api.useFetchDocumentBranchesByDocId(DOC_ID),
			)

			const result = await branches.refresh()

			expect(result.data).toEqual([makeBranch(BRANCH_ID, "main")])
			expect(branchCalls).toHaveLength(1)
		})
	})

	describe("useFetchBranchReviewers", () => {
		it("returns no reviewers without a branch id", async ({ expect }) => {
			const reviewerCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => [],
			)
			const api = makeDocumentAPI()
			const reviewers = runInApp(() =>
				api.useFetchBranchReviewers(DOC_ID, null),
			)

			const result = await reviewers.refresh()

			expect(result.data).toEqual([])
			expect(reviewerCalls).toHaveLength(0)
		})

		it("fetches the reviewers of the branch", async ({ expect }) => {
			const reviewerCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => [makeReviewer(USER_ID, true)],
			)
			const api = makeDocumentAPI()
			const reviewers = runInApp(() =>
				api.useFetchBranchReviewers(DOC_ID, BRANCH_ID),
			)

			const result = await reviewers.refresh()

			expect(result.data).toEqual([makeReviewer(USER_ID, true)])
			expect(reviewerCalls).toHaveLength(1)
		})

		it("maps a null response to an empty list", async ({ expect }) => {
			const reviewerCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => null,
			)
			const api = makeDocumentAPI()
			const reviewers = runInApp(() =>
				api.useFetchBranchReviewers(DOC_ID, BRANCH_ID),
			)

			const result = await reviewers.refresh()

			expect(result.data).toEqual([])
			expect(reviewerCalls).toHaveLength(1)
		})
	})

	describe("updateBranchApproval", () => {
		it("bails out for a non-xid branch id without a request", async ({
			expect,
		}) => {
			const putCalls = mockEndpoint(
				"PUT",
				`/api/documents/${DOC_ID}/branches/${SHORT_ID}/review-approve`,
				() => null,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()

			await api.updateBranchApproval.mutateAsync({
				docId: DOC_ID,
				branchId: SHORT_ID,
				approved: true,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([])
			expect(putCalls).toHaveLength(0)
		})

		it("sends the request without an optimistic update when the session is unknown", async ({
			expect,
		}) => {
			const putCalls = mockEndpoint(
				"PUT",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/review-approve`,
				() => null,
			)
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()

			await api.updateBranchApproval.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				approved: true,
			})

			// the mutation only guards the ids, so the request still goes out
			// while the optimistic update is skipped
			expect(readQueryData(REVIEWERS_KEY)).toEqual([])
			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual({ approved: true })
		})

		it("toggles the approval of the current reviewer optimistically", async ({
			expect,
		}) => {
			const putCalls = mockEndpoint(
				"PUT",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/review-approve`,
				() => null,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID, false)])
			const api = makeDocumentAPI()

			await api.updateBranchApproval.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				approved: true,
			})

			// the reviewers entry has no active query, so the invalidation
			// leaves the optimistic update in place
			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID, true),
			])
			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual({ approved: true })
		})

		it("adds the current user as a reviewer when not present", async ({
			expect,
		}) => {
			mockEndpoint(
				"PUT",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/review-approve`,
				() => null,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID_2, false)])
			const api = makeDocumentAPI()

			await api.updateBranchApproval.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				approved: true,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID_2, false),
				makeReviewer(USER_ID, true),
			])
		})

		it("rolls back the reviewers when the request fails", async ({
			expect,
		}) => {
			mockEndpoint(
				"PUT",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/review-approve`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID, false)])
			const api = makeDocumentAPI()

			await expect(
				api.updateBranchApproval.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					approved: true,
				}),
			).rejects.toThrow()

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID, false),
			])
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			const put = mockDeferredEndpoint(
				"PUT",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/review-approve`,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID, false)])
			const api = makeDocumentAPI()

			const pending = api.updateBranchApproval.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				approved: true,
			})
			await put.reached

			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID_2, true)])
			put.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID_2, true),
			])
		})
	})

	describe("inviteBranchReviewer", () => {
		it.for([
			{ name: "a non-xid document id", docId: SHORT_ID, userId: USER_ID_2 },
			{
				name: "an optimistic-insert user id",
				docId: DOC_ID,
				userId: "optimistic-u1",
			},
		])("bails out for $name", async ({ docId, userId }, { expect }) => {
			const postCalls = mockEndpoint(
				"POST",
				`/api/documents/${docId}/branches/${BRANCH_ID}/reviewers`,
				() => null,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()

			await api.inviteBranchReviewer.mutateAsync({
				docId,
				branchId: BRANCH_ID,
				userId,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([])
			expect(postCalls).toHaveLength(0)
		})

		it("sends the request without an optimistic update when the organization is unknown and refreshes the reviewers", async ({
			expect,
		}) => {
			const serverReviewers = [makeReviewer(USER_ID_2, false)]
			const getCalls = mockEndpoint(
				"GET",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => serverReviewers,
			)
			let reviewersAtRequest: unknown
			const postCalls = mockEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => {
					reviewersAtRequest = readQueryData(REVIEWERS_KEY)

					return null
				},
			)
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()
			runInApp(() => api.useFetchBranchReviewers(DOC_ID, BRANCH_ID))

			await api.inviteBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})

			expect(reviewersAtRequest).toEqual([])
			expect(postCalls).toHaveLength(1)
			expect(postCalls[0]?.body).toEqual({ userId: USER_ID_2 })
			// the request went out, so the active reviewers query is refetched
			// even though the optimistic insert was skipped
			expect(getCalls).toHaveLength(1)
			expect(readQueryData(REVIEWERS_KEY)).toEqual(serverReviewers)
		})

		it("adds a new reviewer optimistically", async ({ expect }) => {
			const postCalls = mockEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => null,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()

			await api.inviteBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID_2, false),
			])
			expect(postCalls).toHaveLength(1)
			expect(postCalls[0]?.body).toEqual({ userId: USER_ID_2 })
		})

		it("resets the approvals of an already invited reviewer", async ({
			expect,
		}) => {
			mockEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => null,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [
				{ ...makeReviewer(USER_ID_2, true), previouslyApproved: true },
			])
			const api = makeDocumentAPI()

			await api.inviteBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})

			// the current approval is remembered as the previous one before
			// the re-invite clears it
			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				{ ...makeReviewer(USER_ID_2, false), previouslyApproved: true },
			])
		})

		it("rolls back the reviewers when the request fails", async ({
			expect,
		}) => {
			mockEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()

			await expect(
				api.inviteBranchReviewer.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					userId: USER_ID_2,
				}),
			).rejects.toThrow()

			expect(readQueryData(REVIEWERS_KEY)).toEqual([])
		})

		it("skips the rollback when the cache changed after the optimistic invite", async ({
			expect,
		}) => {
			const post = mockDeferredEndpoint(
				"POST",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
			)
			seedAuth()
			seedQueryData(REVIEWERS_KEY, [])
			const api = makeDocumentAPI()

			const pending = api.inviteBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})
			await post.reached

			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID, true)])
			post.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID, true),
			])
		})
	})

	describe("removeBranchReviewer", () => {
		it.for([
			{ name: "a non-xid branch id", branchId: SHORT_ID, userId: USER_ID_2 },
			{
				name: "an optimistic-insert user id",
				branchId: BRANCH_ID,
				userId: "optimistic-u1",
			},
		])("bails out for $name", async ({ branchId, userId }, { expect }) => {
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${branchId}/reviewers`,
				() => null,
			)
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID_2, false)])
			const api = makeDocumentAPI()

			await api.removeBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId,
				userId,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID_2, false),
			])
			expect(deleteCalls).toHaveLength(0)
		})

		it("removes the reviewer optimistically", async ({ expect }) => {
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => null,
			)
			seedQueryData(REVIEWERS_KEY, [
				makeReviewer(USER_ID, true),
				makeReviewer(USER_ID_2, false),
			])
			const api = makeDocumentAPI()

			await api.removeBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID, true),
			])
			expect(deleteCalls).toHaveLength(1)
			expect(deleteCalls[0]?.query).toEqual({ userId: USER_ID_2 })
		})

		it("leaves the reviewers unchanged for an unknown user", async ({
			expect,
		}) => {
			const deleteCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => null,
			)
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID, true)])
			const api = makeDocumentAPI()

			await api.removeBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID, true),
			])
			expect(deleteCalls).toHaveLength(1)
		})

		it("rolls back the reviewers when the request fails", async ({
			expect,
		}) => {
			mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID_2, false)])
			const api = makeDocumentAPI()

			await expect(
				api.removeBranchReviewer.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					userId: USER_ID_2,
				}),
			).rejects.toThrow()

			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID_2, false),
			])
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			const del = mockDeferredEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/reviewers`,
			)
			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID_2, false)])
			const api = makeDocumentAPI()

			const pending = api.removeBranchReviewer.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				userId: USER_ID_2,
			})
			await del.reached

			seedQueryData(REVIEWERS_KEY, [makeReviewer(USER_ID, true)])
			del.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(readQueryData(REVIEWERS_KEY)).toEqual([
				makeReviewer(USER_ID, true),
			])
		})
	})
})

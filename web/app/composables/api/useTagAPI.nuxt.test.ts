import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockDeferredEndpoint,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
} from "./test-helpers"
import useTagAPI from "./useTagAPI"

const TREE_KEY = ["tags", "tree"] as const
const TREE_URL = "/api/tags/tree"
const DOCUMENT_TREE_KEY = ["documents", "tree"] as const

const TAG_A = makeXid("taga")
const TAG_B = makeXid("tagb")
const TAG_C = makeXid("tagc")
const DOC_A = makeXid("doca")
const DOC_CHILD = makeXid("docc")
const BRANCH_A = makeXid("bra")
const BRANCH_DRAFT = makeXid("brd")
const SHORT_ID = "local-1"
const BRANCH_A_TAGS_KEY = ["tags", "branch", BRANCH_A] as const
const BRANCH_A_TAGS_URL = `/api/documents/${DOC_A}/branches/${BRANCH_A}/tags`

type TagAPI = ReturnType<typeof useTagAPI>

interface TestTag {
	id: string
	tagName: string
	color: string
	hidden: boolean
	documents: TestDocument[]
}

interface TestDocument {
	id: string
	documentName: string
	icon: string
	protected: boolean
	defaultBranchId: string
	children: TestDocument[] | null
}

function makeTagAPI(): TagAPI {
	return runInApp(() => useTagAPI())
}

function makeTag(id: string, tagName: string, documents: TestDocument[] = []) {
	return { id, tagName, color: "#22c55e", hidden: false, documents }
}

function makeDoc(
	id: string,
	documentName: string,
	children: TestDocument[] | null = null,
): TestDocument {
	return {
		id,
		documentName,
		icon: "lucide:file",
		protected: false,
		defaultBranchId: BRANCH_A,
		children,
	}
}

function branchTagIds() {
	return readQueryData(BRANCH_A_TAGS_KEY) as string[] | undefined
}

function readTree() {
	return readQueryData(TREE_KEY) as TestTag[] | undefined
}

function tagIds() {
	return (readTree() ?? []).map((tag) => tag.id)
}

function documentIds(tagIndex = 0) {
	return (readTree()?.[tagIndex]?.documents ?? []).map((doc) => doc.id)
}

// every mutation writes the tree through one cache entry and invalidates it
// on success, so the tests share a cache and cannot interleave
describe("useTagAPI", { concurrent: false }, () => {
	beforeEach(clearQueryCache)
	afterEach(disposeMockEndpoints)

	describe("fetchTagTree", () => {
		it("fetches the tags with the documents carrying them", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const api = makeTagAPI()

			await api.fetchTagTree.refresh()

			expect(calls).toHaveLength(1)
			expect(api.fetchTagTree.data.value?.[0]?.tagName).toBe("Production")
			expect(api.fetchTagTree.data.value?.[0]?.documents?.[0]?.id).toBe(DOC_A)
		})
	})

	describe("updateTagTree", () => {
		it("bails out for a non-xid tag id without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeTag(SHORT_ID, "Draft")])
			const api = makeTagAPI()

			await api.updateTagTree.mutateAsync({
				id: SHORT_ID,
				insertBeforeId: null,
			})

			expect(tagIds()).toEqual([SHORT_ID])
			expect(putCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("rejects when the tag is not in the tree", async ({ expect }) => {
			const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			await expect(
				api.updateTagTree.mutateAsync({ id: TAG_B, insertBeforeId: null }),
			).rejects.toThrow("invalid tag tree update data")

			expect(tagIds()).toEqual([TAG_A])
			expect(putCalls).toHaveLength(0)
		})

		it("rejects when the tag it should precede is not in the tree", async ({
			expect,
		}) => {
			const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			await expect(
				api.updateTagTree.mutateAsync({ id: TAG_A, insertBeforeId: TAG_B }),
			).rejects.toThrow("invalid tag tree update data")

			expect(putCalls).toHaveLength(0)
		})

		it("moves a tag before a sibling and refreshes the tree", async ({
			expect,
		}) => {
			const serverTree = [
				makeTag(TAG_C, "Incidents"),
				makeTag(TAG_A, "Production"),
			]
			const treeCalls = mockEndpoint("GET", TREE_URL, () => serverTree)
			const put = mockDeferredEndpoint("PUT", TREE_URL)
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production"),
				makeTag(TAG_B, "Staging"),
				makeTag(TAG_C, "Incidents"),
			])
			const api = makeTagAPI()

			const pending = api.updateTagTree.mutateAsync({
				id: TAG_C,
				insertBeforeId: TAG_B,
			})
			await put.reached

			// the move shows before the request answers
			expect(tagIds()).toEqual([TAG_A, TAG_C, TAG_B])
			expect(put.calls[0]?.body).toEqual({ id: TAG_C, sortIndex: 1 })

			put.resolve({})
			await pending
			await flushPromises()

			expect(treeCalls).toHaveLength(1)
			expect(tagIds()).toEqual([TAG_C, TAG_A])
		})

		it("moves a tag to the very top when nothing follows it", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production"),
				makeTag(TAG_B, "Staging"),
			])
			const api = makeTagAPI()

			await api.updateTagTree.mutateAsync({ id: TAG_B, insertBeforeId: null })

			expect(putCalls[0]?.body).toEqual({ id: TAG_B, sortIndex: 0 })
		})

		it("puts the old order back when the move is rejected", async ({
			expect,
		}) => {
			mockEndpoint("PUT", TREE_URL, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production"),
				makeTag(TAG_B, "Staging"),
			])
			const api = makeTagAPI()

			await expect(
				api.updateTagTree.mutateAsync({ id: TAG_B, insertBeforeId: TAG_A }),
			).rejects.toThrow()
			await flushPromises()

			expect(tagIds()).toEqual([TAG_A, TAG_B])
		})
	})

	describe("createTag", () => {
		it("shows the new tag at the end and refreshes the tree", async ({
			expect,
		}) => {
			const serverTree = [
				makeTag(TAG_A, "Production"),
				makeTag(TAG_B, "Staging"),
			]
			const treeCalls = mockEndpoint("GET", TREE_URL, () => serverTree)
			const post = mockDeferredEndpoint("POST", "/api/tags")
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			const pending = api.createTag.mutateAsync({
				tagName: "Staging",
				color: "#f97316",
			})
			await post.reached

			const optimistic = readTree()
			expect(optimistic?.map((tag) => tag.tagName)).toEqual([
				"Production",
				"Staging",
			])
			expect(post.calls[0]?.body).toEqual({
				tagName: "Staging",
				color: "#f97316",
			})

			post.resolve({ id: TAG_B })
			await pending
			await flushPromises()

			expect(treeCalls).toHaveLength(1)
			expect(tagIds()).toEqual([TAG_A, TAG_B])
		})

		it("drops the new tag again when the request fails", async ({ expect }) => {
			mockEndpoint("POST", "/api/tags", () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			await expect(
				api.createTag.mutateAsync({ tagName: "Staging", color: "#f97316" }),
			).rejects.toThrow()
			await flushPromises()

			expect(tagIds()).toEqual([TAG_A])
		})
	})

	describe("updateTagVisibility", () => {
		it("bails out for a non-xid tag id without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const putCalls = mockEndpoint(
				"PUT",
				`/api/tags/${SHORT_ID}/visibility`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [makeTag(SHORT_ID, "Draft")])
			const api = makeTagAPI()

			await api.updateTagVisibility.mutateAsync({
				id: SHORT_ID,
				req: { hidden: true },
			})

			expect(putCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("rejects when the tag is not in the tree", async ({ expect }) => {
			const putCalls = mockEndpoint(
				"PUT",
				`/api/tags/${TAG_B}/visibility`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			await expect(
				api.updateTagVisibility.mutateAsync({
					id: TAG_B,
					req: { hidden: true },
				}),
			).rejects.toThrow("invalid tag visibility data")

			expect(putCalls).toHaveLength(0)
		})

		it("hides the tag before the request answers", async ({ expect }) => {
			mockEndpoint("GET", TREE_URL, () => [
				{ ...makeTag(TAG_A, "Production"), hidden: true },
			])
			const put = mockDeferredEndpoint("PUT", `/api/tags/${TAG_A}/visibility`)
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			const pending = api.updateTagVisibility.mutateAsync({
				id: TAG_A,
				req: { hidden: true },
			})
			await put.reached

			expect(readTree()?.[0]?.hidden).toBe(true)
			expect(put.calls[0]?.body).toEqual({ hidden: true })

			put.resolve({})
			await pending
			await flushPromises()

			expect(readTree()?.[0]?.hidden).toBe(true)
		})

		it("shows the tag again when the request fails", async ({ expect }) => {
			mockEndpoint("PUT", `/api/tags/${TAG_A}/visibility`, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			await expect(
				api.updateTagVisibility.mutateAsync({
					id: TAG_A,
					req: { hidden: true },
				}),
			).rejects.toThrow()
			await flushPromises()

			expect(readTree()?.[0]?.hidden).toBe(false)
		})
	})

	describe("deleteTag", () => {
		it("bails out for a non-xid tag id without a request", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [])
			const delCalls = mockEndpoint(
				"DELETE",
				`/api/tags/${SHORT_ID}/visibility`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [makeTag(SHORT_ID, "Draft")])
			const api = makeTagAPI()

			await api.deleteTag.mutateAsync(SHORT_ID)

			expect(tagIds()).toEqual([SHORT_ID])
			expect(delCalls).toHaveLength(0)
			expect(treeCalls).toHaveLength(0)
		})

		it("removes the tag and refreshes the tree", async ({ expect }) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_B, "Staging"),
			])
			const del = mockDeferredEndpoint("DELETE", `/api/tags/${TAG_A}`)
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production"),
				makeTag(TAG_B, "Staging"),
			])
			const api = makeTagAPI()

			const pending = api.deleteTag.mutateAsync(TAG_A)
			await del.reached

			expect(tagIds()).toEqual([TAG_B])

			del.resolve({})
			await pending
			await flushPromises()

			expect(treeCalls).toHaveLength(1)
			expect(tagIds()).toEqual([TAG_B])
		})

		it("puts the tag back when the deletion fails", async ({ expect }) => {
			mockEndpoint("DELETE", `/api/tags/${TAG_A}`, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production"),
				makeTag(TAG_B, "Staging"),
			])
			const api = makeTagAPI()

			await expect(api.deleteTag.mutateAsync(TAG_A)).rejects.toThrow()
			await flushPromises()

			expect(tagIds()).toEqual([TAG_A, TAG_B])
		})
	})

	describe("useFetchBranchTags", () => {
		it("fetches the ids of the tags the branch carries", async ({ expect }) => {
			const calls = mockEndpoint("GET", BRANCH_A_TAGS_URL, () => [TAG_A])
			const api = makeTagAPI()

			const query = runInApp(() => api.useFetchBranchTags(DOC_A, BRANCH_A))
			await query.refresh()

			expect(calls).toHaveLength(1)
			expect(query.data.value).toEqual([TAG_A])
		})

		it("answers nothing without a branch to ask about", async ({ expect }) => {
			const calls = mockEndpoint("GET", BRANCH_A_TAGS_URL, () => [TAG_A])
			const api = makeTagAPI()

			const query = runInApp(() => api.useFetchBranchTags(DOC_A, null))
			await query.refresh()

			expect(calls).toHaveLength(0)
			expect(query.data.value).toEqual([])
		})
	})

	describe("assignBranchTag", () => {
		it("bails out for a non-xid id without a request", async ({ expect }) => {
			const postCalls = mockEndpoint("POST", BRANCH_A_TAGS_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			const api = makeTagAPI()

			await api.assignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: SHORT_ID,
			})

			expect(postCalls).toHaveLength(0)
		})

		it("rejects when the tag is not in the tree", async ({ expect }) => {
			const postCalls = mockEndpoint("POST", BRANCH_A_TAGS_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [makeDoc(DOC_A, "Runbook")])
			const api = makeTagAPI()

			await expect(
				api.assignBranchTag.mutateAsync({
					documentId: DOC_A,
					branchId: BRANCH_A,
					tagId: TAG_B,
				}),
			).rejects.toThrow("invalid document tag data")

			expect(postCalls).toHaveLength(0)
		})

		it("assigns without an optimistic row when the document tree has not loaded", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const postCalls = mockEndpoint("POST", BRANCH_A_TAGS_URL, () => ({}))
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [])
			const api = makeTagAPI()

			await api.assignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: TAG_A,
			})
			await flushPromises()

			// the request still goes out; the refetch is what draws the row
			expect(postCalls).toHaveLength(1)
			expect(documentIds()).toEqual([DOC_A])
		})

		it("copies the document subtree under the tag and refreshes", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			// the branch list core answers with once the tag is assigned
			const carried: string[] = []
			const branchCalls = mockEndpoint("GET", BRANCH_A_TAGS_URL, () => carried)
			const post = mockDeferredEndpoint("POST", BRANCH_A_TAGS_URL)
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [
				makeDoc(DOC_A, "Runbook", [makeDoc(DOC_CHILD, "Rollback")]),
			])
			const api = makeTagAPI()
			const branchTags = runInApp(() => api.useFetchBranchTags(DOC_A, BRANCH_A))
			await branchTags.refresh()

			const pending = api.assignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: TAG_A,
			})
			await post.reached

			expect(documentIds()).toEqual([DOC_A])
			expect(readTree()?.[0]?.documents[0]?.children).toHaveLength(1)
			expect(branchTagIds()).toEqual([TAG_A])
			expect(post.calls[0]?.body).toEqual({ tagId: TAG_A })

			carried.push(TAG_A)
			post.resolve({})
			await pending
			await flushPromises()

			expect(treeCalls).toHaveLength(1)
			expect(branchCalls).toHaveLength(2)
			expect(documentIds()).toEqual([DOC_A])
			expect(branchTags.data.value).toEqual([TAG_A])
		})

		it("draws no row for a branch that is not the document's default", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [makeTag(TAG_A, "Production")])
			const postCalls = mockEndpoint(
				"POST",
				`/api/documents/${DOC_A}/branches/${BRANCH_DRAFT}/tags`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [makeDoc(DOC_A, "Runbook")])
			const api = makeTagAPI()

			await api.assignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_DRAFT,
				tagId: TAG_A,
			})

			// the tree lists a document under a tag by its default branch,
			// so the draft's tag changes nothing in it
			expect(postCalls).toHaveLength(1)
			expect(documentIds()).toEqual([])
		})

		it("leaves an unread branch list for the refetch to fill", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const post = mockDeferredEndpoint("POST", BRANCH_A_TAGS_URL)
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [])
			const api = makeTagAPI()

			const pending = api.assignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: TAG_A,
			})
			await post.reached

			// a list seeded here would hand a later mount this tag alone
			expect(branchTagIds()).toBeUndefined()

			post.resolve({})
			await pending
		})

		it("finds a document nested deep in the tree", async ({ expect }) => {
			mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_A, "Production", [makeDoc(DOC_CHILD, "Rollback")]),
			])
			const postCalls = mockEndpoint(
				"POST",
				`/api/documents/${DOC_CHILD}/branches/${BRANCH_A}/tags`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [
				makeDoc(DOC_A, "Runbook", [makeDoc(DOC_CHILD, "Rollback")]),
			])
			const api = makeTagAPI()

			await api.assignBranchTag.mutateAsync({
				documentId: DOC_CHILD,
				branchId: BRANCH_A,
				tagId: TAG_A,
			})

			expect(postCalls).toHaveLength(1)
			expect(documentIds()).toEqual([DOC_CHILD])
		})

		it("takes the document and the tag away again when the request fails", async ({
			expect,
		}) => {
			mockEndpoint("POST", BRANCH_A_TAGS_URL, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [makeTag(TAG_A, "Production")])
			seedQueryData(DOCUMENT_TREE_KEY, [makeDoc(DOC_A, "Runbook")])
			seedQueryData(BRANCH_A_TAGS_KEY, [])
			const api = makeTagAPI()

			await expect(
				api.assignBranchTag.mutateAsync({
					documentId: DOC_A,
					branchId: BRANCH_A,
					tagId: TAG_A,
				}),
			).rejects.toThrow()
			await flushPromises()

			expect(documentIds()).toEqual([])
			expect(branchTagIds()).toEqual([])
		})
	})

	describe("unassignBranchTag", () => {
		it("bails out for a non-xid id without a request", async ({ expect }) => {
			const delCalls = mockEndpoint(
				"DELETE",
				`${BRANCH_A_TAGS_URL}/${SHORT_ID}`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const api = makeTagAPI()

			await api.unassignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: SHORT_ID,
			})

			expect(delCalls).toHaveLength(0)
			expect(documentIds()).toEqual([DOC_A])
		})

		it("takes the document out of the tag and refreshes", async ({
			expect,
		}) => {
			const treeCalls = mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_A, "Production"),
			])
			// the branch list core answers with once the tag is removed
			const carried = [TAG_A, TAG_B]
			const branchCalls = mockEndpoint("GET", BRANCH_A_TAGS_URL, () => carried)
			const del = mockDeferredEndpoint(
				"DELETE",
				`${BRANCH_A_TAGS_URL}/${TAG_A}`,
			)
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const api = makeTagAPI()
			const branchTags = runInApp(() => api.useFetchBranchTags(DOC_A, BRANCH_A))
			await branchTags.refresh()

			const pending = api.unassignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: TAG_A,
			})
			await del.reached

			expect(documentIds()).toEqual([])
			expect(branchTagIds()).toEqual([TAG_B])

			carried.shift()
			del.resolve({})
			await pending
			await flushPromises()

			expect(treeCalls).toHaveLength(1)
			expect(branchCalls).toHaveLength(2)
			expect(branchTags.data.value).toEqual([TAG_B])
		})

		it("keeps the row when a branch other than the default loses the tag", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const delCalls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_A}/branches/${BRANCH_DRAFT}/tags/${TAG_A}`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const api = makeTagAPI()

			await api.unassignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_DRAFT,
				tagId: TAG_A,
			})

			expect(delCalls).toHaveLength(1)
			expect(documentIds()).toEqual([DOC_A])
		})

		it("still asks core to detach a tag the cached tree does not carry", async ({
			expect,
		}) => {
			mockEndpoint("GET", TREE_URL, () => [])
			const delCalls = mockEndpoint(
				"DELETE",
				`${BRANCH_A_TAGS_URL}/${TAG_B}`,
				() => ({}),
			)
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			const api = makeTagAPI()

			await api.unassignBranchTag.mutateAsync({
				documentId: DOC_A,
				branchId: BRANCH_A,
				tagId: TAG_B,
			})

			expect(delCalls).toHaveLength(1)
		})

		it("puts the document and the tag back when the request fails", async ({
			expect,
		}) => {
			mockEndpoint("DELETE", `${BRANCH_A_TAGS_URL}/${TAG_A}`, () => {
				throw createError({ statusCode: 500 })
			})
			seedQueryData(TREE_KEY, [
				makeTag(TAG_A, "Production", [makeDoc(DOC_A, "Runbook")]),
			])
			seedQueryData(BRANCH_A_TAGS_KEY, [TAG_A])
			const api = makeTagAPI()

			await expect(
				api.unassignBranchTag.mutateAsync({
					documentId: DOC_A,
					branchId: BRANCH_A,
					tagId: TAG_A,
				}),
			).rejects.toThrow()
			await flushPromises()

			expect(documentIds()).toEqual([DOC_A])
			expect(branchTagIds()).toEqual([TAG_A])
		})
	})
})

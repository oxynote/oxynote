import { afterEach, beforeEach, describe, it } from "vitest"
import {
	ANY_DATE,
	ANY_STRING,
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockDeferredEndpoint,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedAuthOrganization,
	seedQueryData,
} from "./test-helpers"
import useDocumentHookAPI from "./useDocumentHookAPI"

const DOC_ID = makeXid("doc1")
const BRANCH_ID = makeXid("bra1")
const HOOK_ID = makeXid("hoo1")
const OTHER_HOOK_ID = makeXid("hoo2")
const MISSING_HOOK_ID = makeXid("hoo3")
const ORG_ID = makeXid("org1")
const NON_XID = "nano-id"
const HOOKS_KEY = ["documents", "hooks", DOC_ID, BRANCH_ID] as const
const LIST_URL = `/api/documents/${DOC_ID}/hooks`

const CREATE_REQ: DocumentHookCreateRequest = {
	type: DocumentHookType.GitHubTracking,
	branchId: BRANCH_ID,
	blockId: "block1",
	settings: { repository: "acme/docs", branch: "main", paths: ["a.md"] },
}

const UPDATE_REQ: DocumentHookUpdateRequest = {
	settings: { repository: "acme/docs", branch: "dev", paths: ["b.md"] },
}

function makeDocumentHookAPI() {
	return runInApp(() => useDocumentHookAPI())
}

// cache seeds pass through the composable's JSON-based clone, so the
// fixture uses string dates — exactly what a cache round-trip produces.
// The state is non-default so a reset is observable.
function makeHook(id: string): DocumentHook {
	return {
		id,
		type: DocumentHookType.GitHubTracking,
		documentId: DOC_ID,
		organizationId: ORG_ID,
		branchId: BRANCH_ID,
		blockId: "block1",
		settings: { repository: "acme/docs", branch: "main", paths: ["a.md"] },
		state: { pathsChecksums: { "a.md": "sum-a" }, status: "missing_branch" },
		score: "100",
		createdAt: "2024-01-01T00:00:00.000Z",
		updatedAt: "2024-01-01T00:00:00.000Z",
	}
}

function seedOrganization() {
	seedAuthOrganization(ORG_ID)
}

function seedHooks(hooks: DocumentHook[]) {
	seedQueryData(HOOKS_KEY, hooks)
}

function getHooks() {
	return readQueryData(HOOKS_KEY)
}

// creating the composable eagerly loads its queries once; refresh() joins
// that in-flight load (or reuses its fresh result) instead of forcing a
// second request, which keeps the call accounting deterministic
describe("useDocumentHookAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useFetchDocumentHooksByDocID", () => {
		it.for([
			{
				name: "resolves no hooks without a document id",
				docId: null,
				branchId: BRANCH_ID,
			},
			{
				name: "resolves no hooks without a branch id",
				docId: DOC_ID,
				branchId: null,
			},
		])("$name", async ({ docId, branchId }, { expect }) => {
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const api = makeDocumentHookAPI()
			const hooks = runInApp(() =>
				api.useFetchDocumentHooksByDocID(docId, branchId),
			)

			const result = await hooks.refresh()

			expect(result.data).toEqual([])
			expect(listCalls).toHaveLength(0)
		})

		it("fetches the hooks of the branch", async ({ expect }) => {
			const listCalls = mockEndpoint("GET", LIST_URL, () => [makeHook(HOOK_ID)])
			const api = makeDocumentHookAPI()
			const hooks = runInApp(() =>
				api.useFetchDocumentHooksByDocID(DOC_ID, BRANCH_ID),
			)

			const result = await hooks.refresh()

			expect(result.data).toEqual([makeHook(HOOK_ID)])
			expect(listCalls).toHaveLength(1)
			expect(listCalls[0]?.query).toEqual({ branchId: BRANCH_ID })
		})
	})

	describe("createDocumentHookByDocID", () => {
		it.for([
			{
				name: "skips the create for a non-xid document id",
				docId: NON_XID,
				branchId: BRANCH_ID,
			},
			{
				name: "skips the create for a non-xid branch id",
				docId: DOC_ID,
				branchId: NON_XID,
			},
		])("$name", async ({ docId, branchId }, { expect }) => {
			seedOrganization()
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const createCalls = mockEndpoint("POST", LIST_URL, () => ({}))
			const api = makeDocumentHookAPI()

			await api.createDocumentHookByDocID.mutateAsync({
				docId,
				req: { ...CREATE_REQ, branchId },
			})

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(createCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("sends the create but skips the optimistic insert when no organization is loaded", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const createCalls = mockEndpoint("POST", LIST_URL, () =>
				makeHook(OTHER_HOOK_ID),
			)
			const api = makeDocumentHookAPI()

			await api.createDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				req: CREATE_REQ,
			})

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(createCalls).toHaveLength(1)
			expect(createCalls[0]?.body).toEqual(CREATE_REQ)
			expect(listCalls).toHaveLength(0)
		})

		it("optimistically prepends the new hook and refetches the list on success", async ({
			expect,
		}) => {
			const createdHook = {
				...makeHook(OTHER_HOOK_ID),
				settings: CREATE_REQ.settings,
			}
			const serverHooks = [createdHook, makeHook(HOOK_ID)]

			seedOrganization()
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => serverHooks)
			const create = mockDeferredEndpoint("POST", LIST_URL)
			const api = makeDocumentHookAPI()
			const hooks = runInApp(() =>
				api.useFetchDocumentHooksByDocID(DOC_ID, BRANCH_ID),
			)

			const pending = api.createDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				req: CREATE_REQ,
			})
			await create.reached

			// the optimistic insert stamps a nanoid id and fresh dates
			expect(getHooks()).toEqual([
				{
					id: ANY_STRING,
					type: DocumentHookType.GitHubTracking,
					documentId: DOC_ID,
					branchId: BRANCH_ID,
					organizationId: ORG_ID,
					blockId: "block1",
					settings: CREATE_REQ.settings,
					state: { pathsChecksums: {}, status: "active" },
					score: "100",
					createdAt: ANY_DATE,
					updatedAt: ANY_DATE,
				},
				makeHook(HOOK_ID),
			])
			create.resolve(createdHook)

			await pending
			expect(create.calls).toHaveLength(1)
			expect(create.calls[0]?.body).toEqual(CREATE_REQ)
			// the success invalidation refetches the active hooks query
			expect(listCalls).toHaveLength(1)
			expect(hooks.data.value).toEqual(serverHooks)
		})

		it("rolls back the optimistic insert when the request fails", async ({
			expect,
		}) => {
			seedOrganization()
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const createCalls = mockEndpoint("POST", LIST_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeDocumentHookAPI()

			await expect(
				api.createDocumentHookByDocID.mutateAsync({
					docId: DOC_ID,
					req: CREATE_REQ,
				}),
			).rejects.toThrow()

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(createCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic insert", async ({
			expect,
		}) => {
			seedOrganization()
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const create = mockDeferredEndpoint("POST", LIST_URL)
			const api = makeDocumentHookAPI()

			const pending = api.createDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				req: CREATE_REQ,
			})
			await create.reached

			// divergent data written after the optimistic insert must survive
			// the failure
			seedHooks([makeHook(OTHER_HOOK_ID)])
			create.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getHooks()).toEqual([makeHook(OTHER_HOOK_ID)])
			expect(create.calls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})
	})

	describe("updateDocumentHookByDocID", () => {
		it.for([
			{
				name: "skips the update for a non-xid document id",
				docId: NON_XID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			},
			{
				name: "skips the update for a non-xid branch id",
				docId: DOC_ID,
				branchId: NON_XID,
				hookId: HOOK_ID,
			},
			{
				name: "skips the update for a non-xid hook id",
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: NON_XID,
			},
		])("$name", async ({ docId, branchId, hookId }, { expect }) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const updateCalls = mockEndpoint(
				"PUT",
				`${LIST_URL}/${HOOK_ID}`,
				() => ({}),
			)
			const api = makeDocumentHookAPI()

			await api.updateDocumentHookByDocID.mutateAsync({
				docId,
				branchId,
				hookId,
				req: UPDATE_REQ,
			})

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(updateCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("optimistically replaces the hook settings and refetches the list on success", async ({
			expect,
		}) => {
			const updatedHook = {
				...makeHook(HOOK_ID),
				settings: UPDATE_REQ.settings,
				updatedAt: "2024-02-01T00:00:00.000Z",
			}
			const serverHooks = [updatedHook, makeHook(OTHER_HOOK_ID)]

			seedHooks([makeHook(HOOK_ID), makeHook(OTHER_HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => serverHooks)
			const update = mockDeferredEndpoint("PUT", `${LIST_URL}/${HOOK_ID}`)
			const api = makeDocumentHookAPI()
			const hooks = runInApp(() =>
				api.useFetchDocumentHooksByDocID(DOC_ID, BRANCH_ID),
			)

			const pending = api.updateDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
				req: UPDATE_REQ,
			})
			await update.reached

			// only the matching hook gets the new settings and a bumped
			// updatedAt; the other hook stays untouched
			expect(getHooks()).toEqual([
				{
					...makeHook(HOOK_ID),
					settings: UPDATE_REQ.settings,
					updatedAt: ANY_DATE,
				},
				makeHook(OTHER_HOOK_ID),
			])
			update.resolve(updatedHook)

			await pending
			expect(update.calls).toHaveLength(1)
			expect(update.calls[0]?.body).toEqual(UPDATE_REQ)
			// the success invalidation refetches the active hooks query
			expect(listCalls).toHaveLength(1)
			expect(hooks.data.value).toEqual(serverHooks)
		})

		it("rolls back the optimistic update when the request fails", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const updateCalls = mockEndpoint("PUT", `${LIST_URL}/${HOOK_ID}`, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeDocumentHookAPI()

			await expect(
				api.updateDocumentHookByDocID.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					hookId: HOOK_ID,
					req: UPDATE_REQ,
				}),
			).rejects.toThrow()

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(updateCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const update = mockDeferredEndpoint("PUT", `${LIST_URL}/${HOOK_ID}`)
			const api = makeDocumentHookAPI()

			const pending = api.updateDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
				req: UPDATE_REQ,
			})
			await update.reached

			// divergent data written after the optimistic update must survive
			// the failure
			seedHooks([makeHook(OTHER_HOOK_ID)])
			update.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getHooks()).toEqual([makeHook(OTHER_HOOK_ID)])
			expect(update.calls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})
	})

	describe("deleteDocumentHookByDocID", () => {
		it.for([
			{
				name: "skips the delete for a non-xid document id",
				docId: NON_XID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			},
			{
				name: "skips the delete for a non-xid branch id",
				docId: DOC_ID,
				branchId: NON_XID,
				hookId: HOOK_ID,
			},
			{
				name: "skips the delete for a non-xid hook id",
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: NON_XID,
			},
		])("$name", async ({ docId, branchId, hookId }, { expect }) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const deleteCalls = mockEndpoint(
				"DELETE",
				`${LIST_URL}/${HOOK_ID}`,
				() => ({}),
			)
			const api = makeDocumentHookAPI()

			await api.deleteDocumentHookByDocID.mutateAsync({
				docId,
				branchId,
				hookId,
			})

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(deleteCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("optimistically removes the hook and refetches the list on success", async ({
			expect,
		}) => {
			const serverHooks = [makeHook(OTHER_HOOK_ID)]

			seedHooks([makeHook(HOOK_ID), makeHook(OTHER_HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => serverHooks)
			const remove = mockDeferredEndpoint("DELETE", `${LIST_URL}/${HOOK_ID}`)
			const api = makeDocumentHookAPI()
			const hooks = runInApp(() =>
				api.useFetchDocumentHooksByDocID(DOC_ID, BRANCH_ID),
			)

			const pending = api.deleteDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			})
			await remove.reached

			expect(getHooks()).toEqual([makeHook(OTHER_HOOK_ID)])
			remove.resolve({})

			await pending
			expect(remove.calls).toHaveLength(1)
			// the success invalidation refetches the active hooks query
			expect(listCalls).toHaveLength(1)
			expect(hooks.data.value).toEqual(serverHooks)
		})

		it("leaves the list unchanged when the hook id is not in the cache", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const deleteCalls = mockEndpoint(
				"DELETE",
				`${LIST_URL}/${MISSING_HOOK_ID}`,
				() => ({}),
			)
			const api = makeDocumentHookAPI()

			await api.deleteDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: MISSING_HOOK_ID,
			})

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(deleteCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})

		it("rolls back the optimistic removal when the request fails", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const deleteCalls = mockEndpoint(
				"DELETE",
				`${LIST_URL}/${HOOK_ID}`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			const api = makeDocumentHookAPI()

			await expect(
				api.deleteDocumentHookByDocID.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					hookId: HOOK_ID,
				}),
			).rejects.toThrow()

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(deleteCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const remove = mockDeferredEndpoint("DELETE", `${LIST_URL}/${HOOK_ID}`)
			const api = makeDocumentHookAPI()

			const pending = api.deleteDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			})
			await remove.reached

			// divergent data written after the optimistic removal must survive
			// the failure
			seedHooks([makeHook(OTHER_HOOK_ID)])
			remove.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getHooks()).toEqual([makeHook(OTHER_HOOK_ID)])
			expect(remove.calls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})
	})

	describe("resetDocumentHookByDocID", () => {
		it.for([
			{
				name: "skips the reset for a non-xid document id",
				docId: NON_XID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			},
			{
				name: "skips the reset for a non-xid branch id",
				docId: DOC_ID,
				branchId: NON_XID,
				hookId: HOOK_ID,
			},
			{
				name: "skips the reset for a non-xid hook id",
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: NON_XID,
			},
		])("$name", async ({ docId, branchId, hookId }, { expect }) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const resetCalls = mockEndpoint(
				"PUT",
				`${LIST_URL}/${HOOK_ID}/reset`,
				() => ({}),
			)
			const api = makeDocumentHookAPI()

			await api.resetDocumentHookByDocID.mutateAsync({
				docId,
				branchId,
				hookId,
			})

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(resetCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("optimistically resets the hook state and refetches the list on success", async ({
			expect,
		}) => {
			const resetHook = {
				...makeHook(HOOK_ID),
				state: { pathsChecksums: {}, status: "active" },
			}
			const serverHooks = [resetHook, makeHook(OTHER_HOOK_ID)]

			seedHooks([makeHook(HOOK_ID), makeHook(OTHER_HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => serverHooks)
			const reset = mockDeferredEndpoint("PUT", `${LIST_URL}/${HOOK_ID}/reset`)
			const api = makeDocumentHookAPI()
			const hooks = runInApp(() =>
				api.useFetchDocumentHooksByDocID(DOC_ID, BRANCH_ID),
			)

			const pending = api.resetDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			})
			await reset.reached

			// only the matching hook gets the default state for its type and a
			// bumped updatedAt; the other hook stays untouched
			expect(getHooks()).toEqual([
				{
					...makeHook(HOOK_ID),
					state: { pathsChecksums: {}, status: "active" },
					updatedAt: ANY_DATE,
				},
				makeHook(OTHER_HOOK_ID),
			])
			reset.resolve(resetHook)

			await pending
			expect(reset.calls).toHaveLength(1)
			// the success invalidation refetches the active hooks query
			expect(listCalls).toHaveLength(1)
			expect(hooks.data.value).toEqual(serverHooks)
		})

		it("rolls back the optimistic reset when the request fails", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const resetCalls = mockEndpoint(
				"PUT",
				`${LIST_URL}/${HOOK_ID}/reset`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			const api = makeDocumentHookAPI()

			await expect(
				api.resetDocumentHookByDocID.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					hookId: HOOK_ID,
				}),
			).rejects.toThrow()

			expect(getHooks()).toEqual([makeHook(HOOK_ID)])
			expect(resetCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic reset", async ({
			expect,
		}) => {
			seedHooks([makeHook(HOOK_ID)])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const reset = mockDeferredEndpoint("PUT", `${LIST_URL}/${HOOK_ID}/reset`)
			const api = makeDocumentHookAPI()

			const pending = api.resetDocumentHookByDocID.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				hookId: HOOK_ID,
			})
			await reset.reached

			// divergent data written after the optimistic reset must survive
			// the failure
			seedHooks([makeHook(OTHER_HOOK_ID)])
			reset.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getHooks()).toEqual([makeHook(OTHER_HOOK_ID)])
			expect(reset.calls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})
	})
})

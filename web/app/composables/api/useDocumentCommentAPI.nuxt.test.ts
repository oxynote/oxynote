import { afterEach, beforeEach, describe, it } from "vitest"
import type {
	DocumentComment,
	DocumentCommentReply,
	DocumentCommentsResponse,
} from "~/utils/api/comment"
import {
	ANY_DATE,
	ANY_STRING,
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedAuthOrganization,
	seedAuthSession,
	seedQueryData,
} from "./test-helpers"
import useDocumentCommentAPI from "./useDocumentCommentAPI"

// isXid() only checks for a 20-character length, so these constants pass
// the mutations' id guards; NON_XID_ID fails them like a nanoid would
const DOC_ID = "docid123456789012345"
const BRANCH_ID = "branchid123456789012"
const COMMENT_ID = "commentid10000000001"
const COMMENT_ID_2 = "commentid20000000002"
const REPLY_ID = "replyid1000000000001"
const REPLY_ID_2 = "replyid2000000000002"
const USER_ID = "userid10000000000001"
const USER_ID_2 = "userid20000000000002"
const ORG_ID = "orgid100000000000001"
const NON_XID_ID = "nano1"

const COMMENTS_KEY = ["documents", DOC_ID, "comments", BRANCH_ID] as const

const COMMENTS_URL = `/api/documents/${DOC_ID}/comments`
const COMMENT_URL = `${COMMENTS_URL}/${COMMENT_ID}`
const RESOLVE_URL = `${COMMENT_URL}/resolve`
const REPLIES_URL = `${COMMENT_URL}/replies`
const REPLY_URL = `${REPLIES_URL}/${REPLY_ID}`

function makeCommentAPI() {
	return runInApp(() => useDocumentCommentAPI())
}

type DocumentCommentAPI = ReturnType<typeof makeCommentAPI>

function seedAuth() {
	seedAuthSession(USER_ID)
	seedAuthOrganization(ORG_ID)
}

function seedComments(comments: DocumentCommentsResponse) {
	seedQueryData(COMMENTS_KEY, comments)
}

function readComments() {
	return readQueryData(COMMENTS_KEY) as DocumentCommentsResponse | undefined
}

// dates are ISO strings because clone() is JSON-based — cache contents
// round-trip through it, so string dates keep the comparisons simple
function makeComment(
	id: string,
	overrides: Partial<DocumentComment> = {},
): DocumentComment {
	return {
		id,
		organizationId: ORG_ID,
		documentId: DOC_ID,
		branchId: BRANCH_ID,
		anchorBlockId: "block-1",
		userId: USER_ID,
		resolved: false,
		resolvedBy: null,
		content: { text: `comment ${id}` },
		createdAt: "2024-01-01T00:00:00.000Z",
		updatedAt: null,
		diffDeletionContext: null,
		...overrides,
	}
}

function makeReply(
	id: string,
	overrides: Partial<DocumentCommentReply> = {},
): DocumentCommentReply {
	return {
		id,
		organizationId: ORG_ID,
		commentId: COMMENT_ID,
		userId: USER_ID,
		content: { text: `reply ${id}` },
		createdAt: "2024-01-01T00:00:00.000Z",
		updatedAt: null,
		...overrides,
	}
}

// creating the composable eagerly loads its queries once; refresh() joins
// that in-flight load (or reuses its fresh result) instead of forcing a
// second request, which keeps the call accounting deterministic
describe("useDocumentCommentAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useFetchDocumentCommentsByDocId", () => {
		it.for([
			{ name: "a document id", docId: null, branchId: BRANCH_ID },
			{ name: "a branch id", docId: DOC_ID, branchId: null },
		])(
			"returns no comments without $name",
			async ({ docId, branchId }, { expect }) => {
				seedAuth()
				const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [
					makeComment(COMMENT_ID),
				])
				const api = makeCommentAPI()
				const comments = runInApp(() =>
					api.useFetchDocumentCommentsByDocId(docId, branchId),
				)

				const result = await comments.refresh()

				expect(result.data).toEqual([])
				expect(getCalls).toHaveLength(0)
			},
		)

		it("fetches the comments of the branch", async ({ expect }) => {
			seedAuth()
			const serverComments = [makeComment(COMMENT_ID)]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			const api = makeCommentAPI()
			const comments = runInApp(() =>
				api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID),
			)

			const result = await comments.refresh()

			expect(result.data).toEqual(serverComments)
			expect(getCalls).toHaveLength(1)
			expect(getCalls[0]?.query).toEqual({ branchId: BRANCH_ID })
		})
	})

	// every mutation guards each of its ids with isXid() in every hook, so
	// one short id makes the whole mutation a no-op: no optimistic write,
	// no request, no invalidation
	it.for([
		{
			name: "createDocumentCommentByDocId",
			method: "POST",
			url: `/api/documents/${NON_XID_ID}/comments`,
			invoke: (api: DocumentCommentAPI) =>
				api.createDocumentCommentByDocId.mutateAsync({
					docId: NON_XID_ID,
					req: {
						content: { text: "new" },
						anchorBlockID: "block-9",
						branchId: BRANCH_ID,
					},
				}),
		},
		{
			name: "updateDocumentCommentByCommentId",
			method: "PUT",
			url: `/api/documents/${NON_XID_ID}/comments/${COMMENT_ID}`,
			invoke: (api: DocumentCommentAPI) =>
				api.updateDocumentCommentByCommentId.mutateAsync({
					docId: NON_XID_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					req: {
						content: { text: "edited" },
						anchorBlockID: "block-9",
						branchId: BRANCH_ID,
					},
				}),
		},
		{
			name: "updateDocumentCommentResolveByCommentId",
			method: "PUT",
			url: `/api/documents/${NON_XID_ID}/comments/${COMMENT_ID}/resolve`,
			invoke: (api: DocumentCommentAPI) =>
				api.updateDocumentCommentResolveByCommentId.mutateAsync({
					docId: NON_XID_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
				}),
		},
		{
			name: "deleteDocumentCommentByCommentId",
			method: "DELETE",
			url: `/api/documents/${NON_XID_ID}/comments/${COMMENT_ID}`,
			invoke: (api: DocumentCommentAPI) =>
				api.deleteDocumentCommentByCommentId.mutateAsync({
					docId: NON_XID_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
				}),
		},
		{
			name: "createDocumentCommentReplyByCommentId",
			method: "POST",
			url: `/api/documents/${NON_XID_ID}/comments/${COMMENT_ID}/replies`,
			invoke: (api: DocumentCommentAPI) =>
				api.createDocumentCommentReplyByCommentId.mutateAsync({
					docId: NON_XID_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					req: { content: { text: "reply" } },
				}),
		},
		{
			name: "updateDocumentCommentReplyByReplyId",
			method: "PUT",
			url: `/api/documents/${NON_XID_ID}/comments/${COMMENT_ID}/replies/${REPLY_ID}`,
			invoke: (api: DocumentCommentAPI) =>
				api.updateDocumentCommentReplyByReplyId.mutateAsync({
					docId: NON_XID_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					replyId: REPLY_ID,
					req: { content: { text: "edited" } },
				}),
		},
		{
			name: "deleteDocumentCommentReplyByReplyId",
			method: "DELETE",
			url: `/api/documents/${NON_XID_ID}/comments/${COMMENT_ID}/replies/${REPLY_ID}`,
			invoke: (api: DocumentCommentAPI) =>
				api.deleteDocumentCommentReplyByReplyId.mutateAsync({
					docId: NON_XID_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					replyId: REPLY_ID,
				}),
		},
	] as {
		name: string
		method: "GET" | "POST" | "PUT" | "DELETE"
		url: string
		invoke: (api: DocumentCommentAPI) => Promise<unknown>
	}[])(
		"$name sends no request and keeps the cache when the document id is not an xid",
		async ({ method, url, invoke }, { expect }) => {
			seedAuth()
			const existing = makeComment(COMMENT_ID)
			seedComments([existing])
			const requestCalls = mockEndpoint(method, url, () => ({}))
			const api = makeCommentAPI()

			const result = await invoke(api)

			expect(result).toBeUndefined()
			expect(requestCalls).toHaveLength(0)
			expect(readComments()).toEqual([existing])
		},
	)

	describe("createDocumentCommentByDocId", () => {
		it.for([
			{
				name: "prepends an optimistic comment and refetches once the create succeeds",
				makeReq: () => ({
					content: { text: "new" },
					anchorBlockID: "block-9",
					branchId: BRANCH_ID,
				}),
				expectedDiffCtx: null,
			},
			{
				name: "keeps the given diff deletion context on the optimistic comment",
				makeReq: () => ({
					content: { text: "new" },
					anchorBlockID: "block-9",
					branchId: BRANCH_ID,
					diffDeletionContext: {
						textAnchors: [{ nodeUid: "uid-1", fromOffset: 1, toOffset: 3 }],
					},
				}),
				expectedDiffCtx: {
					textAnchors: [{ nodeUid: "uid-1", fromOffset: 1, toOffset: 3 }],
				},
			},
		])("$name", async ({ makeReq, expectedDiffCtx }, { expect }) => {
			seedAuth()
			const existing = makeComment(COMMENT_ID)
			seedComments([existing])
			const serverComments = [
				makeComment(COMMENT_ID_2, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			const serverComment = makeComment(COMMENT_ID_2)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const postCalls = mockEndpoint("POST", COMMENTS_URL, () => {
				commentsAtRequest = readComments()

				return serverComment
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))
			const req = makeReq()

			const result = await api.createDocumentCommentByDocId.mutateAsync({
				docId: DOC_ID,
				req,
			})

			expect(result).toEqual(serverComment)
			expect(postCalls).toHaveLength(1)
			expect(postCalls[0]?.body).toEqual(req)
			expect(commentsAtRequest).toEqual([
				expect.objectContaining({
					id: ANY_STRING,
					organizationId: ORG_ID,
					documentId: DOC_ID,
					branchId: BRANCH_ID,
					anchorBlockId: "block-9",
					userId: USER_ID,
					resolved: false,
					resolvedBy: null,
					content: { text: "new" },
					createdAt: ANY_DATE,
					updatedAt: null,
					diffDeletionContext: expectedDiffCtx,
				}),
				existing,
			])
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("sends the create without an optimistic insert when session data is missing", async ({
			expect,
		}) => {
			const existing = makeComment(COMMENT_ID)
			seedComments([existing])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const postCalls = mockEndpoint("POST", COMMENTS_URL, () => {
				commentsAtRequest = readComments()

				return makeComment(COMMENT_ID_2)
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.createDocumentCommentByDocId.mutateAsync({
				docId: DOC_ID,
				req: {
					content: { text: "new" },
					anchorBlockID: "block-9",
					branchId: BRANCH_ID,
				},
			})

			expect(postCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual([existing])
			// onMutate bailed out, so onSuccess has no mutation context and
			// skips the invalidation refetch
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([existing])
		})

		it("rolls back the optimistic comment when the create fails", async ({
			expect,
		}) => {
			seedAuth()
			const existing = makeComment(COMMENT_ID)
			seedComments([existing])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const postCalls = mockEndpoint("POST", COMMENTS_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.createDocumentCommentByDocId.mutateAsync({
					docId: DOC_ID,
					req: {
						content: { text: "new" },
						anchorBlockID: "block-9",
						branchId: BRANCH_ID,
					},
				}),
			).rejects.toThrow()

			expect(postCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([existing])
		})

		it("skips the rollback when the cache changed after the optimistic insert", async ({
			expect,
		}) => {
			let rejectPost: (err: unknown) => void = () => undefined
			let postReached: () => void = () => undefined
			const postReachedSignal = new Promise<void>((resolve) => {
				postReached = resolve
			})

			seedAuth()
			seedComments([makeComment(COMMENT_ID)])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const postCalls = mockEndpoint("POST", COMMENTS_URL, () => {
				postReached()

				return new Promise((_resolve, reject) => {
					rejectPost = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.createDocumentCommentByDocId.mutateAsync({
				docId: DOC_ID,
				req: {
					content: { text: "new" },
					anchorBlockID: "block-9",
					branchId: BRANCH_ID,
				},
			})
			await postReachedSignal

			// the optimistic insert landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()).toHaveLength(2)
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectPost(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(postCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("updateDocumentCommentByCommentId", () => {
		it("updates the matching comment optimistically and refetches once the update succeeds", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID)
			const other = makeComment(COMMENT_ID_2)
			seedComments([target, other])
			const serverComments = [
				makeComment(COMMENT_ID, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const putCalls = mockEndpoint("PUT", COMMENT_URL, () => {
				commentsAtRequest = readComments()

				return {}
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))
			const req = {
				content: { text: "edited" },
				anchorBlockID: "block-9",
				branchId: BRANCH_ID,
			}

			await api.updateDocumentCommentByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				req,
			})

			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual(req)
			expect(commentsAtRequest).toEqual([
				{
					...target,
					content: { text: "edited" },
					anchorBlockId: "block-9",
					updatedAt: ANY_DATE,
				},
				other,
			])
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("rolls back the optimistic update when the update fails", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID)
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const putCalls = mockEndpoint("PUT", COMMENT_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.updateDocumentCommentByCommentId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					req: {
						content: { text: "edited" },
						anchorBlockID: "block-9",
						branchId: BRANCH_ID,
					},
				}),
			).rejects.toThrow()

			expect(putCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			let rejectPut: (err: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			seedAuth()
			seedComments([makeComment(COMMENT_ID)])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const putCalls = mockEndpoint("PUT", COMMENT_URL, () => {
				putReached()

				return new Promise((_resolve, reject) => {
					rejectPut = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.updateDocumentCommentByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				req: {
					content: { text: "edited" },
					anchorBlockID: "block-9",
					branchId: BRANCH_ID,
				},
			})
			await putReachedSignal

			// the optimistic update landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()?.[0]?.content).toEqual({ text: "edited" })
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(putCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("updateDocumentCommentResolveByCommentId", () => {
		it("removes the resolved comment optimistically and refetches once the resolve succeeds", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID)
			const other = makeComment(COMMENT_ID_2)
			seedComments([target, other])
			const serverComments = [makeComment(COMMENT_ID_2)]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const putCalls = mockEndpoint("PUT", RESOLVE_URL, () => {
				commentsAtRequest = readComments()

				return {}
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.updateDocumentCommentResolveByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
			})

			expect(putCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual([other])
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("rolls back the optimistic removal when the resolve fails", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID)
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const putCalls = mockEndpoint("PUT", RESOLVE_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.updateDocumentCommentResolveByCommentId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
				}),
			).rejects.toThrow()

			expect(putCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			let rejectPut: (err: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			seedAuth()
			seedComments([makeComment(COMMENT_ID), makeComment(COMMENT_ID_2)])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const putCalls = mockEndpoint("PUT", RESOLVE_URL, () => {
				putReached()

				return new Promise((_resolve, reject) => {
					rejectPut = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.updateDocumentCommentResolveByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
			})
			await putReachedSignal

			// the optimistic removal landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()).toHaveLength(1)
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(putCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("deleteDocumentCommentByCommentId", () => {
		it.for([
			{
				name: "replaces the deleted comment with its promoted oldest reply",
				makeSeed: () => [
					makeComment(COMMENT_ID, {
						replies: [
							makeReply(REPLY_ID_2, { createdAt: "2024-01-04T00:00:00.000Z" }),
							makeReply(REPLY_ID, {
								userId: USER_ID_2,
								content: { text: "first reply" },
								createdAt: "2024-01-02T00:00:00.000Z",
								updatedAt: "2024-01-03T00:00:00.000Z",
							}),
						],
					}),
					makeComment(COMMENT_ID_2),
				],
				makeExpectedAtRequest: () => [
					{
						...makeComment(COMMENT_ID),
						userId: USER_ID_2,
						content: { text: "first reply" },
						createdAt: "2024-01-02T00:00:00.000Z",
						updatedAt: "2024-01-03T00:00:00.000Z",
						replies: [
							makeReply(REPLY_ID_2, { createdAt: "2024-01-04T00:00:00.000Z" }),
						],
					},
					makeComment(COMMENT_ID_2),
				],
			},
			{
				name: "removes the deleted comment when it has no replies",
				makeSeed: () => [makeComment(COMMENT_ID), makeComment(COMMENT_ID_2)],
				makeExpectedAtRequest: () => [makeComment(COMMENT_ID_2)],
			},
		])("$name", async ({ makeSeed, makeExpectedAtRequest }, { expect }) => {
			seedAuth()
			seedComments(makeSeed())
			const serverComments = [
				makeComment(COMMENT_ID_2, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const deleteCalls = mockEndpoint("DELETE", COMMENT_URL, () => {
				commentsAtRequest = readComments()

				return {}
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.deleteDocumentCommentByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
			})

			expect(deleteCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual(makeExpectedAtRequest())
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("rolls back the optimistic removal when the delete fails", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID)
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const deleteCalls = mockEndpoint("DELETE", COMMENT_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.deleteDocumentCommentByCommentId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
				}),
			).rejects.toThrow()

			expect(deleteCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			let rejectDelete: (err: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})

			seedAuth()
			seedComments([makeComment(COMMENT_ID), makeComment(COMMENT_ID_2)])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const deleteCalls = mockEndpoint("DELETE", COMMENT_URL, () => {
				deleteReached()

				return new Promise((_resolve, reject) => {
					rejectDelete = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.deleteDocumentCommentByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
			})
			await deleteReachedSignal

			// the optimistic removal landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()).toHaveLength(1)
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectDelete(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(deleteCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("createDocumentCommentReplyByCommentId", () => {
		it("prepends an optimistic reply and refetches once the create succeeds", async ({
			expect,
		}) => {
			seedAuth()
			const existingReply = makeReply(REPLY_ID)
			const target = makeComment(COMMENT_ID, { replies: [existingReply] })
			const other = makeComment(COMMENT_ID_2)
			seedComments([target, other])
			const serverComments = [
				makeComment(COMMENT_ID, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			const serverReply = makeReply(REPLY_ID_2)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const postCalls = mockEndpoint("POST", REPLIES_URL, () => {
				commentsAtRequest = readComments()

				return serverReply
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const result =
				await api.createDocumentCommentReplyByCommentId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					req: { content: { text: "reply" } },
				})

			expect(result).toEqual(serverReply)
			expect(postCalls).toHaveLength(1)
			expect(postCalls[0]?.body).toEqual({ content: { text: "reply" } })
			expect(commentsAtRequest).toEqual([
				{
					...target,
					replies: [
						expect.objectContaining({
							id: ANY_STRING,
							organizationId: ORG_ID,
							commentId: COMMENT_ID,
							userId: USER_ID,
							content: { text: "reply" },
							createdAt: ANY_DATE,
							updatedAt: null,
						}),
						existingReply,
					],
				},
				other,
			])
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("sends the create without an optimistic insert when session data is missing", async ({
			expect,
		}) => {
			const target = makeComment(COMMENT_ID, { replies: [] })
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const postCalls = mockEndpoint("POST", REPLIES_URL, () => {
				commentsAtRequest = readComments()

				return makeReply(REPLY_ID)
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.createDocumentCommentReplyByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				req: { content: { text: "reply" } },
			})

			expect(postCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual([target])
			// onMutate bailed out, so onSuccess has no mutation context and
			// skips the invalidation refetch
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the optimistic insert and the invalidation when the comment is not cached", async ({
			expect,
		}) => {
			seedAuth()
			const other = makeComment(COMMENT_ID_2)
			seedComments([other])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const postCalls = mockEndpoint("POST", REPLIES_URL, () => {
				commentsAtRequest = readComments()

				return makeReply(REPLY_ID)
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.createDocumentCommentReplyByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				req: { content: { text: "reply" } },
			})

			expect(postCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual([other])
			// onMutate returns before writing when the target comment is
			// missing, so onSuccess has no mutation context and skips the
			// invalidation refetch
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([other])
		})

		it("rolls back the optimistic reply when the create fails", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID, { replies: [makeReply(REPLY_ID)] })
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const postCalls = mockEndpoint("POST", REPLIES_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.createDocumentCommentReplyByCommentId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					req: { content: { text: "reply" } },
				}),
			).rejects.toThrow()

			expect(postCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the rollback when the cache changed after the optimistic reply", async ({
			expect,
		}) => {
			let rejectPost: (err: unknown) => void = () => undefined
			let postReached: () => void = () => undefined
			const postReachedSignal = new Promise<void>((resolve) => {
				postReached = resolve
			})

			seedAuth()
			seedComments([
				makeComment(COMMENT_ID, { replies: [makeReply(REPLY_ID)] }),
			])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const postCalls = mockEndpoint("POST", REPLIES_URL, () => {
				postReached()

				return new Promise((_resolve, reject) => {
					rejectPost = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.createDocumentCommentReplyByCommentId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				req: { content: { text: "reply" } },
			})
			await postReachedSignal

			// the optimistic reply landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()?.[0]?.replies).toHaveLength(2)
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectPost(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(postCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("updateDocumentCommentReplyByReplyId", () => {
		it("updates the matching reply optimistically and refetches once the update succeeds", async ({
			expect,
		}) => {
			seedAuth()
			const targetReply = makeReply(REPLY_ID)
			const otherReply = makeReply(REPLY_ID_2)
			const target = makeComment(COMMENT_ID, {
				replies: [targetReply, otherReply],
			})
			const other = makeComment(COMMENT_ID_2)
			seedComments([target, other])
			const serverComments = [
				makeComment(COMMENT_ID, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const putCalls = mockEndpoint("PUT", REPLY_URL, () => {
				commentsAtRequest = readComments()

				return {}
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))
			const req = { content: { text: "edited" } }

			await api.updateDocumentCommentReplyByReplyId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				replyId: REPLY_ID,
				req,
			})

			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual(req)
			expect(commentsAtRequest).toEqual([
				{
					...target,
					replies: [
						{
							...targetReply,
							content: { text: "edited" },
							updatedAt: ANY_DATE,
						},
						otherReply,
					],
				},
				other,
			])
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("rolls back the optimistic update when the update fails", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID, { replies: [makeReply(REPLY_ID)] })
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const putCalls = mockEndpoint("PUT", REPLY_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.updateDocumentCommentReplyByReplyId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					replyId: REPLY_ID,
					req: { content: { text: "edited" } },
				}),
			).rejects.toThrow()

			expect(putCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			let rejectPut: (err: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			seedAuth()
			seedComments([
				makeComment(COMMENT_ID, { replies: [makeReply(REPLY_ID)] }),
			])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const putCalls = mockEndpoint("PUT", REPLY_URL, () => {
				putReached()

				return new Promise((_resolve, reject) => {
					rejectPut = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.updateDocumentCommentReplyByReplyId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				replyId: REPLY_ID,
				req: { content: { text: "edited" } },
			})
			await putReachedSignal

			// the optimistic update landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()?.[0]?.replies?.[0]?.content).toEqual({
				text: "edited",
			})
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(putCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("deleteDocumentCommentReplyByReplyId", () => {
		it("removes the matching reply optimistically and refetches once the delete succeeds", async ({
			expect,
		}) => {
			seedAuth()
			const otherReply = makeReply(REPLY_ID_2)
			const target = makeComment(COMMENT_ID, {
				replies: [makeReply(REPLY_ID), otherReply],
			})
			const other = makeComment(COMMENT_ID_2)
			seedComments([target, other])
			const serverComments = [
				makeComment(COMMENT_ID, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const deleteCalls = mockEndpoint("DELETE", REPLY_URL, () => {
				commentsAtRequest = readComments()

				return {}
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.deleteDocumentCommentReplyByReplyId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				replyId: REPLY_ID,
			})

			expect(deleteCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual([
				{ ...target, replies: [otherReply] },
				other,
			])
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("leaves the replies unchanged when the reply is not cached", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID, { replies: [makeReply(REPLY_ID)] })
			seedComments([target])
			const serverComments = [
				makeComment(COMMENT_ID, { content: { text: "server" } }),
			]
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => serverComments)
			let commentsAtRequest: DocumentCommentsResponse | undefined
			const deleteCalls = mockEndpoint(
				"DELETE",
				`${REPLIES_URL}/${REPLY_ID_2}`,
				() => {
					commentsAtRequest = readComments()

					return {}
				},
			)
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await api.deleteDocumentCommentReplyByReplyId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				replyId: REPLY_ID_2,
			})

			expect(deleteCalls).toHaveLength(1)
			expect(commentsAtRequest).toEqual([target])
			// the target comment is cached, so onMutate still returns a
			// context and onSuccess refetches even though nothing changed
			expect(getCalls).toHaveLength(1)
			expect(readComments()).toEqual(serverComments)
		})

		it("rolls back the optimistic removal when the delete fails", async ({
			expect,
		}) => {
			seedAuth()
			const target = makeComment(COMMENT_ID, { replies: [makeReply(REPLY_ID)] })
			seedComments([target])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const deleteCalls = mockEndpoint("DELETE", REPLY_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			await expect(
				api.deleteDocumentCommentReplyByReplyId.mutateAsync({
					docId: DOC_ID,
					branchId: BRANCH_ID,
					commentId: COMMENT_ID,
					replyId: REPLY_ID,
				}),
			).rejects.toThrow()

			expect(deleteCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([target])
		})

		it("skips the rollback when the cache changed after the optimistic removal", async ({
			expect,
		}) => {
			let rejectDelete: (err: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})

			seedAuth()
			seedComments([
				makeComment(COMMENT_ID, {
					replies: [makeReply(REPLY_ID), makeReply(REPLY_ID_2)],
				}),
			])
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const deleteCalls = mockEndpoint("DELETE", REPLY_URL, () => {
				deleteReached()

				return new Promise((_resolve, reject) => {
					rejectDelete = reject
				})
			})
			const api = makeCommentAPI()
			runInApp(() => api.useFetchDocumentCommentsByDocId(DOC_ID, BRANCH_ID))

			const pending = api.deleteDocumentCommentReplyByReplyId.mutateAsync({
				docId: DOC_ID,
				branchId: BRANCH_ID,
				commentId: COMMENT_ID,
				replyId: REPLY_ID,
			})
			await deleteReachedSignal

			// the optimistic removal landed; divergent data written afterwards
			// must survive the failure
			expect(readComments()?.[0]?.replies).toHaveLength(1)
			const divergent = makeComment(COMMENT_ID_2, {
				content: { text: "divergent" },
			})
			seedComments([divergent])
			rejectDelete(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(deleteCalls).toHaveLength(1)
			expect(getCalls).toHaveLength(0)
			expect(readComments()).toEqual([divergent])
		})
	})

	describe("patchOptimisticCommentEntry", () => {
		it("does nothing when no comments are cached", ({ expect }) => {
			seedAuth()
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const api = makeCommentAPI()

			api.patchOptimisticCommentEntry(
				DOC_ID,
				BRANCH_ID,
				makeComment(COMMENT_ID),
			)

			expect(readComments()).toBeUndefined()
			expect(getCalls).toHaveLength(0)
		})

		it.for([
			{
				name: "leaves the cached comments untouched when no optimistic entry exists",
				makeSeed: () => [makeComment(COMMENT_ID), makeComment(COMMENT_ID_2)],
				makeExpected: (server: DocumentComment) => {
					void server

					return [makeComment(COMMENT_ID), makeComment(COMMENT_ID_2)]
				},
			},
			{
				name: "replaces the optimistic entry with the server comment",
				makeSeed: () => [makeComment(NON_XID_ID), makeComment(COMMENT_ID_2)],
				makeExpected: (server: DocumentComment) => [
					server,
					makeComment(COMMENT_ID_2),
				],
			},
		])("$name", ({ makeSeed, makeExpected }, { expect }) => {
			seedAuth()
			seedComments(makeSeed())
			const getCalls = mockEndpoint("GET", COMMENTS_URL, () => [])
			const api = makeCommentAPI()
			const serverComment = makeComment(COMMENT_ID, {
				content: { text: "server" },
			})

			api.patchOptimisticCommentEntry(DOC_ID, BRANCH_ID, serverComment)

			expect(readComments()).toEqual(makeExpected(serverComment))
			expect(getCalls).toHaveLength(0)
		})
	})
})

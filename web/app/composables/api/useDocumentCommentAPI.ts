import { nanoid } from "nanoid"
import isDeepEqual from "fast-deep-equal"

const DOCUMENT_COMMENT_QUERY_KEYS = {
	documentComments: (docId: string, branchId: string) =>
		["documents", docId, "comments", branchId] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()
	const { fetchAuthSession, fetchOrganization } = useAuthSession()

	function useFetchDocumentCommentsByDocId(
		docIdRef: MaybeRefOrGetter<string | null | undefined>,
		branchIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: () =>
				DOCUMENT_COMMENT_QUERY_KEYS.documentComments(
					toValue(docIdRef) || "",
					toValue(branchIdRef) || "",
				),
			query: async () => {
				const docId = toValue(docIdRef)
				const branchId = toValue(branchIdRef)

				if (!docId || !branchId) {
					return []
				}

				const res = await $coreAPIClient<DocumentCommentsResponse>(
					`/api/documents/${docId}/comments?branchId=${encodeURIComponent(branchId)}`,
					{ method: "GET" },
				)

				return res
			},
			enabled: () => !!toValue(docIdRef) && !!toValue(branchIdRef),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 6 * 60 * 1000, // 6 mins
			autoRefetch: true,
		})
	}

	const createDocumentCommentByDocId = useMutation({
		onMutate: ({
			docId,
			req,
		}: {
			docId: string
			req: DocumentCommentCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.branchId)) {
				// optimisticInserts use nanoid
				return
			}

			const userId = fetchAuthSession.data.value?.data?.user.id
			const organizationId = fetchOrganization.data.value?.data?.id

			if (!userId || !organizationId) {
				// do not pre-insert if we don't have user or organization info
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(
				docId,
				req.branchId,
			)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComment: DocumentComment = {
				id: nanoid(),
				organizationId: organizationId,
				documentId: docId,
				branchId: req.branchId,
				anchorBlockId: req.anchorBlockID,
				userId: userId,
				resolved: false,
				resolvedBy: null,
				content: req.content,
				createdAt: new Date(),
				updatedAt: null,
				diffDeletionContext: req.diffDeletionContext ?? null,
			}
			const newComments = [newComment, ...(oldComments ?? [])]

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			req,
		}: {
			docId: string
			req: DocumentCommentCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.branchId)) {
				// optimisticInserts use nanoid
				return
			}

			return await $coreAPIClient<DocumentCommentCreateResponse>(
				`/api/documents/${docId}/comments`,
				{
					method: "POST",
					body: req,
				},
			)
		},
		async onSuccess(_data, { docId, req }) {
			if (!isXid(docId) || !isXid(req.branchId)) {
				// optimisticInserts use nanoid
				return
			}

			// the key is rebuilt from the variables rather than read off the
			// context: onMutate skips the optimistic insert (and its context)
			// without a user or organization, but the request still went out
			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, req.branchId),
			})
		},
		onError(_err, { docId, req }, ctx) {
			if (!isXid(docId) || !isXid(req.branchId) || !ctx.key) {
				return
			}

			const cachedComments = queryCache.getQueryData<DocumentCommentsResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	const updateDocumentCommentByCommentId = useMutation({
		onMutate: ({
			docId,
			branchId,
			commentId,
			req,
		}: {
			docId: string
			branchId: string
			commentId: string
			req: DocumentCommentUpdateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComments = clone(oldComments) ?? []

			for (const h of newComments) {
				if (h.id === commentId) {
					h.content = req.content
					h.anchorBlockId = req.anchorBlockID
					h.updatedAt = new Date()

					break
				}
			}

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			commentId,
			req,
		}: {
			docId: string
			branchId: string
			commentId: string
			req: DocumentCommentUpdateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(`/api/documents/${docId}/comments/${commentId}`, {
				method: "PUT",
				body: req,
			})
		},
		async onSuccess(_data, { docId, branchId, commentId }) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, commentId }, ctx) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId) || !ctx.key) {
				return
			}

			const cachedComments = queryCache.getQueryData(ctx.key)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	const updateDocumentCommentResolveByCommentId = useMutation({
		onMutate: ({
			docId,
			branchId,
			commentId,
		}: {
			docId: string
			branchId: string
			commentId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComments = clone(oldComments) ?? []

			const index = newComments.findIndex((h) => h.id === commentId)
			if (index !== -1) {
				newComments.splice(index, 1)
			}

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			branchId,
			commentId,
		}: {
			docId: string
			branchId: string
			commentId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(
				`/api/documents/${docId}/comments/${commentId}/resolve`,
				{
					method: "PUT",
				},
			)
		},
		async onSuccess(_data, { docId, branchId, commentId }) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, commentId }, ctx) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId) || !ctx.key) {
				return
			}

			const cachedComments = queryCache.getQueryData<DocumentCommentsResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	const deleteDocumentCommentByCommentId = useMutation({
		onMutate: ({
			docId,
			branchId,
			commentId,
		}: {
			docId: string
			branchId: string
			commentId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComments = clone(oldComments) ?? []

			const index = newComments.findIndex((h) => h.id === commentId)
			const target = newComments[index]
			if (target) {
				const promoted = promoteFirstDocumentReplyToComment(target)
				if (promoted) {
					newComments[index] = promoted
				} else {
					newComments.splice(index, 1)
				}
			}

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			branchId,
			commentId,
		}: {
			docId: string
			branchId: string
			commentId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(`/api/documents/${docId}/comments/${commentId}`, {
				method: "DELETE",
			})
		},
		async onSuccess(_data, { docId, branchId, commentId }) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, commentId }, ctx) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId) || !ctx.key) {
				return
			}

			const cachedComments = queryCache.getQueryData<DocumentCommentsResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	const createDocumentCommentReplyByCommentId = useMutation({
		onMutate: ({
			docId,
			branchId,
			commentId,
			req,
		}: {
			docId: string
			branchId: string
			commentId: string
			req: DocumentCommentReplyCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			const userId = fetchAuthSession.data.value?.data?.user.id
			const organizationId = fetchOrganization.data.value?.data?.id

			if (!userId || !organizationId) {
				// do not pre-insert if we don't have user or organization info
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComments = clone(oldComments)
			const newReply: DocumentCommentReply = {
				id: nanoid(),
				organizationId: organizationId,
				commentId: commentId,
				userId: userId,
				content: req.content,
				createdAt: new Date(),
				updatedAt: null,
			}

			const modComment = newComments?.find((c) => c.id === commentId)
			if (!modComment) {
				return
			}

			modComment.replies = [newReply, ...(modComment.replies ?? [])]

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			branchId,
			commentId,
			req,
		}: {
			docId: string
			branchId: string
			commentId: string
			req: DocumentCommentReplyCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			return await $coreAPIClient<DocumentCommentReplyCreateResponse>(
				`/api/documents/${docId}/comments/${commentId}/replies`,
				{
					method: "POST",
					body: req,
				},
			)
		},
		async onSuccess(_data, { docId, branchId, commentId }) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId)) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, commentId }, ctx) {
			if (!isXid(docId) || !isXid(branchId) || !isXid(commentId) || !ctx.key) {
				return
			}

			const cachedComments = queryCache.getQueryData<DocumentCommentsResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	const updateDocumentCommentReplyByReplyId = useMutation({
		onMutate: ({
			docId,
			branchId,
			commentId,
			replyId,
			req,
		}: {
			docId: string
			branchId: string
			commentId: string
			replyId: string
			req: DocumentCommentReplyUpdateRequest
		}) => {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId)
			) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComments = clone(oldComments)
			const modComment = newComments?.find((c) => c.id === commentId)
			if (!modComment) {
				return
			}

			for (const h of modComment.replies ?? []) {
				if (h.id === replyId) {
					h.content = req.content
					h.updatedAt = new Date()

					break
				}
			}

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			branchId,
			commentId,
			replyId,
			req,
		}: {
			docId: string
			branchId: string
			commentId: string
			replyId: string
			req: DocumentCommentReplyUpdateRequest
		}) => {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId)
			) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(
				`/api/documents/${docId}/comments/${commentId}/replies/${replyId}`,
				{
					method: "PUT",
					body: req,
				},
			)
		},
		async onSuccess(_data, { docId, branchId, commentId, replyId }) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId)
			) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, commentId, replyId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId) ||
				!ctx.key
			) {
				return
			}

			const cachedComments = queryCache.getQueryData(ctx.key)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	const deleteDocumentCommentReplyByReplyId = useMutation({
		onMutate: ({
			docId,
			branchId,
			commentId,
			replyId,
		}: {
			docId: string
			branchId: string
			commentId: string
			replyId: string
		}) => {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId)
			) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
			const oldComments = clone(
				queryCache.getQueryData<DocumentCommentsResponse>(key),
			)
			const newComments = clone(oldComments)
			const modComment = newComments?.find((c) => c.id === commentId)

			if (!modComment) {
				return
			}

			const index = modComment.replies?.findIndex((h) => h.id === replyId)
			if (index !== undefined && index !== -1) {
				modComment.replies?.splice(index, 1)
			}

			queryCache.setQueryData(key, newComments)
			queryCache.cancelQueries({ key })

			return { newComments, oldComments, key }
		},
		mutation: async ({
			docId,
			branchId,
			commentId,
			replyId,
		}: {
			docId: string
			branchId: string
			commentId: string
			replyId: string
		}) => {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId)
			) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(
				`/api/documents/${docId}/comments/${commentId}/replies/${replyId}`,
				{
					method: "DELETE",
				},
			)
		},
		async onSuccess(_data, { docId, branchId, commentId, replyId }) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId)
			) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, commentId, replyId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(commentId) ||
				!isXid(replyId) ||
				!ctx.key
			) {
				return
			}

			const cachedComments = queryCache.getQueryData<DocumentCommentsResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newComments, cachedComments)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldComments)
		},
	})

	// replace the optimistic cache entry (which has a temporary nanoid
	// ID) with the real server response so that computed properties
	// referencing the real ID find the data immediately — without
	// waiting for the background refetch triggered by onSuccess.
	function patchOptimisticCommentEntry(
		docId: string,
		branchId: string,
		serverComment: DocumentComment,
	) {
		const key = DOCUMENT_COMMENT_QUERY_KEYS.documentComments(docId, branchId)
		const cached = queryCache.getQueryData<DocumentCommentsResponse>(key)

		if (!cached) {
			return
		}

		// the optimistic entry is the one whose ID is not a valid XID
		// (it was generated with nanoid in onMutate)
		const idx = cached.findIndex((c) => !isXid(c.id))
		if (idx === -1) {
			return
		}

		const updated = [...cached]
		updated[idx] = serverComment
		queryCache.setQueryData(key, updated)
	}

	return {
		useFetchDocumentCommentsByDocId,
		createDocumentCommentByDocId,
		updateDocumentCommentByCommentId,
		updateDocumentCommentResolveByCommentId,
		deleteDocumentCommentByCommentId,
		createDocumentCommentReplyByCommentId,
		updateDocumentCommentReplyByReplyId,
		deleteDocumentCommentReplyByReplyId,
		patchOptimisticCommentEntry,
	}
}

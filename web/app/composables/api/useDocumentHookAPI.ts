import { nanoid } from "nanoid"
import isDeepEqual from "fast-deep-equal"

const DOCUMENT_HOOK_QUERY_KEYS = {
	list: (docId: string, branchId: string) =>
		["documents", "hooks", docId, branchId] as const,
}

export default function () {
	const { $apiClient } = useNuxtApp()
	const queryCache = useQueryCache()
	const { fetchOrganization } = useAuthSession()

	function useFetchDocumentHooksByDocID(
		docIdRef: MaybeRefOrGetter<string | null | undefined>,
		branchIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: () =>
				DOCUMENT_HOOK_QUERY_KEYS.list(
					toValue(docIdRef) || "",
					toValue(branchIdRef) || "",
				),
			query: async () => {
				const docId = toValue(docIdRef)
				const branchId = toValue(branchIdRef)
				if (!docId || !branchId) {
					return []
				}

				const res = await $apiClient<DocumentHooksResponse>(
					`/api/documents/${docId}/hooks?branchId=${encodeURIComponent(branchId)}`,
					{ method: "GET" },
				)

				return res
			},
			enabled: () => !!toValue(docIdRef) && !!toValue(branchIdRef),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 3 * 60 * 1000, // 3 mins
			autoRefetch: true,
		})
	}

	const createDocumentHookByDocID = useMutation({
		onMutate: ({
			docId,
			req,
		}: {
			docId: string
			req: DocumentHookCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.branchId)) {
				// optimisticInserts use nanoid
				return
			}

			const organizationId = fetchOrganization.data.value?.data?.id
			if (!organizationId) {
				return
			}

			const key = DOCUMENT_HOOK_QUERY_KEYS.list(docId, req.branchId)
			const oldHooks = clone(
				queryCache.getQueryData<DocumentHooksResponse>(key),
			)
			const newHook: DocumentHook = {
				id: nanoid(),
				type: req.type,
				documentId: docId,
				branchId: req.branchId,
				organizationId: organizationId,
				blockId: req.blockId,
				settings: req.settings,
				state: defaultDocumentHookState(req.type),
				score: "100",
				createdAt: new Date(),
				updatedAt: new Date(),
			}
			const newHooks = [newHook, ...(oldHooks || [])]

			queryCache.setQueryData(key, newHooks)
			queryCache.cancelQueries({ key })

			return { newHooks, oldHooks, key }
		},
		mutation: async ({
			docId,
			req,
		}: {
			docId: string
			req: DocumentHookCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.branchId)) {
				// optimisticInserts use nanoid
				return
			}

			return await $apiClient<DocumentHookCreateResponse>(
				`/api/documents/${docId}/hooks`,
				{
					method: "POST",
					body: req,
				},
			)
		},
		async onSuccess(_data, { docId, req }, ctx) {
			if (!isXid(docId) || !isXid(req.branchId) || !ctx || !ctx.key) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: ctx.key,
			})
		},
		onError(_err, { docId, req }, ctx) {
			if (!isXid(docId) || !isXid(req.branchId) || !ctx || !ctx.key) {
				return
			}

			const cachedHooks = queryCache.getQueryData<DocumentHooksResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newHooks, cachedHooks)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldHooks)
		},
	})

	const updateDocumentHookByDocID = useMutation({
		onMutate: ({
			docId,
			branchId,
			hookId,
			req,
		}: {
			docId: string
			branchId: string
			hookId: string
			req: DocumentHookUpdateRequest
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(hookId)) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_HOOK_QUERY_KEYS.list(docId, branchId)
			const oldHooks = clone(
				queryCache.getQueryData<DocumentHooksResponse>(key),
			)
			const newHooks = clone(oldHooks) || []

			for (const h of newHooks) {
				if (h.id === hookId) {
					h.settings = req.settings
					h.updatedAt = new Date()

					break
				}
			}

			queryCache.setQueryData(key, newHooks)
			queryCache.cancelQueries({ key })

			return { newHooks, oldHooks, key }
		},
		mutation: async ({
			docId,
			branchId,
			hookId,
			req,
		}: {
			docId: string
			branchId: string
			hookId: string
			req: DocumentHookUpdateRequest
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(hookId)) {
				// optimisticInserts use nanoid
				return
			}

			return await $apiClient<DocumentHookUpdateResponse>(
				`/api/documents/${docId}/hooks/${hookId}`,
				{
					method: "PUT",
					body: req,
				},
			)
		},
		async onSuccess(_data, { docId, branchId, hookId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(hookId) ||
				!ctx ||
				!ctx.key
			) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: ctx.key,
			})
		},
		onError(_err, { docId, branchId, hookId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(hookId) ||
				!ctx ||
				!ctx.key
			) {
				return
			}

			const cachedHooks = queryCache.getQueryData(ctx.key)
			if (!isDeepEqual(ctx.newHooks, cachedHooks)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldHooks)
		},
	})

	const deleteDocumentHookByDocID = useMutation({
		onMutate: ({
			docId,
			branchId,
			hookId,
		}: {
			docId: string
			branchId: string
			hookId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(hookId)) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_HOOK_QUERY_KEYS.list(docId, branchId)
			const oldHooks = clone(
				queryCache.getQueryData<DocumentHooksResponse>(key),
			)
			const newHooks = clone(oldHooks) || []

			const index = newHooks.findIndex((h) => h.id === hookId)
			if (index !== -1) {
				newHooks.splice(index, 1)
			}

			queryCache.setQueryData(key, newHooks)
			queryCache.cancelQueries({ key })

			return { newHooks, oldHooks, key }
		},
		mutation: async ({
			docId,
			branchId,
			hookId,
		}: {
			docId: string
			branchId: string
			hookId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(hookId)) {
				// optimisticInserts use nanoid
				return
			}

			await $apiClient(`/api/documents/${docId}/hooks/${hookId}`, {
				method: "DELETE",
			})
		},
		async onSuccess(_data, { docId, branchId, hookId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(hookId) ||
				!ctx ||
				!ctx.key
			) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: ctx.key,
			})
		},
		onError(_err, { docId, branchId, hookId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(hookId) ||
				!ctx ||
				!ctx.key
			) {
				return
			}

			const cachedHooks = queryCache.getQueryData(ctx.key)
			if (!isDeepEqual(ctx.newHooks, cachedHooks)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldHooks)
		},
	})

	const resetDocumentHookByDocID = useMutation({
		onMutate: ({
			docId,
			branchId,
			hookId,
		}: {
			docId: string
			branchId: string
			hookId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(hookId)) {
				// optimisticInserts use nanoid
				return
			}

			const key = DOCUMENT_HOOK_QUERY_KEYS.list(docId, branchId)
			const oldHooks = clone(
				queryCache.getQueryData<DocumentHooksResponse>(key),
			)
			const newHooks = clone(oldHooks) || []

			for (const h of newHooks) {
				if (h.id === hookId) {
					h.state = defaultDocumentHookState(h.type)
					h.updatedAt = new Date()

					break
				}
			}

			queryCache.setQueryData(key, newHooks)
			queryCache.cancelQueries({ key })

			return { newHooks, oldHooks, key }
		},
		mutation: async ({
			docId,
			branchId,
			hookId,
		}: {
			docId: string
			branchId: string
			hookId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || !isXid(hookId)) {
				// optimisticInserts use nanoid
				return
			}

			return await $apiClient<DocumentHookResponse>(
				`/api/documents/${docId}/hooks/${hookId}/reset`,
				{
					method: "PUT",
				},
			)
		},
		async onSuccess(_data, { docId, branchId, hookId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(hookId) ||
				!ctx ||
				!ctx.key
			) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: ctx.key,
			})
		},
		onError(_err, { docId, branchId, hookId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				!isXid(hookId) ||
				!ctx ||
				!ctx.key
			) {
				return
			}

			const cachedHooks = queryCache.getQueryData<DocumentHooksResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newHooks, cachedHooks)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldHooks)
		},
	})

	return {
		useFetchDocumentHooksByDocID,
		createDocumentHookByDocID,
		updateDocumentHookByDocID,
		deleteDocumentHookByDocID,
		resetDocumentHookByDocID,
	}
}

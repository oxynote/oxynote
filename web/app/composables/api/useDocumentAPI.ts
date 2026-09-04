import { nanoid } from "nanoid"
import type {
	DocumentBranch,
	DocumentBranchCreateRequest,
	DocumentBranchCreateResponse,
	DocumentBranchesResponse,
	BranchReviewersResponse,
	DocumentCreateRequest,
	DocumentCreateResponse,
	DocumentMaintainersResponse,
	DocumentSearchResponse,
	DocumentTreeElement,
	DocumentTreeResponse,
	DocumentTreeUpdateRequest,
	MetricSimulationCheckResponse,
} from "~/utils"
import isDeepEqual from "fast-deep-equal"

const DOCUMENT_QUERY_KEYS = {
	root: ["documents", "tree"] as const,
	branches: (docId: string) => ["documents", docId, "branches"] as const,
	documentMaintainers: (docId: string) =>
		["documents", docId, "maintainers"] as const,
	branchReviewers: (docId: string, branchId: string) =>
		["documents", docId, "branches", branchId, "reviewers"] as const,
}

export default function () {
	const { $coreAPIClient, $authRealtimeAPIClient } = useNuxtApp()
	const { fetchAuthSession, fetchOrganization } = useAuthSession()
	const queryCache = useQueryCache()

	const fetchDocumentTree = useQuery({
		key: DOCUMENT_QUERY_KEYS.root,
		query: async () => {
			return await $coreAPIClient<DocumentTreeResponse>(`/api/documents/tree`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	const updateDocumentTree = useMutation<
		any,
		UnprocessedDocumentTreeUpdateRequest,
		Error,
		{
			newTree: DocumentTreeResponse
			oldTree: DocumentTreeResponse | undefined
			finalReq: DocumentTreeUpdateRequest
		}
	>({
		onMutate(req) {
			if (!isXid(req.id)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<DocumentTreeResponse>(DOCUMENT_QUERY_KEYS.root),
			)

			const newTree = clone(oldTree) ?? []

			// the assertions keep the declared union as the flow type: walkTree
			// assigns these from a closure, which TS's flow analysis ignores,
			// so without them every check below looks dead to TS.
			let targetItem = null as DocumentTreeElement | null
			let targetItemOldIndex = null as number | null
			let newParent = null as DocumentTreeElement | "root" | null
			let oldParent = null as DocumentTreeElement | "root" | null

			const walkTree = (
				elems: DocumentTreeElement[],
				parent: DocumentTreeElement | "root",
			): boolean => {
				for (const [i, elem] of elems.entries()) {
					if (!targetItem) {
						if (elem.id === req.id) {
							targetItem = elem
							targetItemOldIndex = i
							oldParent = parent
						}
					}

					if (!newParent) {
						if (elem.id === req.parentId) {
							newParent = elem
						}
					}

					if (
						targetItem &&
						targetItemOldIndex !== null &&
						newParent &&
						oldParent
					) {
						return true
					}

					if (elem.children) {
						if (walkTree(elem.children, elem)) {
							return true
						}
					}
				}

				return false
			}

			if (req.parentId === null) {
				newParent = "root"
			}

			walkTree(newTree, "root")

			if (
				!targetItem ||
				targetItemOldIndex === null ||
				!newParent ||
				!oldParent
			) {
				throw new Error("invalid document tree update data")
			}

			if (oldParent === "root") {
				newTree.splice(targetItemOldIndex, 1)
			} else {
				const oldP = oldParent
				oldP.children?.splice(targetItemOldIndex, 1)

				if (!oldP.children?.length) {
					delete oldP.children
				}
			}

			let targetItemNewIndex = 0

			if (newParent === "root") {
				if (req.insertBeforeId) {
					for (const [i, elem] of newTree.entries()) {
						if (elem.id === req.insertBeforeId) {
							// this item will be pushed down and the new item
							// will be inserted in its place
							targetItemNewIndex = i
							break
						}
					}
				}

				newTree.splice(targetItemNewIndex, 0, targetItem)
			} else {
				const newP = newParent

				if (req.insertBeforeId) {
					for (const [i, child] of (newP.children ?? []).entries()) {
						if (child.id === req.insertBeforeId) {
							// this item will be pushed down and the new item
							// will be inserted in its place
							targetItemNewIndex = i
							break
						}
					}
				}

				newP.children ??= []

				newP.children.splice(targetItemNewIndex, 0, targetItem)
			}

			const finalReq: DocumentTreeUpdateRequest = {
				id: req.id,
				parentId: req.parentId,
				sortIndex: targetItemNewIndex,
			}

			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: DOCUMENT_QUERY_KEYS.root })

			return { newTree, oldTree, finalReq }
		},
		mutation: async (req, ctx) => {
			if (!isXid(req.id)) {
				// optimisticInserts use nanoid — onMutate bails out without a
				// context for them, so the guard runs before touching it
				return
			}

			await $coreAPIClient(`/api/documents/tree`, {
				method: "PUT",
				body: ctx.finalReq,
			})
		},
		async onSuccess(_data, req) {
			if (!isXid(req.id)) {
				return
			}

			await queryCache.invalidateQueries({ key: DOCUMENT_QUERY_KEYS.root })
		},
		onError(_err, _title, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(DOCUMENT_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, oldTree)
		},
	})

	const createDocument = useMutation({
		onMutate(req) {
			if (
				req.skipLocalOptimisticInsert ||
				(req.parentId && !isXid(req.parentId))
			) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<DocumentTreeResponse>(DOCUMENT_QUERY_KEYS.root),
			)
			const newTreeElem: DocumentTreeElement = {
				id: nanoid(),
				documentName: req.name,
				icon: req.icon,
				children: null,
				protected: false,
				localOptimisticInsert: true,
			}

			const newTree = clone(oldTree) ?? []
			if (!newTree.length) {
				newTree.push(newTreeElem)
			} else {
				if (!req.parentId) {
					newTree.unshift(newTreeElem)
				} else {
					const walkTree = (elems: DocumentTreeElement[]): boolean => {
						for (const elem of elems) {
							if (elem.id === req.parentId) {
								elem.children ??= []

								elem.children.unshift(newTreeElem)

								return true
							}

							if (elem.children) {
								if (walkTree(elem.children)) {
									return true
								}
							}
						}

						return false
					}

					walkTree(newTree)
				}
			}

			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: DOCUMENT_QUERY_KEYS.root })

			return { newTree, oldTree, newTreeElem }
		},
		mutation: async (req: DocumentCreateRequest) => {
			if (req.parentId && !isXid(req.parentId)) {
				// optimisticInserts use nanoid
				return
			}

			const cleanReq = clone(req)
			delete cleanReq.skipLocalOptimisticInsert

			return await $coreAPIClient<DocumentCreateResponse>(`/api/documents`, {
				method: "POST",
				body: cleanReq,
			})
		},
		async onSuccess(_data, req) {
			if (req.parentId && !isXid(req.parentId)) {
				return
			}

			await queryCache.invalidateQueries({ key: DOCUMENT_QUERY_KEYS.root })
		},
		onError(_err, _data, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(DOCUMENT_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, oldTree)
		},
	})

	const deleteDocument = useMutation({
		onMutate(id) {
			if (!isXid(id)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<DocumentTreeResponse>(DOCUMENT_QUERY_KEYS.root),
			)

			const newTree = clone(oldTree) ?? []
			const walkTree = (elems: DocumentTreeElement[]): boolean => {
				for (const [i, elem] of elems.entries()) {
					if (elem.id === id) {
						elems.splice(i, 1)

						return true
					}

					if (elem.children) {
						if (walkTree(elem.children)) {
							if (!elem.children.length) {
								delete elem.children
							}

							return true
						}
					}
				}

				return false
			}

			walkTree(newTree)

			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: DOCUMENT_QUERY_KEYS.root })

			return { newTree, oldTree }
		},
		mutation: async (id: string) => {
			if (!isXid(id)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(`/api/documents/${id}`, {
				method: "DELETE",
			})
		},
		async onSuccess(_data, id) {
			if (!isXid(id)) {
				return
			}

			await queryCache.invalidateQueries({ key: DOCUMENT_QUERY_KEYS.root })
		},
		onError(_err, _title, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(DOCUMENT_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, oldTree)
		},
	})

	function updateDocumentTreeElementCache(
		id: string,
		data: {
			name: string
			icon: string
			protected: boolean
		},
	) {
		const tree = clone(
			queryCache.getQueryData<DocumentTreeResponse>(DOCUMENT_QUERY_KEYS.root),
		)
		if (!tree) {
			return
		}

		const walkTree = (elems: DocumentTreeElement[]): boolean => {
			for (const elem of elems) {
				if (elem.id === id) {
					elem.documentName = data.name
					elem.icon = data.icon
					elem.protected = data.protected

					return true
				}

				if (elem.children) {
					if (walkTree(elem.children)) {
						return true
					}
				}
			}

			return false
		}

		walkTree(tree)
		queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, tree)
	}

	const duplicateDocument = useMutation({
		onMutate(id: string) {
			if (!isXid(id)) {
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<DocumentTreeResponse>(DOCUMENT_QUERY_KEYS.root),
			)

			// Find the source document to get its name, icon, and parent
			let name = "New Document"
			let icon = "mingcute:document-2-fill"
			// the assertion keeps the declared union as the flow type: findSource
			// assigns parentId from a closure, which TS's flow analysis ignores.
			let parentId = null as string | null

			const findSource = (
				elems: DocumentTreeElement[],
				parent: string | null,
			): boolean => {
				for (const elem of elems) {
					if (elem.id === id) {
						name = elem.documentName
						icon = elem.icon
						parentId = parent
						return true
					}

					if (elem.children) {
						if (findSource(elem.children, elem.id)) {
							return true
						}
					}
				}

				return false
			}

			const newTree = clone(oldTree) ?? []
			findSource(newTree, null)

			const now = new Date()
			const month = now.toLocaleString("en-US", { month: "short" })
			const timestamp = `${now.getFullYear()} ${month}. ${String(now.getDate()).padStart(2, "0")} ${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`

			const newTreeElem: DocumentTreeElement = {
				id: nanoid(),
				documentName: `${name} (${timestamp})`,
				icon: icon,
				children: null,
				protected: false,
				localOptimisticInsert: true,
			}

			// Insert as sibling (same parent as source)
			if (!parentId) {
				newTree.unshift(newTreeElem)
			} else {
				const insertAsSibling = (elems: DocumentTreeElement[]): boolean => {
					for (const elem of elems) {
						if (elem.id === parentId) {
							elem.children ??= []

							elem.children.unshift(newTreeElem)

							return true
						}

						if (elem.children) {
							if (insertAsSibling(elem.children)) {
								return true
							}
						}
					}

					return false
				}

				insertAsSibling(newTree)
			}

			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: DOCUMENT_QUERY_KEYS.root })

			return { newTree, oldTree, newTreeElem }
		},
		mutation: async (id: string) => {
			if (!isXid(id)) {
				return
			}

			return await $coreAPIClient<DocumentCreateResponse>(
				`/api/documents/${id}/duplicate`,
				{
					method: "POST",
				},
			)
		},
		async onSuccess(_data, id) {
			if (!isXid(id)) {
				return
			}

			await queryCache.invalidateQueries({ key: DOCUMENT_QUERY_KEYS.root })
		},
		onError(_err, _id, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(DOCUMENT_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, oldTree)
		},
	})

	async function searchDocuments(q: string): Promise<DocumentSearchResponse> {
		// we don't want to use useQuery here as searches are
		// typically one-off and we don't want to cache them
		return await $coreAPIClient<DocumentSearchResponse>(
			`/api/documents/search?q=${encodeURIComponent(q)}`,
			{
				method: "GET",
			},
		)
	}

	// runs a block, which core resolves from the stored branch and
	// dispatches on its kind. For a metric block that means probing whether
	// the data it simulates has arrived; core owns the answer because it
	// also removes the simulation from the block when it has, and it caches
	// the probe so several viewers do not each hit the data source.
	async function checkMetricBlockSimulation(
		documentId: string,
		branchId: string,
		blockUid: string,
	): Promise<MetricSimulationCheckResponse> {
		return await $coreAPIClient<MetricSimulationCheckResponse>(
			`/api/documents/${documentId}/branches/${branchId}/blocks/${encodeURIComponent(blockUid)}/run`,
			{
				method: "POST",
			},
		)
	}

	const updateDocumentBranch = useMutation({
		onMutate({
			id,
			branchId,
			protectedMode,
		}: {
			id: string
			branchId: string
			protectedMode: boolean
		}) {
			if (!isXid(id) || !isXid(branchId)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<DocumentTreeResponse>(DOCUMENT_QUERY_KEYS.root),
			)
			const newTree = clone(oldTree) ?? []

			const walkTree = (elems: DocumentTreeElement[]): boolean => {
				for (const elem of elems) {
					if (elem.id === id) {
						elem.protected = protectedMode
						return true
					}

					if (elem.children) {
						if (walkTree(elem.children)) {
							return true
						}
					}
				}

				return false
			}

			walkTree(newTree)

			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: DOCUMENT_QUERY_KEYS.root })

			return { newTree, oldTree }
		},
		mutation: async ({
			id,
			branchId,
			protectedMode,
		}: {
			id: string
			branchId: string
			protectedMode: boolean
		}) => {
			if (!isXid(id) || !isXid(branchId)) {
				// optimisticInserts use nanoid
				return
			}

			// auth-realtime stores what the editor still holds before core
			// changes the branch, and resets its connections afterwards.
			await $authRealtimeAPIClient(
				`/api/documents/${id}/branches/${branchId}`,
				{
					method: "PUT",
					body: {
						protected: protectedMode,
					},
				},
			)
		},
		async onSuccess(_data, { id, branchId }) {
			if (!isXid(id) || !isXid(branchId)) {
				return
			}

			await queryCache.invalidateQueries({ key: DOCUMENT_QUERY_KEYS.root })
			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branches(id),
			})
		},
		onError(_err, _title, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(DOCUMENT_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(DOCUMENT_QUERY_KEYS.root, oldTree)
		},
	})

	const createDocumentBranch = useMutation({
		onMutate({
			docId,
			req,
		}: {
			docId: string
			req: DocumentBranchCreateRequest
		}) {
			if (!isXid(docId) || !isXid(req.sourceBranchId)) {
				return
			}

			const key = DOCUMENT_QUERY_KEYS.branches(docId)
			const oldBranches = clone(
				queryCache.getQueryData<DocumentBranchesResponse>(key),
			)

			// Copy metadata from source branch for the optimistic insert
			const sourceBranch = oldBranches?.find(
				(b) => b.branchId === req.sourceBranchId,
			)
			const newBranch: DocumentBranch = {
				branchId: nanoid(),
				branchName: req.branch,
				documentName: sourceBranch?.documentName || "",
				icon: sourceBranch?.icon || "",
				protected: false,
				default: false,
				createdAt: new Date(),
				updatedAt: new Date(),
			}

			const newBranches = [...(oldBranches ?? []), newBranch]

			queryCache.setQueryData(key, newBranches)
			queryCache.cancelQueries({ key })

			return { newBranches, oldBranches, key }
		},
		mutation: async ({
			docId,
			req,
		}: {
			docId: string
			req: DocumentBranchCreateRequest
		}) => {
			if (!isXid(docId) || !isXid(req.sourceBranchId)) {
				return
			}

			// auth-realtime stores what the editor still holds before core
			// copies the source branch.
			return await $authRealtimeAPIClient<DocumentBranchCreateResponse>(
				`/api/documents/${docId}/branches`,
				{
					method: "POST",
					body: req,
				},
			)
		},
		async onSuccess(_data, { docId, req }) {
			if (!isXid(docId) || !isXid(req.sourceBranchId)) {
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branches(docId),
			})
		},
		onError(_err, { docId, req }, ctx) {
			if (!isXid(docId) || !isXid(req.sourceBranchId) || !ctx.key) {
				return
			}

			const cachedBranches = queryCache.getQueryData<DocumentBranchesResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newBranches, cachedBranches)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldBranches)
		},
	})

	const deleteDocumentBranch = useMutation({
		onMutate({ docId, branchId }: { docId: string; branchId: string }) {
			if (!isXid(docId) || !isXid(branchId)) {
				return
			}

			const key = DOCUMENT_QUERY_KEYS.branches(docId)
			const oldBranches = clone(
				queryCache.getQueryData<DocumentBranchesResponse>(key),
			)
			const newBranches = clone(oldBranches) ?? []

			const index = newBranches.findIndex((b) => b.branchId === branchId)
			if (index !== -1) {
				newBranches.splice(index, 1)
			}

			queryCache.setQueryData(key, newBranches)
			queryCache.cancelQueries({ key })

			return { newBranches, oldBranches, key }
		},
		mutation: async ({
			docId,
			branchId,
		}: {
			docId: string
			branchId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId)) {
				return
			}

			await $coreAPIClient(`/api/documents/${docId}/branches/${branchId}`, {
				method: "DELETE",
			})
		},
		async onSuccess(_data, { docId, branchId }) {
			if (!isXid(docId) || !isXid(branchId)) {
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branches(docId),
			})
		},
		onError(_err, { docId, branchId }, ctx) {
			if (!isXid(docId) || !isXid(branchId) || !ctx.key) {
				return
			}

			const cachedBranches = queryCache.getQueryData<DocumentBranchesResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newBranches, cachedBranches)) {
				return
			}

			// rollback
			queryCache.setQueryData(ctx.key, ctx.oldBranches)
		},
	})

	const mergeDocumentBranches = useMutation({
		// merging is too complex for an optimistic update
		mutation: async ({
			docId,
			fromBranchId,
			toBranchId,
		}: {
			docId: string
			fromBranchId: string
			toBranchId: string
		}) => {
			if (!isXid(docId) || !isXid(fromBranchId) || !isXid(toBranchId)) {
				return
			}

			await $authRealtimeAPIClient(`/api/documents/${docId}/merge`, {
				method: "PUT",
				body: { fromBranchId, toBranchId },
			})
		},
		async onSuccess(_data, { docId, fromBranchId, toBranchId }) {
			if (!isXid(docId) || !isXid(fromBranchId) || !isXid(toBranchId)) {
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branchReviewers(docId, fromBranchId),
			})
			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branchReviewers(docId, toBranchId),
			})
		},
	})

	function useFetchDocumentMaintainersByDocId(
		docIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: DOCUMENT_QUERY_KEYS.documentMaintainers(toValue(docIdRef) || ""),
			query: async () => {
				const docId = toValue(docIdRef)
				if (!docId) {
					return []
				}

				return await $coreAPIClient<DocumentMaintainersResponse>(
					`/api/documents/${docId}/maintainers`,
					{
						method: "GET",
					},
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 6 * 60 * 1000, // 6 mins
			autoRefetch: true,
		})
	}

	function useFetchDocumentBranchesByDocId(
		docIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: () => DOCUMENT_QUERY_KEYS.branches(toValue(docIdRef) || ""),
			query: async () => {
				const docId = toValue(docIdRef)
				if (!docId) {
					return []
				}

				return await $coreAPIClient<DocumentBranchesResponse>(
					`/api/documents/${docId}/branches`,
					{
						method: "GET",
					},
				)
			},
			enabled: () =>
				toValue(docIdRef) !== null && toValue(docIdRef) !== undefined,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 6 * 60 * 1000, // 6 mins
			autoRefetch: true,
		})
	}

	function useFetchBranchReviewers(
		docIdRef: MaybeRefOrGetter<string | null | undefined>,
		branchIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: () =>
				DOCUMENT_QUERY_KEYS.branchReviewers(
					toValue(docIdRef) || "",
					toValue(branchIdRef) || "",
				),
			query: async () => {
				const docId = toValue(docIdRef)
				const branchId = toValue(branchIdRef)
				if (!docId || !branchId) {
					return []
				}

				// the core handler responds with the db slice unwrapped, and
				// a branch without reviewers yields a nil slice — JSON null
				return (
					(await $coreAPIClient<BranchReviewersResponse | null>(
						`/api/documents/${docId}/branches/${branchId}/reviewers`,
						{
							method: "GET",
						},
					)) ?? []
				)
			},
			enabled: () => !!toValue(docIdRef) && !!toValue(branchIdRef),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 6 * 60 * 1000, // 6 mins
			autoRefetch: true,
		})
	}

	const updateBranchApproval = useMutation({
		onMutate: ({
			docId,
			branchId,
			approved,
		}: {
			docId: string
			branchId: string
			approved: boolean
		}) => {
			if (!isXid(docId) || !isXid(branchId)) {
				return
			}

			const userId = fetchAuthSession.data.value?.data?.user.id
			if (!userId) {
				return
			}

			const organizationId = fetchOrganization.data.value?.data?.id
			if (!organizationId) {
				return
			}

			const key = DOCUMENT_QUERY_KEYS.branchReviewers(docId, branchId)
			const oldReviewers = clone(
				queryCache.getQueryData<BranchReviewersResponse>(key),
			)
			const newReviewers = clone(oldReviewers) ?? []

			const found = newReviewers.find((r) => r.userId === userId)
			if (found) {
				found.currentlyApproved = approved
			} else {
				newReviewers.push({
					branchId,
					userId,
					organizationId: organizationId,
					currentlyApproved: approved,
					previouslyApproved: false,
				})
			}

			queryCache.setQueryData(key, newReviewers)
			queryCache.cancelQueries({ key })

			return { newReviewers, oldReviewers, key }
		},
		mutation: async ({
			docId,
			branchId,
			approved,
		}: {
			docId: string
			branchId: string
			approved: boolean
		}) => {
			if (!isXid(docId) || !isXid(branchId)) {
				return
			}

			await $coreAPIClient(
				`/api/documents/${docId}/branches/${branchId}/review-approve`,
				{
					method: "PUT",
					body: { approved },
				},
			)
		},
		async onSuccess(_data, { docId, branchId }) {
			if (!isXid(docId) || !isXid(branchId)) {
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branchReviewers(docId, branchId),
			})
		},
		onError(_err, { docId, branchId }, ctx) {
			if (!isXid(docId) || !isXid(branchId) || !ctx.key) {
				return
			}

			const cachedReviewers = queryCache.getQueryData<BranchReviewersResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newReviewers, cachedReviewers)) {
				return
			}

			queryCache.setQueryData(ctx.key, ctx.oldReviewers)
		},
	})

	const inviteBranchReviewer = useMutation({
		onMutate: ({
			docId,
			branchId,
			userId,
		}: {
			docId: string
			branchId: string
			userId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || isOptimisticInsertId(userId)) {
				return
			}

			const organizationId = fetchOrganization.data.value?.data?.id
			if (!organizationId) {
				return
			}

			const key = DOCUMENT_QUERY_KEYS.branchReviewers(docId, branchId)
			const oldReviewers = clone(
				queryCache.getQueryData<BranchReviewersResponse>(key),
			)
			const newReviewers = clone(oldReviewers) ?? []
			const existing = newReviewers.find((r) => r.userId === userId)

			if (existing) {
				existing.previouslyApproved = existing.currentlyApproved
				existing.currentlyApproved = false
			} else {
				newReviewers.push({
					branchId,
					userId,
					organizationId: organizationId,
					currentlyApproved: false,
					previouslyApproved: false,
				})
			}

			queryCache.setQueryData(key, newReviewers)
			queryCache.cancelQueries({ key })

			return { newReviewers, oldReviewers, key }
		},
		mutation: async ({
			docId,
			branchId,
			userId,
		}: {
			docId: string
			branchId: string
			userId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || isOptimisticInsertId(userId)) {
				return
			}

			await $coreAPIClient(
				`/api/documents/${docId}/branches/${branchId}/reviewers`,
				{
					method: "POST",
					body: { userId },
				},
			)
		},
		async onSuccess(_data, { docId, branchId, userId }) {
			if (!isXid(docId) || !isXid(branchId) || isOptimisticInsertId(userId)) {
				return
			}

			// the key is rebuilt from the variables rather than read off the
			// context: onMutate skips the optimistic insert (and its context)
			// without an organization, but the request still went out
			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branchReviewers(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, userId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				isOptimisticInsertId(userId) ||
				!ctx.key
			) {
				return
			}

			const cachedReviewers = queryCache.getQueryData<BranchReviewersResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newReviewers, cachedReviewers)) {
				return
			}

			queryCache.setQueryData(ctx.key, ctx.oldReviewers)
		},
	})

	const removeBranchReviewer = useMutation({
		onMutate: ({
			docId,
			branchId,
			userId,
		}: {
			docId: string
			branchId: string
			userId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || isOptimisticInsertId(userId)) {
				return
			}

			const key = DOCUMENT_QUERY_KEYS.branchReviewers(docId, branchId)
			const oldReviewers = clone(
				queryCache.getQueryData<BranchReviewersResponse>(key),
			)
			const newReviewers = clone(oldReviewers) ?? []

			const index = newReviewers.findIndex((h) => h.userId === userId)
			if (index !== -1) {
				newReviewers.splice(index, 1)
			}

			queryCache.setQueryData(key, newReviewers)
			queryCache.cancelQueries({ key })

			return { newReviewers, oldReviewers, key }
		},
		mutation: async ({
			docId,
			branchId,
			userId,
		}: {
			docId: string
			branchId: string
			userId: string
		}) => {
			if (!isXid(docId) || !isXid(branchId) || isOptimisticInsertId(userId)) {
				return
			}

			await $coreAPIClient(
				`/api/documents/${docId}/branches/${branchId}/reviewers?userId=${encodeURIComponent(userId)}`,
				{
					method: "DELETE",
				},
			)
		},
		async onSuccess(_data, { docId, branchId, userId }) {
			if (!isXid(docId) || !isXid(branchId) || isOptimisticInsertId(userId)) {
				return
			}

			await queryCache.invalidateQueries({
				key: DOCUMENT_QUERY_KEYS.branchReviewers(docId, branchId),
			})
		},
		onError(_err, { docId, branchId, userId }, ctx) {
			if (
				!isXid(docId) ||
				!isXid(branchId) ||
				isOptimisticInsertId(userId) ||
				!ctx.key
			) {
				return
			}

			const cachedReviewers = queryCache.getQueryData<BranchReviewersResponse>(
				ctx.key,
			)
			if (!isDeepEqual(ctx.newReviewers, cachedReviewers)) {
				return
			}

			queryCache.setQueryData(ctx.key, ctx.oldReviewers)
		},
	})

	return {
		fetchDocumentTree,
		updateDocumentTree,
		createDocument,
		duplicateDocument,
		searchDocuments,
		deleteDocument,
		updateDocumentTreeElementCache,
		useFetchDocumentMaintainersByDocId,
		useFetchBranchReviewers,
		inviteBranchReviewer,
		removeBranchReviewer,
		updateBranchApproval,
		useFetchDocumentBranchesByDocId,
		createDocumentBranch,
		deleteDocumentBranch,
		updateDocumentBranch,
		mergeDocumentBranches,
		checkMetricBlockSimulation,
	}
}

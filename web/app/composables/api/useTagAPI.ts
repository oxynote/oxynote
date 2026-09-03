import { nanoid } from "nanoid"
import type {
	DocumentTreeElement,
	DocumentTreeResponse,
	TagCreateRequest,
	TagCreateResponse,
	TagTreeElement,
	TagTreeResponse,
	TagTreeUpdateRequest,
	TagVisibilityRequest,
	UnprocessedTagTreeUpdateRequest,
	DocumentTagRequest,
} from "~/utils"
import isDeepEqual from "fast-deep-equal"
import { DOCUMENT_QUERY_KEYS } from "./useDocumentAPI"

const TAG_QUERY_KEYS = {
	root: ["tags", "tree"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()

	const fetchTagTree = useQuery({
		key: TAG_QUERY_KEYS.root,
		query: async () => {
			return await $coreAPIClient<TagTreeResponse>(`/api/tags/tree`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	const updateTagTree = useMutation<
		any,
		UnprocessedTagTreeUpdateRequest,
		Error,
		{
			newTree: TagTreeResponse
			oldTree: TagTreeResponse | undefined
			finalReq: TagTreeUpdateRequest
		}
	>({
		onMutate(req) {
			if (!isXid(req.id)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<TagTreeResponse>(TAG_QUERY_KEYS.root),
			)
			const newTree = clone(oldTree) ?? []

			const oldIndex = newTree.findIndex((tag) => tag.id === req.id)
			if (oldIndex === -1) {
				throw new Error("invalid tag tree update data")
			}

			const [movedTag] = newTree.splice(oldIndex, 1)
			if (!movedTag) {
				throw new Error("invalid tag tree update data")
			}

			// a null insertBeforeId means the very top; an unknown one would
			// otherwise resolve to -1 and splice the tag in from the end
			let newIndex = 0

			if (req.insertBeforeId) {
				const insertBefore = newTree.findIndex(
					(tag) => tag.id === req.insertBeforeId,
				)
				if (insertBefore === -1) {
					throw new Error("invalid tag tree update data")
				}

				newIndex = insertBefore
			}

			newTree.splice(newIndex, 0, movedTag)

			const finalReq: TagTreeUpdateRequest = {
				id: req.id,
				sortIndex: newIndex,
			}

			queryCache.setQueryData(TAG_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: TAG_QUERY_KEYS.root })

			return { newTree, oldTree, finalReq }
		},
		mutation: async (req, ctx) => {
			if (!isXid(req.id)) {
				// optimisticInserts use nanoid — onMutate bails out without a
				// context for them, so the guard runs before touching it
				return
			}

			await $coreAPIClient(`/api/tags/tree`, {
				method: "PUT",
				body: ctx.finalReq,
			})
		},
		async onSuccess(_data, req) {
			if (!isXid(req.id)) {
				return
			}

			await queryCache.invalidateQueries({ key: TAG_QUERY_KEYS.root })
		},
		onError(_err, _req, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(TAG_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(TAG_QUERY_KEYS.root, oldTree)
		},
	})

	const createTag = useMutation({
		onMutate(req: TagCreateRequest) {
			const oldTree = clone(
				queryCache.getQueryData<TagTreeResponse>(TAG_QUERY_KEYS.root),
			)
			const newTreeElem: TagTreeElement = {
				id: nanoid(),
				tagName: req.tagName,
				color: req.color,
				hidden: false,
				documents: null,
				localOptimisticInsert: true,
			}

			// a new tag lands at the end, which is where core appends it
			const newTree = [...(clone(oldTree) ?? []), newTreeElem]

			queryCache.setQueryData(TAG_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: TAG_QUERY_KEYS.root })

			return { newTree, oldTree, newTreeElem }
		},
		mutation: async (req: TagCreateRequest) => {
			return await $coreAPIClient<TagCreateResponse>(`/api/tags`, {
				method: "POST",
				body: req,
			})
		},
		async onSuccess() {
			await queryCache.invalidateQueries({ key: TAG_QUERY_KEYS.root })
		},
		onError(_err, _req, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(TAG_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(TAG_QUERY_KEYS.root, oldTree)
		},
	})

	const updateTagVisibility = useMutation({
		onMutate({ id, req }: { id: string; req: TagVisibilityRequest }) {
			if (!isXid(id)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<TagTreeResponse>(TAG_QUERY_KEYS.root),
			)
			const newTree = clone(oldTree) ?? []

			const tag = newTree.find((t) => t.id === id)
			if (!tag) {
				throw new Error("invalid tag visibility data")
			}

			tag.hidden = req.hidden

			queryCache.setQueryData(TAG_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: TAG_QUERY_KEYS.root })

			return { newTree, oldTree }
		},
		mutation: async ({
			id,
			req,
		}: {
			id: string
			req: TagVisibilityRequest
		}) => {
			if (!isXid(id)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(`/api/tags/${id}/visibility`, {
				method: "PUT",
				body: req,
			})
		},
		async onSuccess(_data, { id }) {
			if (!isXid(id)) {
				return
			}

			await queryCache.invalidateQueries({ key: TAG_QUERY_KEYS.root })
		},
		onError(_err, _req, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(TAG_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(TAG_QUERY_KEYS.root, oldTree)
		},
	})

	const deleteTag = useMutation({
		onMutate(id: string) {
			if (!isXid(id)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<TagTreeResponse>(TAG_QUERY_KEYS.root),
			)
			const newTree = (clone(oldTree) ?? []).filter((tag) => tag.id !== id)

			queryCache.setQueryData(TAG_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: TAG_QUERY_KEYS.root })

			return { newTree, oldTree }
		},
		mutation: async (id: string) => {
			if (!isXid(id)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(`/api/tags/${id}`, {
				method: "DELETE",
			})
		},
		async onSuccess(_data, id) {
			if (!isXid(id)) {
				return
			}

			await queryCache.invalidateQueries({ key: TAG_QUERY_KEYS.root })
		},
		onError(_err, _id, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(TAG_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(TAG_QUERY_KEYS.root, oldTree)
		},
	})

	const assignDocumentTag = useMutation({
		onMutate(req: DocumentTagRequest) {
			if (!isXid(req.documentId) || !isXid(req.tagId)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<TagTreeResponse>(TAG_QUERY_KEYS.root),
			)
			const newTree = clone(oldTree) ?? []

			const tag = newTree.find((t) => t.id === req.tagId)
			if (!tag) {
				throw new Error("invalid document tag data")
			}

			// the tag tree carries whole document subtrees, so the row is
			// copied out of the document tree rather than invented here. A
			// caller the document tree has not reached yet — the editor, on a
			// document opened before the sidebar loaded — still assigns; it
			// just waits for the refetch to draw the row.
			const doc = findDocument(
				queryCache.getQueryData<DocumentTreeResponse>(
					DOCUMENT_QUERY_KEYS.root,
				) ?? [],
				req.documentId,
			)

			if (doc) {
				tag.documents = [...(tag.documents ?? []), clone(doc)]
			}

			queryCache.setQueryData(TAG_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: TAG_QUERY_KEYS.root })

			return { newTree, oldTree }
		},
		mutation: async (req: DocumentTagRequest) => {
			if (!isXid(req.documentId) || !isXid(req.tagId)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(`/api/documents/${req.documentId}/tags`, {
				method: "POST",
				body: { tagId: req.tagId },
			})
		},
		async onSuccess(_data, req) {
			if (!isXid(req.documentId) || !isXid(req.tagId)) {
				return
			}

			await queryCache.invalidateQueries({ key: TAG_QUERY_KEYS.root })
		},
		onError(_err, _req, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(TAG_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(TAG_QUERY_KEYS.root, oldTree)
		},
	})

	const unassignDocumentTag = useMutation({
		onMutate(req: DocumentTagRequest) {
			if (!isXid(req.documentId) || !isXid(req.tagId)) {
				// optimisticInserts use nanoid
				return
			}

			const oldTree = clone(
				queryCache.getQueryData<TagTreeResponse>(TAG_QUERY_KEYS.root),
			)
			const newTree = clone(oldTree) ?? []

			const tag = newTree.find((t) => t.id === req.tagId)
			if (tag?.documents) {
				tag.documents = tag.documents.filter((doc) => doc.id !== req.documentId)
			}

			queryCache.setQueryData(TAG_QUERY_KEYS.root, newTree)
			queryCache.cancelQueries({ key: TAG_QUERY_KEYS.root })

			return { newTree, oldTree }
		},
		mutation: async (req: DocumentTagRequest) => {
			if (!isXid(req.documentId) || !isXid(req.tagId)) {
				// optimisticInserts use nanoid
				return
			}

			await $coreAPIClient(
				`/api/documents/${req.documentId}/tags/${req.tagId}`,
				{
					method: "DELETE",
				},
			)
		},
		async onSuccess(_data, req) {
			if (!isXid(req.documentId) || !isXid(req.tagId)) {
				return
			}

			await queryCache.invalidateQueries({ key: TAG_QUERY_KEYS.root })
		},
		onError(_err, _req, { oldTree, newTree }) {
			const cachedTree = queryCache.getQueryData(TAG_QUERY_KEYS.root)
			if (!isDeepEqual(newTree, cachedTree)) {
				return
			}

			// rollback
			queryCache.setQueryData(TAG_QUERY_KEYS.root, oldTree)
		},
	})

	return {
		fetchTagTree,
		updateTagTree,
		createTag,
		updateTagVisibility,
		deleteTag,
		assignDocumentTag,
		unassignDocumentTag,
	}
}

// findDocument walks a document tree depth-first and returns the element
// carrying the given id.
function findDocument(
	elems: DocumentTreeElement[],
	id: string,
): DocumentTreeElement | null {
	for (const elem of elems) {
		if (elem.id === id) {
			return elem
		}

		if (elem.children) {
			const found = findDocument(elem.children, id)
			if (found) {
				return found
			}
		}
	}

	return null
}

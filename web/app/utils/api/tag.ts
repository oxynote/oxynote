export const WS_TAG_TREE_CHANGE_TOPIC = "change@tag-tree"

export function makeWsBranchTagsChangeTopic(docId: string): string {
	return `change@documents.${docId}.tags`
}

// what the branch tags change topic carries: the branch whose tags changed,
// so a header showing another branch of the document leaves its list alone
export interface WSBranchTagsChangePayload {
	branchId: string
}

export interface Tag {
	id: string
	organizationId: string
	tagName: string
	color: string
	sortIndex: number
	createdAt: Date | string
	createdBy?: string | null
}

export interface TagTreeElement {
	id: string
	tagName: string
	color: string
	// whether the signed-in user keeps this tag out of their own sidebar;
	// every member of the organization has their own answer
	hidden: boolean
	documents?: DocumentTreeElement[] | null

	// local-only
	localOptimisticInsert?: boolean
}

export type TagTreeResponse = TagTreeElement[]

export interface TagCreateRequest {
	tagName: string
	color: string
}

export type TagCreateResponse = Tag

export interface UnprocessedTagTreeUpdateRequest {
	id: string
	insertBeforeId: string | null
}

export interface TagTreeUpdateRequest {
	id: string
	sortIndex: number
}

export interface TagVisibilityRequest {
	hidden: boolean
}

export interface BranchTagRequest {
	documentId: string
	branchId: string
	tagId: string
}

// the ids of the tags a branch carries, in the tags' display order
export type BranchTagsResponse = string[]

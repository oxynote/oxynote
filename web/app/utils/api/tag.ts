export const WS_TAG_TREE_CHANGE_TOPIC = "change@tag-tree"

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

export interface DocumentTagRequest {
	documentId: string
	tagId: string
}

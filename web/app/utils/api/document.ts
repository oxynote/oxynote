export const WS_DOCUMENT_TREE_CHANGE_TOPIC = "change@document-tree"

export function makeWsDocumentMetadataChangeTopic(docId: string): string {
	return `change@documents.${docId}.metadata`
}

export function makeWsDocumentMaintainersChangeTopic(docId: string): string {
	return `change@documents.${docId}.maintainers`
}

export function makeWsDocumentReviewersChangeTopic(docId: string): string {
	return `change@documents.${docId}.reviewers`
}

export interface DocumentTreeElement {
	id: string
	documentName: string
	icon: string
	protected: boolean
	children?: DocumentTreeElement[] | null

	// local-only
	localOptimisticInsert?: boolean
}

export interface Document {
	id: string
	branchId: string
	branchName: string
	documentName: string
	parentId: string | null
	organizationId: string
	icon: string
	content: any | null
	rawContent: any | null
	protected: boolean
	default: boolean
	createdAt: Date | string
	createdBy?: string | null
	updatedAt?: Date | string | null
	lastUpdatedBy?: string | null
}

export interface DocumentHook {
	id: string
	type: DocumentHookType
	documentId: string
	organizationId: string
	branchId: string
	blockId: string | null
	settings: DocumentHookSettings
	state: DocumentHookState
	score: string // decimal; between 0 and 100 (default: 100)
	createdAt: Date | string
	updatedAt?: Date | string | null
	softDeletedAt?: Date | string | null
}

export enum DocumentFileLocation {
	Document = "document",
	Comment = "comment",
}

export enum DocumentHookType {
	ScheduledReminder = "scheduled-reminder",
	GitHubTracking = "github-tracking",
	URLWatcher = "url-watcher",
	ContainerImageWatcher = "container-image-watcher",
}

export type DocumentHookSettings =
	| DocumentHookSettingsScheduledReminder
	| DocumentHookSettingsGitHubTracking
	| DocumentHookSettingsURLWatcher
	| DocumentHookSettingsContainerImageWatcher
export type DocumentHookState =
	| DocumentHookStateScheduledReminder
	| DocumentHookStateGitHubTracking
	| DocumentHookStateURLWatcher
	| DocumentHookStateContainerImageWatcher

export interface DocumentHookSettingsScheduledReminder {
	scale: "linear"
	duration: string | null // duration
	schedule: Date | string
}

export interface DocumentHookStateScheduledReminder {
	lastActiveAt: Date | string
}

export interface DocumentHookSettingsGitHubTracking {
	repository: string
	branch: string
	paths: string[]
}

export interface DocumentHookStateGitHubTracking {
	pathsChecksums: Record<string, string>
	status:
		| "active"
		| "missing_installation"
		| "missing_repository"
		| "missing_branch"
}

export interface DocumentHookSettingsURLWatcher {
	url: string
}

export interface DocumentHookSettingsContainerImageWatcher {
	image: string
}

export interface DocumentHookStateURLWatcher {
	lastCheckedAt?: Date | string
	status: "active" | "unreachable_url"
}

export interface DocumentHookStateContainerImageWatcher {
	digest: string
	status: "active" | "unauthorized"
}

export type DocumentTreeResponse = DocumentTreeElement[]

export interface UnprocessedDocumentTreeUpdateRequest {
	id: string
	parentId: string | null
	insertBeforeId: string | null
}

export interface DocumentTreeUpdateRequest {
	id: string
	parentId: string | null
	sortIndex: number
}

export interface DocumentCreateRequest {
	name: string
	icon: string
	parentId: string | null

	// local-only
	skipLocalOptimisticInsert?: boolean
}

export type DocumentCreateResponse = Document
export type DocumentHookResponse = DocumentHook
export type DocumentHooksResponse = DocumentHookResponse[]

export interface DocumentHookCreateRequest {
	type: DocumentHookType
	branchId: string
	blockId: string | null
	settings: DocumentHookSettings
}

export type DocumentHookCreateResponse = DocumentHook

export interface DocumentHookUpdateRequest {
	settings: DocumentHookSettings
}

export type DocumentHookUpdateResponse = DocumentHook

export function isIdInDocumentTree(
	tree: DocumentTreeElement[],
	id: string,
): boolean {
	for (const elem of tree) {
		if (elem.id === id) {
			return true
		}

		if (elem.children && isIdInDocumentTree(elem.children, id)) {
			return true
		}
	}

	return false
}

export function docNameByIdInDocumentTree(
	tree: DocumentTreeElement[],
	id: string,
): string | null {
	for (const elem of tree) {
		if (elem.id === id) {
			return elem.documentName
		}

		if (elem.children) {
			const childRes = docNameByIdInDocumentTree(elem.children, id)
			if (childRes !== null) {
				return childRes
			}
		}
	}

	return null
}

export function defaultDocumentHookState(
	type: DocumentHookType,
): DocumentHookState {
	switch (type) {
		case DocumentHookType.ScheduledReminder:
			return {
				lastActiveAt: new Date(),
			}
		case DocumentHookType.GitHubTracking:
			return {
				pathsChecksums: {},
				status: "active",
			}
		case DocumentHookType.URLWatcher:
			return {
				lastCheckedAt: new Date(),
				status: "active",
			}
		case DocumentHookType.ContainerImageWatcher:
			return {
				status: "active",
				digest: "",
			}
	}
}

export interface DocumentSearchResult {
	id: string // block/node ID
	documentId: string
	organizationId: string
	type: "document" | "heading" | string
	text: string
}

export type DocumentSearchResponse = DocumentSearchResult[]
export type DocumentMaintainersResponse = string[]

export interface DocumentBranch {
	branchId: string
	branchName: string
	documentName: string
	icon: string
	protected: boolean
	default: boolean
	createdAt: Date | string
	updatedAt: Date | string
}

export type DocumentBranchesResponse = DocumentBranch[]

export interface DocumentBranchCreateRequest {
	branch: string
	sourceBranchId: string
}

export type DocumentBranchCreateResponse = Document

export interface BranchReviewer {
	branchId: string
	userId: string
	organizationId: string
	currentlyApproved: boolean
	previouslyApproved: boolean
}

export type BranchReviewersResponse = BranchReviewer[]

export interface WSDocumentMetadataPayload {
	branchId: string
	documentName: string
	protected: boolean
	createdAt: Date | string
	createdBy?: string | null
	updatedAt?: Date | string | null
	lastUpdatedBy?: string | null
}

export interface DocumentTimestampUser {
	id: string
	name: string
}

export interface DocumentModeTimestamp {
	at: Date
	user: DocumentTimestampUser | null
}

// the keys are branch IDs
export type DocumentTimestamps = Record<
	string,
	{
		created: DocumentModeTimestamp
		updated: DocumentModeTimestamp
	}
>

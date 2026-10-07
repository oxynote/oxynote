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

export function makeWsHooksChangeTopic(docId: string): string {
	return `change@documents.${docId}.hooks`
}

// what the hooks change topic carries: the branch whose hooks changed, so an
// editor showing another branch of the document leaves its list alone
export interface WSHooksChangePayload {
	branchId: string
}

export interface DocumentTreeElement {
	id: string
	documentName: string
	icon: string
	protected: boolean
	// the branch a reader lands on without naming one. A row the tree
	// edits insert optimistically has none until the server answers
	defaultBranchId?: string
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
	content: any
	rawContent: any
	protected: boolean
	default: boolean
	createdAt: Date | string
	createdBy?: string | null
	updatedAt?: Date | string | null
	lastUpdatedBy?: string | null
}

export interface DocumentHook {
	id: string
	// the id a hook shares with its copies on the document's other branches.
	// A new hook's crossBranchId is its own id.
	crossBranchId: string
	type: DocumentHookType
	documentId: string
	organizationId: string
	branchId: string
	blockId: string | null
	settings: DocumentHookSettings
	// null until the hook is set up
	state: DocumentHookState | null
	// active, initializing, or why the last check failed. Score and state
	// keep their last values while a check fails.
	status: DocumentHookStatus
	score: string // decimal; between 0 and 100 (default: 100)
	createdAt: Date | string
	updatedAt?: Date | string | null
	softDeletedAt?: Date | string | null
}

export enum DocumentFileLocation {
	Document = "document",
	Comment = "comment",
}

// the block an upload belongs to, which decides the size and type rules
// the server admits it under
export enum DocumentFileKind {
	Image = "image",
	File = "file",
}

export interface DocumentFileUpload {
	id: string
	// the file name the upload carried, as the server recorded it
	name: string
	size: number
	// the media type the server detected from the bytes
	contentType: string
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
export type DocumentHookSummary = DocumentHookSummaryGitHubTracking
export type DocumentHookStatus =
	| DocumentHookStatusScheduledReminder
	| DocumentHookStatusGitHubTracking
	| DocumentHookStatusURLWatcher
	| DocumentHookStatusContainerImageWatcher

// initializing marks a copied hook before its first run.
export type DocumentHookStatusScheduledReminder = "active" | "initializing"
export type DocumentHookStatusGitHubTracking =
	| "active"
	| "initializing"
	| "unconfigured"
	| "missing_installation"
	| "missing_repository"
	| "missing_branch"
	| "tree_truncated"
export type DocumentHookStatusURLWatcher =
	"active" | "initializing" | "unconfigured" | "unreachable_url"
export type DocumentHookStatusContainerImageWatcher =
	"active" | "initializing" | "unauthorized" | "image_not_found"

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
}

export interface DocumentHookSummaryGitHubTracking {
	changedPaths: number
}

export interface DocumentHookSettingsURLWatcher {
	url: string
}

export interface DocumentHookSettingsContainerImageWatcher {
	image: string
}

export interface DocumentHookStateURLWatcher {
	lastCheckedAt?: Date | string
}

export interface DocumentHookStateContainerImageWatcher {
	digest: string
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
			}
		case DocumentHookType.URLWatcher:
			return {
				lastCheckedAt: new Date(),
			}
		case DocumentHookType.ContainerImageWatcher:
			return {
				digest: "",
			}
	}
}

// the search endpoints refuse a query outside these lengths
export const DOCUMENT_SEARCH_QUERY_MIN_LENGTH = 2
export const DOCUMENT_SEARCH_QUERY_MAX_LENGTH = 200

export interface DocumentSearchParams {
	q: string
	currentDocId: string | null
}

// which one a hit carries follows from its block type
export type DocumentSearchHitAttrs =
	DocumentSearchHitAttrsCodeBlock | DocumentSearchHitAttrsMetricBlock

export interface DocumentSearchHitAttrsCodeBlock {
	language: string
}

export interface DocumentSearchHitAttrsMetricBlock {
	dataSourceId?: string
	visualizationType?: GenericQueryChartType
}

export interface DocumentSearchHit {
	id: string // block uid
	type: string
	text: string // escaped HTML with the matches wrapped in <mark>
	attrs?: DocumentSearchHitAttrs
}

export interface DocumentSearchDocument {
	id: string
	title: string
	titleHtml: string | null // set only when the name matched
	icon: string
	branch: {
		id: string
		name: string
		default: boolean
	}
	updatedAt: Date | string
	updatedBy: string | null
}

// one branch of a document, so a document with a draft can come back twice etc.
export interface DocumentSearchResult {
	document: DocumentSearchDocument
	hits: DocumentSearchHit[]
	totalHits: number
	nextHitsToken: string | null
}

export interface DocumentSearchResponse {
	total: {
		hits: number
		documents: number
		capped: boolean
	}
	nextToken: string | null
	results: DocumentSearchResult[]
}

export type DocumentViewSource = "all" | "search"

export interface DocumentRecent extends DocumentSearchDocument {
	viewedAt: Date | string
}

export interface DocumentRecentsResponse {
	results: DocumentRecent[]
}

export interface DocumentBranchSearchResponse {
	totalHits: number
	nextToken: string | null
	hits: DocumentSearchHit[]
}

export type DocumentMaintainersResponse = string[]

// the name the review workflow gives the branch it creates
export const DOCUMENT_DRAFT_BRANCH_NAME = "draft"

// the name a branch goes by in the UI. The default branch and the review
// workflow's draft have fixed names, in a long and a short form. Any other
// branch shows its own.
export function documentBranchLabel(
	branch: { name: string; default: boolean },
	t: (key: string) => string,
	short = false,
): string {
	if (branch.default) {
		return short
			? t("general.branch-labels.main-short")
			: t("general.branch-labels.main")
	}

	if (branch.name !== DOCUMENT_DRAFT_BRANCH_NAME) {
		return branch.name
	}

	return short
		? t("general.branch-labels.draft-short")
		: t("general.branch-labels.draft")
}

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

// what running a metric block reports: cleared is true when its data
// source has started returning real data and the backend has removed the
// simulation from the block
export interface MetricSimulationCheckResponse {
	cleared: boolean
}

import { defineStore } from "pinia"
import type { MetricConfig } from "~/components/editor/blocks/metrics/utils"
import type { LogConfig } from "~/components/editor/blocks/logs/utils"
import type { DiffStatus } from "~/components/editor/diff/position-map"

type DocumentId = string
type BranchId = string
type BlockId = string
type BlockMap<T> = Record<DocumentId, Record<BranchId, Record<BlockId, T>>>

export const EDITOR_COMPACT_MAX_WIDTH_PX = 1280

export const useEditorStore = defineStore("editor", () => {
	const locked = ref(false)
	const activeBranchProtected = ref(false) // the active branch refuses edits from everyone; the container sets it from the branch metadata
	const preloadedBranchIds = ref<BranchId[]>([]) // used to track branches that have been preloaded (e.g. for diffing) to avoid refetching data when switching between them
	const activeDocumentId = ref<DocumentId | null>(null)
	const activeBranchId = ref<BranchId | null>(null)
	const mappedDefaultBranchId = ref<BranchId | null>(null) // used to track the branch that the default branch of the document is currently mapped to (can be different from activeBranchId)
	const targetBranchId = ref<BranchId | null>(null) // used for diffs and merges
	const branchReviewableActionsActive = ref(false) // whether the active branch has reviewable actions (approve/merge) that should be shown in the UI or not, determined based on the document and branch metadata and permissions
	const metricBlockConfigs = ref<BlockMap<MetricConfig>>({})
	const activeMetricBlockConfig = ref<string | null>(null) // uid of the active metric block being edited/viewed in modal
	const metricBlockDiffStatuses = ref<BlockMap<DiffStatus | null>>({})
	const metricBlockOldConfigs = ref<BlockMap<MetricConfig | null>>({})
	const metricBlockNextRefreshTimestamps = ref<BlockMap<Date>>({})
	const logBlockConfigs = ref<BlockMap<LogConfig>>({})
	const activeLogBlockConfig = ref<string | null>(null) // uid of the active log block being edited/viewed in modal
	const logBlockDiffStatuses = ref<BlockMap<DiffStatus | null>>({})
	const logBlockOldConfigs = ref<BlockMap<LogConfig | null>>({})
	const logBlockNextRefreshTimestamps = ref<BlockMap<Date>>({})
	const lastDragDropTimestamp = ref<Date | null>(null)
	const reviewableDiffActive = ref(false)
	const aiAssistantOpen = ref(false)
	// the width of the document's main area, measured by the page; compact
	// view only makes a difference once it exceeds the column cap
	const mainAreaWidth = ref(0)
	const compactViewAvailable = computed(
		() => mainAreaWidth.value > EDITOR_COMPACT_MAX_WIDTH_PX,
	)
	// tracks which mermaid blocks have their code visible, synced
	// across branches and diff editors.
	const mermaidBlockShowCode = ref<Record<BlockId, boolean>>({})
	// asynchronously rendered blocks whose first render has
	// settled: drawn, failed, empty or gone. The document container holds
	// its fade-in until every such block of the document is in here.
	const settledBlockRenders = ref(new Set<BlockId>())

	function updateLock(v: boolean) {
		locked.value = v
	}

	function setActiveBranchProtected(v: boolean) {
		activeBranchProtected.value = v
	}

	function updatePreloadedBranchIds(branchIds: BranchId[]) {
		preloadedBranchIds.value = branchIds
	}

	function updateActiveDocumentId(id: string | null) {
		if (activeDocumentId.value !== id) {
			clearBlockData()
			setReviewableDiffActive(false)
			// reset the branch ids, the new document load renews them
			activeBranchId.value = null
			targetBranchId.value = null
			mappedDefaultBranchId.value = null
			preloadedBranchIds.value = []
		}

		activeDocumentId.value = id
	}

	function updateActiveBranchId(id: string | null) {
		if (activeBranchId.value !== id) {
			// don't clear the metrics here (since we want to keep them
			// when switching between branches), but do clear the diff data
			// since that is specific to the active branch
			setReviewableDiffActive(false)
		}

		activeBranchId.value = id
	}

	function updateMappedDefaultBranchId(id: string | null) {
		mappedDefaultBranchId.value = id
	}

	function updateTargetBranchId(id: string | null) {
		targetBranchId.value = id
	}

	function setBranchReviewableActionsActive(v: boolean) {
		branchReviewableActionsActive.value = v
	}

	function setReviewableDiffActive(active: boolean) {
		reviewableDiffActive.value = active

		if (!active) {
			clearBlockDiffData()
		}
	}

	function setMetricBlockConfig(blockId: string, config: MetricConfig) {
		const [docId, branchId] = resolveBranch("set metric block config")

		setBlockEntry(metricBlockConfigs.value, docId, branchId, blockId, config)
	}

	function removeMetricBlockConfig(
		blockId: string,
		docId?: string,
		branchId?: string,
	) {
		const [resolvedDocId, resolvedBranchId] = resolveBranch(
			"remove metric block config",
			docId,
			branchId,
		)

		removeBlockEntry(
			metricBlockConfigs.value,
			resolvedDocId,
			resolvedBranchId,
			blockId,
		)
	}

	function setMetricBlockDiffInfo(
		blockId: string,
		diffStatus: DiffStatus,
		oldConfig: MetricConfig | null,
	) {
		const [docId, branchId] = resolveBranch("set metric block diff info")

		setBlockEntry(
			metricBlockDiffStatuses.value,
			docId,
			branchId,
			blockId,
			diffStatus,
		)
		setBlockEntry(
			metricBlockOldConfigs.value,
			docId,
			branchId,
			blockId,
			oldConfig,
		)
	}

	function removeMetricBlockDiffInfo(
		blockId: string,
		docId?: string,
		branchId?: string,
	) {
		const [resolvedDocId, resolvedBranchId] = resolveBranch(
			"remove metric block diff info",
			docId,
			branchId,
		)

		removeBlockEntry(
			metricBlockDiffStatuses.value,
			resolvedDocId,
			resolvedBranchId,
			blockId,
		)
		removeBlockEntry(
			metricBlockOldConfigs.value,
			resolvedDocId,
			resolvedBranchId,
			blockId,
		)
	}

	function activateMetricBlockConfig(uid: string | null) {
		activeMetricBlockConfig.value = uid
	}

	function setMetricBlockNextRefreshTimestamp(blockId: string, ms: number) {
		const [docId, branchId] = resolveBranch(
			"set metric block next refresh timestamp",
		)

		setBlockEntry(
			metricBlockNextRefreshTimestamps.value,
			docId,
			branchId,
			blockId,
			new Date(Date.now() + ms),
		)
	}

	function isMetricBlockDueForRefresh(blockId: string): boolean {
		const [docId, branchId] = resolveBranch("check metric block refresh status")

		const nextRefresh =
			metricBlockNextRefreshTimestamps.value[docId]?.[branchId]?.[blockId]
		if (!nextRefresh) {
			return true
		}

		return Date.now() >= nextRefresh.getTime()
	}

	function clearMetricBlockNextRefreshTimestamp(blockUid: string) {
		const [docId, branchId] = resolveBranch(
			"clear metric block refresh timestamp",
		)

		removeBlockEntry(
			metricBlockNextRefreshTimestamps.value,
			docId,
			branchId,
			blockUid,
		)
	}

	function setLogBlockConfig(blockId: string, config: LogConfig) {
		const [docId, branchId] = resolveBranch("set log block config")

		setBlockEntry(logBlockConfigs.value, docId, branchId, blockId, config)
	}

	function removeLogBlockConfig(
		blockId: string,
		docId?: string,
		branchId?: string,
	) {
		const [resolvedDocId, resolvedBranchId] = resolveBranch(
			"remove log block config",
			docId,
			branchId,
		)

		removeBlockEntry(
			logBlockConfigs.value,
			resolvedDocId,
			resolvedBranchId,
			blockId,
		)
	}

	function setLogBlockDiffInfo(
		blockId: string,
		diffStatus: DiffStatus,
		oldConfig: LogConfig | null,
	) {
		const [docId, branchId] = resolveBranch("set log block diff info")

		setBlockEntry(
			logBlockDiffStatuses.value,
			docId,
			branchId,
			blockId,
			diffStatus,
		)
		setBlockEntry(logBlockOldConfigs.value, docId, branchId, blockId, oldConfig)
	}

	function removeLogBlockDiffInfo(
		blockId: string,
		docId?: string,
		branchId?: string,
	) {
		const [resolvedDocId, resolvedBranchId] = resolveBranch(
			"remove log block diff info",
			docId,
			branchId,
		)

		removeBlockEntry(
			logBlockDiffStatuses.value,
			resolvedDocId,
			resolvedBranchId,
			blockId,
		)
		removeBlockEntry(
			logBlockOldConfigs.value,
			resolvedDocId,
			resolvedBranchId,
			blockId,
		)
	}

	function activateLogBlockConfig(uid: string | null) {
		activeLogBlockConfig.value = uid
	}

	function setLogBlockNextRefreshTimestamp(blockId: string, ms: number) {
		const [docId, branchId] = resolveBranch(
			"set log block next refresh timestamp",
		)

		setBlockEntry(
			logBlockNextRefreshTimestamps.value,
			docId,
			branchId,
			blockId,
			new Date(Date.now() + ms),
		)
	}

	function isLogBlockDueForRefresh(blockId: string): boolean {
		const [docId, branchId] = resolveBranch("check log block refresh status")

		const nextRefresh =
			logBlockNextRefreshTimestamps.value[docId]?.[branchId]?.[blockId]
		if (!nextRefresh) {
			return true
		}

		return Date.now() >= nextRefresh.getTime()
	}

	function clearLogBlockNextRefreshTimestamp(blockUid: string) {
		const [docId, branchId] = resolveBranch("clear log block refresh timestamp")

		removeBlockEntry(
			logBlockNextRefreshTimestamps.value,
			docId,
			branchId,
			blockUid,
		)
	}

	function updateLastDragDropTimestamp() {
		lastDragDropTimestamp.value = new Date()
	}

	function isLastDragDropRecent(thresholdMs = 2000): boolean {
		if (!lastDragDropTimestamp.value) {
			return false
		}

		return Date.now() - lastDragDropTimestamp.value.getTime() <= thresholdMs
	}

	function toggleAiAssistantOpen() {
		aiAssistantOpen.value = !aiAssistantOpen.value
	}

	function updateMainAreaWidth(v: number) {
		mainAreaWidth.value = v
	}

	function setMermaidBlockShowCode(id: string, show: boolean) {
		mermaidBlockShowCode.value[id] = show
	}

	function markBlockRenderSettled(id: string) {
		settledBlockRenders.value.add(id)
	}

	function clearBlockData() {
		if (!activeDocumentId.value) {
			return
		}

		clearMetricBlockData(activeDocumentId.value)
		clearLogBlockData(activeDocumentId.value)
		mermaidBlockShowCode.value = {}
		settledBlockRenders.value = new Set()
	}

	function clearBlockDiffData() {
		clearMetricBlockDiffData()
		clearLogBlockDiffData()
	}

	function clearMetricBlockData(docId: DocumentId) {
		activeMetricBlockConfig.value = null
		delete metricBlockConfigs.value[docId]
		delete metricBlockDiffStatuses.value[docId]
		delete metricBlockOldConfigs.value[docId]
		delete metricBlockNextRefreshTimestamps.value[docId]
	}

	function clearMetricBlockDiffData() {
		clearAllEntries(metricBlockDiffStatuses.value)
		clearAllEntries(metricBlockOldConfigs.value)
	}

	function clearLogBlockData(docId: DocumentId) {
		activeLogBlockConfig.value = null
		delete logBlockConfigs.value[docId]
		delete logBlockDiffStatuses.value[docId]
		delete logBlockOldConfigs.value[docId]
		delete logBlockNextRefreshTimestamps.value[docId]
	}

	function clearLogBlockDiffData() {
		clearAllEntries(logBlockDiffStatuses.value)
		clearAllEntries(logBlockOldConfigs.value)
	}

	// resolveBranch returns the given document and branch, falling back to
	// the active ones. It throws when either is missing, naming the action
	// that needed them.
	function resolveBranch(
		action: string,
		docId?: DocumentId,
		branchId?: BranchId,
	): [DocumentId, BranchId] {
		const resolvedDocId = docId ?? activeDocumentId.value
		if (!resolvedDocId) {
			throw new Error(`Cannot ${action} without an active document`)
		}

		const resolvedBranchId = branchId ?? activeBranchId.value
		if (!resolvedBranchId) {
			throw new Error(`Cannot ${action} without an active branch`)
		}

		return [resolvedDocId, resolvedBranchId]
	}

	function setBlockEntry<T>(
		map: BlockMap<T>,
		docId: DocumentId,
		branchId: BranchId,
		blockId: BlockId,
		value: T,
	) {
		const docEntries = map[docId] ?? {}
		const branchEntries = docEntries[branchId] ?? {}

		branchEntries[blockId] = value
		docEntries[branchId] = branchEntries
		map[docId] = docEntries
	}

	function removeBlockEntry<T>(
		map: BlockMap<T>,
		docId: DocumentId,
		branchId: BranchId,
		blockId: BlockId,
	) {
		delete map[docId]?.[branchId]?.[blockId]
	}

	function clearAllEntries<T>(map: BlockMap<T>) {
		for (const docId of Object.keys(map)) {
			delete map[docId]
		}
	}

	return {
		locked,
		updateLock,
		activeBranchProtected,
		setActiveBranchProtected,
		preloadedBranchIds,
		updatePreloadedBranchIds,
		activeDocumentId,
		updateActiveDocumentId,
		activeBranchId,
		updateActiveBranchId,
		mappedDefaultBranchId,
		updateMappedDefaultBranchId,
		targetBranchId,
		updateTargetBranchId,
		branchReviewableActionsActive,
		setBranchReviewableActionsActive,
		reviewableDiffActive,
		setReviewableDiffActive,
		metricBlockConfigs,
		setMetricBlockConfig,
		removeMetricBlockConfig,
		metricBlockDiffStatuses,
		metricBlockOldConfigs,
		setMetricBlockDiffInfo,
		removeMetricBlockDiffInfo,
		activeMetricBlockConfig,
		activateMetricBlockConfig,
		logBlockConfigs,
		setLogBlockConfig,
		removeLogBlockConfig,
		logBlockDiffStatuses,
		logBlockOldConfigs,
		setLogBlockDiffInfo,
		removeLogBlockDiffInfo,
		activeLogBlockConfig,
		activateLogBlockConfig,
		setLogBlockNextRefreshTimestamp,
		isLogBlockDueForRefresh,
		clearLogBlockNextRefreshTimestamp,
		setMetricBlockNextRefreshTimestamp,
		isMetricBlockDueForRefresh,
		clearMetricBlockNextRefreshTimestamp,
		updateLastDragDropTimestamp,
		isLastDragDropRecent,
		aiAssistantOpen,
		toggleAiAssistantOpen,
		mainAreaWidth,
		updateMainAreaWidth,
		compactViewAvailable,
		mermaidBlockShowCode,
		setMermaidBlockShowCode,
		settledBlockRenders,
		markBlockRenderSettled,
	}
})

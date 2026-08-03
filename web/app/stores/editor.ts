import { defineStore } from "pinia"
import type { MetricConfig } from "~/components/editor/blocks/metrics/utils"
import type { DiffStatus } from "~/components/editor/diff/position-map"

type DocumentId = string
type BranchId = string
type BlockId = string

export const useEditorStore = defineStore("editor", () => {
	const locked = ref(false)
	const preloadedBranchIds = ref<BranchId[]>([]) // used to track branches that have been preloaded (e.g. for diffing) to avoid refetching data when switching between them
	const activeDocumentId = ref<DocumentId | null>(null)
	const activeBranchId = ref<BranchId | null>(null)
	const mappedDefaultBranchId = ref<BranchId | null>(null) // used to track the branch that the default branch of the document is currently mapped to (can be different from activeBranchId)
	const targetBranchId = ref<BranchId | null>(null) // used for diffs and merges
	const branchReviewableActionsActive = ref(false) // whether the active branch has reviewable actions (approve/merge) that should be shown in the UI or not, determined based on the document and branch metadata and permissions
	const metricBlockConfigs = ref<
		Record<DocumentId, Record<BranchId, Record<BlockId, MetricConfig>>>
	>({})
	const activeMetricBlockConfig = ref<string | null>(null) // uid of the active metric block being edited/viewed in modal
	const metricBlockDiffStatuses = ref<
		Record<DocumentId, Record<BranchId, Record<BlockId, DiffStatus | null>>>
	>({})
	const metricBlockOldConfigs = ref<
		Record<DocumentId, Record<BranchId, Record<BlockId, MetricConfig | null>>>
	>({})
	const metricBlockNextRefreshTimestamps = ref<
		Record<DocumentId, Record<BranchId, Record<BlockId, Date>>>
	>({})
	const lastDragDropTimestamp = ref<Date | null>(null)
	const reviewableDiffActive = ref(false)
	const aiAssistantOpen = ref(false)
	// tracks which mermaid blocks have their code visible, synced
	// across branches and diff editors.
	const mermaidBlockShowCode = ref<Record<BlockId, boolean>>({})

	function updateLock(v: boolean) {
		locked.value = v
	}

	function updatePreloadedBranchIds(branchIds: BranchId[]) {
		preloadedBranchIds.value = branchIds
	}

	function updateActiveDocumentId(id: string | null) {
		if (activeDocumentId.value !== id) {
			clearMetricBlockData()
			setReviewableDiffActive(false)
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
			clearMetricBlockDiffData()
		}
	}

	function setMetricBlockConfig(blockId: string, config: MetricConfig) {
		if (!activeDocumentId.value) {
			throw new Error(
				"Cannot set metric block config without an active document",
			)
		}

		if (!activeBranchId.value) {
			throw new Error("Cannot set metric block config without an active branch")
		}

		if (!metricBlockConfigs.value[activeDocumentId.value]) {
			metricBlockConfigs.value[activeDocumentId.value] = {}
		}

		if (
			!metricBlockConfigs.value[activeDocumentId.value]![activeBranchId.value]
		) {
			metricBlockConfigs.value[activeDocumentId.value]![activeBranchId.value] =
				{}
		}

		metricBlockConfigs.value[activeDocumentId.value]![activeBranchId.value]![
			blockId
		] = config
	}

	function removeMetricBlockConfig(
		blockId: string,
		docId?: string,
		branchId?: string,
	) {
		if (!activeDocumentId.value && !docId) {
			throw new Error(
				"Cannot remove metric block config without an active document",
			)
		}

		docId = docId || activeDocumentId.value!

		if (!activeBranchId.value && !branchId) {
			throw new Error(
				"Cannot remove metric block config without an active branch",
			)
		}

		branchId = branchId || activeBranchId.value!

		delete metricBlockConfigs.value[docId]?.[branchId]?.[blockId]
	}

	function setMetricBlockDiffInfo(
		blockId: string,
		diffStatus: DiffStatus,
		oldConfig: MetricConfig | null,
	) {
		if (!activeDocumentId.value) {
			throw new Error(
				"Cannot set metric block diff info without an active document",
			)
		}

		if (!activeBranchId.value) {
			throw new Error(
				"Cannot set metric block diff info without an active branch",
			)
		}

		if (!metricBlockDiffStatuses.value[activeDocumentId.value]) {
			metricBlockDiffStatuses.value[activeDocumentId.value] = {}
		}

		if (!metricBlockOldConfigs.value[activeDocumentId.value]) {
			metricBlockOldConfigs.value[activeDocumentId.value] = {}
		}

		if (
			!metricBlockDiffStatuses.value[activeDocumentId.value]![
				activeBranchId.value
			]
		) {
			metricBlockDiffStatuses.value[activeDocumentId.value]![
				activeBranchId.value
			] = {}
		}

		if (
			!metricBlockOldConfigs.value[activeDocumentId.value]![
				activeBranchId.value
			]
		) {
			metricBlockOldConfigs.value[activeDocumentId.value]![
				activeBranchId.value
			] = {}
		}

		metricBlockDiffStatuses.value[activeDocumentId.value]![
			activeBranchId.value
		]![blockId] = diffStatus
		metricBlockOldConfigs.value[activeDocumentId.value]![activeBranchId.value]![
			blockId
		] = oldConfig
	}

	function removeMetricBlockDiffInfo(
		blockId: string,
		docId?: string,
		branchId?: string,
	) {
		if (!activeDocumentId.value && !docId) {
			throw new Error(
				"Cannot remove metric block diff info without an active document",
			)
		}

		docId = docId || activeDocumentId.value!

		if (!activeBranchId.value && !branchId) {
			throw new Error(
				"Cannot remove metric block diff info without an active branch",
			)
		}

		branchId = branchId || activeBranchId.value!

		delete metricBlockDiffStatuses.value[docId]?.[branchId]?.[blockId]
		delete metricBlockOldConfigs.value[docId]?.[branchId]?.[blockId]
	}

	function activateMetricBlockConfig(uid: string | null) {
		activeMetricBlockConfig.value = uid
	}

	function setMetricBlockNextRefreshTimestamp(blockId: string, ms: number) {
		if (!activeDocumentId.value) {
			throw new Error(
				"Cannot set metric block next refresh timestamp without an active document",
			)
		}

		if (!activeBranchId.value) {
			throw new Error(
				"Cannot set metric block next refresh timestamp without an active branch",
			)
		}

		if (!metricBlockNextRefreshTimestamps.value[activeDocumentId.value]) {
			metricBlockNextRefreshTimestamps.value[activeDocumentId.value] = {}
		}

		if (
			!metricBlockNextRefreshTimestamps.value[activeDocumentId.value]![
				activeBranchId.value
			]
		) {
			metricBlockNextRefreshTimestamps.value[activeDocumentId.value]![
				activeBranchId.value
			] = {}
		}

		metricBlockNextRefreshTimestamps.value[activeDocumentId.value]![
			activeBranchId.value
		]![blockId] = new Date(Date.now() + ms)
	}

	function isMetricBlockDueForRefresh(blockId: string): boolean {
		if (!activeDocumentId.value) {
			throw new Error(
				"Cannot check metric block refresh status without an active document",
			)
		}

		if (!activeBranchId.value) {
			throw new Error(
				"Cannot check metric block refresh status without an active branch",
			)
		}

		const nextRefresh =
			metricBlockNextRefreshTimestamps.value[activeDocumentId.value]?.[
				activeBranchId.value
			]?.[blockId]
		if (!nextRefresh) {
			return true
		}

		return Date.now() >= nextRefresh.getTime()
	}

	function clearMetricBlockNextRefreshTimestamp(blockUid: string) {
		if (!activeDocumentId.value) {
			throw new Error(
				"Cannot clear metric block refresh timestamp without an active document",
			)
		}

		if (!activeBranchId.value) {
			throw new Error(
				"Cannot clear metric block refresh timestamp without an active branch",
			)
		}

		delete metricBlockNextRefreshTimestamps.value[activeDocumentId.value]?.[
			activeBranchId.value
		]?.[blockUid]
	}

	function updateLastDragDropTimestamp() {
		lastDragDropTimestamp.value = new Date()
	}

	function isLastDragDropRecent(thresholdMs: number = 2000): boolean {
		if (!lastDragDropTimestamp.value) {
			return false
		}

		return Date.now() - lastDragDropTimestamp.value.getTime() <= thresholdMs
	}

	function toggleAiAssistantOpen() {
		aiAssistantOpen.value = !aiAssistantOpen.value
	}

	function setMermaidBlockShowCode(id: string, show: boolean) {
		mermaidBlockShowCode.value[id] = show
	}

	function clearMetricBlockData() {
		if (!activeDocumentId.value) {
			return
		}

		activeMetricBlockConfig.value = null
		delete metricBlockNextRefreshTimestamps.value[activeDocumentId.value]
		delete metricBlockConfigs.value[activeDocumentId.value]
		delete metricBlockDiffStatuses.value[activeDocumentId.value]
		delete metricBlockOldConfigs.value[activeDocumentId.value]
		mermaidBlockShowCode.value = {}
	}

	function clearMetricBlockDiffData() {
		for (const docId of Object.keys(metricBlockDiffStatuses.value)) {
			delete metricBlockDiffStatuses.value[docId]
		}

		for (const docId of Object.keys(metricBlockOldConfigs.value)) {
			delete metricBlockOldConfigs.value[docId]
		}
	}

	return {
		locked,
		updateLock,
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
		setMetricBlockNextRefreshTimestamp,
		isMetricBlockDueForRefresh,
		clearMetricBlockNextRefreshTimestamp,
		updateLastDragDropTimestamp,
		isLastDragDropRecent,
		aiAssistantOpen,
		toggleAiAssistantOpen,
		mermaidBlockShowCode,
		setMermaidBlockShowCode,
	}
})

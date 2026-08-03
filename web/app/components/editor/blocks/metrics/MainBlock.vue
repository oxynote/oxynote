<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import VisualizationContainer from "./VisualizationContainer.vue"
import { cn } from "~/lib/utils"
import PopoverConfigBox from "./PopoverConfigBox.vue"
import {
	type MetricConfig,
	MetricBlockWidth,
	NODE_ATTR_UPDATE_REFRESH_DISABLE_MS,
	buildConfigFromNodeAttrs,
} from "./utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps(nodeViewProps)

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { editingUsersRef } = useCollaborationAwareness(() => props.editor)

const sizeClass = computed(() => {
	const size =
		(props.node.attrs.width as MetricBlockWidth) || MetricBlockWidth.Standard
	return `metric-block-${size}`
})
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

// guard against updating a node that's been moved/deleted after drag-drop
function safeUpdateAttributes(attrs: Record<string, any>) {
	if (isEditingDisabled.value) {
		return
	}

	const pos = props.getPos()
	if (typeof pos !== "number" || pos < 0) {
		return
	}

	const nodeAtPos = props.editor.state.doc.nodeAt(pos)
	if (!nodeAtPos || nodeAtPos.attrs.uid !== props.node.attrs.uid) {
		return
	}

	props.updateAttributes(attrs)
}

// reads the legacy monolithic config attr reactively (null when already migrated)
const legacyConfig = computed(
	() => props.node.attrs.config as MetricConfig | null,
)

// when the legacy blob still exists, the first edit on any field migrates all
// fields into individual attrs and clears the blob in a single transaction.
// subsequent edits skip migration since legacyConfig will be null.
function migrateLegacyIfNeeded(fieldAttrs: Record<string, any>) {
	const legacy = legacyConfig.value
	if (!legacy) {
		safeUpdateAttributes(fieldAttrs)
		return
	}

	// decompose legacy blob into individual attrs, then overlay the caller's
	// edit on top so the user's change wins over the legacy value
	safeUpdateAttributes({
		title: legacy.title ?? "",
		dataSourceId: legacy.dataSourceId ?? null,
		visualizationType: (legacy as any).type ?? legacy.visualizationType ?? null,
		queries: legacy.queries ?? null,
		timeRange: legacy.timeRange ?? null,
		refreshInterval: legacy.refreshInterval ?? null,
		thresholds: legacy.thresholds ?? null,
		baseThresholdColor: legacy.baseThresholdColor ?? "",
		decimals: legacy.decimals ?? null,
		unitType: legacy.unit?.type ?? null,
		unitCustom: legacy.unit?.custom ?? null,
		axisBoundsMin: legacy.axisBounds?.min ?? null,
		axisBoundsMax: legacy.axisBounds?.max ?? null,
		// overlay caller's edit and clear legacy blob
		...fieldAttrs,
		config: null,
	})
}

// reactive() is used to auto-unwrap the computed refs inside this object.
// without it, each property would be a ComputedRef requiring .value access,
// and the object wouldn't satisfy the MetricConfig interface. reactive()
// makes config.title a string (not ComputedRef<string>), so consumers can
// use this object as a plain MetricConfig with transparent get/set behavior.
//
// when legacy config exists, its values take priority over flat attrs because
// the flat attrs just hold schema defaults that would shadow real legacy values.
// once migrated (legacyConfig is null), flat attrs are the source of truth.
const config = reactive({
	title: computed<MetricConfig["title"]>({
		get: () => legacyConfig.value?.title ?? props.node.attrs.title ?? "",
		set: (v) => migrateLegacyIfNeeded({ title: v }),
	}),
	dataSourceId: computed<MetricConfig["dataSourceId"]>({
		get: () =>
			legacyConfig.value?.dataSourceId ?? props.node.attrs.dataSourceId ?? null,
		set: (v) => migrateLegacyIfNeeded({ dataSourceId: v }),
	}),
	visualizationType: computed<MetricConfig["visualizationType"]>({
		get: () =>
			(legacyConfig.value as any)?.type ??
			legacyConfig.value?.visualizationType ??
			props.node.attrs.visualizationType ??
			null,
		set: (v) => migrateLegacyIfNeeded({ visualizationType: v }),
	}),
	queries: computed<MetricConfig["queries"]>({
		get: () => legacyConfig.value?.queries ?? props.node.attrs.queries ?? null,
		set: (v) => migrateLegacyIfNeeded({ queries: v }),
	}),
	timeRange: computed<MetricConfig["timeRange"]>({
		get: () =>
			legacyConfig.value?.timeRange ?? props.node.attrs.timeRange ?? null,
		set: (v) => migrateLegacyIfNeeded({ timeRange: v }),
	}),
	refreshInterval: computed<MetricConfig["refreshInterval"]>({
		get: () =>
			legacyConfig.value?.refreshInterval ??
			props.node.attrs.refreshInterval ??
			null,
		set: (v) => migrateLegacyIfNeeded({ refreshInterval: v }),
	}),
	thresholds: computed<Required<MetricConfig>["thresholds"]>({
		get: () =>
			legacyConfig.value?.thresholds ?? props.node.attrs.thresholds ?? null,
		set: (v) => migrateLegacyIfNeeded({ thresholds: v }),
	}),
	baseThresholdColor: computed<MetricConfig["baseThresholdColor"]>({
		get: () =>
			legacyConfig.value?.baseThresholdColor ??
			props.node.attrs.baseThresholdColor ??
			"",
		set: (v) => migrateLegacyIfNeeded({ baseThresholdColor: v }),
	}),
	decimals: computed<Required<MetricConfig>["decimals"]>({
		get: () =>
			legacyConfig.value?.decimals ?? props.node.attrs.decimals ?? null,
		set: (v) => migrateLegacyIfNeeded({ decimals: v }),
	}),
	unit: reactive({
		type: computed<Required<MetricConfig["unit"]>["type"]>({
			get: () =>
				legacyConfig.value?.unit?.type ?? props.node.attrs.unitType ?? null,
			set: (v) => migrateLegacyIfNeeded({ unitType: v }),
		}),
		custom: computed<Required<MetricConfig["unit"]>["custom"]>({
			get: () =>
				legacyConfig.value?.unit?.custom ?? props.node.attrs.unitCustom ?? null,
			set: (v) => migrateLegacyIfNeeded({ unitCustom: v }),
		}),
	}),
	axisBounds: reactive({
		min: computed<Required<NonNullable<MetricConfig["axisBounds"]>>["min"]>({
			get: () =>
				legacyConfig.value?.axisBounds?.min ??
				props.node.attrs.axisBoundsMin ??
				null,
			set: (v) => migrateLegacyIfNeeded({ axisBoundsMin: v }),
		}),
		max: computed<Required<NonNullable<MetricConfig["axisBounds"]>>["max"]>({
			get: () =>
				legacyConfig.value?.axisBounds?.max ??
				props.node.attrs.axisBoundsMax ??
				null,
			set: (v) => migrateLegacyIfNeeded({ axisBoundsMax: v }),
		}),
	}),
})

const otherEditingUsers = editingUsersRef(() => props.node.attrs.uid)
const lastOtherEditingUser = computed(() => {
	return otherEditingUsers.value.length
		? otherEditingUsers.value[otherEditingUsers.value.length - 1]
		: null
})

const disableVisualizationRefresh = ref(false)

watchImmediate(
	[
		() => editorStore.activeDocumentId,
		() => editorStore.activeBranchId,
		() => props.node.attrs.uid,
		() => props.node.attrs.diffStatus,
	],
	([docId, branchId, uid], [prevDocId, prevBranchId, prevUid]) => {
		if (
			prevDocId &&
			prevBranchId &&
			prevUid &&
			(prevDocId !== docId || prevBranchId !== branchId || prevUid !== uid)
		) {
			editorStore.removeMetricBlockConfig(prevUid, prevDocId, prevBranchId)
			editorStore.removeMetricBlockDiffInfo(prevUid, prevDocId, prevBranchId)
		}

		if (docId && branchId && uid) {
			const diffStatus = props.node.attrs.diffStatus as DiffStatus | null

			// diff editor blocks should not overwrite the real config in the store
			if (!diffStatus && !editorStore.reviewableDiffActive) {
				editorStore.setMetricBlockConfig(uid, config)
			}

			if (diffStatus) {
				let oldConfig: MetricConfig | null = null
				if (diffStatus === DiffStatus.Modified && props.node.attrs.oldNode) {
					const oldNode = props.node.attrs.oldNode
					oldConfig = buildConfigFromNodeAttrs(oldNode.attrs ?? {})
				}
				editorStore.setMetricBlockDiffInfo(uid, diffStatus, oldConfig)
			} else {
				editorStore.removeMetricBlockDiffInfo(uid)
			}
		}
	},
)

onMounted(() => {
	if (editorStore.isLastDragDropRecent()) {
		// sometimes after drag-drop the affected nodes get remounted and sometimes
		// not (only the attributes are updated), so we need to disable the refresh
		// temporarily on mount as well as on uid changes
		disableVisualizationRefreshTemporarily()
	}
})

// disable animation/data refresh briefly on UID change (this happens when
// nodes are drag-and-dropped and switch places with each other; this happens
// because tiptap doesn't always delete/recreate the node, it just switches the
// attributes)
watch(
	() => props.node.attrs.uid,
	() => {
		disableVisualizationRefreshTemporarily()
	},
)

function disableVisualizationRefreshTemporarily() {
	disableVisualizationRefresh.value = true
	nextTick(() => {
		setTimeout(() => {
			disableVisualizationRefresh.value = false
		}, NODE_ATTR_UPDATE_REFRESH_DISABLE_MS)
	})
}
</script>
<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		:class="
			cn(
				'not-prose relative h-80 rounded-md border border-border caret-transparent select-none',
				sizeClass,
				lastOtherEditingUser && 'rounded-tl-none',
			)
		"
		:style="{
			borderColor: lastOtherEditingUser?.color ?? undefined,
		}"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<ShadcnUiPopover>
			<div class="relative size-full">
				<div
					v-if="lastOtherEditingUser"
					:class="
						cn(
							'pointer-events-none absolute -top-4 -left-px z-5 rounded-sm px-[0.1rem] text-xs font-medium whitespace-nowrap text-white caret-transparent select-none',
						)
					"
					:style="{
						backgroundColor: lastOtherEditingUser?.color ?? undefined,
					}"
				>
					{{ lastOtherEditingUser.name }}
				</div>
				<div class="absolute top-1 right-1.5 z-1 flex items-center gap-1">
					<ShadcnUiPopoverTrigger as-child>
						<ShadcnUiButton
							variant="ghost-plain"
							size="icon-sm"
							:class="cn('size-6')"
						>
							<Icon name="lucide:settings" />
						</ShadcnUiButton>
					</ShadcnUiPopoverTrigger>
				</div>
				<VisualizationContainer
					:config="config"
					:uid="props.node.attrs.uid"
					:disable-refresh="disableVisualizationRefresh"
				/>
			</div>
			<ShadcnUiPopoverContent class="w-50" side="right" side-flip align="start">
				<PopoverConfigBox
					v-model="config"
					:uid="props.node.attrs.uid"
					:editor="props.editor"
					:get-pos="props.getPos"
				/>
			</ShadcnUiPopoverContent>
		</ShadcnUiPopover>
	</NodeViewWrapper>
</template>

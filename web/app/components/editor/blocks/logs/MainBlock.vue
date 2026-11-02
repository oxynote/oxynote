<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import type { JSONContent } from "@tiptap/core"
import { cn } from "#layers/ui/app/lib/utils"
import { GridItemWidth } from "../grid/utils"
import { buildLogConfigFromNodeAttrs, type LogConfig } from "./utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import DiffChangeMarker from "~/components/editor/diff/DiffChangeMarker.vue"
import LogDiffDetails from "./LogDiffDetails.vue"

const props = defineProps(nodeViewProps)

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { editingUsersRef } = useCollaborationAwareness(() => props.editor)

const sizeClass = computed(() => {
	const size =
		(props.node.attrs.width as GridItemWidth | null) ?? GridItemWidth.Standard
	return `grid-item-${size}`
})
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
// reactive() unwraps the computed refs, so the modal reads and writes the
// fields like a plain LogConfig while each write updates the node
const config = reactive({
	title: configField("title"),
	dataSourceId: configField("dataSourceId"),
	queries: configField("queries"),
	timeRange: configField("timeRange"),
	refreshInterval: configField("refreshInterval"),
	timestampFormat: configField("timestampFormat"),
	order: configField("order"),
})
const otherEditingUsers = editingUsersRef(() => props.node.attrs.uid as string)
const lastOtherEditingUser = computed(() => {
	return otherEditingUsers.value.length
		? otherEditingUsers.value[otherEditingUsers.value.length - 1]
		: null
})

watchImmediate(
	[
		() => editorStore.activeDocumentId,
		() => editorStore.activeBranchId,
		() => props.node.attrs.uid as string,
		() => props.node.attrs.diffStatus as DiffStatus | null,
	],
	([docId, branchId, uid], [prevDocId, prevBranchId, prevUid]) => {
		if (
			prevDocId &&
			prevBranchId &&
			prevUid &&
			(prevDocId !== docId || prevBranchId !== branchId || prevUid !== uid)
		) {
			editorStore.removeLogBlockConfig(prevUid, prevDocId, prevBranchId)
			editorStore.removeLogBlockDiffInfo(prevUid, prevDocId, prevBranchId)
		}

		if (!docId || !branchId || !uid) {
			return
		}

		const diffStatus = props.node.attrs.diffStatus as DiffStatus | null

		// a block of the diff editor must not replace the real config
		if (!diffStatus && !editorStore.reviewableDiffActive) {
			editorStore.setLogBlockConfig(uid, config)
		}

		if (!diffStatus) {
			editorStore.removeLogBlockDiffInfo(uid)

			return
		}

		const oldNode = props.node.attrs.oldNode as JSONContent | null
		const oldConfig =
			diffStatus === DiffStatus.Modified && oldNode
				? buildLogConfigFromNodeAttrs(oldNode.attrs ?? {})
				: null

		editorStore.setLogBlockDiffInfo(uid, diffStatus, oldConfig)
	},
)

function configField<K extends keyof LogConfig>(key: K) {
	return computed<LogConfig[K]>({
		get: () => buildLogConfigFromNodeAttrs(props.node.attrs)[key],
		set: (value) => {
			safeUpdateAttributes({ [key]: value })
		},
	})
}

// guard against updating a node that's been moved/deleted after drag-drop
function safeUpdateAttributes(attrs: Record<string, unknown>) {
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

function openModal() {
	// select this node first. Otherwise the page jumps when the editor
	// regains focus after the modal closes.
	const pos = props.getPos()
	if (typeof pos === "number") {
		props.editor.commands.setNodeSelection(pos)
	}

	editorStore.activateLogBlockConfig(props.node.attrs.uid as string)
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
		<div class="relative size-full">
			<div
				v-if="lastOtherEditingUser"
				class="pointer-events-none absolute -top-4 -left-px z-5 rounded-sm px-[0.1rem] text-xs font-medium whitespace-nowrap text-white caret-transparent select-none"
				:style="{
					backgroundColor: lastOtherEditingUser.color,
				}"
			>
				{{ lastOtherEditingUser.name }}
			</div>
			<div
				class="absolute top-1 right-1.5 z-1 flex items-center rounded-md bg-background has-[>:not(button)]:pl-0.5"
			>
				<DiffChangeMarker :node="props.node" class="mr-1">
					<LogDiffDetails :node="props.node" />
				</DiffChangeMarker>
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="size-6"
					@click="openModal"
				>
					<Icon name="lucide:settings" />
					<span class="sr-only">
						{{
							!isEditingDisabled
								? $t("editor.logs.config.modal-trigger-button-normal")
								: $t("editor.logs.config.modal-trigger-button-readonly")
						}}
					</span>
				</ShadcnUiButton>
			</div>
			<!-- TODO: render the log rows the block's queries return -->
		</div>
	</NodeViewWrapper>
</template>

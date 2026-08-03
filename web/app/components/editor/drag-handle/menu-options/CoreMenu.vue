<script lang="ts" setup>
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import type { JSONContent } from "@tiptap/core"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import DefaultOptions from "./DefaultOptions.vue"
import {
	SplitDocumentation,
	SplitDocumentationRightSide,
} from "../../blocks/split-documentation"
import SplitDocRightSideMenu from "./SplitDocRightSideMenu.vue"
import SplitDocMenu from "./SplitDocMenu.vue"
import {
	ParameterList,
	ParameterListItem,
} from "../../blocks/split-documentation/parameter-list"
import SplitDocParameterListMenu from "./SplitDocParameterListMenu.vue"
import SplitDocParameterListItemMenu from "./SplitDocParameterListItemMenu.vue"
import { deleteEmptyMetricGrids } from "../../blocks/metrics"
import MetricBlockMenu from "./MetricBlockMenu.vue"
import { isNodeOrDescendantsBeingEditedByOther } from "../../collaboration"
import { showToastMessage } from "~/components/toast"
import { METRIC_BLOCK_NAME } from "../../blocks/node-names"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"

const props = defineProps<{
	editor: Editor
	dataSyncProvider: HocuspocusProvider | null | undefined
	hovered: {
		node: Node
		nodePos: number
		nodeId: string
		nodeHooks: DocumentHook[] | null
		nodeHookStatus: "stale" | "fresh" | null
	} | null
}>()
const emit = defineEmits<{
	(e: "add-node-comment" | "open-node-comment", pos: number): void
}>()

const { createCurrentUrl } = useCurrentUrl()

const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const canAddComment = computed(() => {
	// NOTE reviewable actions checks should be removed once we start using more branches
	return isEditable.value || editorStore.branchReviewableActionsActive
})

// this is needed to keep track of the last valid hovered state
// when the menu is in the process of closing (and props.hovered becomes null)
// but we still need to know which buttons to show until the menu is fully
// closed (to avoid flicker and jumping menu items)
const lastValidHovered = useLastValidRef(() => {
	return props.hovered
		? {
				hovered: props.hovered,
				uid: props.editor.state.doc.nodeAt(props.hovered.nodePos)?.attrs.uid,
				insideSplitDocRightSide: isNodeInsideTiptapNode(
					props.editor.state,
					props.hovered.nodePos,
					[SplitDocumentationRightSide.name],
				),
				hasNodeComment: props.editor.commands.hasNodeComment(
					props.hovered.nodePos,
				),
			}
		: null
})
const hoveredState = computed(() => {
	return {
		lastData: lastValidHovered.value,
		hoverActive: !!props.hovered,
	}
})

const optTypes = computed(() => {
	if (!hoveredState.value.lastData) {
		return []
	}

	const res: string[] = []

	if (hoveredState.value.lastData.insideSplitDocRightSide) {
		res.push("split-doc-right-side")
	} else if (
		hoveredState.value.lastData.hovered.node.type.name === ParameterList.name
	) {
		res.push("split-doc-parameter-list")
	} else if (
		hoveredState.value.lastData.hovered.node.type.name ===
		ParameterListItem.name
	) {
		res.push("split-doc-parameter-list-item")
	} else if (
		hoveredState.value.lastData.hovered.node.type.name ===
		SplitDocumentation.name
	) {
		res.push("split-doc", "default")
	} else if (
		hoveredState.value.lastData.hovered.node.type.name === METRIC_BLOCK_NAME
	) {
		res.push("metric-block", "default")
	} else {
		res.push("default")
	}

	return res
})

const hasNodeComment = computed(() => {
	return lastValidHovered.value?.hasNodeComment ?? false
})

const nodeUid = computed(() => {
	return lastValidHovered.value?.hovered.node.attrs.uid ?? null
})

async function addNodeComment() {
	// Wait for next tick to allow dropdown menu to fully close
	// before making document changes that trigger plugin updates
	await nextTick()

	if (!props.editor || !props.hovered) {
		return
	}

	emit("add-node-comment", props.hovered.nodePos)
}

function showNodeComment() {
	if (!props.editor || !props.hovered) {
		return
	}

	emit("open-node-comment", props.hovered.nodePos)
}

const { t } = useI18n({ useScope: "global" })

async function deleteBlock() {
	// Wait for next tick to allow dropdown menu to fully close
	// before making document changes that trigger plugin updates
	await nextTick()

	if (!props.editor || !props.hovered) {
		return
	}

	const pos = props.hovered.nodePos
	const node = props.editor.state.doc.nodeAt(pos)

	if (!node) {
		return
	}

	// check if another user is editing this node or any of its descendants
	if (
		isNodeOrDescendantsBeingEditedByOther(
			props.dataSyncProvider,
			props.editor.state.doc,
			pos,
		)
	) {
		showToastMessage("error", t("editor.errors.node-being-edited"))
		return
	}

	let tr = props.editor.state.tr.delete(pos, pos + node.nodeSize)

	// Clean up empty MetricGrids in the same transaction to avoid
	// position corruption from appendTransaction
	tr = deleteEmptyMetricGrids(tr, props.editor.state.schema)

	props.editor.view.dispatch(tr)
	props.editor.commands.focus(undefined, { scrollIntoView: false })
}

function copyLink() {
	const url = createCurrentUrl({ hash: nodeUid.value })
	navigator.clipboard.writeText(url.toString())
}

function stripNode(node: JSONContent): JSONContent {
	const nextNode: JSONContent = { ...node }

	if (nextNode.attrs && "uid" in nextNode.attrs) {
		nextNode.attrs = { ...nextNode.attrs, uid: null }
	}

	if (nextNode.attrs && "nodeCommentId" in nextNode.attrs) {
		nextNode.attrs = { ...nextNode.attrs, nodeCommentId: null }
	}

	if (nextNode.marks) {
		nextNode.marks = nextNode.marks.filter((mark) => mark.type !== "comment")
	}

	if (nextNode.content) {
		nextNode.content = nextNode.content.map(stripNode)
	}

	return nextNode
}

function duplicateBlock() {
	if (!props.editor || props.hovered == null) {
		return
	}

	const pos = props.hovered.nodePos
	const node = props.editor.state.doc.nodeAt(pos)

	if (!node) {
		return
	}

	const insertPos = pos + node.nodeSize
	const clonedNode = stripNode(node.toJSON())

	const chain = props.editor.chain()

	if (
		node.type.name === METRIC_BLOCK_NAME ||
		node.type.name === "metricGrid" ||
		node.type.name === "image"
	) {
		chain.setMeta(SUPPRESS_SCROLL_TO_SELECTION_META, true)
	}

	chain.focus().insertContentAt(insertPos, clonedNode).run()
}
</script>
<template>
	<div class="w-full">
		<template v-if="optTypes.includes('split-doc') && !isEditingDisabled">
			<SplitDocMenu :editor="props.editor" :hovered="props.hovered" />
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<template
			v-if="optTypes.includes('split-doc-right-side') && !isEditingDisabled"
		>
			<SplitDocRightSideMenu :editor="props.editor" :hovered="props.hovered" />
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<template
			v-if="optTypes.includes('split-doc-parameter-list') && !isEditingDisabled"
		>
			<SplitDocParameterListMenu
				:editor="props.editor"
				:hovered="props.hovered"
			/>
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<template
			v-if="
				optTypes.includes('split-doc-parameter-list-item') && !isEditingDisabled
			"
		>
			<SplitDocParameterListItemMenu
				:editor="props.editor"
				:hovered="props.hovered"
			/>
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<template v-if="optTypes.includes('metric-block') && !isEditingDisabled">
			<MetricBlockMenu :editor="props.editor" :hovered="props.hovered" />
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<template v-if="optTypes.includes('default') && !isEditingDisabled">
			<DefaultOptions :editor="props.editor" :hovered="props.hovered" />
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<ShadcnUiDropdownMenuItem v-if="nodeUid" @click="copyLink">
			<div class="flex w-full items-center gap-1">
				<Icon name="lucide:link" class="scale-x-[-1] transform" />
				<span>
					{{ $t("editor.drag-handle.options.copy-link") }}
				</span>
			</div>
		</ShadcnUiDropdownMenuItem>
		<ShadcnUiDropdownMenuItem v-if="!isEditingDisabled" @click="duplicateBlock">
			<div class="flex w-full items-center gap-1">
				<Icon name="lucide:copy" />
				<span>
					{{ $t("editor.drag-handle.options.default.duplicate-block") }}
				</span>
			</div>
		</ShadcnUiDropdownMenuItem>
		<template v-if="hasNodeComment">
			<ShadcnUiDropdownMenuItem @click="showNodeComment">
				<div class="flex w-full items-center gap-1">
					<Icon name="lucide:message-square-text" />
					<span>
						{{ $t("editor.drag-handle.options.open-node-comment") }}
					</span>
				</div>
			</ShadcnUiDropdownMenuItem>
		</template>
		<ShadcnUiDropdownMenuItem v-else-if="canAddComment" @click="addNodeComment">
			<div class="flex w-full items-center gap-1">
				<Icon name="lucide:message-square-plus" />
				<span>
					{{ $t("editor.drag-handle.options.add-node-comment") }}
				</span>
			</div>
		</ShadcnUiDropdownMenuItem>
		<ShadcnUiDropdownMenuItem v-if="!isEditingDisabled" @click="deleteBlock">
			<div class="flex w-full items-center gap-1">
				<Icon name="lucide:trash-2" />
				<span>
					{{ $t("editor.drag-handle.options.delete-block") }}
				</span>
			</div>
		</ShadcnUiDropdownMenuItem>
	</div>
</template>

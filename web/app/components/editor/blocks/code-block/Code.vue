<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import CodeButtons from "./CodeButtons.vue"
import { cn } from "~/lib/utils"
import { explicitContentPlaceholder } from "../../placeholder"
import { TITLED_CODE_BLOCK_NAME } from "../node-names"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

// we keep the <pre> class here to make the <pre> element short, because
// we cannot format it/its content with newlines as that would insert
// extra whitespaces
const diffStatus = computed(() => props.node.attrs.diffStatus)

const preClass = computed(() =>
	cn(
		"not-prose group relative flex flex-col rounded-lg",
		diffStatus.value !== DiffStatus.Added &&
			diffStatus.value !== DiffStatus.Removed &&
			"bg-muted!",
		"font-mono font-normal text-foreground!",
		// hide the default placeholder
		"before:hidden!",
		props.extension.options.type === "comment" ? "text-2sm" : "text-sm",
	),
)

const codeContentClass = cn(
	"block py-3 pl-4 w-full",
	"whitespace-pre min-w-max break-normal",
	props.extension.options.type === "comment" ? "min-h-9" : "min-h-11",
)

const placeholderText = computed(() => {
	return !props.node.textContent.length
		? explicitContentPlaceholder(t, props.node.type.name, isEditingDisabled)
		: ""
})

const contentProps = computed(() => {
	return {
		class: codeContentClass,
	}
})

const isActive = computed(() => {
	const { state } = props.editor
	const pos = props.getPos?.()
	if (typeof pos !== "number") {
		return false
	}

	const selection = state.selection
	return selection.from >= pos && selection.to <= pos + props.node.nodeSize
})

const isTitledCodeBlock = computed(() => {
	const pos = props.getPos()
	if (typeof pos !== "number") {
		return false
	}

	const parent = props.editor.state.doc.resolve(pos).node().type.name
	return parent === TITLED_CODE_BLOCK_NAME
})
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="pre"
		:class="
			cn(
				preClass,
				isTitledCodeBlock && 'rounded-t-none border border-t-0 border-border',
				!isTitledCodeBlock && 'border-0',
			)
		"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<div class="relative overflow-x-scroll">
			<div
				v-if="placeholderText"
				class="pointer-events-none absolute top-3 left-0 pl-4 text-muted-foreground/60"
			>
				{{ placeholderText }}
			</div>
			<NodeViewContent as="code" v-bind="contentProps" />
		</div>
		<CodeButtons
			v-bind="props"
			:class="
				cn(
					'transition-opacity',
					isActive || 'group-hover:pointer-events-auto group-hover:opacity-100',
					isActive
						? 'pointer-events-auto opacity-100'
						: 'pointer-events-none opacity-0',
				)
			"
		/>
	</NodeViewWrapper>
</template>

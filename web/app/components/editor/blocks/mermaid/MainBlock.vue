<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import { explicitContentPlaceholder } from "../../placeholder"
import { DiffStatus } from "../../diff/position-map"
import DiffChangeMarker from "../../diff/DiffChangeMarker.vue"
import MermaidPreview from "./MermaidPreview.vue"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()
const { editingUsersRef } = useCollaborationAwareness(() => props.editor)

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

const uid = computed(() => props.node.attrs.uid as string)

const showCode = computed({
	get: () => editorStore.mermaidBlockShowCode[uid.value] ?? false,
	set: (v: boolean) => {
		editorStore.setMermaidBlockShowCode(uid.value, v)
	},
})

// open the code section by default when the block is empty.
if (
	!props.node.textContent.length &&
	!(uid.value in editorStore.mermaidBlockShowCode)
) {
	editorStore.setMermaidBlockShowCode(uid.value, true)
}

const placeholderText = computed(() => {
	return !props.node.textContent.length
		? explicitContentPlaceholder(t, props.node.type.name, isEditingDisabled)
		: ""
})

const otherEditingUsers = editingUsersRef(uid)
const lastOtherEditingUser = computed(() => {
	return otherEditingUsers.value.length
		? otherEditingUsers.value[otherEditingUsers.value.length - 1]
		: null
})
const showCollaboratorIndicator = computed(() => {
	return (
		!showCode.value && !!lastOtherEditingUser.value && !isEditingDisabled.value
	)
})

const diffStatus = computed(
	() => props.node.attrs.diffStatus as DiffStatus | null,
)

const diffClass = computed(() => {
	if (diffStatus.value === DiffStatus.Added) {
		return "diff-added"
	}

	if (diffStatus.value === DiffStatus.Removed) {
		return "diff-removed"
	}

	return null
})

// in diff mode the node content is merged with both added and
// removed marks. use modifiedTextContent (set during inline diff
// expansion) so the preview only renders the modified version.
const previewSource = computed(() => {
	return (
		(props.node.attrs.modifiedTextContent as string | undefined) ??
		props.node.textContent
	)
})

watch(showCode, () => {
	void nextTick(() => {
		props.editor.commands.refreshTextCommentIndicators()
	})
})
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		:class="
			cn(
				'not-prose group @container relative flex flex-col rounded-md border border-border',
				showCollaboratorIndicator && 'rounded-tl-none',
				diffClass,
			)
		"
		:style="{
			borderColor: showCollaboratorIndicator
				? lastOtherEditingUser?.color
				: undefined,
		}"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<div
			v-if="showCollaboratorIndicator"
			:class="
				cn(
					'pointer-events-none absolute -top-4 -left-px z-5 rounded-sm px-[0.1rem] text-xs font-medium whitespace-nowrap text-white caret-transparent select-none',
				)
			"
			:style="{
				backgroundColor: lastOtherEditingUser?.color ?? undefined,
			}"
		>
			{{ lastOtherEditingUser?.name }}
		</div>
		<div class="absolute top-1 right-1.5 z-1 flex items-center gap-1">
			<DiffChangeMarker :node="props.node" />
			<ShadcnUiButton
				variant="ghost-plain"
				size="icon-sm"
				:aria-pressed="showCode"
				:class="cn('size-6')"
				type="button"
				@click="showCode = !showCode"
			>
				<Icon :name="showCode ? 'lucide:eye' : 'lucide:eye-off'" />
			</ShadcnUiButton>
		</div>
		<div
			:class="
				cn(
					'flex min-h-24',
					'flex-col divide-y divide-border @3xl:flex-row @3xl:divide-x @3xl:divide-y-0',
					!showCode && '@3xl:flex-col',
				)
			"
		>
			<div
				v-show="showCode"
				:class="cn('relative overflow-x-auto', showCode && 'flex-1')"
			>
				<div
					v-if="placeholderText"
					class="pointer-events-none absolute top-3 left-0 pl-4 text-muted-foreground/60"
				>
					{{ placeholderText }}
				</div>
				<!-- prettier-ignore -->
				<pre class="m-0 min-w-max px-4 py-3"><NodeViewContent
						as="code"
						:class="
							cn(
								'block w-full break-normal',
								'min-h-24 font-mono text-sm text-foreground',
							)
						"
					/></pre>
			</div>
			<div
				contenteditable="false"
				:class="cn('px-4 py-3', showCode && 'flex-1')"
			>
				<MermaidPreview
					:source="previewSource"
					:uid="props.node.attrs.uid as string"
				/>
			</div>
		</div>
	</NodeViewWrapper>
</template>

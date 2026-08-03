<script lang="ts" setup>
import { cn } from "~/lib/utils"

interface Props {
	icon: string | null
	size?: "icon" | "icon-xsm"
	isModified?: boolean
}

const props = withDefaults(defineProps<Props>(), {
	size: "icon",
})
const emit = defineEmits<{
	(e: "select", v: string): void
}>()

const { isEditable } = useEditorMeta()
const { openIconPicker } = useIconPicker()
const editorStore = useEditorStore()

const buttonElem = useTemplateRef("icon-picker-trigger")

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

function handleClick() {
	if (isEditingDisabled.value || !buttonElem.value) {
		return
	}

	openIconPicker(buttonElem.value, (icon) => {
		emit("select", icon)
	})
}
</script>

<template>
	<div ref="icon-picker-trigger">
		<ShadcnUiButton
			:data-disabled="!props.icon || isEditingDisabled ? '' : undefined"
			:data-small="props.size === 'icon-xsm'"
			:class="
				cn(
					'mt-0.5 shrink-0 rounded-md data-disabled:pointer-events-none data-[small=false]:size-7 data-[small=true]:size-5',
					props.isModified && 'bg-diff-added',
				)
			"
			:size="props.size"
			variant="ghost"
			@click="handleClick"
		>
			<Icon v-if="props.icon" class="size-6.5" :name="props.icon" />
		</ShadcnUiButton>
	</div>
</template>

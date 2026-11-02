<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import { LogOrder } from "../utils"

const order = defineModel<LogOrder>({ required: true })
const props = defineProps<{
	isModified?: boolean
	oldOrder?: LogOrder | null
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const options = [
	{ value: LogOrder.NewestFirst, icon: "mingcute:sort-descending-line" },
	{ value: LogOrder.OldestFirst, icon: "mingcute:sort-ascending-line" },
]

function select(value: LogOrder) {
	if (!isEditingDisabled.value) {
		order.value = value
	}
}
</script>
<template>
	<div class="flex gap-1">
		<button
			v-for="option in options"
			:key="option.value"
			type="button"
			:aria-pressed="order === option.value"
			:aria-disabled="isEditingDisabled"
			:class="
				cn(
					'flex flex-1 flex-col items-center justify-center gap-0.5 rounded-md border py-1 opacity-100 transition-all duration-150 select-none',
					'pointer-events-auto cursor-pointer',
					order === option.value && !props.isModified
						? 'cursor-default border-primary'
						: 'hover:opacity-70 active:opacity-90',
					isEditingDisabled && 'pointer-events-none select-text',
					order === option.value && props.isModified && 'bg-diff-field-added',
					props.oldOrder === option.value &&
						props.isModified &&
						'bg-diff-field-removed',
				)
			"
			@click="select(option.value)"
		>
			<Icon :name="option.icon" class="size-5 shrink-0" />
			<span class="text-2sm text-foreground">
				{{ $t(`editor.logs.config.order-options.${option.value}`) }}
			</span>
		</button>
	</div>
</template>

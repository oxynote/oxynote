<script lang="ts" setup>
import { cn } from "~/lib/utils"

const props = defineProps<{
	commentType: "node" | "text"
	top: number
	left: number
	forcedHighlight: boolean
	hovered: boolean
}>()

const { isScrolling } = useWindowScroll()

const offsets = computed(() => {
	if (props.commentType === "node") {
		return {
			top: -8,
			topHovered: -14,
			left: -6,
		}
	}

	return {
		top: -12,
		topHovered: -16,
		left: -6,
	}
})
</script>
<template>
	<div
		:data-hovered="props.hovered || props.forcedHighlight ? '' : undefined"
		:class="
			cn(
				'pointer-events-auto absolute z-editor-comment-indicators flex size-fit cursor-pointer rounded-full bg-background opacity-80 duration-150 data-hovered:opacity-100',
				isScrolling ? 'transition-none' : 'transition-[top,left,opacity]',
			)
		"
		:style="{
			top: `${props.hovered || props.forcedHighlight ? props.top + offsets.topHovered : props.top + offsets.top}px`,
			left: `${props.left + offsets.left}px`,
		}"
	>
		<div
			:class="
				cn(
					'flex items-center justify-center rounded-full border',
					'border-comment-highlight/60 bg-comment-highlight/12 p-0.75 dark:border-comment-highlight/45',
				)
			"
		>
			<Icon
				name="mingcute:message-4-fill"
				class="relative size-3 text-foreground"
			/>
		</div>
	</div>
</template>

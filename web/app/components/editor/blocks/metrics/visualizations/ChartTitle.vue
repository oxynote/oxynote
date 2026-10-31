<script lang="ts" setup>
// the title stays out of the echarts option, which would otherwise change
// with every width the chart passes through while it resizes. It comes after
// the chart, so it paints over the canvas but under the block's controls.
const props = defineProps<{
	title: string
	rightPadding?: number
}>()
</script>

<template>
	<div
		class="pointer-events-none absolute top-[0.5625rem] left-2.5 truncate text-2base leading-[1.0625rem] text-foreground"
		:style="{
			maxWidth: `max(calc(90% - ${props.rightPadding ?? 0}px), 3.125rem)`,
		}"
	>
		{{ props.title }}
	</div>
</template>

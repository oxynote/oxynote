<script lang="ts" setup>
import { cn } from "~/lib/utils"

const props = defineProps<{
	item: { icon?: string; title: string; shortcut?: string; disabled?: boolean }
	itemIndex: number | null
	selectedIndex: number | null
}>()
const emit = defineEmits<{
	(e: "click" | "hover"): void
}>()
</script>

<template>
	<button
		:data-selected="props.selectedIndex === props.itemIndex ? '' : undefined"
		:data-disabled="props.item.disabled ? '' : undefined"
		:class="
			cn(
				'relative flex w-full cursor-pointer items-center justify-between gap-4.5 rounded px-2 py-2 text-2sm outline-hidden select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
				'data-[selected]:bg-accent/50 data-[selected]:text-accent-foreground data-[selected]:active:bg-accent data-[selected]:active:text-accent-foreground',
			)
		"
		@click="emit('click')"
		@mouseover="emit('hover')"
	>
		<div class="flex items-center gap-2">
			<Icon
				v-if="props.item.icon"
				:name="props.item.icon"
				class="size-[1.2em]"
			/>
			<span>
				{{ props.item.title }}
			</span>
		</div>
		<div
			v-if="props.item.shortcut"
			class="text-[0.625rem] text-muted-foreground/50"
		>
			{{ props.item.shortcut }}
		</div>
	</button>
</template>

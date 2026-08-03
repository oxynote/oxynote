<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
	class?: HTMLAttributes["class"]
	disableFocusEffect?: boolean
	disableDestructiveEffect?: boolean
}>()
</script>

<template>
	<div
		data-slot="input-group"
		role="group"
		:class="
			cn(
				'group/input-group relative flex w-full items-center rounded-md border border-input bg-transparent shadow-none transition-[color,box-shadow] outline-none',
				'h-9 min-w-0 has-[>textarea]:h-auto',

				// Borders for non-last children.
				'[&>*:not(:last-child)]:border-r [&>*:not(:last-child)]:border-input',

				// Variants based on alignment.
				'has-[>[data-align=inline-start]]:[&>input]:pl-2',
				'has-[>[data-align=inline-end]]:[&>input]:pr-2',
				'has-[>[data-align=block-start]]:h-auto has-[>[data-align=block-start]]:flex-col has-[>[data-align=block-start]]:[&>*:not(:last-child)]:border-r-0 has-[>[data-align=block-start]]:[&>*:not(:last-child)]:border-b has-[>[data-align=block-start]]:[&>input]:pb-3',
				'has-[>[data-align=block-end]]:h-auto has-[>[data-align=block-end]]:flex-col has-[>[data-align=block-end]]:[&>*:not(:last-child)]:border-r-0 has-[>[data-align=block-end]]:[&>*:not(:last-child)]:border-b has-[>[data-align=block-end]]:[&>input]:pt-3',

				// Focus state.
				!props.disableFocusEffect &&
					'has-[[data-slot=input-group-control]:focus-visible]:border-ring has-[[data-slot=input-group-control]:focus-visible]:ring-[3px] has-[[data-slot=input-group-control]:focus-visible]:ring-ring/50',

				// Error state.
				!props.disableDestructiveEffect &&
					'has-[[data-slot][aria-invalid=true]]:border-destructive has-[[data-slot][aria-invalid=true]]:ring-destructive/20 dark:has-[[data-slot][aria-invalid=true]]:ring-destructive/40',

				props.class,
			)
		"
	>
		<slot />
	</div>
</template>

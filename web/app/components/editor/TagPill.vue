<script lang="ts" setup>
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
	name: string
	// a pill without a colour is neutral: the counter standing in for the
	// tags that did not fit
	color?: string
	class?: HTMLAttributes["class"]
}>()

// the two modes get different tints, so both are handed to css as custom
// properties and the dark variant picks its own
const treatment = computed(() =>
	props.color ? tagColorFor(props.color) : null,
)
</script>

<template>
	<span
		:class="
			cn(
				// note that the dark mode has a border, which means the
				// light mode has to account for that in its padding to keep
				// the pill the same size
				'flex max-w-45 items-center rounded-full px-1.75 py-0.5 text-xs leading-4 font-semibold dark:border dark:py-px',
				treatment
					? [
							'bg-(--tag-bg) text-(--tag-fg)',
							'dark:border-(--tag-dark-border) dark:bg-(--tag-dark-bg) dark:text-(--tag-dark-fg)',
						]
					: 'bg-muted text-muted-foreground dark:border-border',
				props.class,
			)
		"
		:style="
			treatment
				? {
						'--tag-bg': treatment.lightBg,
						'--tag-fg': treatment.lightFg,
						'--tag-dark-bg': treatment.darkBg,
						'--tag-dark-border': treatment.darkBorder,
						'--tag-dark-fg': treatment.darkFg,
					}
				: undefined
		"
	>
		<span class="truncate">{{ props.name }}</span>
	</span>
</template>

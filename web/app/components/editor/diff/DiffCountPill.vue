<script lang="ts" setup>
import { cn } from "~/lib/utils"

const props = defineProps<{
	removed: number
	added: number
	vertical?: boolean
}>()
const slots = useSlots()

// a stacked cell pads its outer edge more than an edge it shares, or two
// cells meeting would add their padding into one wide gap
const removedPadding = computed(() =>
	cn(slots.default ? "pt-0.5" : "pt-0.75", props.added ? "pb-0.5" : "pb-0.75"),
)
const addedPadding = computed(() =>
	cn(slots.default || props.removed ? "pt-0.5" : "pt-0.75", "pb-0.75"),
)
</script>

<template>
	<span
		:class="
			cn(
				'flex overflow-hidden rounded-full border border-border bg-background text-[0.625rem] leading-3.5 font-semibold select-none',
				// a stack sits in the page margin, which is narrow below lg
				props.vertical && 'flex-col max-lg:text-[0.5625rem]',
			)
		"
	>
		<!-- a lead-in the pill carries before its counts -->
		<slot />
		<span class="sr-only">
			{{
				$t("editor.diff-change-marker.label", {
					removed: props.removed,
					added: props.added,
				})
			}}
		</span>
		<span
			v-if="props.removed"
			aria-hidden="true"
			:class="
				cn(
					'bg-diff-removed text-center text-diff-removed-foreground',
					props.vertical ? ['px-0.25 lg:px-0.75', removedPadding] : 'px-1.5',
				)
			"
		>
			{{ $t("editor.diff-change-marker.removed", { count: props.removed }) }}
		</span>
		<span
			v-if="props.added"
			aria-hidden="true"
			:class="
				cn(
					'bg-diff-added text-center text-diff-added-foreground',
					props.vertical ? ['px-0.25 lg:px-0.75', addedPadding] : 'px-1.5',
				)
			"
		>
			{{ $t("editor.diff-change-marker.added", { count: props.added }) }}
		</span>
	</span>
</template>

<script lang="ts" setup>
import { cn } from "~/lib/utils"
import DiffCountPill from "../diff/DiffCountPill.vue"
import type { HookChangeCount } from "./hook-diff"
import { HOOK_ICON_STATUS_CLASS, type HookGroupStatus } from "./hook-status"

const props = defineProps<{
	count: HookChangeCount
	// the hook icon takes the colour the hook button would have
	hookStatus: HookGroupStatus | null
	// the handle shows in the marker's spot, so the marker moves below it
	lifted: boolean
}>()

const isMinWidth1024px = useMediaQuery("(min-width: 1024px)")

// the hook icon stands in for the hook button, so it gives way once the
// button shows. The narrow handle has no hook button, so there it stays
const showHookIcon = computed(() => !props.lifted || !isMinWidth1024px.value)
</script>

<template>
	<div
		:class="
			cn(
				'pointer-events-none transition-transform duration-100 will-change-transform',
				props.lifted && 'translate-y-5.5 lg:translate-y-6',
			)
		"
	>
		<!--
			the hook icon has to sit on a whole pixel, or it jumps by one when the
			move ends. So the marker keeps a layer of its own rather than
			getting one only while it moves, and the pill's minimum width is
			a whole number of pixels
		-->
		<DiffCountPill
			:removed="props.count.removed"
			:added="props.count.added"
			vertical
			class="min-w-4 lg:min-w-5"
		>
			<!--
				the slot collapses to nothing once lifted, which padding would
				stop, so the icon's own margin pads the top
			-->
			<span
				aria-hidden="true"
				:class="
					cn(
						'flex h-3.75 items-start justify-center overflow-hidden transition-[height,opacity] duration-100 lg:h-4.25',
						!showHookIcon && 'h-0 opacity-0 lg:h-0',
					)
				"
			>
				<Icon
					name="mingcute:leaf-line"
					:data-hook-status="props.hookStatus"
					:class="
						cn(
							'mt-0.75 size-2.5 shrink-0 text-foreground/50 lg:size-3',
							HOOK_ICON_STATUS_CLASS,
						)
					"
				/>
			</span>
		</DiffCountPill>
	</div>
</template>

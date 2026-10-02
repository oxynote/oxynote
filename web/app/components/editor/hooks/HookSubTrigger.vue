<script lang="ts" setup>
import { cn } from "~/lib/utils"
import DiffCountPill from "../diff/DiffCountPill.vue"
import { DiffStatus } from "../diff/position-map"
import { hookChangeCount, type HookDiffContext } from "./hook-diff"

const props = defineProps<{
	hook?: DocumentHook | null
	// set only while the diff is shown
	diff?: HookDiffContext | null
}>()

const diffStatus = computed(() => props.diff?.status ?? null)
const count = computed(() =>
	props.hook && props.diff?.targetHook
		? hookChangeCount(props.hook, props.diff.targetHook)
		: null,
)

const rowClass = computed(() =>
	cn(
		// the label's line is taller than the menu's text, so truncating it
		// does not clip the descenders. The row's padding gives that back
		"py-1",
		// a tinted row deepens its own tint on hover, rather than taking the
		// grey the other rows use
		diffStatus.value === DiffStatus.Added &&
			"bg-diff-added/30 focus:bg-diff-added/50 data-[state=open]:bg-diff-added/50",
		diffStatus.value === DiffStatus.Removed &&
			"bg-diff-removed/30 focus:bg-diff-removed/50 data-[state=open]:bg-diff-removed/50",
	),
)
</script>

<template>
	<ShadcnUiDropdownMenuSubTrigger :class="rowClass">
		<!-- every row keeps the sign's column so the icons line up -->
		<span
			v-if="props.diff?.signColumn"
			aria-hidden="true"
			:class="
				cn(
					'mr-0.5 w-2 shrink-0 text-center font-semibold',
					diffStatus === DiffStatus.Added && 'text-diff-added-foreground',
					diffStatus === DiffStatus.Removed && 'text-diff-removed-foreground',
				)
			"
		>
			<template v-if="diffStatus === DiffStatus.Added">
				{{ $t("editor.hooks.diff.added-sign") }}
			</template>
			<template v-else-if="diffStatus === DiffStatus.Removed">
				{{ $t("editor.hooks.diff.removed-sign") }}
			</template>
		</span>
		<span
			:class="
				cn(
					'flex min-w-0 flex-1 items-center gap-1.5 leading-4.25',
					diffStatus === DiffStatus.Removed && 'line-through',
				)
			"
		>
			<slot />
		</span>
		<span v-if="diffStatus === DiffStatus.Added" class="sr-only">
			{{ $t("editor.hooks.diff.added") }}
		</span>
		<span v-else-if="diffStatus === DiffStatus.Removed" class="sr-only">
			{{ $t("editor.hooks.diff.removed") }}
		</span>
		<DiffCountPill
			v-if="diffStatus === DiffStatus.Modified && count"
			:removed="count.removed"
			:added="count.added"
			class="shrink-0"
		/>
	</ShadcnUiDropdownMenuSubTrigger>
</template>

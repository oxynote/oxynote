<script lang="ts" setup>
import { cn } from "~/lib/utils"
import { DiffStatus } from "../diff/position-map"
import type { HookFieldRow } from "./hook-diff"

const props = defineProps<{
	label: string
	rows: HookFieldRow[]
	// the diff status of the whole hook. A hook added or removed marks
	// every value its own way
	diffStatus?: DiffStatus | null
}>()

const shownRows = computed(() => {
	const status = props.diffStatus
	if (status !== DiffStatus.Added && status !== DiffStatus.Removed) {
		return props.rows
	}

	return props.rows.map((row) => ({ value: row.value, status: status }))
})
</script>

<template>
	<div class="mt-0.5 flex flex-col gap-1">
		<div class="text-left text-2sm font-medium text-foreground">
			{{ props.label }}
		</div>
		<div
			v-for="(row, index) in shownRows"
			:key="index"
			role="textbox"
			aria-readonly="true"
			tabindex="0"
			:aria-label="props.label"
			:class="
				cn(
					'flex min-h-[1.775rem] items-center rounded-md border border-input px-2 py-1 text-2sm',
					row.status === DiffStatus.Added && 'bg-diff-field-added',
					row.status === DiffStatus.Removed &&
						'bg-diff-field-removed text-muted-foreground line-through',
				)
			"
		>
			<span class="min-w-0 break-words">{{ row.value }}</span>
		</div>
	</div>
</template>

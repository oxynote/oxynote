<script lang="ts" setup>
import { cn } from "~/lib/utils"
import { DiffStatus } from "../diff/position-map"
import type { HookFieldRow } from "./hook-diff"

const props = defineProps<{
	label: string
	rows: HookFieldRow[]
}>()
</script>

<template>
	<div class="mt-0.5 flex flex-col gap-1">
		<div class="text-left text-2sm font-medium text-foreground">
			{{ props.label }}
		</div>
		<ShadcnUiInput
			v-for="(row, index) in props.rows"
			:key="index"
			:model-value="row.value"
			:aria-label="props.label"
			transparent-disable
			disable-focus-effect
			disable-destructive-effect
			:class="
				cn(
					'h-[1.775rem] px-2 text-2sm md:text-2sm',
					row.status === DiffStatus.Added && 'bg-diff-field-added',
					row.status === DiffStatus.Removed && 'bg-diff-field-removed',
				)
			"
		/>
	</div>
</template>

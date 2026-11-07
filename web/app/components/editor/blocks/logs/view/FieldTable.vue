<script lang="ts" setup>
import DisabledHint from "./DisabledHint.vue"
import type { LogFilter } from "../dialect"
import type { FieldRow } from "../rows"

const props = defineProps<{
	entries: FieldRow[]
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()

function filter(row: FieldRow, exclude: boolean) {
	if (row.filter) {
		emit("filter", { ...row.filter, exclude: exclude })
	}
}
</script>
<template>
	<div
		class="divide-y divide-muted-highlight overflow-hidden rounded-md border bg-card"
	>
		<div
			v-if="!props.entries.length"
			class="flex h-7.5 items-center px-2.5 text-2sm text-muted-foreground"
		>
			{{ $t("editor.logs.view.section-empty") }}
		</div>
		<div
			v-for="(row, index) in props.entries"
			:key="index"
			class="flex h-7.5 items-center"
		>
			<div
				class="flex h-full w-31 shrink-0 items-center border-r border-muted-highlight px-2.5 text-2sm text-muted-foreground @xl:w-44"
			>
				<span class="truncate">{{ row.key }}</span>
			</div>
			<div
				class="min-w-0 flex-1 truncate px-2.5 text-2sm text-foreground tabular-nums"
			>
				{{ row.value }}
			</div>
			<component
				:is="row.filter ? 'div' : DisabledHint"
				v-bind="row.filter ? {} : { reason: row.disabledReason }"
				class="flex shrink-0 items-center pr-0.5"
			>
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="size-6 text-muted-foreground/70 hover:text-muted-foreground"
					:disabled="!row.filter"
					@click="filter(row, false)"
				>
					<Icon name="mingcute:zoom-in-line" />
					<span class="sr-only">
						{{ $t("editor.logs.view.include-value", { key: row.key }) }}
					</span>
				</ShadcnUiButton>
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="size-6 text-muted-foreground/70 hover:text-muted-foreground"
					:disabled="!row.filter"
					@click="filter(row, true)"
				>
					<Icon name="mingcute:zoom-out-line" />
					<span class="sr-only">
						{{ $t("editor.logs.view.exclude-value", { key: row.key }) }}
					</span>
				</ShadcnUiButton>
			</component>
		</div>
	</div>
</template>

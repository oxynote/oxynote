<script lang="ts" setup>
import type { LogFilter } from "../logql"

const INCLUDE_MARK = "="
const EXCLUDE_MARK = "≠"

const filters = defineModel<LogFilter[]>({ required: true })

function remove(filter: LogFilter) {
	filters.value = filters.value.filter((f) => f !== filter)
}
</script>
<template>
	<div class="flex shrink-0 flex-wrap items-center gap-1.5 px-2.5 pb-2">
		<span
			v-for="filter in filters"
			:key="filter.key"
			class="inline-flex h-6 max-w-full items-center gap-1 rounded-md border bg-muted pr-0.5 pl-1.5 font-mono text-xs text-foreground"
		>
			<span class="shrink-0 text-muted-foreground">{{ filter.key }}</span>
			<span class="shrink-0">
				{{ filter.exclude ? EXCLUDE_MARK : INCLUDE_MARK }}
			</span>
			<span class="truncate">{{ filter.value }}</span>
			<button
				type="button"
				class="flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				@click="remove(filter)"
			>
				<Icon name="mingcute:close-line" class="size-3" />
				<span class="sr-only">
					{{ $t("editor.logs.view.remove-filter", { key: filter.key }) }}
				</span>
			</button>
		</span>
		<button
			type="button"
			class="cursor-pointer text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
			@click="filters = []"
		>
			{{ $t("editor.logs.view.clear-filters") }}
		</button>
	</div>
</template>

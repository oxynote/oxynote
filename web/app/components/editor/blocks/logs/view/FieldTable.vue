<script lang="ts" setup>
import type { KeyValue } from "../lines"
import type { LogFilter } from "../logql"

const props = defineProps<{
	entries: KeyValue[]
	indexed: boolean
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()

function filter(key: string, value: string, exclude: boolean) {
	emit("filter", {
		key: key,
		value: value,
		indexed: props.indexed,
		exclude: exclude,
	})
}
</script>
<template>
	<div
		class="divide-y divide-muted-highlight overflow-hidden rounded-md border bg-card"
	>
		<div
			v-for="[key, value] in props.entries"
			:key="key"
			class="group flex h-7.5 items-center"
		>
			<div
				class="flex h-full w-31 shrink-0 items-center border-r border-muted-highlight px-2.5 font-mono text-xs text-muted-foreground @xl:w-44"
			>
				<span class="truncate">{{ key }}</span>
			</div>
			<div
				class="min-w-0 flex-1 truncate px-2.5 font-mono text-xs text-foreground tabular-nums"
			>
				{{ value }}
			</div>
			<div class="flex shrink-0 items-center pr-0.5">
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="size-6 text-muted-foreground/70 group-hover:text-muted-foreground"
					@click="filter(key, value, false)"
				>
					<Icon name="mingcute:zoom-in-line" />
					<span class="sr-only">
						{{ $t("editor.logs.view.include-value", { key: key }) }}
					</span>
				</ShadcnUiButton>
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="size-6 text-muted-foreground/70 group-hover:text-muted-foreground"
					@click="filter(key, value, true)"
				>
					<Icon name="mingcute:zoom-out-line" />
					<span class="sr-only">
						{{ $t("editor.logs.view.exclude-value", { key: key }) }}
					</span>
				</ShadcnUiButton>
			</div>
		</div>
	</div>
</template>

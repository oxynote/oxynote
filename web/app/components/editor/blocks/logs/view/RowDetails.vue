<script lang="ts" setup>
import FieldTable from "./FieldTable.vue"
import type { LogFilter } from "../logql"
import { sortedEntries, type KeyValue } from "../lines"

// how long the copy button shows that it copied
const COPIED_MS = 700

const props = defineProps<{
	line: LogLine
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()

const copied = ref(false)
const labels = computed<KeyValue[]>(() => sortedEntries(props.line.labels))
const fields = computed<KeyValue[]>(() => sortedEntries(props.line.fields))

async function copyLine() {
	try {
		await navigator.clipboard.writeText(props.line.line)
	} catch {
		// the browser refused the clipboard, so there is no copy to show
		return
	}

	copied.value = true
	setTimeout(() => (copied.value = false), COPIED_MS)
}
</script>
<template>
	<div
		class="flex cursor-default flex-col gap-3 border-t border-muted-highlight bg-muted px-2.5 pt-2.5 pb-4 select-text"
	>
		<section class="flex flex-col gap-1.5">
			<div class="flex items-center gap-2">
				<h4
					class="text-2xs font-semibold tracking-wide text-muted-foreground uppercase"
				>
					{{ $t("editor.logs.view.line-heading") }}
				</h4>
				<ShadcnUiButton
					variant="outline"
					size="custom"
					class="ml-auto h-6 gap-1 px-2 text-xs"
					@click="copyLine"
				>
					<Icon
						:name="copied ? 'mingcute:check-line' : 'mingcute:copy-2-line'"
						class="size-3.5"
					/>
					{{
						copied
							? $t("editor.logs.view.line-copied")
							: $t("editor.logs.view.copy-line")
					}}
				</ShadcnUiButton>
			</div>
			<div
				class="rounded-md border bg-card px-3 py-2 font-mono text-xs leading-5 break-all whitespace-pre-wrap text-foreground"
			>
				{{ props.line.line }}
			</div>
		</section>
		<section v-if="labels.length" class="flex flex-col gap-1.5">
			<div class="flex min-w-0 items-baseline gap-2">
				<h4
					class="shrink-0 text-2xs font-semibold tracking-wide text-muted-foreground uppercase"
				>
					{{ $t("editor.logs.view.indexed-heading") }}
				</h4>
				<span class="truncate text-2xs text-muted-foreground/70">
					{{ $t("editor.logs.view.indexed-description") }}
				</span>
			</div>
			<FieldTable
				:entries="labels"
				indexed
				@filter="(filter) => emit('filter', filter)"
			/>
		</section>
		<section v-if="fields.length" class="flex flex-col gap-1.5">
			<div class="flex min-w-0 items-baseline gap-2">
				<h4
					class="shrink-0 text-2xs font-semibold tracking-wide text-muted-foreground uppercase"
				>
					{{ $t("editor.logs.view.unindexed-heading") }}
				</h4>
				<span class="truncate text-2xs text-muted-foreground/70">
					{{ $t("editor.logs.view.unindexed-description") }}
				</span>
			</div>
			<FieldTable
				:entries="fields"
				:indexed="false"
				@filter="(filter) => emit('filter', filter)"
			/>
		</section>
	</div>
</template>

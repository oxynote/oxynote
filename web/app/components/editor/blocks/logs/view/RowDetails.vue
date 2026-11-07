<script lang="ts" setup>
import FieldTable from "./FieldTable.vue"
import type { LogFilter } from "../logql"
import { sortedEntries, type KeyValue } from "../rows"

// how long the copy button shows that it copied
const COPIED_MS = 700

const props = defineProps<{
	log: Log
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()

const copied = ref(false)
const labels = computed<KeyValue[]>(() => sortedEntries(props.log.labels))
// a parsed key wins over a field of the same name
const fields = computed<KeyValue[]>(() =>
	sortedEntries({ ...props.log.fields, ...props.log.parsed }),
)

async function copyLine() {
	try {
		await navigator.clipboard.writeText(props.log.line)
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
			<h4
				class="text-2xs font-semibold tracking-wide text-muted-foreground uppercase"
			>
				{{ $t("editor.logs.view.line-heading") }}
			</h4>
			<div
				class="relative rounded-md border bg-card py-2 pr-9 pl-3 text-2sm leading-5 break-all whitespace-pre-wrap text-foreground"
			>
				<span>{{ props.log.line }}</span>
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="absolute top-1.5 right-1.5 size-6 text-muted-foreground hover:text-foreground"
					@click="copyLine"
				>
					<Icon
						:name="copied ? 'mingcute:check-line' : 'mingcute:copy-2-line'"
					/>
					<span class="sr-only">
						{{
							copied
								? $t("editor.logs.view.line-copied")
								: $t("editor.logs.view.copy-line")
						}}
					</span>
				</ShadcnUiButton>
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

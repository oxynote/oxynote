<script lang="ts" setup>
import FieldTable from "./FieldTable.vue"
import type { LineFormat, LogDialect, LogFilter } from "../dialect"
import {
	extractedFields,
	sortedEntries,
	type ExtractedField,
	type FieldRow,
} from "../rows"

// how long the copy button shows that it copied
const COPIED_MS = 700

const props = defineProps<{
	log: Log
	dialect: LogDialect
	lineFilterFormats: LineFormat[]
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()

const { t } = useI18n({ useScope: "global" })

const copied = ref(false)
const sections = computed<
	{ heading: string; description: string; entries: FieldRow[] }[]
>(() => [
	{
		heading: t("editor.logs.view.origin-heading"),
		description: t("editor.logs.view.origin-description"),
		entries: sortedEntries(props.log.labels).map(([key, value]) => ({
			key: key,
			value: value,
			filter: { key: key, value: value, indexed: true },
		})),
	},
	{
		heading: t("editor.logs.view.metadata-heading"),
		description: t("editor.logs.view.metadata-description"),
		entries: sortedEntries(props.log.fields).map(([key, value]) => ({
			key: key,
			value: value,
			filter: { key: key, value: value, indexed: false },
		})),
	},
	{
		heading: t("editor.logs.view.content-heading"),
		description: t("editor.logs.view.content-description"),
		entries: extractedFields(props.log, props.dialect).map(extractedRow),
	},
])

// extractedRow gives a key the query's parser added a plain filter. A key
// read from the line filters by the source's name for it, and brings the
// format of its line.
function extractedRow(field: ExtractedField): FieldRow {
	const row = { key: field.key, value: field.value }
	if (!field.format) {
		return {
			...row,
			filter: { key: field.key, value: field.value, indexed: false },
		}
	}

	if (field.isNull) {
		return {
			...row,
			filter: null,
			disabledReason: t("editor.logs.view.filter-null"),
		}
	}

	if (field.nested) {
		return {
			...row,
			filter: null,
			disabledReason: t("editor.logs.view.filter-nested"),
		}
	}

	if (!props.lineFilterFormats.includes(field.format)) {
		return {
			...row,
			filter: null,
			disabledReason: t("editor.logs.view.filter-query-blocked"),
		}
	}

	return {
		...row,
		filter: {
			key: props.dialect.lineKeyName(field.key, props.log),
			value: field.value,
			indexed: false,
			format: field.format,
		},
	}
}

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
				<span v-if="props.log.line.trim()">{{ props.log.line }}</span>
				<span v-else class="text-muted-foreground">
					{{ $t("editor.logs.view.section-empty") }}
				</span>
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
		<section
			v-for="section in sections"
			:key="section.heading"
			class="flex flex-col gap-1.5"
		>
			<div class="flex min-w-0 items-baseline gap-2">
				<h4
					class="shrink-0 text-2xs font-semibold tracking-wide text-muted-foreground uppercase"
				>
					{{ section.heading }}
				</h4>
				<span
					class="hidden truncate text-2xs text-muted-foreground/70 @xl:block"
				>
					{{ section.description }}
				</span>
			</div>
			<FieldTable
				:entries="section.entries"
				@filter="(filter) => emit('filter', filter)"
			/>
		</section>
	</div>
</template>

<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import { LogTimestampFormat } from "../utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

// the first option of each group of formats, which a line separates from
// the group before
const GROUP_STARTS = new Set([
	LogTimestampFormat.Readable,
	LogTimestampFormat.ReadableTime,
	LogTimestampFormat.Numeric,
	LogTimestampFormat.NumericTime,
])

const format = defineModel<LogTimestampFormat>({ required: true })
const props = defineProps<{
	diffStatus?: DiffStatus | null
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
</script>
<template>
	<ShadcnUiDropdownMenu>
		<ShadcnUiDropdownMenuTrigger as-child :disabled="isEditingDisabled">
			<div
				:class="
					cn(
						'flex w-full min-w-0 cursor-pointer items-center gap-1.5 rounded-md border px-1.5 py-1 transition-opacity duration-150 hover:opacity-60 active:opacity-90',
						isEditingDisabled && 'pointer-events-none select-text',
						props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
						props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
					)
				"
			>
				<div class="flex min-w-0 flex-1 flex-col">
					<span class="truncate text-sm text-foreground">
						{{
							$t(`editor.logs.config.timestamp-format-options.${format}.title`)
						}}
					</span>
					<span class="truncate text-2sm text-muted-foreground">
						{{
							$t(
								`editor.logs.config.timestamp-format-options.${format}.example`,
							)
						}}
					</span>
				</div>
				<Icon
					v-show="!isEditingDisabled"
					name="lucide:chevron-down"
					class="size-4 shrink-0"
				/>
			</div>
		</ShadcnUiDropdownMenuTrigger>
		<ShadcnUiDropdownMenuContent
			class="max-h-[40dvh] w-(--reka-dropdown-menu-trigger-width) min-w-0"
			inside-modal
			side="bottom"
			align="start"
			loop
		>
			<template
				v-for="option in Object.values(LogTimestampFormat)"
				:key="option"
			>
				<ShadcnUiDropdownMenuSeparator v-if="GROUP_STARTS.has(option)" />
				<ShadcnUiDropdownMenuItem
					:value="option"
					:active="option === format"
					class="min-w-0"
					@click="format = option"
				>
					<div class="flex min-w-0 flex-1 flex-col">
						<div
							class="min-w-0 truncate text-2sm whitespace-nowrap text-popover-foreground"
						>
							{{
								$t(
									`editor.logs.config.timestamp-format-options.${option}.title`,
								)
							}}
						</div>
						<div
							class="min-w-0 truncate text-xs whitespace-nowrap text-muted-foreground"
						>
							{{
								$t(
									`editor.logs.config.timestamp-format-options.${option}.example`,
								)
							}}
						</div>
					</div>
				</ShadcnUiDropdownMenuItem>
			</template>
		</ShadcnUiDropdownMenuContent>
	</ShadcnUiDropdownMenu>
</template>

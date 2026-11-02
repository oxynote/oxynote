<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import { LogTimestampFormat } from "../utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

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
						'flex w-full min-w-0 cursor-pointer flex-col rounded-md px-0.75 py-1 transition-opacity duration-150 hover:opacity-60 active:opacity-90',
						isEditingDisabled && 'pointer-events-none select-text',
						props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
						props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
					)
				"
			>
				<div class="truncate text-sm text-foreground">
					{{
						$t(`editor.logs.config.timestamp-format-options.${format}.title`)
					}}
				</div>
				<div class="truncate font-mono text-2sm text-muted-foreground">
					{{
						$t(`editor.logs.config.timestamp-format-options.${format}.example`)
					}}
				</div>
			</div>
		</ShadcnUiDropdownMenuTrigger>
		<ShadcnUiDropdownMenuContent
			class="max-h-[40dvh] w-(--reka-dropdown-menu-trigger-width) min-w-0"
			inside-modal
			side="bottom"
			align="start"
			loop
		>
			<ShadcnUiDropdownMenuItem
				v-for="option in Object.values(LogTimestampFormat)"
				:key="option"
				:value="option"
				:active="option === format"
				class="min-w-0"
				@click="format = option"
			>
				<div class="flex min-w-0 flex-col">
					<div
						class="min-w-0 truncate text-2sm whitespace-nowrap text-popover-foreground"
					>
						{{
							$t(`editor.logs.config.timestamp-format-options.${option}.title`)
						}}
					</div>
					<div
						class="min-w-0 truncate font-mono text-xs whitespace-nowrap text-muted-foreground"
					>
						{{
							$t(
								`editor.logs.config.timestamp-format-options.${option}.example`,
							)
						}}
					</div>
				</div>
			</ShadcnUiDropdownMenuItem>
		</ShadcnUiDropdownMenuContent>
	</ShadcnUiDropdownMenu>
</template>

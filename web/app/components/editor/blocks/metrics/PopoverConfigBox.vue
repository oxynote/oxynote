<script lang="ts" setup>
import type { Editor } from "@tiptap/core"
import { RefreshInterval, TimeRangePreset, type MetricConfig } from "./utils"

const props = defineProps<{
	uid: string
	editor: Editor
	getPos: () => number | undefined
}>()
const config = defineModel<MetricConfig>({ required: true })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

function openModal() {
	// Set selection to this node before opening modal to prevent scroll jump
	// when editor regains focus
	const pos = props.getPos()
	if (typeof pos === "number") {
		props.editor.commands.setNodeSelection(pos)
	}

	editorStore.activateMetricBlockConfig(props.uid)
}
</script>
<template>
	<div class="flex size-full flex-col gap-1 pb-0.75">
		<div class="flex flex-col gap-1 px-0.75">
			<ShadcnUiSelect v-model="config.timeRange">
				<ShadcnUiSelectLabel>
					<span class="text-2sm">
						{{ $t("editor.metrics.config.time-range-label") }}
					</span>
				</ShadcnUiSelectLabel>
				<ShadcnUiSelectTrigger
					class="h-fit! w-full px-1.5 py-1 text-2sm"
					:disable="isEditingDisabled"
				>
					<ShadcnUiSelectValue
						:placeholder="$t('editor.metrics.config.time-range-placeholder')"
					/>
				</ShadcnUiSelectTrigger>
				<ShadcnUiSelectContent class="max-h-[40dvh]">
					<ShadcnUiSelectItem
						v-for="timeRange in Object.values(TimeRangePreset)"
						:key="timeRange"
						:value="timeRange"
					>
						<span class="text-2sm text-popover-foreground">
							{{ $t(`editor.metrics.config.time-range-options.${timeRange}`) }}
						</span>
					</ShadcnUiSelectItem>
				</ShadcnUiSelectContent>
			</ShadcnUiSelect>
		</div>
		<div class="flex flex-col gap-1 px-0.75">
			<ShadcnUiSelect v-model="config.refreshInterval">
				<ShadcnUiSelectLabel>
					<span class="text-2sm">
						{{ $t("editor.metrics.config.refresh-interval-label") }}
					</span>
				</ShadcnUiSelectLabel>
				<ShadcnUiSelectTrigger
					class="h-fit! w-full px-1.5 py-1 text-2sm"
					:disable="isEditingDisabled"
				>
					<ShadcnUiSelectValue
						:placeholder="
							$t('editor.metrics.config.refresh-interval-placeholder')
						"
					/>
				</ShadcnUiSelectTrigger>
				<ShadcnUiSelectContent class="max-h-[40dvh]">
					<ShadcnUiSelectItem
						v-for="refreshInterval in Object.values(RefreshInterval)"
						:key="refreshInterval"
						:value="refreshInterval"
					>
						<span class="text-2sm text-popover-foreground">
							{{
								$t(
									`editor.metrics.config.refresh-interval-options.${refreshInterval}`,
								)
							}}
						</span>
					</ShadcnUiSelectItem>
				</ShadcnUiSelectContent>
			</ShadcnUiSelect>
		</div>
		<div class="-mx-1 my-0.75 h-px bg-border" />
		<ShadcnUiButton
			class="mx-0.75 shrink-0 gap-1"
			size="2sm"
			@click.stop="openModal"
		>
			<Icon name="lucide:square-pen" />
			{{
				!isEditingDisabled
					? $t("editor.metrics.config.modal-trigger-button-normal")
					: $t("editor.metrics.config.modal-trigger-button-readonly")
			}}
		</ShadcnUiButton>
	</div>
</template>

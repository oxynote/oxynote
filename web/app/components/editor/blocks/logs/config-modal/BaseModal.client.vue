<script lang="ts" setup>
import type { LogConfig } from "../utils"
import BottomConfig from "./BottomConfig.vue"
import ConfigSidebar from "./ConfigSidebar.vue"
import DataView from "../view/DataView.vue"
import RefreshIntervalSelect from "../../config-modal-utils/RefreshIntervalSelect.vue"
import TimeRangeSelect from "../../config-modal-utils/TimeRangeSelect.vue"
import { DiffStatus } from "~/components/editor/diff/position-map"

const emit = defineEmits<{
	(e: "open-settings"): void
}>()

const editorStore = useEditorStore()
const isMinWidth1024px = useMediaQuery("(min-width: 1024px)")
const { isEditable } = useEditorMeta()

// getter-only: the reactive config from MainBlock is stored directly in Pinia,
// so property mutations by child components flow through without replacement
const config = computed(() => {
	if (
		!editorStore.activeLogBlockConfig ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return null
	}

	return (
		editorStore.logBlockConfigs[editorStore.activeDocumentId]?.[
			editorStore.activeBranchId
		]?.[editorStore.activeLogBlockConfig] ?? null
	)
})
const diffStatus = computed<DiffStatus | null>(() => {
	if (
		!editorStore.activeLogBlockConfig ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return null
	}

	return (
		editorStore.logBlockDiffStatuses[editorStore.activeDocumentId]?.[
			editorStore.activeBranchId
		]?.[editorStore.activeLogBlockConfig] ?? null
	)
})
const oldConfig = computed<LogConfig | null>(() => {
	if (
		!editorStore.activeLogBlockConfig ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return null
	}

	return (
		editorStore.logBlockOldConfigs[editorStore.activeDocumentId]?.[
			editorStore.activeBranchId
		]?.[editorStore.activeLogBlockConfig] ?? null
	)
})
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const timeOptionsElem = useTemplateRef<HTMLDivElement>("time-options")
// the border box holds the controls' own padding, which the header has to
// clear as well
const { width: timeOptionsWidth } = useElementSize(timeOptionsElem, undefined, {
	box: "border-box",
})

// the size observer reports only after the first paint, and the data
// view's buttons would jump left then. Measuring once the element is in
// the page sets the width before anything is drawn.
watch(
	timeOptionsElem,
	(element) => {
		// a width of 0 means the element is not laid out yet
		if (element?.offsetWidth) {
			timeOptionsWidth.value = element.offsetWidth
		}
	},
	{ flush: "post" },
)
const isDiffModified = computed(() => diffStatus.value === DiffStatus.Modified)
const isTimeRangeModified = computed(() => {
	return (
		isDiffModified.value &&
		!!oldConfig.value &&
		oldConfig.value.timeRange !== config.value?.timeRange
	)
})
const isRefreshIntervalModified = computed(() => {
	return (
		isDiffModified.value &&
		!!oldConfig.value &&
		oldConfig.value.refreshInterval !== config.value?.refreshInterval
	)
})

function updateOpenStatus(open: boolean) {
	if (!open) {
		editorStore.activateLogBlockConfig(null)
	}
}

function openSettings() {
	editorStore.activateLogBlockConfig(null)
	emit("open-settings")
}
</script>
<template>
	<ShadcnUiDialog
		:open="!!editorStore.activeLogBlockConfig"
		@update:open="updateOpenStatus"
	>
		<ShadcnUiDialogContent
			class="max-h-[90dvh] w-[90dvw] overflow-y-auto p-0 text-foreground outline-none md:max-h-[85dvh] lg:w-240"
			@open-auto-focus.prevent
			@interact-outside="
				(event) => {
					const target = event.target as HTMLElement
					if (
						target?.closest('[data-sonner-toaster]') ||
						target?.closest('.cm-tooltip-autocomplete')
					) {
						return event.preventDefault()
					}
				}
			"
		>
			<div class="flex flex-col gap-5 p-6">
				<ShadcnUiDialogHeader>
					<ShadcnUiDialogTitle class="text-2xl">
						{{
							diffStatus
								? $t(`editor.logs.config.modal-title-diff-${diffStatus}`)
								: !isEditingDisabled
									? $t("editor.logs.config.modal-title-normal")
									: $t("editor.logs.config.modal-title-readonly")
						}}
					</ShadcnUiDialogTitle>
					<ShadcnUiDialogDescription class="sr-only">
						{{
							!isEditingDisabled
								? $t("editor.logs.config.modal-description-normal")
								: $t("editor.logs.config.modal-description-readonly")
						}}
					</ShadcnUiDialogDescription>
					<ShadcnUiButton
						variant="ghost-plain"
						class="absolute top-1/2 right-0 -translate-y-1/2 p-0"
						@click="updateOpenStatus(false)"
					>
						<Icon name="lucide:x" size="1.3rem" />
						<span class="sr-only">
							{{ $t("general.modal-close-screen-reader-hint") }}
						</span>
					</ShadcnUiButton>
				</ShadcnUiDialogHeader>
				<div
					v-if="config"
					class="flex flex-col rounded-md border bg-muted/50 lg:h-126 lg:flex-row"
				>
					<div class="flex min-w-0 flex-1 flex-col self-stretch">
						<div class="relative h-82.5 min-w-0 shrink-0 border-b">
							<div
								ref="time-options"
								class="absolute top-1.5 right-1.5 z-1 flex flex-col items-end gap-0.5 pr-1 pl-2 md:top-1 md:h-7 md:flex-row md:items-center md:gap-2 md:bg-muted-50-real"
							>
								<TimeRangeSelect
									v-model="config.timeRange"
									:disabled="isEditingDisabled"
									:old-value="
										isTimeRangeModified ? oldConfig?.timeRange : undefined
									"
								/>
								<RefreshIntervalSelect
									v-model="config.refreshInterval"
									:disabled="isEditingDisabled"
									:old-value="
										isRefreshIntervalModified
											? oldConfig?.refreshInterval
											: undefined
									"
								/>
							</div>
							<DataView
								:config="config"
								:uid="editorStore.activeLogBlockConfig!"
								hide-empty-action-button
								compact
								:header-right-padding="timeOptionsWidth"
							/>
						</div>
						<div v-if="!isMinWidth1024px" class="self-stretch border-b">
							<ConfigSidebar
								:model-value="config"
								:diff-status="diffStatus"
								:old-config="oldConfig"
								@open-settings="openSettings"
							/>
						</div>
						<div class="lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
							<BottomConfig
								:model-value="config"
								:diff-status="diffStatus"
								:old-config="oldConfig"
							/>
						</div>
					</div>
					<div
						v-if="isMinWidth1024px"
						class="w-70 self-stretch border-l lg:overflow-y-auto"
					>
						<ConfigSidebar
							:model-value="config"
							:diff-status="diffStatus"
							:old-config="oldConfig"
							@open-settings="openSettings"
						/>
					</div>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>

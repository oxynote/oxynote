<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type { MetricConfig } from "../utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

// either config or oldConfig must be provided; not both
const config = defineModel<MetricConfig>()
const props = defineProps<{
	diffMode?: boolean
	diffStatus?: DiffStatus | null
	oldConfig?: MetricConfig | null
}>()
const emit = defineEmits<{
	(e: "open-settings"): void
}>()

const { isEditable } = useEditorMeta()
const { fetchDataSources } = useDataSourceAPI()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const selectedDataSource = computed(() => {
	let id: string | null = null

	if (config.value) {
		id = config.value.dataSourceId
	} else if (props.oldConfig) {
		id = props.oldConfig.dataSourceId
	}

	const res = fetchDataSources.state.value.data?.find((v) => v.id === id)
	if (res) {
		return res
	}

	if (id && !res) {
		return "selected-but-not-found"
	}

	return null
})

function handleDataSourceSelection(dataSourceId: string) {
	if (config.value && !isEditingDisabled.value) {
		config.value.dataSourceId = dataSourceId
	}
}
</script>
<template>
	<ShadcnUiDropdownMenu>
		<ShadcnUiDropdownMenuTrigger as-child :disabled="isEditingDisabled">
			<div
				:class="
					cn(
						'flex w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-0.75 py-1 transition-opacity duration-150 hover:opacity-60 active:opacity-90',
						isEditingDisabled && 'pointer-events-none select-text',
						props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
						props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
					)
				"
			>
				<DataSourceStatusIcon
					:data-source="
						selectedDataSource === 'selected-but-not-found'
							? null
							: selectedDataSource
					"
					size="8.5"
				/>
				<div class="flex min-w-0 flex-col">
					<template
						v-if="
							selectedDataSource &&
							selectedDataSource !== 'selected-but-not-found'
						"
					>
						<div class="truncate text-sm text-foreground">
							{{ selectedDataSource.name }}
						</div>
						<div class="truncate text-2sm text-muted-foreground">
							{{ selectedDataSource.url }}
						</div>
					</template>
					<template v-else>
						<div
							v-if="isEditingDisabled"
							class="truncate text-sm text-accent-foreground/30"
						>
							{{
								selectedDataSource === "selected-but-not-found"
									? $t("editor.metrics.config.data-source-label-deleted")
									: $t("editor.metrics.config.data-source-label-missing")
							}}
						</div>
						<template v-else>
							<div class="truncate text-sm text-muted-foreground">
								{{
									selectedDataSource === "selected-but-not-found"
										? $t("editor.metrics.config.data-source-label-deleted")
										: $t("editor.metrics.config.data-source-label-missing")
								}}
							</div>
							<div class="truncate text-xs text-muted-foreground/70">
								{{
									$t("editor.metrics.config.data-source-description-missing")
								}}
							</div>
						</template>
					</template>
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
			<template v-if="!fetchDataSources.state.value.data?.length">
				<i18n-t
					keypath="editor.metrics.config.data-source-no-options.main"
					class="px-1 pt-0.75 pb-1 text-2sm text-popover-foreground"
					tag="div"
				>
					<template #icon>
						<Icon
							name="mingcute:information-fill"
							class="mr-0.5 inline-block -translate-y-px align-middle text-status-info"
						/>
					</template>
					<template #placeholder>
						<ShadcnUiButton
							type="button"
							variant="link"
							size="custom"
							class="h-fit p-0 text-2sm"
							@click="emit('open-settings')"
						>
							{{
								$t("editor.metrics.config.data-source-no-options.placeholder")
							}}
						</ShadcnUiButton>
					</template>
				</i18n-t>
			</template>
			<template v-else>
				<ShadcnUiDropdownMenuItem
					v-for="dataSource in fetchDataSources.state.value.data"
					:key="dataSource.id"
					:value="dataSource.id"
					:disabled="dataSource.status !== DataSourceStatus.Success"
					:active="
						selectedDataSource !== 'selected-but-not-found' &&
						dataSource.id === selectedDataSource?.id
					"
					class="min-w-0"
					@click="
						() => {
							handleDataSourceSelection(dataSource.id)
						}
					"
				>
					<div class="flex min-w-0 flex-1 items-center gap-2">
						<DataSourceStatusIcon :data-source="dataSource" size="6.5" />
						<div class="flex min-w-0 flex-col">
							<div
								class="min-w-0 truncate text-2sm whitespace-nowrap text-popover-foreground"
							>
								{{ dataSource.name }}
							</div>
							<div
								class="min-w-0 truncate text-xs whitespace-nowrap text-muted-foreground"
							>
								{{ dataSource.url }}
							</div>
						</div>
					</div>
				</ShadcnUiDropdownMenuItem>
			</template>
		</ShadcnUiDropdownMenuContent>
	</ShadcnUiDropdownMenu>
</template>

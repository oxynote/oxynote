<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type { MetricConfig } from "../utils"

const config = defineModel<MetricConfig>()
const props = defineProps<{
	diffMode?: boolean
	isModified?: boolean
	oldConfig?: MetricConfig | null
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

function handleTypeSelection(type: GenericQueryChartType) {
	if (config.value && !isEditingDisabled.value) {
		config.value.visualizationType = type
	}
}
</script>
<template>
	<div class="flex gap-1 px-1.75">
		<div
			:class="
				cn(
					'flex flex-1 flex-col items-center justify-center gap-0.5 rounded-md border py-1 opacity-100 transition-all duration-150 select-none',
					'pointer-events-auto cursor-pointer',
					config?.visualizationType === GenericQueryChartType.Line &&
						!props.isModified
						? 'cursor-default border-primary'
						: 'hover:opacity-70 active:opacity-90',
					isEditingDisabled && 'pointer-events-none select-text',
					config?.visualizationType === GenericQueryChartType.Line &&
						props.isModified &&
						'bg-diff-field-added',
					props.oldConfig?.visualizationType === GenericQueryChartType.Line &&
						props.isModified &&
						'bg-diff-field-removed',
				)
			"
			@click="handleTypeSelection(GenericQueryChartType.Line)"
		>
			<Icon name="lucide:chart-line" class="size-5 shrink-0" />
			<div class="text-2sm text-foreground">
				{{ $t("editor.metrics.config.type-options.line-chart.title") }}
			</div>
		</div>
		<div
			:class="
				cn(
					'flex flex-1 flex-col items-center justify-center gap-0.5 rounded-md border py-1 opacity-100 transition-all duration-150 select-none',
					'pointer-events-auto cursor-pointer',
					config?.visualizationType === GenericQueryChartType.Bar &&
						!props.isModified
						? 'cursor-default border-primary'
						: 'hover:opacity-70 active:opacity-90',
					isEditingDisabled && 'pointer-events-none select-text',
					config?.visualizationType === GenericQueryChartType.Bar &&
						props.isModified &&
						'bg-diff-field-added',
					props.oldConfig?.visualizationType === GenericQueryChartType.Bar &&
						props.isModified &&
						'bg-diff-field-removed',
				)
			"
			@click="handleTypeSelection(GenericQueryChartType.Bar)"
		>
			<Icon name="lucide:bar-chart-3" class="size-5 shrink-0" />
			<div class="text-2sm text-foreground">
				{{ $t("editor.metrics.config.type-options.bar-chart.title") }}
			</div>
		</div>
		<div
			:class="
				cn(
					'flex flex-1 flex-col items-center justify-center rounded-md border py-1 opacity-100 transition-all duration-150 select-none',
					'pointer-events-auto cursor-pointer',
					config?.visualizationType === GenericQueryChartType.Gauge &&
						!props.isModified
						? 'cursor-default border-primary'
						: 'hover:opacity-70 active:opacity-90',
					isEditingDisabled && 'pointer-events-none select-text',
					config?.visualizationType === GenericQueryChartType.Gauge &&
						props.isModified &&
						'bg-diff-field-added',
					props.oldConfig?.visualizationType === GenericQueryChartType.Gauge &&
						props.isModified &&
						'bg-diff-field-removed',
				)
			"
			@click="handleTypeSelection(GenericQueryChartType.Gauge)"
		>
			<Icon name="lucide:gauge" class="size-5.5 shrink-0" />
			<div class="text-2sm text-foreground">
				{{ $t("editor.metrics.config.type-options.gauge-chart.title") }}
			</div>
		</div>
	</div>
</template>

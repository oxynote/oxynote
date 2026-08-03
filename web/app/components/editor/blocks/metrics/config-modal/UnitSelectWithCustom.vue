<script lang="ts" setup>
import {
	VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH,
	VisualizationCoreUnit,
	type MetricConfig,
} from "../utils"
import { cn } from "~/lib/utils"
import UnitSelect from "./UnitSelect.vue"
import { DiffStatus } from "~/components/editor/diff/position-map"

// either config or oldConfig must be provided; not both
const config = defineModel<MetricConfig>()
const props = defineProps<{
	diffMode?: boolean
	diffStatus?: DiffStatus | null
	oldConfig?: MetricConfig | null
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const configToUse = computed(() => {
	if (config.value) {
		return config.value
	}

	if (props.oldConfig) {
		return props.oldConfig
	}

	return null
})
const configCustomUnit = computed({
	get: () => {
		return configToUse.value?.unit.custom ?? ""
	},
	set: (value) => {
		if (config.value) {
			config.value.unit.custom = value
				.trim()
				.slice(0, VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH)
		}
	},
})
</script>
<template>
	<div class="flex flex-col gap-1">
		<UnitSelect
			v-if="config"
			v-model="config.unit.type"
			:diff-status="props.diffStatus"
			:diff-mode="props.diffMode"
		/>
		<UnitSelect
			v-else
			:diff-status="props.diffStatus"
			:diff-mode="props.diffMode"
			:old-unit="props.oldConfig?.unit.type"
		/>
		<ShadcnUiInput
			v-if="configToUse?.unit.type === VisualizationCoreUnit.Custom"
			v-model="configCustomUnit"
			type="text"
			:placeholder="
				isEditingDisabled
					? $t('editor.metrics.config.unit-custom-empty-value-placeholder')
					: $t('editor.metrics.config.unit-custom-placeholder')
			"
			disable-focus-effect
			disable-destructive-effect
			:transparent-disable="isEditingDisabled"
			:class="
				cn(
					'h-[1.775rem]! w-full px-1.5 text-2sm!',
					props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
					props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
				)
			"
			:maxlength="VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH"
		/>
	</div>
</template>

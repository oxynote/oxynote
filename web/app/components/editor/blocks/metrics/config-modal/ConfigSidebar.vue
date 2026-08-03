<script lang="ts" setup>
import { cn } from "~/lib/utils"
import ConfigField from "../ConfigField.vue"
import { VISUALIZATION_MAX_DECIMALS, type MetricConfig } from "../utils"
import ThresholdInput from "./ThresholdInput.vue"
import { chartStyles } from "~/assets/css"
import { DiffStatus } from "~/components/editor/diff/position-map"
import VisualizationTypeSelect from "./VisualizationTypeSelect.vue"
import DataSourceSelect from "./DataSourceSelect.vue"
import UnitSelectWithCustom from "./UnitSelectWithCustom.vue"

const config = defineModel<MetricConfig>({ required: true })
const props = defineProps<{
	diffStatus?: DiffStatus | null
	oldConfig?: MetricConfig | null
}>()
const emit = defineEmits<{
	(e: "open-settings"): void
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const configThresholdBounds = computed(() => {
	const min = config.value.thresholds?.reduce((min, threshold) => {
		if (threshold.value !== undefined && threshold.value !== null) {
			return Math.min(min, threshold.value)
		}

		return min
	}, Infinity)
	const max = config.value.thresholds?.reduce((max, threshold) => {
		if (threshold.value !== undefined && threshold.value !== null) {
			return Math.max(max, threshold.value)
		}

		return max
	}, -Infinity)

	return {
		min: min,
		max: max,
	}
})
const configDecimals = computed({
	get: () => config.value.decimals ?? undefined,
	set: (value) => {
		config.value.decimals = value
	},
})
const configBoundsMin = computed({
	get: () => config.value.axisBounds.min ?? undefined,
	set: (value) => {
		if (value === undefined || value === null) {
			config.value.axisBounds.min = null
			return
		}

		if (
			configThresholdBounds.value.min !== undefined &&
			value > configThresholdBounds.value.min
		) {
			value = configThresholdBounds.value.min
		}

		config.value.axisBounds.min = value
	},
})
const configBoundsMax = computed({
	get: () => config.value.axisBounds.max ?? undefined,
	set: (value) => {
		if (value === undefined || value === null) {
			config.value.axisBounds.max = null
			return
		}

		if (
			configThresholdBounds.value.max !== undefined &&
			value < configThresholdBounds.value.max
		) {
			value = configThresholdBounds.value.max
		}

		config.value.axisBounds.max = value
	},
})

const isDiffModified = computed(() => props.diffStatus === DiffStatus.Modified)
const isVisualizationTypeModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.visualizationType !== config.value.visualizationType
	)
})
const isDataSourceIdModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.dataSourceId !== config.value.dataSourceId
	)
})
const isTitleModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.title !== config.value.title
	)
})
const isUnitModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		(props.oldConfig.unit?.type !== config.value.unit?.type ||
			props.oldConfig.unit?.custom !== config.value.unit?.custom)
	)
})
const areThresholdsModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		jsonStableStringify(props.oldConfig.thresholds) !==
			jsonStableStringify(config.value.thresholds)
	)
})
const isBaseThresholdColorModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.baseThresholdColor !== config.value.baseThresholdColor
	)
})
const areDecimalsModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.decimals !== config.value.decimals
	)
})
const isBoundsMinModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.axisBounds?.min !== config.value.axisBounds?.min
	)
})
const isBoundsMaxModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		props.oldConfig.axisBounds?.max !== config.value.axisBounds?.max
	)
})

type ThresholdEntry = {
	value?: number
	label?: string
	color?: string
}

type DiffThresholdEntry = {
	threshold: ThresholdEntry
	diffStatus: DiffStatus
	key: string
}

const diffThresholdEntries = computed<DiffThresholdEntry[]>(() => {
	if (!areThresholdsModified.value) {
		return []
	}

	const newArr = config.value.thresholds ?? []
	const oldArr = props.oldConfig?.thresholds ?? []
	const maxLen = Math.max(newArr.length, oldArr.length)
	const entries: DiffThresholdEntry[] = []

	for (let i = 0; i < maxLen; i++) {
		const newTh = newArr[i]
		const oldTh = oldArr[i]

		if (newTh && oldTh) {
			const same =
				newTh.value === oldTh.value &&
				newTh.label === oldTh.label &&
				newTh.color === oldTh.color
			if (same) {
				entries.push({
					threshold: newTh,
					diffStatus: DiffStatus.Unchanged,
					key: `unchanged-${i}`,
				})
			} else {
				entries.push({
					threshold: newTh,
					diffStatus: DiffStatus.Added,
					key: `new-${i}`,
				})
				entries.push({
					threshold: oldTh,
					diffStatus: DiffStatus.Removed,
					key: `old-${i}`,
				})
			}
		} else if (newTh) {
			entries.push({
				threshold: newTh,
				diffStatus: DiffStatus.Added,
				key: `added-${i}`,
			})
		} else if (oldTh) {
			entries.push({
				threshold: oldTh,
				diffStatus: DiffStatus.Removed,
				key: `removed-${i}`,
			})
		}
	}

	return entries
})

function updateThresholdField(
	index: number,
	field: "value" | "label" | "color",
	val: any,
) {
	config.value.thresholds =
		config.value.thresholds?.map((t, i) =>
			i === index ? { ...t, [field]: val } : t,
		) ?? null
}

function addThreshold() {
	config.value.thresholds = [
		...(config.value.thresholds || []),
		{
			value: undefined,
			label: undefined,
			color: chartStyles().thresholdColors.default,
		},
	]
}

function removeThreshold(index: number) {
	if (!config.value.thresholds) {
		return
	}

	config.value.thresholds = config.value.thresholds.filter(
		(_, i) => i !== index,
	)
}
</script>
<template>
	<div
		:class="
			cn('flex w-full flex-col gap-2 py-1.75', isEditingDisabled && 'pb-2')
		"
	>
		<div class="flex flex-col gap-1">
			<VisualizationTypeSelect
				v-model="config"
				:diff-mode="!!props.diffStatus"
				:is-modified="isVisualizationTypeModified"
				:old-config="isVisualizationTypeModified ? props.oldConfig : null"
			/>
		</div>
		<div>
			<div class="mb-1 h-px bg-border" />
			<div class="flex flex-col gap-1 px-1">
				<DataSourceSelect
					v-model="config"
					:diff-mode="!!props.diffStatus"
					:diff-status="isDataSourceIdModified ? DiffStatus.Added : null"
					@open-settings="emit('open-settings')"
				/>
				<DataSourceSelect
					v-if="isDataSourceIdModified"
					:diff-mode="!!props.diffStatus"
					:old-config="props.oldConfig"
					:diff-status="isDataSourceIdModified ? DiffStatus.Removed : null"
				/>
			</div>
			<div class="mt-1 h-px bg-border" />
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.metrics.config.title-label") }}
					</span>
				</template>
				<ShadcnUiInput
					v-model="config.title"
					type="text"
					:placeholder="
						isEditingDisabled
							? $t('editor.metrics.config.title-empty-value-placeholder')
							: $t('editor.metrics.config.title-placeholder')
					"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled || isTitleModified"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							isTitleModified && 'bg-diff-field-added',
						)
					"
				/>
				<ShadcnUiInput
					v-if="isTitleModified"
					:model-value="props.oldConfig?.title"
					type="text"
					:placeholder="
						$t('editor.metrics.config.title-empty-value-placeholder')
					"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled || isTitleModified"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							isTitleModified && 'bg-diff-field-removed',
						)
					"
				/>
			</ConfigField>
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.metrics.config.unit-label") }}
					</span>
				</template>
				<UnitSelectWithCustom
					v-model="config"
					:diff-mode="!!props.diffStatus"
					:diff-status="isUnitModified ? DiffStatus.Added : null"
				/>
				<UnitSelectWithCustom
					v-if="isUnitModified"
					:diff-mode="!!props.diffStatus"
					:diff-status="isUnitModified ? DiffStatus.Removed : null"
					:old-config="props.oldConfig"
				/>
			</ConfigField>
		</div>
		<div class="h-px bg-border" />
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.metrics.config.thresholds-label") }}
					</span>
				</template>
				<div class="flex w-full flex-col gap-2">
					<div class="flex w-full flex-col gap-1">
						<template
							v-if="config.visualizationType === GenericQueryChartType.Gauge"
						>
							<ThresholdInput
								v-model:color="config.baseThresholdColor"
								:visualization-type="config.visualizationType"
								:base-threshold="{
									label: $t('editor.metrics.config.base-threshold-label'),
								}"
								:diff-status="
									isBaseThresholdColorModified ? DiffStatus.Added : null
								"
								:diff-mode="!!props.diffStatus"
							/>
							<ThresholdInput
								v-if="isBaseThresholdColorModified"
								:color="props.oldConfig?.baseThresholdColor"
								:visualization-type="config.visualizationType"
								:base-threshold="{
									label: $t('editor.metrics.config.base-threshold-label'),
								}"
								:diff-status="DiffStatus.Removed"
								:diff-mode="true"
							/>
						</template>
						<template v-if="areThresholdsModified">
							<ThresholdInput
								v-for="entry in diffThresholdEntries"
								:key="entry.key"
								:value="entry.threshold.value"
								:label="entry.threshold.label"
								:color="entry.threshold.color"
								:visualization-type="config.visualizationType"
								:diff-status="entry.diffStatus"
								:diff-mode="true"
							/>
						</template>
						<template v-else>
							<ShadcnUiButton
								v-if="
									!config.thresholds?.length &&
									config.visualizationType !== GenericQueryChartType.Gauge
								"
								variant="outline-dim"
								size="custom"
								:class="
									cn(
										'h-[1.775rem]! gap-1 text-2sm',
										isEditingDisabled &&
											'pointer-events-none border-dashed border-border select-text',
									)
								"
								@click="addThreshold"
							>
								<Icon v-if="!isEditingDisabled" name="lucide:circle-plus" />
								<Icon v-else name="lucide:chart-no-axes-gantt" />
								{{
									!isEditingDisabled
										? $t("editor.metrics.config.add-threshold-button")
										: $t("editor.metrics.config.no-thresholds-button")
								}}
							</ShadcnUiButton>
							<ThresholdInput
								v-for="(threshold, index) in config.thresholds"
								:key="index"
								:value="threshold.value"
								:label="threshold.label"
								:color="threshold.color"
								:visualization-type="config.visualizationType"
								:diff-mode="!!props.diffStatus"
								@update:value="
									(v: number | undefined) =>
										updateThresholdField(index, 'value', v)
								"
								@update:label="
									(v: string | undefined) =>
										updateThresholdField(index, 'label', v)
								"
								@update:color="
									(v: string | undefined) =>
										updateThresholdField(index, 'color', v)
								"
								@delete="removeThreshold(index)"
							/>
						</template>
					</div>
					<ShadcnUiButton
						v-if="
							!isEditingDisabled &&
							(config.visualizationType === GenericQueryChartType.Gauge ||
								(config.thresholds && config.thresholds?.length > 0))
						"
						variant="dim"
						size="custom"
						class="gap-1 text-2sm"
						@click="addThreshold"
					>
						<Icon name="lucide:circle-plus" />
						{{ $t("editor.metrics.config.add-threshold-button") }}
					</ShadcnUiButton>
				</div>
			</ConfigField>
		</div>
		<div class="h-px bg-border" />
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.metrics.config.decimals-label") }}
					</span>
				</template>
				<NumberInput
					v-model="configDecimals"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							areDecimalsModified && 'bg-diff-field-added',
						)
					"
					:placeholder="$t('editor.metrics.config.decimals-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled"
					positive
					zero
					:max="VISUALIZATION_MAX_DECIMALS"
				/>
				<NumberInput
					v-if="areDecimalsModified"
					:model-value="props.oldConfig?.decimals ?? undefined"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							areDecimalsModified && 'bg-diff-field-removed',
						)
					"
					:placeholder="$t('editor.metrics.config.decimals-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled"
					positive
					zero
					:max="VISUALIZATION_MAX_DECIMALS"
				/>
			</ConfigField>
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.metrics.config.bounds-min-label") }}
					</span>
				</template>
				<NumberInput
					v-model="configBoundsMin"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							isBoundsMinModified && 'bg-diff-field-added',
						)
					"
					:placeholder="$t('editor.metrics.config.bounds-min-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled"
					positive
					negative
					zero
					decimal
				/>
				<NumberInput
					v-if="isBoundsMinModified"
					:model-value="props.oldConfig?.axisBounds.min ?? undefined"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							isBoundsMinModified && 'bg-diff-field-removed',
						)
					"
					:placeholder="$t('editor.metrics.config.bounds-min-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled"
					positive
					negative
					zero
					decimal
				/>
			</ConfigField>
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.metrics.config.bounds-max-label") }}
					</span>
				</template>
				<NumberInput
					v-model="configBoundsMax"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							isBoundsMaxModified && 'bg-diff-field-added',
						)
					"
					:placeholder="$t('editor.metrics.config.bounds-max-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled"
					positive
					negative
					zero
					decimal
				/>
				<NumberInput
					v-if="isBoundsMaxModified"
					:model-value="props.oldConfig?.axisBounds.max ?? undefined"
					:class="
						cn(
							'h-[1.775rem]! w-full px-1.5 text-2sm!',
							isBoundsMaxModified && 'bg-diff-field-removed',
						)
					"
					:placeholder="$t('editor.metrics.config.bounds-max-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					:transparent-disable="isEditingDisabled"
					positive
					negative
					zero
					decimal
				/>
			</ConfigField>
		</div>
	</div>
</template>

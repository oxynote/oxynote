<script lang="ts" setup>
import { cn } from "~/lib/utils"
import ColorSelect from "../../../ColorSelect.vue"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps<{
	visualizationType: GenericQueryChartType | null
	baseThreshold?: {
		label: string
	}
	diffMode?: boolean
	diffStatus?: DiffStatus | null
}>()
const thresholdValue = defineModel<number | undefined>("value")
const thresholdLabel = defineModel<string | undefined>("label")
const thresholdColor = defineModel<string | undefined>("color", {
	required: true,
})
const emit = defineEmits<{
	(e: "input-blur" | "delete"): void
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)

watchImmediate(
	() => props.baseThreshold,
	(newVal) => {
		if (newVal) {
			thresholdLabel.value = newVal.label
		}
	},
)
</script>
<template>
	<ShadcnUiButtonGroup
		:class="
			cn(
				'h-[1.775rem]! w-full gap-0! rounded-md',
				props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
				props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
				isEditingDisabled && 'pointer-events-none cursor-default',
			)
		"
	>
		<ShadcnUiPopover>
			<ShadcnUiPopoverTrigger as-child>
				<ShadcnUiButton
					size="icon"
					variant="outline-transparent"
					:class="
						cn('h-[1.775rem]!', isEditingDisabled && 'pointer-events-none')
					"
				>
					<div
						class="size-3.5 rounded-full transition-colors"
						:style="{ backgroundColor: thresholdColor }"
					/>
				</ShadcnUiButton>
			</ShadcnUiPopoverTrigger>
			<ShadcnUiPopoverContent
				v-if="!isEditingDisabled"
				side="bottom"
				align="start"
				class="w-fit min-w-0"
				inside-modal
			>
				<ColorSelect v-model="thresholdColor" />
			</ShadcnUiPopoverContent>
		</ShadcnUiPopover>
		<ShadcnUiInputGroup
			disable-destructive-effect
			disable-focus-effect
			class="h-[1.775rem]!"
		>
			<ShadcnUiInputGroupInput
				v-if="!props.baseThreshold"
				v-model="thresholdValue"
				type="text"
				inputmode="numeric"
				class="text-2sm!"
				:placeholder="
					isEditingDisabled
						? $t('editor.metrics.config.threshold-value-empty-placeholder')
						: $t('editor.metrics.config.threshold-value-placeholder')
				"
				:transparent-disable="isEditingDisabled"
				number
				decimal
				positive
				zero
				negative
				@blur="$emit('input-blur')"
			/>
			<ShadcnUiInputGroupInput
				v-if="
					props.visualizationType !== GenericQueryChartType.Gauge ||
					props.baseThreshold
				"
				v-model="thresholdLabel"
				type="text"
				class="text-2sm!"
				:placeholder="
					isEditingDisabled
						? $t('editor.metrics.config.threshold-label-empty-placeholder')
						: $t('editor.metrics.config.threshold-label-placeholder')
				"
				:disabled="props.baseThreshold"
				:transparent-disable="isEditingDisabled"
			/>
		</ShadcnUiInputGroup>
		<ShadcnUiButton
			v-if="!isEditingDisabled && !props.baseThreshold"
			size="icon"
			variant="outline-transparent"
			class="h-[1.775rem]!"
			@click="emit('delete')"
		>
			<Icon name="lucide:trash-2" />
		</ShadcnUiButton>
	</ShadcnUiButtonGroup>
</template>

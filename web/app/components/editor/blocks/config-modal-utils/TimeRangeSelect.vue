<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import { TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_TIME_RANGE_REFERENCE,
	type TimeRangeSetting,
} from "~/utils/variable"
import CustomTimeRangeDialog from "./CustomTimeRangeDialog.vue"

const CUSTOM_VALUE = "custom"

const props = defineProps<{
	disabled: boolean
	oldValue?: TimeRangeSetting | null
}>()
const model = defineModel<TimeRangeSetting | null>({ required: true })
const { t } = useI18n({ useScope: "global" })
const label = useTimeRangeLabel()
const customOpen = ref(false)
const selected = computed({
	get: () => (model.value?.startsWith("custom:") ? CUSTOM_VALUE : model.value),
	set: (value: TimeRangeSetting | typeof CUSTOM_VALUE | null) => {
		if (value === CUSTOM_VALUE) {
			// let the select release its focus trap before opening the dialog
			void nextTick(() => {
				customOpen.value = true
			})
			return
		}

		model.value = value
	},
})

// the old value is given only when the diff changed it
const isModified = computed(() => props.oldValue !== undefined)
</script>
<template>
	<div class="flex flex-col items-end gap-0.5">
		<ShadcnUiSelect v-model="selected" :disabled="props.disabled">
			<ShadcnUiSelectTrigger
				disable-default-styles
				:disable="props.disabled"
				as-child
			>
				<ShadcnUiButton
					variant="ghost-plain"
					size="custom"
					:title="label(model)"
					:class="
						cn(
							'max-w-[min(65dvw,30rem)] gap-1 bg-muted-50-real text-2sm text-foreground md:bg-none',
							props.disabled &&
								'pointer-events-none select-text disabled:opacity-100 dark:disabled:opacity-100',
							isModified && 'bg-diff-field-added px-0.5',
						)
					"
				>
					<Icon name="lucide:clock-9" class="size-3.5 shrink-0" />
					<span class="truncate">{{ label(model) }}</span>
				</ShadcnUiButton>
			</ShadcnUiSelectTrigger>
			<ShadcnUiSelectContent class="max-h-[40dvh]" inside-modal>
				<ShadcnUiSelectItem :value="CUSTOM_VALUE" class="text-2sm">
					{{ t("general.custom-time-range.title") }}
				</ShadcnUiSelectItem>
				<ShadcnUiSelectSeparator />
				<ShadcnUiSelectItem :value="METRIC_TIME_RANGE_REFERENCE">
					<span class="text-2sm text-popover-foreground">
						{{ $t("editor.config-modal-utils.variable-time-range") }}
					</span>
				</ShadcnUiSelectItem>
				<ShadcnUiSelectSeparator />
				<ShadcnUiSelectItem
					v-for="timeRange in Object.values(TimeRangePreset)"
					:key="timeRange"
					:value="timeRange"
				>
					<span class="text-2sm text-popover-foreground">
						{{ $t(`general.time-range-options.${timeRange}`) }}
					</span>
				</ShadcnUiSelectItem>
			</ShadcnUiSelectContent>
		</ShadcnUiSelect>
		<ShadcnUiButton
			v-if="isModified"
			variant="ghost-plain"
			size="custom"
			:class="
				cn(
					'gap-1 bg-muted-50-real text-2sm text-foreground md:bg-none',
					props.disabled && 'pointer-events-none select-text',
					isModified && 'bg-diff-field-removed px-0.5 md:bg-diff-field-removed',
				)
			"
		>
			<Icon name="lucide:clock-9" class="size-3.5" />
			{{ label(props.oldValue ?? null) }}
		</ShadcnUiButton>
		<CustomTimeRangeDialog
			v-if="customOpen && !props.disabled"
			v-model:open="customOpen"
			:time-range="model === METRIC_TIME_RANGE_REFERENCE ? null : model"
			@apply="model = $event"
		/>
	</div>
</template>

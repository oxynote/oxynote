<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import type { TimeRangeSetting } from "~/utils/variable"
import TimeRangePicker from "./TimeRangePicker.vue"
import { useBlockVariables } from "../variables"

const props = defineProps<{
	disabled: boolean
	oldValue?: TimeRangeSetting | null
}>()
const model = defineModel<TimeRangeSetting | null>({ required: true })
const { t } = useI18n({ useScope: "global" })
const label = useTimeRangeLabel()
const open = ref(false)
const blockVariables = useBlockVariables()

function select(value: TimeRangeSetting) {
	model.value = value
	open.value = false
}

// the old value is given only when the diff changed it
const isModified = computed(() => props.oldValue !== undefined)
</script>
<template>
	<div class="flex flex-col items-end gap-0.5">
		<ShadcnUiPopover v-model:open="open">
			<ShadcnUiPopoverTrigger as-child :disabled="props.disabled">
				<ShadcnUiButton
					variant="ghost-plain"
					size="custom"
					:title="label(model)"
					:disabled="props.disabled"
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
			</ShadcnUiPopoverTrigger>
			<ShadcnUiPopoverContent
				align="end"
				inside-modal
				class="w-[min(40rem,calc(100dvw-2rem))] overflow-hidden p-0"
				:aria-label="t('general.custom-time-range.title')"
			>
				<TimeRangePicker
					v-if="open"
					:time-range="blockVariables.timeRange(model)"
					@apply="select"
				/>
			</ShadcnUiPopoverContent>
		</ShadcnUiPopover>
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
	</div>
</template>

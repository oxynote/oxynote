<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import { RefreshInterval } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	type RefreshIntervalSetting,
} from "~/utils/variable"

const props = defineProps<{
	disabled: boolean
	oldValue?: RefreshIntervalSetting | null
}>()
const model = defineModel<RefreshIntervalSetting | null>({ required: true })
const { t } = useI18n({ useScope: "global" })

// the old value is given only when the diff changed it
const isModified = computed(() => props.oldValue !== undefined)

// a block that uses a built-in variable has no value of its own to name,
// so the label says the shared setting decides
function label(value: RefreshIntervalSetting | null) {
	if (value === METRIC_REFRESH_INTERVAL_REFERENCE) {
		return t("editor.config-modal-utils.variable-refresh-interval")
	}

	return value ? t(`general.refresh-interval-options-short.${value}`) : ""
}
</script>
<template>
	<div class="flex flex-col items-end gap-0.5 md:gap-1">
		<ShadcnUiSelect v-model="model" :disabled="props.disabled">
			<ShadcnUiSelectTrigger
				disable-default-styles
				:disable="props.disabled"
				as-child
			>
				<ShadcnUiButton
					variant="ghost-plain"
					size="custom"
					:class="
						cn(
							'gap-1 bg-muted-50-real text-2sm text-foreground md:bg-none',
							props.disabled &&
								'pointer-events-none select-text disabled:opacity-100',
							isModified && 'bg-diff-field-added px-0.5',
						)
					"
				>
					<Icon name="lucide:refresh-ccw" class="size-3.5" />
					{{ label(model) }}
				</ShadcnUiButton>
			</ShadcnUiSelectTrigger>
			<ShadcnUiSelectContent class="max-h-[40dvh]" inside-modal>
				<ShadcnUiSelectItem :value="METRIC_REFRESH_INTERVAL_REFERENCE">
					<span class="text-2sm text-popover-foreground">
						{{ $t("editor.config-modal-utils.variable-refresh-interval") }}
					</span>
				</ShadcnUiSelectItem>
				<ShadcnUiSelectSeparator />
				<ShadcnUiSelectItem
					v-for="refreshInterval in Object.values(RefreshInterval)"
					:key="refreshInterval"
					:value="refreshInterval"
				>
					<span class="text-2sm text-popover-foreground">
						{{ $t(`general.refresh-interval-options.${refreshInterval}`) }}
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
			<Icon name="lucide:refresh-ccw" class="size-3.5" />
			{{ label(props.oldValue ?? null) }}
		</ShadcnUiButton>
	</div>
</template>

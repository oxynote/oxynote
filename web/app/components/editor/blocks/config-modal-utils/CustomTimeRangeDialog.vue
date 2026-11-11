<script lang="ts" setup>
import { format } from "date-fns"
import {
	resolveTimeRange,
	serializeCustomTimeRange,
	TimeRangePreset,
	type CustomTimeRange,
	type TimeRange,
} from "~/utils/time-range"

const props = defineProps<{ timeRange: TimeRange | null }>()
const open = defineModel<boolean>("open", { default: false })
const emit = defineEmits<{ apply: [value: CustomTimeRange] }>()
const { t } = useI18n({ useScope: "global" })
const id = useId()
const from = ref("")
const to = ref("")
const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
const invalid = computed(() => {
	const start = new Date(from.value)
	const end = new Date(to.value)

	return (
		!Number.isFinite(start.getTime()) ||
		!Number.isFinite(end.getTime()) ||
		start >= end ||
		// a daylight-saving gap must not silently change the entered time
		format(start, "yyyy-MM-dd'T'HH:mm") !== from.value.slice(0, 16) ||
		format(end, "yyyy-MM-dd'T'HH:mm") !== to.value.slice(0, 16)
	)
})

watchImmediate(open, (value) => {
	if (!value) {
		return
	}

	const range = resolveTimeRange(props.timeRange ?? TimeRangePreset.Last24Hours)
	from.value = format(range.from, "yyyy-MM-dd'T'HH:mm:ss.SSS")
	to.value = format(range.to, "yyyy-MM-dd'T'HH:mm:ss.SSS")
})

function apply() {
	if (invalid.value) {
		return
	}

	emit(
		"apply",
		serializeCustomTimeRange(new Date(from.value), new Date(to.value)),
	)
	open.value = false
}
</script>

<template>
	<ShadcnUiDialog v-model:open="open">
		<ShadcnUiDialogContent class="sm:max-w-md">
			<ShadcnUiDialogHeader>
				<ShadcnUiDialogTitle>
					{{ t("general.custom-time-range.title") }}
				</ShadcnUiDialogTitle>
				<ShadcnUiDialogDescription>
					{{ t("general.custom-time-range.description", { timeZone }) }}
				</ShadcnUiDialogDescription>
			</ShadcnUiDialogHeader>
			<form class="flex flex-col gap-4" @submit.prevent="apply">
				<div class="flex flex-col gap-2">
					<ShadcnUiLabel :for="`${id}-from`">
						{{ t("general.custom-time-range.from") }}
					</ShadcnUiLabel>
					<ShadcnUiInput
						:id="`${id}-from`"
						v-model="from"
						type="datetime-local"
						step="0.001"
						required
					/>
				</div>
				<div class="flex flex-col gap-2">
					<ShadcnUiLabel :for="`${id}-to`">
						{{ t("general.custom-time-range.to") }}
					</ShadcnUiLabel>
					<ShadcnUiInput
						:id="`${id}-to`"
						v-model="to"
						type="datetime-local"
						step="0.001"
						required
					/>
				</div>
				<p
					v-if="invalid"
					role="alert"
					class="text-2sm text-destructive-foreground"
				>
					{{ t("general.custom-time-range.invalid") }}
				</p>
				<ShadcnUiDialogFooter>
					<ShadcnUiButton type="button" variant="outline" @click="open = false">
						{{ t("general.custom-time-range.cancel") }}
					</ShadcnUiButton>
					<ShadcnUiButton type="submit" :disabled="invalid">
						{{ t("general.custom-time-range.apply") }}
					</ShadcnUiButton>
				</ShadcnUiDialogFooter>
			</form>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>

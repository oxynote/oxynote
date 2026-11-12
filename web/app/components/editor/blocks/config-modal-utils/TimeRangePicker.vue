<script lang="ts" setup>
import {
	fromDate,
	parseDateTime,
	toCalendarDateTime,
} from "@internationalized/date"
import {
	parseCustomTimeRange,
	resolveTimeRange,
	serializeCustomTimeRange,
	TimeRangePreset,
	type TimeRange,
} from "~/utils/time-range"
import {
	METRIC_TIME_RANGE_REFERENCE,
	type TimeRangeSetting,
} from "~/utils/variable"

const props = defineProps<{ timeRange: TimeRange | null; pending?: boolean }>()
const emit = defineEmits<{ apply: [value: TimeRangeSetting] }>()
const { t, locale } = useI18n({ useScope: "global" })
const id = useId()
const { choice: zoneChoice, zone } = useTimeRangeZone()
const label = useTimeRangeLabel(zone)
const from = ref("")
const to = ref("")
const edited = ref(false)
const fromOccurrence = ref<string>()
const toOccurrence = ref<string>()
const history = usePersistentState<string[]>({
	key: "custom-time-range-history",
	defaultValue: [],
	storage: { web: "local-storage", desktop: "local-storage" },
})
const recent = computed(() =>
	(Array.isArray(history.value) ? history.value : [])
		.filter((value) => typeof value === "string" && parseCustomTimeRange(value))
		.slice(0, 4),
)
const fromCandidates = computed(() => candidates(from.value))
const toCandidates = computed(() => candidates(to.value))
const range = computed(() => {
	const start = boundary(fromCandidates.value, fromOccurrence.value)
	const end = boundary(toCandidates.value, toOccurrence.value)

	return start && end && start < end ? { from: start, to: end } : null
})

watchImmediate(
	() => props.timeRange,
	(value) => {
		if (!edited.value) {
			setDraft(resolveTimeRange(value ?? TimeRangePreset.Last24Hours))
		}
	},
)

function candidates(value: string): Date[] {
	try {
		const local = parseDateTime(value)
		const earlier = local.toDate(zone.value, "earlier")
		const later = local.toDate(zone.value, "later")
		if (
			local.compare(toCalendarDateTime(fromDate(earlier, zone.value))) !== 0
		) {
			return []
		}

		return earlier.getTime() === later.getTime() ? [earlier] : [earlier, later]
	} catch {
		return []
	}
}

function boundary(values: Date[], occurrence?: string): Date | undefined {
	return values.length === 1
		? values[0]
		: values.find((value) => String(value.getTime()) === occurrence)
}

function editBoundary(which: "from" | "to") {
	edited.value = true
	if (which === "from") {
		fromOccurrence.value = undefined
	} else {
		toOccurrence.value = undefined
	}
}

function offsetLabel(value: Date) {
	return (
		new Intl.DateTimeFormat(locale.value, {
			timeZone: zone.value,
			timeZoneName: "longOffset",
		})
			.formatToParts(value)
			.find((part) => part.type === "timeZoneName")?.value ??
		value.toISOString()
	)
}

function setDraft(value: { from: Date; to: Date }) {
	from.value = toCalendarDateTime(fromDate(value.from, zone.value)).toString()
	to.value = toCalendarDateTime(fromDate(value.to, zone.value)).toString()
	fromOccurrence.value = String(value.from.getTime())
	toOccurrence.value = String(value.to.getTime())
}

function changeZone(value: unknown) {
	if ((value !== "browser" && value !== "UTC") || !range.value) {
		return
	}

	const current = range.value
	zoneChoice.value = value
	setDraft(current)
}

function apply() {
	if (!range.value || props.pending) {
		return
	}

	const value = serializeCustomTimeRange(range.value.from, range.value.to)
	history.value = [
		value,
		...recent.value.filter((entry) => entry !== value),
	].slice(0, 4)
	emit("apply", value)
}
</script>

<template>
	<div>
		<div
			class="grid max-h-[min(32rem,65dvh)] grid-cols-1 overflow-y-auto sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]"
		>
			<section
				class="min-w-0 border-b p-3 sm:border-r sm:border-b-0 sm:p-4"
				:aria-labelledby="`${id}-absolute`"
			>
				<h3 :id="`${id}-absolute`" class="mb-3 text-2sm font-semibold">
					{{ t("general.custom-time-range.absolute") }}
				</h3>
				<form class="flex flex-col gap-3" @submit.prevent="apply">
					<div class="flex flex-col gap-1.5">
						<ShadcnUiLabel :for="`${id}-from`" class="text-xs">
							{{ t("general.custom-time-range.from") }}
						</ShadcnUiLabel>
						<ShadcnUiInput
							:id="`${id}-from`"
							v-model="from"
							type="datetime-local"
							step="0.001"
							required
							class="min-w-0 text-xs md:text-xs dark:scheme-dark"
							@update:model-value="editBoundary('from')"
						/>
						<ShadcnUiSelect
							v-if="fromCandidates.length > 1"
							v-model="fromOccurrence"
							@update:model-value="edited = true"
						>
							<ShadcnUiSelectTrigger
								class="h-8 text-xs"
								:aria-label="
									t('general.custom-time-range.offset-label', {
										boundary: t('general.custom-time-range.from'),
									})
								"
							>
								<ShadcnUiSelectValue
									:placeholder="t('general.custom-time-range.choose-offset')"
								/>
							</ShadcnUiSelectTrigger>
							<ShadcnUiSelectContent inside-modal>
								<ShadcnUiSelectItem
									v-for="candidate in fromCandidates"
									:key="candidate.getTime()"
									:value="String(candidate.getTime())"
								>
									{{ offsetLabel(candidate) }}
								</ShadcnUiSelectItem>
							</ShadcnUiSelectContent>
						</ShadcnUiSelect>
					</div>
					<div class="flex flex-col gap-1.5">
						<ShadcnUiLabel :for="`${id}-to`" class="text-xs">
							{{ t("general.custom-time-range.to") }}
						</ShadcnUiLabel>
						<ShadcnUiInput
							:id="`${id}-to`"
							v-model="to"
							type="datetime-local"
							step="0.001"
							required
							class="min-w-0 text-xs md:text-xs dark:scheme-dark"
							@update:model-value="editBoundary('to')"
						/>
						<ShadcnUiSelect
							v-if="toCandidates.length > 1"
							v-model="toOccurrence"
							@update:model-value="edited = true"
						>
							<ShadcnUiSelectTrigger
								class="h-8 text-xs"
								:aria-label="
									t('general.custom-time-range.offset-label', {
										boundary: t('general.custom-time-range.to'),
									})
								"
							>
								<ShadcnUiSelectValue
									:placeholder="t('general.custom-time-range.choose-offset')"
								/>
							</ShadcnUiSelectTrigger>
							<ShadcnUiSelectContent inside-modal>
								<ShadcnUiSelectItem
									v-for="candidate in toCandidates"
									:key="candidate.getTime()"
									:value="String(candidate.getTime())"
								>
									{{ offsetLabel(candidate) }}
								</ShadcnUiSelectItem>
							</ShadcnUiSelectContent>
						</ShadcnUiSelect>
					</div>
					<p
						v-if="!range"
						role="alert"
						class="text-xs text-destructive-foreground"
					>
						{{ t("general.custom-time-range.invalid") }}
					</p>
					<ShadcnUiButton
						type="submit"
						size="sm"
						:disabled="!range || props.pending"
						class="self-start"
					>
						{{ t("general.custom-time-range.apply") }}
					</ShadcnUiButton>
				</form>
				<h3 class="mt-5 mb-2 text-xs font-semibold text-muted-foreground">
					{{ t("general.custom-time-range.recent") }}
				</h3>
				<p v-if="!recent.length" class="text-xs text-muted-foreground">
					{{ t("general.custom-time-range.empty-recent") }}
				</p>
				<ShadcnUiButton
					v-for="value in recent"
					:key="value"
					variant="ghost"
					size="custom"
					class="mb-1 w-full justify-start px-1.5 py-2 text-left text-xs whitespace-normal"
					@click="emit('apply', value as TimeRange)"
				>
					{{ label(value) }}
				</ShadcnUiButton>
			</section>
			<section class="min-w-0 p-2 sm:p-3" :aria-labelledby="`${id}-relative`">
				<h3 :id="`${id}-relative`" class="px-2 py-1 text-2sm font-semibold">
					{{ t("general.custom-time-range.relative") }}
				</h3>
				<ShadcnUiButton
					variant="ghost"
					size="custom"
					class="my-1 w-full justify-start px-2 py-2 text-left text-xs whitespace-normal"
					@click="emit('apply', METRIC_TIME_RANGE_REFERENCE)"
				>
					{{ t("editor.config-modal-utils.variable-time-range") }}
				</ShadcnUiButton>
				<div class="max-h-40 overflow-y-auto border-t pt-1 sm:max-h-64">
					<ShadcnUiButton
						v-for="preset in Object.values(TimeRangePreset)"
						:key="preset"
						variant="ghost"
						size="custom"
						class="w-full justify-start px-2 py-2 text-left text-xs whitespace-normal"
						:aria-pressed="props.timeRange === preset"
						@click="emit('apply', preset)"
					>
						{{ t(`general.time-range-options.${preset}`) }}
					</ShadcnUiButton>
				</div>
			</section>
		</div>
		<footer
			class="flex flex-wrap items-center justify-between gap-2 border-t bg-muted-50-real px-3 py-2"
		>
			<span class="min-w-0 truncate text-xs text-muted-foreground">
				{{
					zoneChoice === "UTC"
						? t("general.custom-time-range.utc")
						: t("general.custom-time-range.zone-label", {
								label: t("general.custom-time-range.browser-time"),
								zone,
							})
				}}
			</span>
			<ShadcnUiSelect
				:model-value="zoneChoice"
				:disabled="!range"
				@update:model-value="changeZone"
			>
				<ShadcnUiSelectTrigger
					class="h-7 w-auto gap-2 text-xs"
					:aria-label="t('general.custom-time-range.time-zone')"
				>
					<ShadcnUiSelectValue />
				</ShadcnUiSelectTrigger>
				<ShadcnUiSelectContent inside-modal>
					<ShadcnUiSelectItem value="browser">
						{{ t("general.custom-time-range.browser-time") }}
					</ShadcnUiSelectItem>
					<ShadcnUiSelectItem value="UTC">
						{{ t("general.custom-time-range.utc") }}
					</ShadcnUiSelectItem>
				</ShadcnUiSelectContent>
			</ShadcnUiSelect>
		</footer>
	</div>
</template>

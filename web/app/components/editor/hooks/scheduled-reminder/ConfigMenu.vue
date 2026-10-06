<script setup lang="ts">
import { type DateValue, getLocalTimeZone } from "@internationalized/date"
import { formatDistanceToNowStrict } from "date-fns"
import { presetDurations } from "./durations"
import HookConfigPanel from "../HookConfigPanel.vue"
import HookNoticeValue from "../HookNoticeValue.vue"
import HookReadonlyField from "../HookReadonlyField.vue"
import { useHookActions } from "../hook-actions"
import { scalarFieldRows, type HookDiffContext } from "../hook-diff"
import { hookNoticeKeypath, type HookSubtitle } from "../hook-menu"
import { hookStatus } from "../hook-status"

const props = defineProps<{
	hook?: DocumentHook | null | undefined // null/undefined means creating new
	// set only while the diff is shown
	diff?: HookDiffContext | null
	nodeId: string | null // null means global
}>()
const emit = defineEmits<{
	(e: "force-close"): void
}>()

const { t, d, locale } = useI18n({ useScope: "global" })
const { isEditable, isReadOnlyOrDiff } = useEditorMeta()
const actions = useHookActions({
	type: DocumentHookType.ScheduledReminder,
	nodeId: () => props.nodeId,
	hook: () => props.hook,
	close: close,
})

const activeBranchSettings = computed(
	() =>
		props.hook?.settings as DocumentHookSettingsScheduledReminder | undefined,
)
const targetBranchSettings = computed(
	() =>
		props.diff?.targetHook?.settings as
			DocumentHookSettingsScheduledReminder | undefined,
)
const status = computed(() => (props.hook ? hookStatus(props.hook) : null))
const isTriggered = computed(() => status.value === "triggered")
const storedPreset = computed(() => presetOf(activeBranchSettings.value))
const selectedDuration = ref<string | undefined>(initialDuration())
const selectedSchedule = ref<DateValue | undefined>(initialSchedule())
const isSubOpen = ref(false)
const isComplete = computed(
	() =>
		!!selectedDuration.value &&
		(selectedDuration.value !== "custom" || !!selectedSchedule.value),
)
// a waiting reminder starts on its own setting, so it saves once that
// changes
const isChanged = computed(() => {
	if (!isComplete.value || !activeBranchSettings.value) {
		return false
	}

	if (selectedDuration.value !== "custom") {
		return selectedDuration.value !== storedPreset.value
	}

	return (
		storedPreset.value !== null ||
		selectedSchedule.value?.toString() !==
			dateToCalendarDate(activeBranchSettings.value.schedule).toString()
	)
})
const submitDisabled = computed(() =>
	props.hook && !isTriggered.value ? !isChanged.value : !isComplete.value,
)
const subtitle = computed<HookSubtitle>(() => {
	if (!activeBranchSettings.value) {
		return { text: t("editor.hooks.scheduled-reminder.description") }
	}

	const schedule = new Date(activeBranchSettings.value.schedule)
	const date = d(schedule, "short-with-time")

	return {
		text: isTriggered.value
			? t("editor.hooks.scheduled-reminder.subtext-triggered", { date: date })
			: date,
		detail: formatDistanceToNowStrict(schedule, {
			addSuffix: true,
			locale: convertDateFnsLocale(locale.value),
		}),
	}
})
// the notice follows the pick while one is being made, and the stored
// date otherwise. It is null until there is either
const noticeWhen = computed(() => {
	const picking = !isReadOnlyOrDiff.value && (!props.hook || isChanged.value)
	if (picking && selectedDuration.value === "custom") {
		return selectedSchedule.value
			? t("editor.hooks.scheduled-reminder.when-on-date", {
					date: d(selectedSchedule.value.toDate(getLocalTimeZone()), "long"),
				})
			: null
	}

	if (picking && selectedDuration.value) {
		return t(
			`editor.hooks.scheduled-reminder.when-options.${selectedDuration.value}`,
		)
	}

	if (activeBranchSettings.value) {
		return t("editor.hooks.scheduled-reminder.when-on-date", {
			date: d(new Date(activeBranchSettings.value.schedule), "short-with-time"),
		})
	}

	return null
})
// a reader who cannot pick a new date renews a triggered reminder by its
// own preset. A custom date has no preset to renew by
const remindAgainLabel = computed(() =>
	isTriggered.value && storedPreset.value && !isEditable.value
		? t(
				`editor.hooks.scheduled-reminder.remind-again-options.${storedPreset.value}`,
			)
		: undefined,
)

// a reminder that triggers while its menu exists asks for a new pick
watch(isTriggered, () => {
	selectedDuration.value = initialDuration()
	selectedSchedule.value = initialSchedule()
})

async function submit() {
	const duration = selectedDuration.value
	if (!duration || !isComplete.value) {
		return
	}

	const next = {
		scale: "linear" as const,
		duration: duration,
		schedule:
			duration === "custom" && selectedSchedule.value
				? selectedSchedule.value.toDate(getLocalTimeZone())
				: addDurationToDate(new Date(), duration),
	}
	if (props.hook) {
		await actions.update(next, isTriggered.value)
		return
	}

	// the same form creates the next hook, so it is emptied once a hook is
	// created
	if (await actions.create(next)) {
		selectedDuration.value = undefined
		selectedSchedule.value = undefined
	}
}

async function remindAgain() {
	const preset = storedPreset.value
	if (!preset) {
		return
	}

	await actions.update(
		{
			scale: "linear",
			duration: preset,
			schedule: addDurationToDate(new Date(), preset),
		},
		true,
	)
}

// a waiting reminder shows its own setting. A new or triggered one waits
// for a pick
function initialDuration(): string | undefined {
	if (!activeBranchSettings.value || isTriggered.value) {
		return undefined
	}

	return storedPreset.value ?? "custom"
}

function initialSchedule(): DateValue | undefined {
	if (!activeBranchSettings.value || isTriggered.value || storedPreset.value) {
		return undefined
	}

	return dateToCalendarDate(activeBranchSettings.value.schedule)
}

function presetOf(
	reminder: DocumentHookSettingsScheduledReminder | undefined,
): string | null {
	return reminder?.duration && reminder.duration !== "custom"
		? reminder.duration
		: null
}

function fieldValue(reminder: DocumentHookSettingsScheduledReminder): string {
	const date = d(new Date(reminder.schedule), "short-with-time")
	const preset = presetOf(reminder)

	return preset
		? t("editor.hooks.scheduled-reminder.field-value", {
				duration: t(
					`editor.hooks.scheduled-reminder.duration-options.${preset}`,
				),
				date: date,
			})
		: t("editor.hooks.scheduled-reminder.field-value-custom", { date: date })
}

function close() {
	isSubOpen.value = false
	emit("force-close")
}
</script>

<template>
	<HookConfigPanel
		v-model:open="isSubOpen"
		:hook="props.hook"
		:diff="props.diff"
		icon="mingcute:stopwatch-line"
		:acknowledge-label="remindAgainLabel"
		:submit-label="isTriggered ? $t('editor.hooks.renew') : undefined"
		:submit-icon="isTriggered ? 'mingcute:check-fill' : undefined"
		:submit-disabled="submitDisabled"
		@submit="submit"
		@delete="actions.remove"
		@acknowledge="remindAgain"
	>
		<template #title>
			{{ $t("editor.hooks.scheduled-reminder.title") }}
		</template>
		<template #subtitle>
			{{ subtitle.text }}
		</template>
		<template v-if="subtitle.detail" #subtitle-detail>
			{{ subtitle.detail }}
		</template>
		<template #notice>
			<template v-if="isTriggered && activeBranchSettings">
				<i18n-t
					scope="global"
					:keypath="
						hookNoticeKeypath(DocumentHookType.ScheduledReminder, status)
					"
					tag="span"
				>
					<template #date>
						<HookNoticeValue
							:value="
								$d(new Date(activeBranchSettings.schedule), 'short-with-time')
							"
						/>
					</template>
				</i18n-t>
				<p v-if="isReadOnlyOrDiff && !storedPreset" class="mt-1">
					{{ $t("editor.hooks.scheduled-reminder.one-off-note") }}
				</p>
			</template>
			<i18n-t
				v-else
				scope="global"
				:keypath="hookNoticeKeypath(DocumentHookType.ScheduledReminder, status)"
				tag="span"
			>
				<template #when>
					<HookNoticeValue
						:value="noticeWhen"
						:fallback="$t('editor.hooks.scheduled-reminder.when-unset')"
					/>
				</template>
			</i18n-t>
		</template>
		<HookReadonlyField
			v-if="isReadOnlyOrDiff && activeBranchSettings"
			:label="$t('editor.hooks.scheduled-reminder.duration-label')"
			:rows="
				scalarFieldRows(
					fieldValue(activeBranchSettings),
					targetBranchSettings ? fieldValue(targetBranchSettings) : null,
				)
			"
			:diff-status="props.diff?.status"
		/>
		<div v-else class="flex flex-col gap-1">
			<ShadcnUiSelect v-model="selectedDuration">
				<ShadcnUiSelectLabel>
					<span class="text-2sm">
						{{
							isTriggered
								? $t("editor.hooks.scheduled-reminder.duration-label-triggered")
								: $t("editor.hooks.scheduled-reminder.duration-label")
						}}
					</span>
				</ShadcnUiSelectLabel>
				<ShadcnUiSelectTrigger class="w-full" size="custom">
					<ShadcnUiSelectValue
						class="text-2sm"
						:placeholder="
							$t('editor.hooks.scheduled-reminder.select-placeholder')
						"
					/>
				</ShadcnUiSelectTrigger>
				<ShadcnUiSelectContent
					class="max-h-[40dvh]"
					side="bottom"
					align="start"
				>
					<ShadcnUiSelectItem
						v-for="durationKey in presetDurations"
						:key="durationKey"
						:value="durationKey"
						class="text-2sm"
					>
						{{
							$t(
								`editor.hooks.scheduled-reminder.duration-options.${durationKey}`,
							)
						}}
					</ShadcnUiSelectItem>
				</ShadcnUiSelectContent>
			</ShadcnUiSelect>
			<CalendarInput
				v-if="selectedDuration === 'custom'"
				v-model="selectedSchedule"
				:placeholder="
					$t('editor.hooks.scheduled-reminder.calendar-placeholder')
				"
				class="w-full"
				available-from-tomorrow
			/>
		</div>
	</HookConfigPanel>
</template>

<script setup lang="ts">
import { type DateValue, getLocalTimeZone } from "@internationalized/date"
import { presetDurations } from "./durations"
import { showToastMessage } from "~/components/toast"

const props = defineProps<{
	hook?: DocumentHook | null | undefined // null/undefined means creating new
	nodeId: string | null // null means global
}>()
const emit = defineEmits<{
	(e: "force-close"): void
}>()

const hookData = computed(() => {
	if (!props.hook) {
		return null
	}

	return {
		score: Number(props.hook.score),
		state: props.hook.state as DocumentHookStateScheduledReminder,
		settings: props.hook.settings as DocumentHookSettingsScheduledReminder,
	}
})
const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const documentHookAPI = useDocumentHookAPI()
const selectedDuration = ref<string | undefined>(undefined)
const selectedSchedule = ref<DateValue | undefined>(undefined)
const confirmedSchedule = ref<DateValue | undefined>(
	hookData.value
		? dateToCalendarDate(hookData.value.settings.schedule)
		: undefined,
)
const isSubOpen = ref(false)

async function upsertHook() {
	if (
		!selectedDuration.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	const newSchedule =
		selectedDuration.value === "custom" && selectedSchedule.value
			? selectedSchedule.value.toDate(getLocalTimeZone())
			: addDurationToDate(new Date(), selectedDuration.value)

	isSubOpen.value = false
	emit("force-close")

	if (!props.hook) {
		try {
			await documentHookAPI.createDocumentHookByDocID.mutateAsync({
				docId: editorStore.activeDocumentId,
				req: {
					type: DocumentHookType.ScheduledReminder,
					branchId: editorStore.activeBranchId,
					blockId: props.nodeId,
					settings: {
						scale: "linear",
						duration: selectedDuration.value,
						schedule: newSchedule,
					},
				},
			})
		} catch {
			showToastMessage("error", t("editor.hooks.errors.create-failed"))
			return
		}

		confirmedSchedule.value = dateToCalendarDate(newSchedule)

		// since "create new" is reused, reset the state
		selectedDuration.value = undefined
		selectedSchedule.value = undefined

		return
	}

	try {
		await documentHookAPI.updateDocumentHookByDocID.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			hookId: props.hook.id,
			req: {
				settings: {
					scale: "linear",
					duration: selectedDuration.value,
					schedule: newSchedule,
				},
			},
		})
	} catch {
		showToastMessage("error", t("editor.hooks.errors.renew-failed"))
		return
	}

	confirmedSchedule.value = dateToCalendarDate(newSchedule)
}

async function deleteHook() {
	if (
		!props.hook ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	isSubOpen.value = false
	emit("force-close")

	try {
		await documentHookAPI.deleteDocumentHookByDocID.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			hookId: props.hook.id,
		})
	} catch {
		showToastMessage("error", t("editor.hooks.errors.delete-failed"))
		return
	}

	selectedDuration.value = undefined
	selectedSchedule.value = undefined
	confirmedSchedule.value = undefined
}
</script>
<template>
	<ShadcnUiDropdownMenuSub v-model:open="isSubOpen">
		<ShadcnUiDropdownMenuSubTrigger>
			<div class="relative h-[0.8125rem] w-[0.8125rem] shrink-0">
				<Icon
					class="absolute top-1/2 left-1/2 size-3.75 -translate-x-1/2 -translate-y-1/2"
					:name="
						!hookData || hookData.score !== 0
							? 'lucide:timer'
							: 'lucide:timer-reset'
					"
				/>
			</div>
			<span v-if="!hookData">
				{{ $t("editor.hooks.time-expiration.title") }}
			</span>
			<i18n-t
				v-else-if="Number(hookData.score) !== 0"
				scope="global"
				keypath="editor.hooks.time-expiration.existing-item"
				tag="span"
				class="truncate"
			>
				<template #value>
					{{ $d(new Date(hookData.settings.schedule), "short") }}
				</template>
			</i18n-t>
			<i18n-t
				v-else
				scope="global"
				keypath="editor.hooks.time-expiration.triggered-item"
				tag="span"
				class="truncate"
			>
				<template #value>
					{{ $d(new Date(hookData.settings.schedule), "short") }}
				</template>
			</i18n-t>
		</ShadcnUiDropdownMenuSubTrigger>
		<ShadcnUiDropdownMenuSubContent
			side="right"
			align="start"
			loop
			:class="[
				'pointer-events-auto!' /* for some reason ShadcnUiSelect disables pointer events, which closes the whole sub menu when the select is closed, so we must override this */,
			]"
		>
			<div class="flex w-[12rem] flex-col">
				<div class="flex flex-col gap-1 px-0.75 pb-0.75">
					<template v-if="!hookData || hookData.score === 0">
						<ShadcnUiSelect v-model="selectedDuration">
							<ShadcnUiSelectLabel>
								<span class="text-2sm">
									{{
										!hookData || hookData.score !== 0
											? $t("editor.hooks.time-expiration.duration-label")
											: $t(
													"editor.hooks.time-expiration.duration-label-triggered",
												)
									}}
								</span>
							</ShadcnUiSelectLabel>
							<ShadcnUiSelectTrigger class="w-full" size="custom">
								<ShadcnUiSelectValue
									class="text-2sm"
									:placeholder="
										$t(`editor.hooks.time-expiration.select-placeholder`)
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
											`editor.hooks.time-expiration.duration-options.${durationKey}`,
										)
									}}
								</ShadcnUiSelectItem>
							</ShadcnUiSelectContent>
						</ShadcnUiSelect>
						<CalendarInput
							v-if="selectedDuration === 'custom'"
							v-model="selectedSchedule"
							:placeholder="
								$t('editor.hooks.time-expiration.calendar-placeholder')
							"
							class="w-full"
							available-from-tomorrow
						/>
					</template>
					<template v-else>
						<i18n-t
							scope="global"
							:keypath="
								props.nodeId
									? 'editor.hooks.time-expiration.existing-item-block-explanation'
									: 'editor.hooks.time-expiration.existing-item-full-document-explanation'
							"
							tag="div"
							class="w-full text-center text-xs text-muted-foreground"
						>
							<template #value>
								{{ $d(new Date(hookData.settings.schedule), "short") }}
							</template>
						</i18n-t>
					</template>
				</div>
				<ShadcnUiDropdownMenuSeparator />
				<div class="p-0.75">
					<div v-if="hookData" class="flex gap-1">
						<ShadcnUiButton
							v-if="hookData.score === 0"
							class="flex-1 gap-1"
							size="2sm"
							:disabled="
								!selectedDuration ||
								(selectedDuration === 'custom' && !selectedSchedule)
							"
							@click.stop="upsertHook"
						>
							<Icon name="mingcute:check-fill" />
							{{ $t("editor.hooks.renew") }}
						</ShadcnUiButton>
						<ShadcnUiButton
							class="flex-1 gap-1"
							variant="secondary"
							size="2sm"
							@click.stop="deleteHook"
						>
							<Icon name="mingcute:delete-2-line" />
							{{ $t("editor.hooks.delete") }}
						</ShadcnUiButton>
					</div>
					<template v-else>
						<ShadcnUiButton
							class="w-full gap-1"
							size="2sm"
							:disabled="
								!selectedDuration ||
								(selectedDuration === 'custom' && !selectedSchedule)
							"
							@click.stop="upsertHook"
						>
							<Icon name="mingcute:check-fill" />
							{{ $t("editor.hooks.create") }}
						</ShadcnUiButton>
					</template>
				</div>
			</div>
		</ShadcnUiDropdownMenuSubContent>
	</ShadcnUiDropdownMenuSub>
</template>

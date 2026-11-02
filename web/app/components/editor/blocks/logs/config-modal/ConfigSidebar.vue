<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import type { LogConfig } from "../utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import ConfigField from "../../config-modal-utils/ConfigField.vue"
import DataSourceSelect from "../../config-modal-utils/DataSourceSelect.vue"
import TimestampFormatSelect from "./TimestampFormatSelect.vue"
import OrderSelect from "./OrderSelect.vue"

const config = defineModel<LogConfig>({ required: true })
const props = defineProps<{
	diffStatus?: DiffStatus | null
	oldConfig?: LogConfig | null
}>()
const emit = defineEmits<{
	(e: "open-settings"): void
}>()

const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
// the old config of a modified block, the one its fields compare against
const modifiedFrom = computed(() =>
	props.diffStatus === DiffStatus.Modified ? (props.oldConfig ?? null) : null,
)
const isDataSourceIdModified = computed(
	() =>
		!!modifiedFrom.value &&
		modifiedFrom.value.dataSourceId !== config.value.dataSourceId,
)
const isTitleModified = computed(
	() => !!modifiedFrom.value && modifiedFrom.value.title !== config.value.title,
)
const isTimestampFormatModified = computed(
	() =>
		!!modifiedFrom.value &&
		modifiedFrom.value.timestampFormat !== config.value.timestampFormat,
)
const isOrderModified = computed(
	() => !!modifiedFrom.value && modifiedFrom.value.order !== config.value.order,
)
</script>
<template>
	<div
		:class="
			cn('flex w-full flex-col gap-2 py-1.75', isEditingDisabled && 'pb-2')
		"
	>
		<div>
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
					:old-config="modifiedFrom"
					:diff-status="DiffStatus.Removed"
				/>
			</div>
			<div class="mt-1 h-px bg-border" />
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.logs.config.title-label") }}
					</span>
				</template>
				<ShadcnUiInput
					v-model="config.title"
					type="text"
					:placeholder="
						isEditingDisabled
							? $t('editor.logs.config.title-empty-value-placeholder')
							: $t('editor.logs.config.title-placeholder')
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
					:model-value="modifiedFrom?.title"
					type="text"
					:placeholder="$t('editor.logs.config.title-empty-value-placeholder')"
					disable-focus-effect
					disable-destructive-effect
					transparent-disable
					class="h-[1.775rem]! w-full bg-diff-field-removed px-1.5 text-2sm!"
				/>
			</ConfigField>
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.logs.config.timestamp-format-label") }}
					</span>
				</template>
				<TimestampFormatSelect
					v-model="config.timestampFormat"
					:diff-status="isTimestampFormatModified ? DiffStatus.Added : null"
				/>
				<TimestampFormatSelect
					v-if="isTimestampFormatModified && modifiedFrom"
					:model-value="modifiedFrom.timestampFormat"
					:diff-status="DiffStatus.Removed"
				/>
			</ConfigField>
		</div>
		<div class="px-1.75">
			<ConfigField>
				<template #label>
					<span>
						{{ $t("editor.logs.config.order-label") }}
					</span>
				</template>
				<OrderSelect
					v-model="config.order"
					:is-modified="isOrderModified"
					:old-order="isOrderModified ? modifiedFrom?.order : null"
				/>
			</ConfigField>
		</div>
	</div>
</template>

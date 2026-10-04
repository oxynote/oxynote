<script setup lang="ts">
import HookConfigPanel from "../HookConfigPanel.vue"
import HookInputField from "../HookInputField.vue"
import HookNoticeValue from "../HookNoticeValue.vue"
import HookReadonlyField from "../HookReadonlyField.vue"
import { DiffStatus } from "../../diff/position-map"
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

const { t } = useI18n({ useScope: "global" })
const { isReadOnlyOrDiff } = useEditorMeta()
const actions = useHookActions({
	type: DocumentHookType.ContainerImageWatcher,
	nodeId: () => props.nodeId,
	hook: () => props.hook,
	close: close,
})

const activeBranchSettings = computed(
	() =>
		props.hook?.settings as
			DocumentHookSettingsContainerImageWatcher | undefined,
)
const targetBranchSettings = computed(
	() =>
		props.diff?.targetHook?.settings as
			DocumentHookSettingsContainerImageWatcher | undefined,
)
const status = computed(() => (props.hook ? hookStatus(props.hook) : null))
const selectedImage = ref<string | undefined>(activeBranchSettings.value?.image)
const isSubOpen = ref(false)
const isUnchanged = computed(
	() =>
		!selectedImage.value ||
		selectedImage.value === activeBranchSettings.value?.image,
)
const failure = computed(() =>
	props.hook
		? checkFailure(props.hook.status as DocumentHookStatusContainerImageWatcher)
		: null,
)
const subtitle = computed<HookSubtitle>(() => {
	if (!activeBranchSettings.value) {
		return { text: t("editor.hooks.container-image-watcher.description") }
	}

	const activeBranchImage = activeBranchSettings.value.image
	const targetBranchImage = targetBranchSettings.value?.image
	if (
		props.diff?.status === DiffStatus.Modified &&
		targetBranchImage &&
		targetBranchImage !== activeBranchImage
	) {
		return {
			text: t("editor.hooks.value-change", {
				old: targetBranchImage,
				new: activeBranchImage,
			}),
		}
	}

	return { text: activeBranchImage, detail: failure.value?.detail }
})
// the notice names the image as it is typed, and the stored one where it
// cannot be changed
const noticeImage = computed(() =>
	isReadOnlyOrDiff.value
		? activeBranchSettings.value?.image
		: selectedImage.value,
)

async function submit() {
	if (!selectedImage.value) {
		return
	}

	const next = { image: selectedImage.value }
	if (props.hook) {
		await actions.update(next)
		return
	}

	// the same form creates the next hook, so it is emptied once a hook is
	// created
	if (await actions.create(next)) {
		selectedImage.value = undefined
	}
}

// checkFailure says what the row and the notice show for a failed check,
// and is null while the check works. It has a case for every status and no
// default, so a status added later fails to compile until it is handled.
function checkFailure(
	checkStatus: DocumentHookStatusContainerImageWatcher,
): { detail: string; notice: string } | null {
	switch (checkStatus) {
		case "active":
			return null
		case "unauthorized":
			return {
				detail: t("editor.hooks.container-image-watcher.detail-unreachable"),
				notice: t("editor.hooks.container-image-watcher.unreachable-image"),
			}
		case "image_not_found":
			return {
				detail: t("editor.hooks.container-image-watcher.detail-missing"),
				notice: t("editor.hooks.container-image-watcher.missing-image"),
			}
	}
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
		icon="simple-icons:docker"
		:acknowledge-label="
			status === 'triggered' ? $t('editor.hooks.reset') : undefined
		"
		:submit-disabled="isUnchanged"
		@submit="submit"
		@delete="actions.remove"
		@acknowledge="actions.reset"
	>
		<template #title>
			{{ $t("editor.hooks.container-image-watcher.title") }}
		</template>
		<template #subtitle>
			{{ subtitle.text }}
		</template>
		<template v-if="subtitle.detail" #subtitle-detail>
			{{ subtitle.detail }}
		</template>
		<template #notice>
			<template v-if="failure">
				{{ failure.notice }}
			</template>
			<i18n-t
				v-else
				scope="global"
				:keypath="
					hookNoticeKeypath(DocumentHookType.ContainerImageWatcher, status)
				"
				tag="span"
			>
				<template #image>
					<HookNoticeValue
						:value="noticeImage"
						:fallback="
							$t('editor.hooks.container-image-watcher.image-fallback')
						"
					/>
				</template>
			</i18n-t>
		</template>
		<HookReadonlyField
			v-if="isReadOnlyOrDiff && activeBranchSettings"
			:label="$t('editor.hooks.container-image-watcher.image-input-label')"
			:rows="
				scalarFieldRows(
					activeBranchSettings.image,
					targetBranchSettings?.image ?? null,
				)
			"
			:diff-status="props.diff?.status"
		/>
		<HookInputField
			v-else
			v-model="selectedImage"
			:placeholder="
				$t('editor.hooks.container-image-watcher.image-input-placeholder')
			"
		>
			<template #label>
				<span>
					{{ $t("editor.hooks.container-image-watcher.image-input-label") }}
				</span>
			</template>
		</HookInputField>
	</HookConfigPanel>
</template>

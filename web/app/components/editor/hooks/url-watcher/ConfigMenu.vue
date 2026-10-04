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
	type: DocumentHookType.URLWatcher,
	nodeId: () => props.nodeId,
	hook: () => props.hook,
	close: close,
})

const activeBranchSettings = computed(
	() => props.hook?.settings as DocumentHookSettingsURLWatcher | undefined,
)
const targetBranchSettings = computed(
	() =>
		props.diff?.targetHook?.settings as
			DocumentHookSettingsURLWatcher | undefined,
)
const status = computed(() => (props.hook ? hookStatus(props.hook) : null))
const selectedURL = ref<string | undefined>(activeBranchSettings.value?.url)
const isSubOpen = ref(false)
const isUnchanged = computed(
	() =>
		!selectedURL.value ||
		ensureHttps(selectedURL.value) === activeBranchSettings.value?.url,
)
const failure = computed(() =>
	props.hook
		? checkFailure(props.hook.status as DocumentHookStatusURLWatcher)
		: null,
)
const subtitle = computed<HookSubtitle>(() => {
	if (!activeBranchSettings.value) {
		return { text: t("editor.hooks.url-watcher.description") }
	}

	const activeBranchURL = displayURL(activeBranchSettings.value.url)
	const targetBranchURL = targetBranchSettings.value?.url
	if (
		props.diff?.status === DiffStatus.Modified &&
		targetBranchURL &&
		targetBranchURL !== activeBranchSettings.value.url
	) {
		return {
			text: t("editor.hooks.value-change", {
				old: displayURL(targetBranchURL),
				new: activeBranchURL,
			}),
		}
	}

	if (failure.value) {
		return { text: activeBranchURL, detail: failure.value.detail }
	}

	return status.value === "triggered"
		? {
				text: activeBranchURL,
				detail: t("editor.hooks.url-watcher.detail-triggered"),
			}
		: { text: activeBranchURL }
})
// the notice names the page as it is typed, and the stored one where it
// cannot be changed
const noticeURL = computed(() => {
	const url = isReadOnlyOrDiff.value
		? activeBranchSettings.value?.url
		: selectedURL.value
	return url ? displayURL(url) : null
})

async function submit() {
	if (!selectedURL.value) {
		return
	}

	const next = { url: ensureHttps(selectedURL.value) }
	if (props.hook) {
		await actions.update(next)
		return
	}

	// the same form creates the next hook, so it is emptied once a hook is
	// created
	if (await actions.create(next)) {
		selectedURL.value = undefined
	}
}

// checkFailure says what the row and the notice show for a failed check,
// and is null while the check works. It has a case for every status and no
// default, so a status added later fails to compile until it is handled.
function checkFailure(
	checkStatus: DocumentHookStatusURLWatcher,
): { detail: string; notice: string } | null {
	switch (checkStatus) {
		case "active":
			return null
		case "unconfigured":
			return {
				detail: t("editor.hooks.url-watcher.detail-unconfigured"),
				notice: t("editor.hooks.url-watcher.unconfigured"),
			}
		case "unreachable_url":
			return {
				detail: t("editor.hooks.url-watcher.detail-unreachable"),
				notice: t("editor.hooks.url-watcher.unreachable-url"),
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
		icon="mingcute:earth-2-line"
		:acknowledge-label="
			status === 'triggered' ? $t('editor.hooks.reset') : undefined
		"
		:submit-disabled="isUnchanged"
		@submit="submit"
		@delete="actions.remove"
		@acknowledge="actions.reset"
	>
		<template #title>
			{{ $t("editor.hooks.url-watcher.title") }}
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
				:keypath="hookNoticeKeypath(DocumentHookType.URLWatcher, status)"
				tag="span"
			>
				<template #url>
					<HookNoticeValue
						:value="noticeURL"
						:fallback="$t('editor.hooks.url-watcher.url-fallback')"
					/>
				</template>
			</i18n-t>
		</template>
		<HookReadonlyField
			v-if="isReadOnlyOrDiff && activeBranchSettings"
			:label="$t('editor.hooks.url-watcher.url-input-label')"
			:rows="
				scalarFieldRows(
					activeBranchSettings.url,
					targetBranchSettings?.url ?? null,
				)
			"
			:diff-status="props.diff?.status"
		/>
		<HookInputField
			v-else
			v-model="selectedURL"
			type="url"
			:placeholder="$t('editor.hooks.url-watcher.url-input-placeholder')"
		>
			<template #label>
				<span>
					{{ $t("editor.hooks.url-watcher.url-input-label") }}
				</span>
			</template>
		</HookInputField>
	</HookConfigPanel>
</template>

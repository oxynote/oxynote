<script setup lang="ts">
import { showToastMessage } from "~/components/toast"
import HookInputField from "./HookInputField.vue"
import HookReadonlyField from "../HookReadonlyField.vue"
import HookSubTrigger from "../HookSubTrigger.vue"
import HookExplanation from "../HookExplanation.vue"
import { scalarFieldRows, type HookDiffContext } from "../hook-diff"

const props = defineProps<{
	hook?: DocumentHook | null | undefined // null/undefined means creating new
	// set only while the diff is shown
	diff?: HookDiffContext | null
	nodeId: string | null // null means global
}>()
const emit = defineEmits<{
	(e: "force-close"): void
}>()

const editorStore = useEditorStore()
const { isReadOnlyOrDiff } = useEditorMeta()
const targetSettings = computed(
	() =>
		props.diff?.targetHook?.settings as
			DocumentHookSettingsURLWatcher | undefined,
)
const { t } = useI18n({ useScope: "global" })
const hookData = computed(() => {
	if (!props.hook) {
		return null
	}

	return {
		score: Number(props.hook.score),
		state: props.hook.state as DocumentHookStateURLWatcher,
		settings: props.hook.settings as DocumentHookSettingsURLWatcher,
	}
})

const documentHookAPI = useDocumentHookAPI()
const confirmedURL = ref<string | undefined>(
	hookData.value ? hookData.value.settings.url : undefined,
)
const selectedURL = ref<string | undefined>(confirmedURL.value)

const isSubOpen = ref(false)

async function upsertHook() {
	if (
		!selectedURL.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	isSubOpen.value = false
	emit("force-close")

	if (!props.hook) {
		try {
			await documentHookAPI.createDocumentHookByDocID.mutateAsync({
				docId: editorStore.activeDocumentId,
				req: {
					type: DocumentHookType.URLWatcher,
					branchId: editorStore.activeBranchId,
					blockId: props.nodeId,
					settings: {
						url: ensureHttps(selectedURL.value),
					},
				},
			})
		} catch {
			showToastMessage("error", t("editor.hooks.errors.create-failed"))
			return
		}

		confirmedURL.value = selectedURL.value

		// since "create new" is reused, reset the state
		selectedURL.value = undefined

		return
	}

	try {
		await documentHookAPI.updateDocumentHookByDocID.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			hookId: props.hook.id,
			req: {
				settings: {
					url: ensureHttps(selectedURL.value),
				},
			},
		})
	} catch {
		showToastMessage("error", t("editor.hooks.errors.update-failed"))
		return
	}

	confirmedURL.value = selectedURL.value
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

	selectedURL.value = undefined
}

async function resetHook() {
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
		await documentHookAPI.resetDocumentHookByDocID.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			hookId: props.hook.id,
		})
	} catch {
		showToastMessage("error", t("editor.hooks.errors.reset-failed"))
		return
	}

	selectedURL.value = undefined
}
</script>
<template>
	<ShadcnUiDropdownMenuSub v-model:open="isSubOpen">
		<HookSubTrigger :hook="props.hook" :diff="props.diff">
			<div class="relative h-[0.8125rem] w-[0.8125rem] shrink-0">
				<Icon
					name="mingcute:earth-2-line"
					class="absolute top-1/2 left-1/2 size-3.75 -translate-x-1/2 -translate-y-1/2"
				/>
			</div>
			<span v-if="!hookData">
				{{ $t("editor.hooks.url-watcher.title") }}
			</span>
			<i18n-t
				v-else-if="Number(hookData.score) !== 0"
				scope="global"
				keypath="editor.hooks.url-watcher.existing-item"
				tag="span"
				class="truncate"
			>
				<template #url>
					{{ extractDomain(confirmedURL || "") }}
				</template>
			</i18n-t>
			<i18n-t
				v-else
				scope="global"
				keypath="editor.hooks.url-watcher.triggered-item"
				tag="span"
				class="truncate"
			>
				<template #url>
					{{ extractDomain(confirmedURL || "") }}
				</template>
			</i18n-t>
		</HookSubTrigger>
		<ShadcnUiDropdownMenuSubContent
			side="right"
			align="start"
			loop
			:class="[
				'pointer-events-auto!' /* for some reason ShadcnUiSelect disables pointer events, which closes the whole sub menu when the select is closed, so we must override this */,
			]"
		>
			<div class="flex w-[14rem] flex-col">
				<template v-if="hookData?.state.status === 'unreachable_url'">
					<i18n-t
						scope="global"
						keypath="editor.hooks.url-watcher.unreachable-url"
						tag="div"
						class="px-0.75 pb-0.75 text-center text-2sm"
					>
						<template #icon>
							<Icon
								name="mingcute:alert-fill"
								class="mr-0.5 inline-block -translate-y-px align-middle text-status-warning"
							/>
						</template>
					</i18n-t>
					<ShadcnUiDropdownMenuSeparator />
				</template>
				<template v-if="hookData && hookData.state.status === 'active'">
					<HookExplanation
						:type="DocumentHookType.URLWatcher"
						:triggered="hookData.score === 0"
						:node-id="props.nodeId"
					/>
					<ShadcnUiDropdownMenuSeparator />
				</template>
				<div class="flex flex-col gap-1 px-0.75 pb-0.75">
					<HookReadonlyField
						v-if="isReadOnlyOrDiff && hookData"
						:label="$t('editor.hooks.url-watcher.url-input-label')"
						:rows="
							scalarFieldRows(
								hookData.settings.url,
								targetSettings?.url ?? null,
							)
						"
					/>
					<HookInputField
						v-else
						v-model="selectedURL"
						:placeholder="$t('editor.hooks.url-watcher.url-input-placeholder')"
					>
						<template #label>
							<span>
								{{ $t("editor.hooks.url-watcher.url-input-label") }}
							</span>
						</template>
					</HookInputField>
				</div>
				<ShadcnUiDropdownMenuSeparator v-if="!isReadOnlyOrDiff" />
				<div v-if="!isReadOnlyOrDiff" class="p-0.75">
					<div v-if="hookData" class="flex flex-col gap-1">
						<div class="flex gap-1">
							<ShadcnUiButton
								class="flex-1 gap-1"
								:variant="hookData?.score === 0 ? 'outline' : 'default'"
								size="2sm"
								:disabled="!selectedURL || selectedURL === confirmedURL"
								@click.stop="upsertHook"
							>
								<Icon name="mingcute:save-2-line" />
								{{ $t("editor.hooks.update") }}
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
						<ShadcnUiButton
							v-if="hookData.score === 0"
							class="gap-1"
							size="2sm"
							@click.stop="resetHook"
						>
							<Icon name="mingcute:check-fill" />
							{{ $t("editor.hooks.reset") }}
						</ShadcnUiButton>
					</div>
					<template v-else>
						<ShadcnUiButton
							class="w-full gap-1"
							size="2sm"
							:disabled="!selectedURL || selectedURL === confirmedURL"
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

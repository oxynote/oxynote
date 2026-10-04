<script setup lang="ts">
import FileSelectInput from "./FileSelectInput.vue"
import { cn } from "~/lib/utils"
import HookConfigPanel from "../HookConfigPanel.vue"
import HookNoticeValue from "../HookNoticeValue.vue"
import HookReadonlyField from "../HookReadonlyField.vue"
import { useHookActions } from "../hook-actions"
import {
	listFieldRows,
	scalarFieldRows,
	type HookDiffContext,
} from "../hook-diff"
import { hookNoticeKeypath, type HookSubtitle } from "../hook-menu"
import { hookStatus, type HookStatus } from "../hook-status"

const props = defineProps<{
	hook?: DocumentHook | null | undefined // null/undefined means creating new
	// set only while the diff is shown
	diff?: HookDiffContext | null
	nodeId: string | null // null means global
}>()
const emit = defineEmits<{
	(e: "force-close"): void
	(e: "open-settings", target: "github"): void
}>()

const { t } = useI18n({ useScope: "global" })
const { isReadOnlyOrDiff } = useEditorMeta()
const actions = useHookActions({
	type: DocumentHookType.GitHubTracking,
	nodeId: () => props.nodeId,
	hook: () => props.hook,
	close: close,
})
const {
	fetchGitHubConnectionStatus,
	fetchGitHubRepositories,
	useFetchGitHubBranchesByRepositoryName,
	useFetchGitHubFileTreeByRepositoryNameAndBranch,
} = useGitHubAPI()

const activeBranchSettings = computed(
	() => props.hook?.settings as DocumentHookSettingsGitHubTracking | undefined,
)
const targetBranchSettings = computed(
	() =>
		props.diff?.targetHook?.settings as
			DocumentHookSettingsGitHubTracking | undefined,
)
const checkStatus = computed(
	() => props.hook?.status as DocumentHookStatusGitHubTracking | undefined,
)
const status = computed(() => (props.hook ? hookStatus(props.hook) : null))
const selectedRepository = ref<string | undefined>(
	activeBranchSettings.value?.repository,
)
const selectedBranch = ref<string | undefined>(
	activeBranchSettings.value?.branch,
)
const selectedPaths = ref<string[] | undefined>(
	activeBranchSettings.value?.paths,
)
const fetchGitHubBranches =
	useFetchGitHubBranchesByRepositoryName(selectedRepository)
const fetchGitHubPaths = useFetchGitHubFileTreeByRepositoryNameAndBranch(
	selectedRepository,
	selectedBranch,
)
const isSubOpen = ref(false)
const invalidData = computed(() => {
	return (
		!selectedRepository.value ||
		!selectedBranch.value ||
		!selectedPaths.value?.length ||
		(selectedRepository.value === activeBranchSettings.value?.repository &&
			selectedBranch.value === activeBranchSettings.value.branch &&
			arraysEqual(selectedPaths.value, activeBranchSettings.value.paths))
	)
})
const failure = computed(() =>
	checkStatus.value ? checkFailure(checkStatus.value) : null,
)
// a server without GitHub comes first, since no setup an editor does can
// fix it. Then the setup an editor still has to do, then what the last
// check found.
const notice = computed(() => {
	if (failure.value?.notice === "unconfigured") {
		return "unconfigured"
	}

	if (
		failure.value?.notice === "not-connected" ||
		(!isReadOnlyOrDiff.value &&
			!fetchGitHubConnectionStatus.data.value?.connected)
	) {
		return "not-connected"
	}

	if (
		!isReadOnlyOrDiff.value &&
		!fetchGitHubRepositories.state.value.data?.length
	) {
		return "no-repositories"
	}

	return failure.value?.notice ?? "summary"
})
// setup is not the hook's own state, so it shows plain unless the last
// check failed over it
const noticeStatus = computed<HookStatus | undefined>(() =>
	(notice.value === "not-connected" || notice.value === "no-repositories") &&
	status.value !== "needs-attention"
		? "fresh"
		: undefined,
)
const subtitle = computed<HookSubtitle>(() => {
	if (!activeBranchSettings.value) {
		return { text: t("editor.hooks.github-tracking.description") }
	}

	return {
		text: t("editor.hooks.github-tracking.subtext", {
			repository: activeBranchSettings.value.repository,
			branch: activeBranchSettings.value.branch,
		}),
		detail:
			failure.value?.detail ??
			fileCount(activeBranchSettings.value.paths.length),
	}
})
// the notice names the picks as they are made, and the stored ones where
// they cannot be changed
const noticeFiles = computed(() => {
	const paths = isReadOnlyOrDiff.value
		? activeBranchSettings.value?.paths
		: selectedPaths.value
	if (!paths?.length) {
		return null
	}

	return paths.length > 2 ? fileCount(paths.length) : paths.join(", ")
})
const noticeBranch = computed(() =>
	isReadOnlyOrDiff.value
		? activeBranchSettings.value?.branch
		: selectedBranch.value,
)

onMounted(async () => {
	await Promise.all([
		fetchGitHubConnectionStatus.refresh(),
		fetchGitHubRepositories.refresh(),
	])
})

async function submit() {
	const repository = selectedRepository.value
	const branch = selectedBranch.value
	const paths = selectedPaths.value
	if (invalidData.value || !repository || !branch || !paths) {
		return
	}

	const next = { repository: repository, branch: branch, paths: paths }
	if (props.hook) {
		await actions.update(next)
		return
	}

	// the same form creates the next hook, so it is emptied once a hook is
	// created
	if (await actions.create(next)) {
		selectedRepository.value = undefined
		selectedBranch.value = undefined
		selectedPaths.value = undefined
	}
}

function fileCount(count: number): string {
	return count === 1
		? t("editor.hooks.github-tracking.file-count.one")
		: t("editor.hooks.github-tracking.file-count.other", { count: count })
}

// checkFailure says what the row and the notice show for a failed check,
// and is null while the check works. It has a case for every status and no
// default, so a status added later fails to compile until it is handled.
function checkFailure(checkStatus: DocumentHookStatusGitHubTracking): {
	detail: string
	notice:
		| "unconfigured"
		| "not-connected"
		| "missing-repository"
		| "missing-branch"
		| "tree-truncated"
} | null {
	switch (checkStatus) {
		case "active":
			return null
		case "unconfigured":
			return {
				detail: t("editor.hooks.github-tracking.problems.unconfigured"),
				notice: "unconfigured",
			}
		case "missing_installation":
			return {
				detail: t("editor.hooks.github-tracking.problems.missing-installation"),
				notice: "not-connected",
			}
		case "missing_repository":
			return {
				detail: t("editor.hooks.github-tracking.problems.missing-repository"),
				notice: "missing-repository",
			}
		case "missing_branch":
			return {
				detail: t("editor.hooks.github-tracking.problems.missing-branch"),
				notice: "missing-branch",
			}
		case "tree_truncated":
			return {
				detail: t("editor.hooks.github-tracking.problems.tree-truncated"),
				notice: "tree-truncated",
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
		icon="simple-icons:github"
		:notice-status="noticeStatus"
		:notice-icon="noticeStatus ? 'mingcute:information-fill' : undefined"
		:acknowledge-label="
			status === 'triggered' && checkStatus === 'active'
				? $t('editor.hooks.reset')
				: undefined
		"
		:submit-disabled="invalidData"
		@submit="submit"
		@delete="actions.remove"
		@acknowledge="actions.reset"
	>
		<template #title>
			{{ $t("editor.hooks.github-tracking.title") }}
		</template>
		<template #subtitle>
			{{ subtitle.text }}
		</template>
		<template v-if="subtitle.detail" #subtitle-detail>
			{{ subtitle.detail }}
		</template>
		<template #notice>
			<i18n-t
				v-if="notice === 'not-connected'"
				scope="global"
				keypath="editor.hooks.github-tracking.not-connected.main"
				tag="span"
			>
				<template #placeholder>
					<ShadcnUiButton
						type="button"
						variant="link"
						size="custom"
						class="h-fit p-0 text-xs"
						@click="emit('open-settings', 'github')"
					>
						{{ $t("editor.hooks.github-tracking.not-connected.placeholder") }}
					</ShadcnUiButton>
				</template>
			</i18n-t>
			<template v-else-if="notice === 'no-repositories'">
				{{ $t("editor.hooks.github-tracking.no-repositories") }}
			</template>
			<template v-else-if="notice === 'missing-repository'">
				{{ $t("editor.hooks.github-tracking.missing-target-repository") }}
			</template>
			<template v-else-if="notice === 'missing-branch'">
				{{ $t("editor.hooks.github-tracking.missing-target-branch") }}
			</template>
			<template v-else-if="notice === 'tree-truncated'">
				{{ $t("editor.hooks.github-tracking.tree-truncated") }}
			</template>
			<template v-else-if="notice === 'unconfigured'">
				{{ $t("editor.hooks.github-tracking.unconfigured") }}
			</template>
			<i18n-t
				v-else
				scope="global"
				:keypath="hookNoticeKeypath(DocumentHookType.GitHubTracking, status)"
				tag="span"
			>
				<template #files>
					<HookNoticeValue
						:value="noticeFiles"
						:fallback="$t('editor.hooks.github-tracking.files-fallback')"
					/>
				</template>
				<template #branch>
					<HookNoticeValue
						:value="noticeBranch"
						:fallback="$t('editor.hooks.github-tracking.branch-fallback')"
					/>
				</template>
			</i18n-t>
		</template>
		<div
			v-if="isReadOnlyOrDiff && activeBranchSettings"
			class="flex flex-col gap-1"
		>
			<HookReadonlyField
				:label="$t('editor.hooks.github-tracking.repository-select-label')"
				:rows="
					scalarFieldRows(
						activeBranchSettings.repository,
						targetBranchSettings?.repository ?? null,
					)
				"
				:diff-status="props.diff?.status"
			/>
			<HookReadonlyField
				:label="$t('editor.hooks.github-tracking.branch-select-label')"
				:rows="
					scalarFieldRows(
						activeBranchSettings.branch,
						targetBranchSettings?.branch ?? null,
					)
				"
				:diff-status="props.diff?.status"
			/>
			<HookReadonlyField
				:label="$t('editor.hooks.github-tracking.path-select-label')"
				:rows="
					listFieldRows(
						activeBranchSettings.paths,
						targetBranchSettings?.paths ?? null,
					)
				"
				:diff-status="props.diff?.status"
			/>
		</div>
		<div
			v-else
			:class="
				cn(
					'flex flex-col gap-1 opacity-100 transition-opacity duration-200',
					(!fetchGitHubConnectionStatus.data.value?.connected ||
						checkStatus === 'missing_installation' ||
						checkStatus === 'unconfigured') &&
						'pointer-events-none opacity-60',
				)
			"
		>
			<ShadcnUiSelect
				v-model="selectedRepository"
				:disabled="
					!fetchGitHubConnectionStatus.data.value?.connected ||
					fetchGitHubRepositories.isLoading.value ||
					fetchGitHubRepositories.state.value.data?.length === 0 ||
					(!!checkStatus &&
						checkStatus !== 'active' &&
						checkStatus !== 'missing_repository' &&
						checkStatus !== 'missing_branch' &&
						checkStatus !== 'tree_truncated')
				"
			>
				<ShadcnUiSelectLabel>
					<span class="text-2sm">
						{{ $t("editor.hooks.github-tracking.repository-select-label") }}
					</span>
				</ShadcnUiSelectLabel>
				<ShadcnUiSelectTrigger class="w-full" size="custom">
					<ShadcnUiSelectValue
						class="text-2sm"
						:placeholder="
							fetchGitHubRepositories.isLoading.value
								? $t(
										'editor.hooks.github-tracking.repository-select-loading-placeholder',
									)
								: $t(
										'editor.hooks.github-tracking.repository-select-placeholder',
									)
						"
					/>
				</ShadcnUiSelectTrigger>
				<ShadcnUiSelectContent
					class="max-h-[40dvh] max-w-[15rem]"
					side="bottom"
					align="start"
					body-lock
				>
					<ShadcnUiSelectItem
						v-for="item in fetchGitHubRepositories.state.value.data"
						:key="item.name"
						:value="item.name"
						class="text-2sm"
					>
						<div class="flex-1 truncate">
							{{ item.name }}
						</div>
					</ShadcnUiSelectItem>
				</ShadcnUiSelectContent>
			</ShadcnUiSelect>
			<ShadcnUiSelect
				v-model="selectedBranch"
				:disabled="
					!selectedRepository ||
					fetchGitHubBranches.isLoading.value ||
					(!!checkStatus &&
						checkStatus !== 'active' &&
						checkStatus !== 'missing_branch' &&
						checkStatus !== 'tree_truncated')
				"
			>
				<ShadcnUiSelectLabel>
					<span class="text-2sm">
						{{ $t("editor.hooks.github-tracking.branch-select-label") }}
					</span>
				</ShadcnUiSelectLabel>
				<ShadcnUiSelectTrigger class="w-full" size="custom">
					<ShadcnUiSelectValue
						class="text-2sm"
						:placeholder="
							!selectedRepository
								? $t(
										'editor.hooks.github-tracking.branch-select-repository-placeholder',
									)
								: fetchGitHubBranches.isLoading.value
									? $t(
											'editor.hooks.github-tracking.branch-select-loading-placeholder',
										)
									: $t('editor.hooks.github-tracking.branch-select-placeholder')
						"
					/>
				</ShadcnUiSelectTrigger>
				<ShadcnUiSelectContent
					class="max-h-[40dvh] max-w-[15rem]"
					side="bottom"
					align="start"
				>
					<ShadcnUiSelectItem
						v-for="branch in fetchGitHubBranches.state.value.data"
						:key="branch"
						:value="branch"
						class="text-2sm"
					>
						<div class="flex-1 truncate">
							{{ branch }}
						</div>
					</ShadcnUiSelectItem>
				</ShadcnUiSelectContent>
			</ShadcnUiSelect>
			<FileSelectInput
				v-model="selectedPaths"
				class="w-full"
				:disabled="
					!selectedRepository ||
					!selectedBranch ||
					fetchGitHubPaths.isLoading.value ||
					(!!checkStatus && checkStatus !== 'active')
				"
				:options="fetchGitHubPaths.state.value.data || []"
				:placeholder="
					!selectedRepository
						? $t(
								'editor.hooks.github-tracking.path-select-repository-placeholder',
							)
						: !selectedBranch
							? $t(
									'editor.hooks.github-tracking.path-select-branch-placeholder',
								)
							: fetchGitHubPaths.isLoading.value
								? $t(
										'editor.hooks.github-tracking.path-select-loading-placeholder',
									)
								: $t('editor.hooks.github-tracking.path-select-placeholder')
				"
				:empty-folder-placeholder="
					$t('editor.hooks.github-tracking.empty-folder-placeholder')
				"
			>
				<template #label>
					<span>
						{{ $t("editor.hooks.github-tracking.path-select-label") }}
					</span>
				</template>
				<template #selection-text="{ count }">
					<i18n-t
						v-if="count > 1"
						scope="global"
						keypath="editor.hooks.github-tracking.selection-text.other"
						tag="span"
					>
						<template #count>{{ count }}</template>
					</i18n-t>
					<i18n-t
						v-if="count === 1"
						scope="global"
						keypath="editor.hooks.github-tracking.selection-text.one"
						tag="span"
					/>
				</template>
			</FileSelectInput>
		</div>
	</HookConfigPanel>
</template>

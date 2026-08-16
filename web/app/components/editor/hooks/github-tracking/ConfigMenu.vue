<script setup lang="ts">
import { showToastMessage } from "~/components/toast"
import FileSelectInput from "./FileSelectInput.vue"
import { cn } from "~/lib/utils"

const props = defineProps<{
	hook?: DocumentHook | null | undefined // null/undefined means creating new
	nodeId: string | null // null means global
}>()
const emit = defineEmits<{
	(e: "force-close"): void
	(e: "open-settings", target: "github"): void
}>()

const editorStore = useEditorStore()
const hookData = computed(() => {
	if (!props.hook) {
		return null
	}

	return {
		score: Number(props.hook.score),
		state: props.hook.state as DocumentHookStateGitHubTracking,
		settings: props.hook.settings as DocumentHookSettingsGitHubTracking,
	}
})
const documentHookAPI = useDocumentHookAPI()
const { fetchGitHubConnectionStatus } = useGitHubAPI()
const { t } = useI18n({ useScope: "global" })
const confirmedRepository = ref<string | undefined>(
	hookData.value ? hookData.value.settings.repository : undefined,
)
const selectedRepository = ref<string | undefined>(confirmedRepository.value)
const confirmedBranch = ref<string | undefined>(
	hookData.value ? hookData.value.settings.branch : undefined,
)
const selectedBranch = ref<string | undefined>(confirmedBranch.value)
const confirmedPaths = ref<string[] | undefined>(
	hookData.value ? hookData.value.settings.paths : undefined,
)
const selectedPaths = ref<string[] | undefined>(confirmedPaths.value)
const invalidData = computed(() => {
	return (
		!selectedRepository.value ||
		!selectedBranch.value ||
		!selectedPaths.value?.length ||
		(selectedRepository.value === confirmedRepository.value &&
			selectedBranch.value === confirmedBranch.value &&
			arraysEqual(selectedPaths.value, confirmedPaths.value))
	)
})
const {
	fetchGitHubRepositories,
	useFetchGitHubBranchesByRepositoryName,
	useFetchGitHubFileTreeByRepositoryNameAndBranch,
} = useGitHubAPI()
const fetchGitHubBranches =
	useFetchGitHubBranchesByRepositoryName(selectedRepository)
const fetchGitHubPaths = useFetchGitHubFileTreeByRepositoryNameAndBranch(
	selectedRepository,
	selectedBranch,
)

const isSubOpen = ref(false)

onMounted(async () => {
	await Promise.all([
		fetchGitHubConnectionStatus.refresh(),
		fetchGitHubRepositories.refresh(),
	])
})

async function upsertHook() {
	if (
		invalidData.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	isSubOpen.value = false
	emit("force-close")

	const repository = selectedRepository.value
	const branch = selectedBranch.value
	const paths = selectedPaths.value

	if (!repository || !branch || !paths) {
		return
	}

	if (!props.hook) {
		try {
			await documentHookAPI.createDocumentHookByDocID.mutateAsync({
				docId: editorStore.activeDocumentId,
				req: {
					type: DocumentHookType.GitHubTracking,
					branchId: editorStore.activeBranchId,
					blockId: props.nodeId,
					settings: {
						repository,
						branch,
						paths,
					},
				},
			})
		} catch {
			showToastMessage("error", t("editor.hooks.errors.create-failed"))
			return
		}

		confirmedRepository.value = selectedRepository.value
		confirmedBranch.value = selectedBranch.value
		confirmedPaths.value = selectedPaths.value

		// since "create new" is reused, reset the state
		selectedRepository.value = undefined
		selectedBranch.value = undefined
		selectedPaths.value = undefined

		return
	}

	try {
		await documentHookAPI.updateDocumentHookByDocID.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			hookId: props.hook.id,
			req: {
				settings: {
					repository,
					branch,
					paths,
				},
			},
		})
	} catch {
		showToastMessage("error", t("editor.hooks.errors.update-failed"))
		return
	}

	confirmedRepository.value = selectedRepository.value
	confirmedBranch.value = selectedBranch.value
	confirmedPaths.value = selectedPaths.value
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

	selectedRepository.value = undefined
	selectedBranch.value = undefined
	selectedPaths.value = undefined
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
}
</script>
<template>
	<ShadcnUiDropdownMenuSub v-model:open="isSubOpen">
		<ShadcnUiDropdownMenuSubTrigger>
			<Icon name="simple-icons:github" class="shrink-0" />
			<span v-if="!hookData">
				{{ $t("editor.hooks.github-tracking.title") }}
			</span>
			<i18n-t
				v-else-if="Number(hookData.score) !== 0"
				keypath="editor.hooks.github-tracking.existing-item"
				tag="span"
				class="truncate"
			>
				<template #repository>
					{{ confirmedRepository }}
				</template>
			</i18n-t>
			<i18n-t
				v-else
				keypath="editor.hooks.github-tracking.triggered-item"
				tag="span"
				class="truncate"
			>
				<template #repository>
					{{ confirmedRepository }}
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
			<div class="flex w-[14rem] flex-col">
				<template
					v-if="
						!fetchGitHubConnectionStatus.data.value?.connected ||
						hookData?.state.status === 'missing_installation'
					"
				>
					<i18n-t
						keypath="editor.hooks.github-tracking.not-connected.main"
						tag="div"
						class="px-0.75 pb-0.75 text-center text-2sm"
					>
						<template #icon>
							<Icon
								name="mingcute:information-fill"
								class="mr-0.5 inline-block -translate-y-px align-middle text-status-info"
							/>
						</template>
						<template #placeholder>
							<ShadcnUiButton
								type="button"
								variant="link"
								size="custom"
								class="h-fit p-0 text-2sm"
								@click="emit('open-settings', 'github')"
							>
								{{
									$t("editor.hooks.github-tracking.not-connected.placeholder")
								}}
							</ShadcnUiButton>
						</template>
					</i18n-t>
					<ShadcnUiDropdownMenuSeparator />
				</template>
				<template v-else-if="!fetchGitHubRepositories.state.value.data?.length">
					<i18n-t
						keypath="editor.hooks.github-tracking.no-repositories"
						tag="div"
						class="px-0.75 pb-0.75 text-center text-2sm"
					>
						<template #icon>
							<Icon
								name="mingcute:information-fill"
								class="mr-0.5 inline-block -translate-y-px align-middle text-status-info"
							/>
						</template>
					</i18n-t>
					<ShadcnUiDropdownMenuSeparator />
				</template>
				<template v-else-if="hookData?.state.status === 'missing_repository'">
					<i18n-t
						keypath="editor.hooks.github-tracking.missing-target-repository"
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
				<template v-else-if="hookData?.state.status === 'missing_branch'">
					<i18n-t
						keypath="editor.hooks.github-tracking.missing-target-branch"
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
				<div
					:class="
						cn(
							'flex flex-col opacity-100 transition-opacity duration-200',
							(!fetchGitHubConnectionStatus.data.value?.connected ||
								hookData?.state.status === 'missing_installation') &&
								'pointer-events-none opacity-60',
						)
					"
				>
					<template
						v-if="
							hookData &&
							hookData.score !== 0 &&
							hookData.state.status === 'active'
						"
					>
						<div class="flex flex-col gap-1 px-0.75 pb-0.75 text-2sm">
							<i18n-t
								:keypath="
									props.nodeId
										? 'editor.hooks.github-tracking.existing-item-block-explanation'
										: 'editor.hooks.github-tracking.existing-item-full-document-explanation'
								"
								tag="div"
								class="w-full text-center text-xs break-words text-muted-foreground"
							/>
						</div>
						<ShadcnUiDropdownMenuSeparator />
					</template>
					<div class="flex flex-col gap-1 px-0.75 pb-0.75">
						<ShadcnUiSelect
							v-model="selectedRepository"
							:disabled="
								!fetchGitHubConnectionStatus.data.value?.connected ||
								fetchGitHubRepositories.isLoading.value ||
								fetchGitHubRepositories.state.value.data?.length === 0 ||
								(!!hookData &&
									hookData.state.status !== 'active' &&
									hookData.state.status !== 'missing_repository' &&
									hookData.state.status !== 'missing_branch')
							"
						>
							<ShadcnUiSelectLabel>
								<span class="text-2sm">
									{{
										$t("editor.hooks.github-tracking.repository-select-label")
									}}
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
								(!!hookData &&
									hookData.state.status !== 'active' &&
									hookData.state.status !== 'missing_branch')
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
												: $t(
														'editor.hooks.github-tracking.branch-select-placeholder',
													)
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
								(!!hookData && hookData.state.status !== 'active')
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
											: $t(
													'editor.hooks.github-tracking.path-select-placeholder',
												)
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
									keypath="editor.hooks.github-tracking.selection-text.other"
									tag="span"
								>
									<template #count>{{ count }}</template>
								</i18n-t>
								<i18n-t
									v-if="count === 1"
									keypath="editor.hooks.github-tracking.selection-text.one"
									tag="span"
								/>
							</template>
						</FileSelectInput>
					</div>
					<ShadcnUiDropdownMenuSeparator />
				</div>
				<div class="p-0.75">
					<div v-if="hookData" class="flex flex-col gap-1">
						<div class="flex gap-1">
							<ShadcnUiButton
								class="flex-1 gap-1"
								:variant="hookData?.score === 0 ? 'outline' : 'default'"
								size="2sm"
								:disabled="invalidData"
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
							v-if="hookData?.score === 0 && hookData.state.status === 'active'"
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
							:disabled="invalidData"
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

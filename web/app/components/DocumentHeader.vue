<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type { DocumentBreadcrumb } from "./sidebar"
import { ExperimentalFeature } from "~/composables/useExperimentalFeatures"
import { showToastMessage } from "./toast"

const props = defineProps<{
	allInitialSectionsLoaded: boolean
	breadcrumbs: DocumentBreadcrumb[] | null // null indicates that the document is still loading or missing
	timestamps: DocumentTimestamps | null // null indicates that the document is still loading or missing
}>()
const emit = defineEmits<{
	(e: "delete-document" | "duplicate-document"): void
}>()

const { isEditable, toggleIsEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { isExperimentalFeatureEnabled } = useExperimentalFeatures()
const aiAssistantEnabled = isExperimentalFeatureEnabled(
	ExperimentalFeature.AIAssistant,
)
const { useFetchDocumentBranchesByDocId } = useDocumentAPI()
const fetchBranches = useFetchDocumentBranchesByDocId(
	() => editorStore.activeDocumentId,
)
const header = useTemplateRef("header")
const { height } = useElementSize(header)
const { t } = useI18n({ useScope: "global" })
const { updateDocumentBranch, createDocumentBranch, deleteDocumentBranch } =
	useDocumentAPI()
const isReviewable = computed(() => {
	// NOTE this will be removed in the future
	const branches = fetchBranches.state.value.data
	return !!branches && branches.length > 1
})
const activeBranchMode = computed(() => {
	// NOTE this will be removed in the future
	const branches = fetchBranches.state.value.data
	if (!branches) {
		return "main"
	}

	const activeBranch = branches.find(
		(b) => b.branchId === editorStore.activeBranchId,
	)
	if (!activeBranch) {
		return "main"
	}

	return activeBranch.default ? "main" : "draft"
})

if (import.meta.client) {
	watchImmediate(height, (v) => {
		document.documentElement.style.setProperty(
			"--document-header-height",
			`${v}px`,
		)
	})
}

async function toggleReviewability() {
	if (!editorStore.activeDocumentId || !editorStore.mappedDefaultBranchId) {
		return
	}

	// NOTE: for now we can only have 1 or 2 branches

	try {
		if (!isReviewable.value) {
			await createDocumentBranch.mutateAsync({
				docId: editorStore.activeDocumentId,
				req: {
					branch: "draft",
					sourceBranchId: editorStore.mappedDefaultBranchId,
				},
			})
			await updateDocumentBranch.mutateAsync({
				id: editorStore.activeDocumentId,
				branchId: editorStore.mappedDefaultBranchId,
				protectedMode: true,
			})
		} else {
			// read once here: the narrowing from the guard above does not reach
			// inside the map callback
			const docId = editorStore.activeDocumentId

			await Promise.all(
				fetchBranches.state.value.data
					?.filter((b) => !b.default)
					.map((b) =>
						deleteDocumentBranch.mutateAsync({
							docId,
							branchId: b.branchId,
						}),
					) ?? [],
			)

			await updateDocumentBranch.mutateAsync({
				id: editorStore.activeDocumentId,
				branchId: editorStore.mappedDefaultBranchId,
				protectedMode: false,
			})
		}
	} catch {
		showToastMessage("error", t("editor.errors.reviewability-update-failed"))
	}
}

function activateBranch(branch: "default" | "draft") {
	if (!editorStore.activeDocumentId) {
		return
	}

	const branches = fetchBranches.state.value.data
	const defaultBranch = branches?.find((b) => b.default)
	const draftBranch = branches?.find((b) => !b.default)

	if (branch === "default" && defaultBranch) {
		editorStore.updateActiveBranchId(defaultBranch.branchId)
		editorStore.updateTargetBranchId(null)
	} else if (branch === "draft" && draftBranch && defaultBranch) {
		editorStore.updateActiveBranchId(draftBranch.branchId)
		editorStore.updateTargetBranchId(defaultBranch.branchId)
	}
}
</script>
<template>
	<header
		ref="header"
		class="sticky top-0 z-navbar h-10 w-full border-b border-border bg-background pr-2 pl-2"
	>
		<Transition v-bind="defaultTransitionProps">
			<div
				v-if="props.allInitialSectionsLoaded"
				class="flex h-full w-full items-center justify-between gap-4"
			>
				<div class="flex min-w-0 items-center gap-3">
					<ShortcutTooltip
						side="bottom"
						:shortcut="SHORTCUT_ACTIONS.toggleSidebar"
					>
						<ShadcnUiSidebarTrigger />
					</ShortcutTooltip>
					<HeaderBreadcrumbs
						v-if="editorStore.activeDocumentId && props.breadcrumbs"
						:breadcrumbs="props.breadcrumbs"
					/>
				</div>
				<div
					v-if="editorStore.activeDocumentId && editorStore.activeBranchId"
					class="flex items-center gap-2"
				>
					<ShadcnUiTooltip
						v-if="props.timestamps?.[editorStore.activeBranchId]"
					>
						<ShadcnUiTooltipTrigger as-child>
							<div
								class="hidden text-sm whitespace-nowrap text-muted-foreground lg:block"
							>
								{{
									$t("editor.navbar.timestamp.short-edited", {
										time: $d(
											props.timestamps[editorStore.activeBranchId]!.updated.at,
											"month-day-short",
										),
									})
								}}
							</div>
						</ShadcnUiTooltipTrigger>
						<ShadcnUiTooltipContent side="bottom" align="end">
							<div class="flex flex-col gap-1">
								<i18n-t keypath="editor.navbar.timestamp.full-edited" tag="div">
									<template #user>
										<span class="font-bold">
											{{
												props.timestamps[editorStore.activeBranchId]!.updated
													.user?.name || $t("editor.navbar.unknown-user")
											}}
										</span>
									</template>
									<template #time>
										{{
											$d(
												props.timestamps[editorStore.activeBranchId]!.updated
													.at,
												"month-day-short",
											)
										}}
									</template>
								</i18n-t>
								<i18n-t
									keypath="editor.navbar.timestamp.full-created"
									tag="div"
								>
									<template #user>
										<span class="font-bold">
											{{
												props.timestamps[editorStore.activeBranchId]!.created
													.user?.name || $t("editor.navbar.unknown-user")
											}}
										</span>
									</template>
									<template #time>
										{{
											$d(
												props.timestamps[editorStore.activeBranchId]!.created
													.at,
												"month-day-short",
											)
										}}
									</template>
								</i18n-t>
							</div>
						</ShadcnUiTooltipContent>
					</ShadcnUiTooltip>
					<div class="flex items-center gap-px">
						<ShadcnUiDropdownMenu v-if="isReviewable">
							<ShadcnUiDropdownMenuTrigger as-child>
								<ShadcnUiButton
									variant="ghost"
									size="sm"
									:class="cn('h-fit gap-0.75 px-2 py-1')"
								>
									{{
										$t(`editor.navbar.document-modes.${activeBranchMode}.title`)
									}}
									<Icon name="lucide:chevron-down" class="mt-0.5" />
								</ShadcnUiButton>
							</ShadcnUiDropdownMenuTrigger>
							<ShadcnUiDropdownMenuContent
								side="bottom"
								align="end"
								loop
								class="max-w-75"
							>
								<ShadcnUiDropdownMenuItem @click="activateBranch('default')">
									<div class="flex flex-col gap-0.5">
										<div class="text-2sm font-medium">
											{{ $t(`editor.navbar.document-modes.main.title`) }}
										</div>
										<div class="text-xs text-muted-foreground">
											{{ $t(`editor.navbar.document-modes.main.description`) }}
										</div>
									</div>
								</ShadcnUiDropdownMenuItem>
								<div class="mx-1.75 my-0.5 h-px bg-border last:hidden" />
								<ShadcnUiDropdownMenuItem @click="activateBranch('draft')">
									<div class="flex flex-col gap-0.5">
										<div class="text-2sm font-medium">
											{{ $t(`editor.navbar.document-modes.draft.title`) }}
										</div>
										<div class="text-xs text-muted-foreground">
											{{ $t(`editor.navbar.document-modes.draft.description`) }}
										</div>
									</div>
								</ShadcnUiDropdownMenuItem>
							</ShadcnUiDropdownMenuContent>
						</ShadcnUiDropdownMenu>
						<ShadcnUiTooltip v-else :delay-duration="600">
							<ShadcnUiTooltipTrigger as-child>
								<ShadcnUiButton
									variant="ghost"
									size="icon"
									:class="
										cn(
											'h-7 w-7 cursor-pointer hover:bg-accent/50 active:bg-accent',
										)
									"
									@click="toggleIsEditable"
								>
									<Icon
										:name="
											!isEditable ? 'lucide:book-open' : 'lucide:pencil-line'
										"
									/>
									<span class="sr-only">
										{{
											!isEditable
												? $t("editor.navbar.toggle-read-mode")
												: $t("editor.navbar.toggle-edit-mode")
										}}
									</span>
								</ShadcnUiButton>
							</ShadcnUiTooltipTrigger>
							<ShadcnUiTooltipContent side="bottom" align="end">
								<div class="flex max-w-48 flex-col gap-2">
									<div class="break-normal whitespace-normal">
										{{
											!isEditable
												? $t("editor.navbar.mode-tooltip.read-line-1")
												: $t("editor.navbar.mode-tooltip.edit-line-1")
										}}
									</div>
									<div class="break-normal whitespace-normal">
										{{
											!isEditable
												? $t("editor.navbar.mode-tooltip.read-line-2")
												: $t("editor.navbar.mode-tooltip.edit-line-2")
										}}
									</div>
								</div>
							</ShadcnUiTooltipContent>
						</ShadcnUiTooltip>
						<ShadcnUiDropdownMenu>
							<ShadcnUiDropdownMenuTrigger as-child>
								<ShadcnUiButton
									variant="ghost"
									size="icon"
									:class="
										cn(
											'h-7 w-7 cursor-pointer hover:bg-accent/50 active:bg-accent',
										)
									"
								>
									<Icon name="lucide:ellipsis" />
									<span class="sr-only">
										{{ $t("editor.navbar.open-document-options") }}
									</span>
								</ShadcnUiButton>
							</ShadcnUiDropdownMenuTrigger>
							<ShadcnUiDropdownMenuContent side="bottom" align="end" loop>
								<ShadcnUiDropdownMenuItem @click="toggleReviewability">
									<Icon
										:name="
											isReviewable
												? 'lucide:message-square-off'
												: 'lucide:message-square-diff'
										"
									/>
									<span>
										{{
											isReviewable
												? $t(
														"editor.navbar.document-options.review-workflow.disable-title",
													)
												: $t(
														"editor.navbar.document-options.review-workflow.enable-title",
													)
										}}
									</span>
								</ShadcnUiDropdownMenuItem>
								<ShadcnUiDropdownMenuItem @click="emit('duplicate-document')">
									<Icon name="mingcute:copy-2-line" />
									<span>
										{{ $t("editor.navbar.document-options.duplicate.title") }}
									</span>
								</ShadcnUiDropdownMenuItem>
								<ShadcnUiDropdownMenuItem @click="emit('delete-document')">
									<Icon name="lucide:trash-2" />
									<span>
										{{ $t("editor.navbar.document-options.delete.title") }}
									</span>
								</ShadcnUiDropdownMenuItem>
							</ShadcnUiDropdownMenuContent>
						</ShadcnUiDropdownMenu>
						<div class="mx-2 h-5.5 w-px bg-border" />
						<ShadcnUiTooltip v-if="aiAssistantEnabled" :delay-duration="600">
							<ShadcnUiTooltipTrigger as-child>
								<ShadcnUiButton
									variant="ghost"
									size="icon"
									:class="
										cn(
											'h-7 w-7 cursor-pointer hover:bg-accent/50 active:bg-accent',
											editorStore.aiAssistantOpen && 'bg-accent/50',
										)
									"
									@click="editorStore.toggleAiAssistantOpen"
								>
									<Icon name="hugeicons:rubber-duck" class="size-4" />
									<span class="sr-only">
										{{
											editorStore.aiAssistantOpen
												? $t("editor.navbar.close-ai-assistant")
												: $t("editor.navbar.open-ai-assistant")
										}}
									</span>
								</ShadcnUiButton>
							</ShadcnUiTooltipTrigger>
							<ShadcnUiTooltipContent side="bottom" align="end">
								{{
									editorStore.aiAssistantOpen
										? $t("editor.navbar.close-ai-assistant")
										: $t("editor.navbar.open-ai-assistant")
								}}
							</ShadcnUiTooltipContent>
						</ShadcnUiTooltip>
					</div>
				</div>
			</div>
		</Transition>
	</header>
</template>

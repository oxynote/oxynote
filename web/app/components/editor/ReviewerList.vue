<script setup lang="ts">
import IconStack from "./IconStack.vue"
import { showToastMessage } from "../toast"

interface ReviewerEntry {
	id: string
	name: string
	email: string
	url?: string
	approved: boolean
	previouslyApproved?: boolean
}

interface ReviewerInviteEntry {
	id: string
	name: string
	email?: string | null
	url?: string | null
}

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { useFetchBranchReviewers, inviteBranchReviewer, removeBranchReviewer } =
	useDocumentAPI()
const { fetchOrganization, fetchAuthSession } = useAuthSession()
const fetchActiveBranchReviewers = useFetchBranchReviewers(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)
const fetchTargetBranchReviewers = useFetchBranchReviewers(
	() => editorStore.activeDocumentId,
	() => editorStore.targetBranchId,
)

const wsState = useWebSocketStateStore()
let unsubWsReviewersChange: (() => void) | null | undefined = null

// reviewers that have approved the branch or are invited to review (but
// haven't approved yet)
const participatingReviewers = computed<ReviewerEntry[]>(() => {
	const reviewerData = fetchActiveBranchReviewers.state.value.data ?? []

	return reviewerData.flatMap((r) => {
		const member = fetchOrganization.state.value.data?.data?.members.find(
			(m) => m.userId === r.userId,
		)
		if (!member) {
			return []
		}

		return [
			{
				id: member.userId,
				name: member.user.name,
				email: member.user.email,
				url: member.user.image,
				approved: r.currentlyApproved,
			},
		]
	})
})
const reviewerSuggestions = computed<Omit<ReviewerEntry, "approved">[]>(() => {
	const existingReviewerIds = new Set(
		participatingReviewers.value.map((reviewer) => reviewer.id),
	)

	return (
		fetchOrganization.state.value.data?.data?.members
			.filter((m) =>
				fetchTargetBranchReviewers.state.value.data?.some(
					(r) =>
						!existingReviewerIds.has(m.userId) &&
						fetchAuthSession.data.value?.data?.user.id !== m.userId &&
						r.userId === m.userId,
				),
			)
			.map((m) => ({
				id: m.userId,
				name: m.user.name,
				email: m.user.email,
				url: m.user.image,
			})) ?? []
	)
})

// other users that aren't participating or suggested for review, but can be
// invited
const invitableReviewers = computed<ReviewerInviteEntry[]>(() => {
	const existingReviewerIds = new Set(
		participatingReviewers.value.map((reviewer) => reviewer.id),
	)
	const suggestionIds = new Set(
		reviewerSuggestions.value.map((reviewer) => reviewer.id),
	)

	return (
		fetchOrganization.state.value.data?.data?.members
			.filter(
				(m) =>
					!existingReviewerIds.has(m.userId) &&
					!suggestionIds.has(m.userId) &&
					fetchAuthSession.data.value?.data?.user.id !== m.userId,
			)
			.map((m) => ({
				id: m.userId,
				name: m.user.name,
				email: m.user.email,
				url: m.user.image,
			})) ?? []
	)
})

const reviewerGroups = computed(() => {
	const groups: {
		approved: ReviewerEntry[]
		invited: ReviewerEntry[]
	} = {
		approved: [],
		invited: [],
	}

	for (const reviewer of participatingReviewers.value) {
		if (reviewer.approved) {
			groups.approved.push(reviewer)
			continue
		}

		groups.invited.push(reviewer)
	}

	return groups
})

watchImmediate(
	() => editorStore.activeDocumentId,
	(newV) => {
		if (!newV) {
			return
		}

		if (unsubWsReviewersChange) {
			unsubWsReviewersChange()
			unsubWsReviewersChange = null
		}

		unsubWsReviewersChange = wsState.state?.subscribe(
			makeWsDocumentReviewersChangeTopic(newV),
			() => {
				void fetchTargetBranchReviewers.refetch()
				void fetchActiveBranchReviewers.refetch()
			},
		)
	},
)

async function requestReview(member: ReviewerInviteEntry) {
	if (!editorStore.activeDocumentId || !editorStore.activeBranchId) {
		return
	}

	try {
		await inviteBranchReviewer.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			userId: member.id,
		})
		showToastMessage(
			"success",
			t("editor.name-editor.reviewer-popover.request-review-success.title"),
			t(
				"editor.name-editor.reviewer-popover.request-review-success.description",
			),
		)
	} catch {
		showToastMessage(
			"error",
			t("editor.name-editor.reviewer-popover.request-review-error"),
		)
	}
}

async function handleRemoveInvite(reviewer: ReviewerEntry) {
	if (!editorStore.activeDocumentId || !editorStore.activeBranchId) {
		return
	}

	try {
		await removeBranchReviewer.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			userId: reviewer.id,
		})
		showToastMessage(
			"success",
			t("editor.name-editor.reviewer-popover.invite-remove-success"),
		)
	} catch {
		showToastMessage(
			"error",
			t("editor.name-editor.reviewer-popover.invite-remove-error"),
		)
	}
}
</script>

<template>
	<ShadcnUiPopover
		v-if="
			editorStore.branchReviewableActionsActive ||
			participatingReviewers.length > 0
		"
	>
		<ShadcnUiPopoverTrigger as-child>
			<div class="flex">
				<IconStack
					:title="$t('editor.name-editor.reviewers')"
					clickable
					:icons="participatingReviewers"
				/>
			</div>
		</ShadcnUiPopoverTrigger>
		<ShadcnUiPopoverContent
			side="bottom"
			align="end"
			class="max-w-50 min-w-45 p-2"
		>
			<div class="flex flex-col gap-2">
				<div v-if="reviewerGroups.approved.length" class="flex flex-col gap-2">
					<div class="text-2sm font-medium text-muted-foreground">
						{{ $t("editor.name-editor.reviewer-popover.approved-by-label") }}
					</div>
					<ul class="flex flex-col gap-1.5">
						<li
							v-for="reviewer in reviewerGroups.approved"
							:key="reviewer.id"
							class="flex items-center gap-2"
						>
							<div class="flex min-w-0 flex-1 items-center gap-1.5">
								<ShadcnUiAvatar class="size-6 border">
									<ShadcnUiAvatarImage
										v-if="reviewer.url"
										:src="reviewer.url"
										:alt="$t('settings.profile.image-alt')"
									/>
									<ShadcnUiAvatarFallback class="rounded-md text-2xs">
										{{ extractInitials(reviewer.name || "", 2) }}
									</ShadcnUiAvatarFallback>
								</ShadcnUiAvatar>
								<span class="truncate text-2sm break-all text-foreground">
									{{ reviewer.name }}
								</span>
							</div>
							<ShadcnUiButton
								v-if="editorStore.branchReviewableActionsActive"
								size="icon-xsm"
								variant="ghost-plain"
								class="text-muted-foreground"
								@click.stop="requestReview(reviewer)"
							>
								<Icon name="lucide:refresh-ccw" />
								<span class="sr-only">
									{{
										$t(
											"editor.name-editor.reviewer-popover.request-review-screen-reader-hint",
										)
									}}
								</span>
							</ShadcnUiButton>
						</li>
					</ul>
				</div>
				<div v-if="reviewerGroups.invited.length" class="flex flex-col gap-2">
					<div class="text-2sm font-medium text-muted-foreground">
						{{ $t("editor.name-editor.reviewer-popover.invited-label") }}
					</div>
					<ul class="flex flex-col gap-1.5">
						<li
							v-for="reviewer in reviewerGroups.invited"
							:key="reviewer.id"
							class="flex items-center gap-2"
						>
							<div class="flex min-w-0 flex-1 items-center gap-1.5">
								<ShadcnUiAvatar class="size-6 border">
									<ShadcnUiAvatarImage
										v-if="reviewer.url"
										:src="reviewer.url"
										:alt="$t('settings.profile.image-alt')"
									/>
									<ShadcnUiAvatarFallback class="rounded-md text-2xs">
										{{ extractInitials(reviewer.name || "", 2) }}
									</ShadcnUiAvatarFallback>
								</ShadcnUiAvatar>
								<span class="truncate text-2sm text-foreground">
									{{ reviewer.name }}
								</span>
							</div>
							<ShadcnUiButton
								v-if="editorStore.branchReviewableActionsActive"
								size="icon-xsm"
								variant="ghost-plain"
								class="text-muted-foreground"
								@click.stop="handleRemoveInvite(reviewer)"
							>
								<Icon name="lucide:trash-2" />
								<span class="sr-only">
									{{
										$t(
											"editor.name-editor.reviewer-popover.invite-remove-screen-reader-hint",
										)
									}}
								</span>
							</ShadcnUiButton>
						</li>
					</ul>
				</div>
				<template v-if="editorStore.branchReviewableActionsActive">
					<ShadcnUiDropdownMenuSeparator
						v-if="participatingReviewers.length"
						class="-mx-2 my-0"
					/>
					<ShadcnUiDropdownMenu>
						<ShadcnUiDropdownMenuTrigger as-child>
							<ShadcnUiButton
								size="2sm"
								variant="outline"
								:disabled="
									!invitableReviewers.length && !reviewerSuggestions.length
								"
							>
								<Icon name="lucide:mail-plus" />
								{{ $t("editor.name-editor.reviewer-popover.invite-button") }}
							</ShadcnUiButton>
						</ShadcnUiDropdownMenuTrigger>
						<ShadcnUiDropdownMenuContent
							align="end"
							side="bottom"
							loop
							class="max-h-[40dvh] max-w-50 min-w-45"
						>
							<template v-if="reviewerSuggestions.length">
								<div
									class="mb-0.5 pl-0.5 text-xs font-medium text-muted-foreground"
								>
									{{
										$t("editor.name-editor.reviewer-popover.suggestions-label")
									}}
								</div>
								<div class="flex flex-col">
									<ShadcnUiDropdownMenuItem
										v-for="reviewer in reviewerSuggestions"
										:key="reviewer.id"
										class="h-auto py-1.5"
										@click="requestReview(reviewer)"
									>
										<div class="flex min-w-0 items-center gap-1.5">
											<ShadcnUiAvatar class="size-6 border">
												<ShadcnUiAvatarImage
													v-if="reviewer.url"
													:src="reviewer.url"
													:alt="$t('settings.profile.image-alt')"
												/>
												<ShadcnUiAvatarFallback class="rounded-md text-2xs">
													{{ extractInitials(reviewer.name || "", 2) }}
												</ShadcnUiAvatarFallback>
											</ShadcnUiAvatar>
											<div class="flex min-w-0 flex-col">
												<span
													class="truncate text-2sm break-all text-foreground"
												>
													{{ reviewer.name }}
												</span>
												<span
													class="truncate text-2xs break-all text-muted-foreground"
												>
													{{ reviewer.email }}
												</span>
											</div>
										</div>
									</ShadcnUiDropdownMenuItem>
								</div>
								<ShadcnUiDropdownMenuSeparator
									v-if="invitableReviewers.length"
								/>
							</template>
							<ShadcnUiDropdownMenuItem
								v-for="member in invitableReviewers"
								:key="member.id"
								class="h-auto py-1.5"
								@click="requestReview(member)"
							>
								<div class="flex min-w-0 items-center gap-1.5">
									<ShadcnUiAvatar class="size-6 border">
										<ShadcnUiAvatarImage
											v-if="member.url"
											:src="member.url"
											:alt="$t('settings.profile.image-alt')"
										/>
										<ShadcnUiAvatarFallback class="rounded-md text-2xs">
											{{ extractInitials(member.name || "", 2) }}
										</ShadcnUiAvatarFallback>
									</ShadcnUiAvatar>
									<div class="flex min-w-0 flex-col">
										<span class="truncate text-2sm break-all text-foreground">
											{{ member.name }}
										</span>
										<span
											class="truncate text-2xs break-all text-muted-foreground"
										>
											{{ member.email }}
										</span>
									</div>
								</div>
							</ShadcnUiDropdownMenuItem>
						</ShadcnUiDropdownMenuContent>
					</ShadcnUiDropdownMenu>
				</template>
			</div>
		</ShadcnUiPopoverContent>
	</ShadcnUiPopover>
</template>

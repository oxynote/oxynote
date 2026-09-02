<script lang="ts" setup>
import type { Notification } from "~/utils/api/notification"
import { showToastMessage } from "./toast"
import { formatDistanceToNowStrict } from "date-fns"
import { cn } from "~/lib/utils"

const props = defineProps<{
	mobile?: boolean
}>()
const emit = defineEmits<{
	(event: "close-notification-box" | "notification-navigation"): void
}>()
const { t, locale } = useI18n({ useScope: "global" })
const NOW_TIME_LABEL_THRESHOLD_MS = 1000 * 60 // 1 minute
const {
	useFetchManyNotifications,
	useFetchNotificationCount,
	markNotificationsRead,
} = useNotificationAPI()
const { fetchDocumentTree } = useDocumentAPI()
const wsState = useWebSocketStateStore()
let unsubWsNotifications: (() => void) | null | undefined = null

const { fetchOrganization } = useAuthSession()
const fetchNotificationCount = useFetchNotificationCount({ read: false })
const fetchNotifications = useFetchManyNotifications({ limit: 100, page: 1 }) // static for now

const notifications = computed(
	() => fetchNotifications.data.value?.notifications ?? [],
)

onMounted(() => {
	unsubWsNotifications = wsState.state?.subscribe(
		WS_NOTIFICATION_CREATION_TOPIC,
		() => {
			void fetchNotificationCount.refetch()
			void fetchNotifications.refetch()
		},
	)
})
onUnmounted(() => {
	unsubWsNotifications?.()
})

function findDocumentName(documentId: string) {
	return docNameByIdInDocumentTree(
		fetchDocumentTree.data.value ?? [],
		documentId,
	)
}

function findUserName(userId: string) {
	return (
		fetchOrganization.state.value.data?.data?.members.find(
			(m) => m.userId === userId,
		)?.user.name || "deleted"
	)
}

async function buildNotificationHref(notification: Notification) {
	const doc = docNameByIdInDocumentTree(
		fetchDocumentTree.data.value ?? [],
		notification.metadata.documentId,
	)
	if (!doc) {
		return null
	}

	const orgRealName = createNameSlug(
		(await fetchOrganization.refresh()).data?.data?.slug || "",
	)

	switch (notification.code) {
		case NotificationCode.DocumentReviewRequest: {
			const metadata = notification.metadata

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)

			return `/${orgRealName}/${docSlug}`
		}
		case NotificationCode.DocumentHookTrigerred: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentHookTriggered

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)
			const baseHref = `/${orgRealName}/${docSlug}`

			return metadata.blockId ? `${baseHref}#${metadata.blockId}` : baseHref
		}
		case NotificationCode.DocumentNewComment: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentNewComment

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)
			const baseHref = `/${orgRealName}/${docSlug}`

			return metadata.anchorBlockId
				? `${baseHref}#${metadata.anchorBlockId}`
				: baseHref
		}
		case NotificationCode.DocumentNewCommentReply: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentNewCommentReply

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)
			const baseHref = `/${orgRealName}/${docSlug}`

			return metadata.anchorBlockId
				? `${baseHref}#${metadata.anchorBlockId}`
				: baseHref
		}
		default:
			return null
	}
}

function findNotificationDocumentName(notification: Notification) {
	return (
		findDocumentName(notification.metadata.documentId) ||
		t("notification.document-fallback")
	)
}

function findNotificationIcon(notification: Notification) {
	switch (notification.code) {
		case NotificationCode.DocumentReviewRequest:
			return "lucide:file-user"
		case NotificationCode.DocumentHookTrigerred:
			switch (
				(notification.metadata as NotificationMetadataDocumentHookTriggered)
					.type
			) {
				case DocumentHookType.URLWatcher:
					return "mingcute:earth-2-line"
				case DocumentHookType.GitHubTracking:
					return "simple-icons:github"
				case DocumentHookType.ScheduledReminder:
					return "lucide:timer"
				case DocumentHookType.ContainerImageWatcher:
					return "lucide:container"
				default:
					// a newer server may send a hook type this build does
					// not know
					return "lucide:file-exclamation-point"
			}
		case NotificationCode.DocumentNewComment:
			return "mingcute:message-4-fill"
		case NotificationCode.DocumentNewCommentReply:
			return "mingcute:message-4-fill"
		default:
			return "lucide:file-exclamation-point"
	}
}

function buildNotificationDescription(notification: Notification) {
	switch (notification.code) {
		case NotificationCode.DocumentReviewRequest: {
			return t("notification.messages.document-review-request-description")
		}
		case NotificationCode.DocumentHookTrigerred: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentHookTriggered
			let hook = t("notification.hook-fallback")

			switch (metadata.type) {
				case DocumentHookType.URLWatcher:
					hook = t("editor.hooks.url-watcher.title")
					break
				case DocumentHookType.GitHubTracking:
					hook = t("editor.hooks.github-tracking.title")
					break
				case DocumentHookType.ScheduledReminder:
					hook = t("editor.hooks.time-expiration.title")
					break
				case DocumentHookType.ContainerImageWatcher:
					hook = t("editor.hooks.container-image-watcher.title")
					break
			}

			return t("notification.messages.document-hook-triggered-description", {
				hook: hook,
			})
		}
		case NotificationCode.DocumentNewComment: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentNewComment

			return t("notification.messages.document-new-comment-description", {
				user: findUserName(metadata.userId),
			})
		}
		case NotificationCode.DocumentNewCommentReply: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentNewCommentReply

			return t("notification.messages.document-new-comment-reply-description", {
				user: findUserName(metadata.userId),
			})
		}
		default:
			return t("notification.messages.default-description")
	}
}

async function handleNotificationClick(notification: Notification) {
	const href = await buildNotificationHref(notification)
	if (href) {
		emit("notification-navigation")
		void handleMarkRead(notification, true) // non blocking
		await navigateTo(href)
	}
}

async function handleMarkRead(
	notification: Notification,
	noErrorToast = false,
) {
	if (notification.read) {
		return
	}

	try {
		await markNotificationsRead.mutateAsync({ ids: [notification.id] })
	} catch {
		if (!noErrorToast) {
			showToastMessage("error", t("notification.errors.mark-read-failed"))
		}

		return
	}
}

async function handleMarkAllRead() {
	const unreadNotificationIds = notifications.value.filter((n) => !n.read)
	if (unreadNotificationIds.length === 0) {
		return
	}

	try {
		// send an empty array to mark all as read
		await markNotificationsRead.mutateAsync({ ids: [] })
	} catch {
		showToastMessage("error", t("notification.errors.mark-all-read-failed"))
		return
	}
}
</script>
<template>
	<aside class="flex max-h-svh shrink-0 flex-col bg-background">
		<header
			ref="header"
			class="top-0 z-navbar h-10 shrink-0 border-b border-border bg-background pr-3 pl-4"
		>
			<div class="flex h-full w-full items-center justify-between gap-4">
				<div class="flex text-sm">
					{{ t("notification.inbox") }}
				</div>
				<div class="flex items-center gap-2.5">
					<ShadcnUiButton
						variant="ghost"
						size="2sm"
						:class="cn('px-2 py-1')"
						@click="handleMarkAllRead"
					>
						{{ t("notification.read-all-button") }}
					</ShadcnUiButton>
					<template v-if="props.mobile">
						<div class="w-px self-stretch bg-border" />
						<ShadcnUiButton
							size="icon-sm"
							variant="ghost"
							class="shrink-0"
							@click.stop="emit('close-notification-box')"
						>
							<Icon name="lucide:x" />
							<span class="sr-only">
								{{ t("notification.actions.close-notification-box") }}
							</span>
						</ShadcnUiButton>
					</template>
				</div>
			</div>
		</header>
		<div class="flex flex-1 overflow-y-auto p-1.25">
			<div
				v-if="!notifications.length"
				class="mx-auto flex flex-col items-center gap-2 px-5 py-10 text-center"
			>
				<div class="text-base text-muted-foreground">
					{{ t("notification.empty-title") }}
				</div>
				<div class="text-sm text-muted-foreground">
					{{ t("notification.empty-description") }}
				</div>
			</div>
			<div v-else class="flex min-w-0 flex-1 flex-col gap-1">
				<div
					v-for="notification in notifications"
					:key="notification.id"
					class="flex min-w-0 cursor-pointer items-center gap-2 rounded-md p-2 hover:bg-accent/40 [&:active:not(:has(button:active))]:bg-accent/70"
					:class="notification.read && 'opacity-60'"
					@click="handleNotificationClick(notification)"
				>
					<ShadcnUiAvatar class="mt-0.25 size-8.25 border">
						<ShadcnUiAvatarFallback>
							<Icon :name="findNotificationIcon(notification)" />
						</ShadcnUiAvatarFallback>
					</ShadcnUiAvatar>
					<div
						class="flex min-w-0 flex-1 flex-col items-center justify-between gap-2"
					>
						<div class="flex w-full min-w-0 flex-row justify-between gap-0.5">
							<ShadcnUiTooltip :delay-duration="600">
								<ShadcnUiTooltipTrigger as-child>
									<div class="min-w-0 flex-2 truncate text-2sm text-foreground">
										{{ findNotificationDocumentName(notification) }}
									</div>
								</ShadcnUiTooltipTrigger>
								<ShadcnUiTooltipContent
									side="bottom"
									class="max-w-64 wrap-anywhere"
								>
									{{ findNotificationDocumentName(notification) }}
								</ShadcnUiTooltipContent>
							</ShadcnUiTooltip>
							<span class="flex-1 text-right text-2xs text-muted-foreground/70">
								{{
									new Date().getTime() -
										new Date(notification.createdAt).getTime() <
									NOW_TIME_LABEL_THRESHOLD_MS
										? $t("notification.now-time-label")
										: formatDistanceToNowStrict(
												new Date(notification.createdAt),
												{
													addSuffix: true,
													locale: convertDateFnsLocale(locale),
												},
											)
								}}
							</span>
						</div>
						<div
							class="flex w-full flex-row items-center justify-between gap-0.5"
						>
							<div class="min-w-0 text-xs text-muted-foreground/70">
								{{ buildNotificationDescription(notification) }}
							</div>
							<ShadcnUiButton
								v-if="!notification.read"
								size="icon-2xsm"
								variant="ghost"
								class="shrink-0"
								@click.stop="handleMarkRead(notification)"
							>
								<Icon name="lucide:check" />
								<span class="sr-only">
									{{ t("notification.actions.mark-read") }}
								</span>
							</ShadcnUiButton>
						</div>
					</div>
				</div>
			</div>
		</div>
	</aside>
</template>

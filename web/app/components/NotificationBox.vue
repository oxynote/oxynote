<script lang="ts" setup>
import type { Notification } from "~/utils/api/notification"
import { showToastMessage } from "./toast"
import { formatDistanceToNowStrict } from "date-fns"
import { cn } from "~/lib/utils"

const props = defineProps<{
	mobile?: boolean
}>()
const emit = defineEmits<{
	(event: "close-notification-box"): void
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

const filterMode = ref<"all" | "unread" | "read">("all")

const { fetchOrganization } = useAuthSession()
const fetchNotificationCount = useFetchNotificationCount({ read: false })
const fetchNotifications = useFetchManyNotifications({ limit: 100, page: 1 }) // static for now

const notifications = computed(() => {
	const all = fetchNotifications.data.value?.notifications ?? []
	if (filterMode.value === "unread") {
		return all.filter((n) => !n.read)
	}
	if (filterMode.value === "read") {
		return all.filter((n) => n.read)
	}
	return all
})

const hasUnreadNotifications = computed(() =>
	(fetchNotifications.data.value?.notifications ?? []).some((n) => !n.read),
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
		)?.user.name || t("general.deleted-user")
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

			return `/${orgRealName}/${docSlug}?branch=${encodeURIComponent(metadata.branchId)}`
		}
		case NotificationCode.DocumentHookTrigerred:
		case NotificationCode.DocumentHookNeedsAttention: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentHookTriggered

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)
			const baseHref = `/${orgRealName}/${docSlug}?branch=${encodeURIComponent(metadata.branchId)}`

			return metadata.blockId ? `${baseHref}#${metadata.blockId}` : baseHref
		}
		case NotificationCode.DocumentNewComment: {
			const metadata = notification.metadata as NotificationMetadataDocumentNewComment

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)
			const baseHref = `/${orgRealName}/${docSlug}?branch=${encodeURIComponent(metadata.branchId)}`

			return metadata.anchorBlockId
				? `${baseHref}#${metadata.anchorBlockId}`
				: baseHref
		}
		case NotificationCode.DocumentNewCommentReply: {
			const metadata =
				notification.metadata as NotificationMetadataDocumentNewCommentReply

			const documentName = findDocumentName(metadata.documentId) || ""
			const docSlug = createNameSlugWithId(documentName, metadata.documentId)
			const baseHref = `/${orgRealName}/${docSlug}?branch=${encodeURIComponent(metadata.branchId)}`

			return metadata.anchorBlockId
				? `${baseHref}#${metadata.anchorBlockId}`
				: baseHref
		}
		default:
			return null
	}
}

async function handleNotificationClick(notification: Notification) {
	if (!notification.read) {
		await markNotificationsRead.mutateAsync({ ids: [notification.id] })
		void fetchNotifications.refetch()
		void fetchNotificationCount.refetch()
	}

	const href = await buildNotificationHref(notification)
	if (!href) {
		showToastMessage({
			type: "error",
			title: t("notifications.toast.document-not-found.title"),
			description: t("notifications.toast.document-not-found.description"),
		})
		return
	}

	await navigateTo(href)
	emit("close-notification-box")
}

async function handleMarkAllRead() {
	await markNotificationsRead.mutateAsync({ ids: [] })
	void fetchNotifications.refetch()
	void fetchNotificationCount.refetch()
}
</script>

<template>
	<div
		:class="
			cn(
				'flex h-svh flex-col bg-background text-foreground',
				props.class,
			)
		"
	>
		<div
			class="flex items-center justify-between border-b border-border px-4 py-3"
		>
			<h2 class="text-sm font-semibold">
				{{ t("notifications.title") }}
			</h2>
			<div class="flex items-center gap-1">
				<ShadcnUiButton
					v-if="hasUnreadNotifications"
					variant="ghost"
					size="sm"
					class="h-8 text-xs text-muted-foreground hover:text-foreground"
					@click="handleMarkAllRead"
				>
					{{ t("notifications.mark-all-read") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					variant="ghost"
					size="icon"
					class="h-8 w-8 text-muted-foreground hover:text-foreground"
					@click="emit('close-notification-box')"
				>
					<Icon name="lucide:x" class="h-4 w-4" />
				</ShadcnUiButton>
			</div>
		</div>

		<!-- Filter Bar -->
		<div class="flex items-center gap-1 border-b border-border px-4 py-2 bg-muted/30">
			<ShadcnUiButton
				variant="ghost"
				size="sm"
				:class="cn('h-7 px-2 text-xs', filterMode === 'all' && 'bg-accent text-accent-foreground font-medium')"
				@click="filterMode = 'all'"
			>
				{{ t("general.all") || "All" }}
			</ShadcnUiButton>
			<ShadcnUiButton
				variant="ghost"
				size="sm"
				:class="cn('h-7 px-2 text-xs', filterMode === 'unread' && 'bg-accent text-accent-foreground font-medium')"
				@click="filterMode = 'unread'"
			>
				{{ t("notifications.unread") || "Unread" }}
			</ShadcnUiButton>
			<ShadcnUiButton
				variant="ghost"
				size="sm"
				:class="cn('h-7 px-2 text-xs', filterMode === 'read' && 'bg-accent text-accent-foreground font-medium')"
				@click="filterMode = 'read'"
			>
				{{ t("notifications.read") || "Read" }}
			</ShadcnUiButton>
		</div>

		<div class="flex-1 overflow-y-auto p-4">
			<div v-if="fetchNotifications.status.value === 'pending'" class="flex justify-center py-8">
				<ShadcnUiSpinner class="h-6 w-6 text-muted-foreground" />
			</div>
			<div v-else-if="notifications.length === 0" class="flex flex-col items-center justify-center py-12 text-center text-muted-foreground">
				<Icon name="lucide:bell-off" class="mb-2 h-8 w-8 opacity-50" />
				<p class="text-xs">{{ t("notifications.empty") }}</p>
			</div>
			<div v-else class="space-y-2">
				<div
					v-for="notification in notifications"
					:key="notification.id"
					:class="
						cn(
							'group relative flex cursor-pointer items-start gap-3 rounded-lg p-3 text-left transition-colors hover:bg-accent/50',
							!notification.read && 'bg-accent/20 font-medium',
						)
					"
					@click="handleNotificationClick(notification)"
				>
					<div class="flex-1 space-y-1">
						<p class="text-xs text-foreground">
							{{ notification.code }}
						</p>
						<span class="text-[10px] text-muted-foreground">
							{{ formatDistanceToNowStrict(new Date(notification.createdAt), { addSuffix: true, locale: locale === 'it' ? undefined : undefined }) }}
						</span>
					</div>
					<div v-if="!notification.read" class="mt-1 h-2 w-2 rounded-full bg-primary" />
				</div>
			</div>
		</div>
	</div>
</template>

<script lang="ts" setup>
import type { Notification } from "~/utils/api/notification"
import { showToastMessage } from "./toast"
import {
	NOTIFICATION_DAYS,
	notificationDay,
	type NotificationActor,
	type NotificationDay,
} from "./notification"
import { extractDocumentTreeElement } from "./sidebar"

const emit = defineEmits<{
	(event: "close-notification-box"): void
}>()
const { t } = useI18n({ useScope: "global" })
const {
	useFetchManyNotifications,
	useFetchNotificationCount,
	markNotificationsRead,
	invalidateNotifications,
} = useNotificationAPI()
const { fetchDocumentTree } = useDocumentAPI()
const wsState = useWebSocketStateStore()
const { fetchOrganization } = useAuthSession()
const fetchNotificationCount = useFetchNotificationCount({ read: false })
const readFilter = ref<"all" | "unread" | "read">("all")
const fetchNotifications = useFetchManyNotifications(() => ({
	limit: 50,
	read: readFilter.value === "all" ? undefined : readFilter.value === "read",
}))
const now = useNow({ interval: 60 * 1000 })
const list = useTemplateRef("list")
// a failed load leaves the list at its end, so without the status check
// the next page is asked for again at once, and without end
useInfiniteScroll(list, handleLoadMore, {
	distance: 100,
	canLoadMore: () =>
		fetchNotifications.hasNextPage.value &&
		fetchNotifications.status.value !== "error" &&
		!markNotificationsRead.isLoading.value,
})

let unsubWsNotifications: (() => void) | null | undefined = null
const filters = computed(() => [
	{ value: "all" as const, label: t("notification.filters.all") },
	{ value: "unread" as const, label: t("notification.filters.unread") },
	{ value: "read" as const, label: t("notification.filters.read") },
])

const notifications = computed(() => {
	const seen = new Set<string>()

	// a notification that arrives between two page loads moves the rows
	// down, so the next page can repeat one
	return (fetchNotifications.data.value?.pages ?? [])
		.flatMap((page) => page.notifications)
		.filter((notification) => {
			if (seen.has(notification.id)) {
				return false
			}

			seen.add(notification.id)

			return true
		})
})
const sections = computed(() =>
	NOTIFICATION_DAYS.map((day) => ({
		day: day,
		notifications: notifications.value.filter(
			(notification) =>
				notificationDay(notification.createdAt, now.value) === day,
		),
	})).filter((section) => section.notifications.length),
)
// unread notifications can sit on pages that are not loaded yet, so the
// list cannot answer this
const hasUnreadNotifications = computed(
	() => (fetchNotificationCount.data.value?.count ?? 0) > 0,
)

onMounted(() => {
	unsubWsNotifications = wsState.state?.subscribe(
		WS_NOTIFICATION_CREATION_TOPIC,
		() => {
			// refresh failures are already exposed by the queries
			void invalidateNotifications().catch(() => undefined)
		},
	)
})
onUnmounted(() => {
	unsubWsNotifications?.()
})

watch(readFilter, () => {
	if (list.value) {
		list.value.scrollTop = 0
	}
})

function findDocument(notification: Notification) {
	return extractDocumentTreeElement(
		fetchDocumentTree.data.value ?? [],
		notification.metadata.documentId,
	)
}

function findActor(notification: Notification): NotificationActor | null {
	if (!("userId" in notification.metadata)) {
		return null
	}

	const userId = notification.metadata.userId
	const user = fetchOrganization.state.value.data?.data?.members.find(
		(m) => m.userId === userId,
	)?.user

	return {
		id: userId,
		name: user?.name || t("general.deleted-user"),
		image: user?.image,
	}
}

function dayLabel(day: NotificationDay) {
	switch (day) {
		case "today":
			return t("notification.days.today")
		case "yesterday":
			return t("notification.days.yesterday")
		default:
			return t("notification.days.earlier")
	}
}

async function buildNotificationHref(notification: Notification) {
	const doc = findDocument(notification)
	if (!doc?.documentName) {
		return null
	}

	const { metadata } = notification
	let blockId: string | null = null

	switch (notification.code) {
		case NotificationCode.DocumentReviewRequest:
			break
		case NotificationCode.DocumentHookTriggered:
		case NotificationCode.DocumentHookNeedsAttention:
			blockId = (metadata as NotificationMetadataDocumentHookTriggered).blockId
			break
		case NotificationCode.DocumentNewComment:
		case NotificationCode.DocumentNewCommentReply:
		case NotificationCode.DocumentCommentResolved:
			blockId = (metadata as NotificationMetadataDocumentNewComment)
				.anchorBlockId
			break
		default:
			return null
	}

	const orgSlug = createNameSlug(
		(await fetchOrganization.refresh()).data?.data?.slug || "",
	)
	const docSlug = createNameSlugWithId(doc.documentName, metadata.documentId)
	const href = `/${orgSlug}/${docSlug}?branch=${encodeURIComponent(metadata.branchId)}`

	return blockId ? `${href}#${blockId}` : href
}

async function handleLoadMore() {
	await fetchNotifications.loadNextPage()
}

async function handleNotificationClick(notification: Notification) {
	const href = await buildNotificationHref(notification)
	if (href) {
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
	if (!hasUnreadNotifications.value) {
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
			class="top-0 z-navbar h-10 shrink-0 border-b border-border bg-background pr-2 pl-4"
		>
			<div class="flex h-full w-full items-center justify-between gap-4">
				<div class="flex text-sm">
					{{ t("notification.inbox") }}
				</div>
				<div class="flex items-center gap-0.5">
					<ShadcnUiButton
						v-if="hasUnreadNotifications"
						variant="ghost"
						size="icon-sm"
						class="shrink-0"
						@click="handleMarkAllRead"
					>
						<Icon name="ph:checks-bold" class="size-4" />
						<span class="sr-only">
							{{ t("notification.read-all-button") }}
						</span>
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						class="shrink-0"
						@click.stop="emit('close-notification-box')"
					>
						<Icon name="ph:caret-double-left-bold" class="size-4" />
						<span class="sr-only">
							{{ t("notification.actions.close-notification-box") }}
						</span>
					</ShadcnUiButton>
				</div>
			</div>
		</header>
		<div
			role="group"
			:aria-label="t('notification.filters.label')"
			class="flex shrink-0 gap-1 border-b border-border p-1.25"
		>
			<ShadcnUiButton
				v-for="filter in filters"
				:key="filter.value"
				variant="ghost"
				size="2sm"
				class="flex-1"
				:aria-pressed="readFilter === filter.value"
				:data-status="readFilter === filter.value ? 'active' : undefined"
				@click="readFilter = filter.value"
			>
				{{ filter.label }}
			</ShadcnUiButton>
		</div>
		<div ref="list" class="flex flex-1 overflow-y-auto p-1.25">
			<div
				v-if="!notifications.length"
				class="mx-auto flex flex-col items-center gap-2 px-5 py-10 text-center"
			>
				<div class="text-base text-muted-foreground">
					{{
						readFilter === "all"
							? t("notification.empty-title")
							: t("notification.filtered-empty-title")
					}}
				</div>
				<div class="text-sm text-muted-foreground">
					{{
						readFilter === "all"
							? t("notification.empty-description")
							: t("notification.filtered-empty-description")
					}}
				</div>
			</div>
			<div v-else class="flex min-w-0 flex-1 flex-col">
				<section
					v-for="section in sections"
					:key="section.day"
					class="flex min-w-0 flex-col gap-0.5"
				>
					<h3
						class="px-2 pt-3 pb-1 text-2xs font-semibold tracking-wide text-muted-foreground/70 uppercase"
					>
						{{ dayLabel(section.day) }}
					</h3>
					<NotificationRow
						v-for="notification in section.notifications"
						:key="notification.id"
						:notification="notification"
						:document="findDocument(notification)"
						:actor="findActor(notification)"
						:now="now"
						@open="handleNotificationClick(notification)"
						@mark-read="handleMarkRead(notification)"
					/>
				</section>
				<div
					v-if="fetchNotifications.isLoading.value"
					class="flex shrink-0 items-center justify-center py-3 text-muted-foreground"
				>
					<Icon
						name="svg-spinners:blocks-shuffle-3"
						class="size-5 opacity-50"
					/>
				</div>
			</div>
		</div>
	</aside>
</template>

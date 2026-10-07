<script lang="ts" setup>
import type { Notification } from "~/utils/api/notification"
import { cn } from "~/lib/utils"
import { hookNotificationLine, type NotificationActor } from "./notification"

const props = defineProps<{
	notification: Notification
	document: DocumentTreeElement | null
	actor: NotificationActor | null
	now: Date
}>()
const emit = defineEmits<{
	(event: "open" | "mark-read"): void
}>()
const { t, d } = useI18n({ useScope: "global" })

const HOOK_ICONS: Record<string, string> = {
	[DocumentHookType.URLWatcher]: "mingcute:earth-2-line",
	[DocumentHookType.GitHubTracking]: "simple-icons:github",
	[DocumentHookType.ScheduledReminder]: "mingcute:stopwatch-line",
	[DocumentHookType.ContainerImageWatcher]: "simple-icons:docker",
}
const FALLBACK_ICON = "mingcute:notification-line"

const isHook = computed(
	() =>
		props.notification.code === NotificationCode.DocumentHookTriggered ||
		props.notification.code === NotificationCode.DocumentHookNeedsAttention,
)
// a newer server may send a hook type this build does not know
const circleIcon = computed(() =>
	isHook.value
		? (HOOK_ICONS[
				(
					props.notification
						.metadata as NotificationMetadataDocumentHookTriggered
				).type
			] ?? FALLBACK_ICON)
		: FALLBACK_ICON,
)
const badge = computed(() => {
	switch (props.notification.code) {
		case NotificationCode.DocumentNewComment:
			return { icon: "mingcute:message-4-fill", class: "bg-muted-foreground" }
		case NotificationCode.DocumentNewCommentReply:
			return { icon: "mingcute:back-2-fill", class: "bg-muted-foreground" }
		case NotificationCode.DocumentCommentResolved:
			return { icon: "mingcute:check-fill", class: "bg-muted-foreground" }
		case NotificationCode.DocumentReviewRequest:
			return { icon: "mingcute:eye-2-fill", class: "bg-muted-foreground" }
		case NotificationCode.DocumentHookTriggered:
			return {
				icon: "mingcute:flash-fill",
				class: "bg-hook-status-triggered",
			}
		case NotificationCode.DocumentHookNeedsAttention:
			return {
				icon: "ph:exclamation-mark-bold",
				class: "bg-hook-status-needs-attention",
			}
		default:
			return null
	}
})
const title = computed(
	() => props.document?.documentName || t("notification.document-fallback"),
)
const timeLabel = computed(() =>
	relativeTimeLabel(props.notification.createdAt, props.now, t, d),
)
const hookLine = computed(() =>
	isHook.value ? hookNotificationLine(props.notification, t, d) : null,
)
const detail = computed(() =>
	hookLine.value
		? hookLine.value.detail
		: (props.notification.metadata as NotificationMetadataDocumentNewComment)
				.commentExcerpt,
)
const actorClass = computed(() =>
	cn("font-medium", !props.notification.read && "text-foreground"),
)
</script>

<template>
	<div
		role="link"
		tabindex="0"
		class="group flex min-w-0 cursor-pointer items-center gap-2.5 rounded-md p-2 hover:bg-accent/40 [&:active:not(:has(button:active))]:bg-accent/70"
		@click="emit('open')"
		@keydown.enter.self="emit('open')"
	>
		<div class="relative shrink-0">
			<ShadcnUiAvatar v-if="props.actor" class="size-9 border">
				<ShadcnUiAvatarImage
					v-if="props.actor.image"
					:src="props.actor.image"
					:alt="$t('settings.profile.image-alt')"
				/>
				<ShadcnUiAvatarFallback v-else>
					<LazyDefaultAvatar kind="user" :seed="props.actor.id" decorative />
				</ShadcnUiAvatarFallback>
			</ShadcnUiAvatar>
			<div
				v-else
				class="flex size-9 items-center justify-center rounded-full border bg-muted text-muted-foreground"
			>
				<Icon :name="circleIcon" class="size-4.5" />
			</div>
			<div
				v-if="badge"
				:class="
					cn(
						'absolute -right-0.5 -bottom-0.5 flex size-4 items-center justify-center rounded-full text-background ring-2 ring-background',
						badge.class,
					)
				"
			>
				<Icon :name="badge.icon" class="size-2.5" />
			</div>
		</div>
		<div class="flex min-w-0 flex-1 flex-col gap-0.5">
			<div class="flex h-5 min-w-0 items-center gap-2">
				<div class="flex min-w-0 flex-1 items-center gap-1.5">
					<div
						v-if="!props.notification.read"
						class="size-1.5 shrink-0 rounded-full bg-hook-status-triggered"
					/>
					<Icon
						v-if="props.document?.icon"
						:name="props.document.icon"
						class="size-3.5 shrink-0 text-muted-foreground"
					/>
					<div
						:class="
							cn(
								'truncate text-2sm',
								props.notification.read
									? 'font-medium text-muted-foreground'
									: 'font-semibold text-foreground',
							)
						"
					>
						{{ title }}
					</div>
				</div>
				<div
					:class="
						cn(
							'shrink-0 text-2xs text-muted-foreground/70',
							!props.notification.read &&
								'group-focus-within:hidden group-hover:hidden',
						)
					"
				>
					{{ timeLabel }}
				</div>
				<ShadcnUiButton
					v-if="!props.notification.read"
					size="icon-xsm"
					variant="outline"
					class="hidden shrink-0 group-focus-within:inline-flex group-hover:inline-flex"
					@click.stop="emit('mark-read')"
				>
					<Icon name="mingcute:check-line" class="size-3.5" />
					<span class="sr-only">
						{{ t("notification.actions.mark-read") }}
					</span>
				</ShadcnUiButton>
			</div>
			<div
				:class="
					cn(
						'truncate text-xs',
						props.notification.read
							? 'text-muted-foreground/70'
							: 'text-muted-foreground',
					)
				"
			>
				<span v-if="hookLine">{{ hookLine.text }}</span>
				<i18n-t
					v-else-if="
						props.notification.code === NotificationCode.DocumentNewComment
					"
					scope="global"
					keypath="notification.messages.document-new-comment-description"
					tag="span"
				>
					<template #user>
						<span :class="actorClass">{{ props.actor?.name }}</span>
					</template>
				</i18n-t>
				<i18n-t
					v-else-if="
						props.notification.code === NotificationCode.DocumentNewCommentReply
					"
					scope="global"
					keypath="notification.messages.document-new-comment-reply-description"
					tag="span"
				>
					<template #user>
						<span :class="actorClass">{{ props.actor?.name }}</span>
					</template>
				</i18n-t>
				<i18n-t
					v-else-if="
						props.notification.code === NotificationCode.DocumentCommentResolved
					"
					scope="global"
					keypath="notification.messages.document-comment-resolved-description"
					tag="span"
				>
					<template #user>
						<span :class="actorClass">{{ props.actor?.name }}</span>
					</template>
				</i18n-t>
				<i18n-t
					v-else-if="
						props.notification.code === NotificationCode.DocumentReviewRequest
					"
					scope="global"
					keypath="notification.messages.document-review-request-description"
					tag="span"
				>
					<template #user>
						<span :class="actorClass">{{ props.actor?.name }}</span>
					</template>
				</i18n-t>
				<span v-else>
					{{ t("notification.messages.default-description") }}
				</span>
				<template v-if="detail">
					<span class="mx-1 text-muted-foreground/40">
						{{ t("notification.messages.separator") }}
					</span>
					<span>{{ detail }}</span>
				</template>
			</div>
		</div>
	</div>
</template>

import {
	differenceInCalendarDays,
	differenceInHours,
	differenceInMinutes,
} from "date-fns"
import type { Notification } from "~/utils/api/notification"

type Translate = (key: string, params?: Record<string, unknown>) => string
type FormatDate = (value: Date, format: string) => string

export const NOTIFICATION_DAYS = ["today", "yesterday", "earlier"] as const

export type NotificationDay = (typeof NOTIFICATION_DAYS)[number]

export interface NotificationActor {
	id: string
	name: string
	image?: string | null
}

export interface NotificationLine {
	text: string
	detail?: string
}

// goes by the calendar day, not by the hours passed
export function notificationDay(
	createdAt: Date | string,
	now: Date,
): NotificationDay {
	const days = differenceInCalendarDays(now, new Date(createdAt))
	if (days <= 0) {
		return "today"
	}

	return days === 1 ? "yesterday" : "earlier"
}

export function notificationTimeLabel(
	createdAt: Date | string,
	now: Date,
	t: Translate,
	d: FormatDate,
): string {
	const date = new Date(createdAt)
	if (notificationDay(date, now) === "earlier") {
		return d(date, "month-day-short")
	}

	const minutes = differenceInMinutes(now, date)
	if (minutes < 1) {
		return t("notification.now-time-label")
	}

	if (minutes < 60) {
		return t("notification.time.minutes", { count: minutes })
	}

	const hours = differenceInHours(now, date)
	if (hours < 24) {
		return t("notification.time.hours", { count: hours })
	}

	return t("notification.time.days", { count: 1 })
}

export function hookNotificationLine(
	notification: Notification,
	t: Translate,
	d: FormatDate,
): NotificationLine {
	const metadata =
		notification.metadata as NotificationMetadataDocumentHookTriggered
	const needsAttention =
		notification.code === NotificationCode.DocumentHookNeedsAttention
	const line = needsAttention
		? attentionLine(metadata, t)
		: triggeredLine(metadata, t, d)
	if (line) {
		return line
	}

	const hook = hookTitle(metadata.type, t)

	return {
		text: needsAttention
			? t("notification.messages.document-hook-needs-attention-description", {
					hook: hook,
				})
			: t("notification.messages.document-hook-triggered-description", {
					hook: hook,
				}),
	}
}

function triggeredLine(
	metadata: NotificationMetadataDocumentHookTriggered,
	t: Translate,
	d: FormatDate,
): NotificationLine | null {
	if (!metadata.hookSettings) {
		return null
	}

	switch (metadata.type) {
		case DocumentHookType.URLWatcher: {
			const settings = metadata.hookSettings as DocumentHookSettingsURLWatcher

			return {
				text: t("notification.messages.hook-triggered.url-watcher"),
				detail: displayURL(settings.url),
			}
		}
		case DocumentHookType.ContainerImageWatcher: {
			const settings =
				metadata.hookSettings as DocumentHookSettingsContainerImageWatcher

			return {
				text: t("notification.messages.hook-triggered.container-image-watcher"),
				detail: settings.image,
			}
		}
		case DocumentHookType.GitHubTracking: {
			const settings =
				metadata.hookSettings as DocumentHookSettingsGitHubTracking

			return {
				text: metadata.hookSummary?.changedPaths
					? t("notification.messages.hook-triggered.github-tracking", {
							changed: metadata.hookSummary.changedPaths,
							total: settings.paths.length,
						})
					: t("notification.messages.hook-triggered.github-tracking-fallback"),
				detail: settings.branch,
			}
		}
		case DocumentHookType.ScheduledReminder: {
			const settings =
				metadata.hookSettings as DocumentHookSettingsScheduledReminder

			return {
				text: t("editor.hooks.scheduled-reminder.subtext-triggered", {
					date: d(new Date(settings.schedule), "short-with-time"),
				}),
			}
		}
		default:
			return null
	}
}

function attentionLine(
	metadata: NotificationMetadataDocumentHookTriggered,
	t: Translate,
): NotificationLine | null {
	if (!metadata.hookSettings) {
		return null
	}

	switch (metadata.status) {
		case "unreachable_url": {
			const settings = metadata.hookSettings as DocumentHookSettingsURLWatcher

			return {
				text: t("notification.messages.hook-attention.unreachable-url", {
					url: displayURL(settings.url),
				}),
			}
		}
		// both the website and the GitHub hook can be unconfigured
		case "unconfigured":
			return {
				text:
					metadata.type === DocumentHookType.GitHubTracking
						? t("editor.hooks.github-tracking.problems.unconfigured")
						: t(
								"notification.messages.hook-attention.url-watcher-unconfigured",
							),
			}
		case "missing_installation":
			return {
				text: t("editor.hooks.github-tracking.problems.missing-installation"),
			}
		case "missing_repository": {
			const settings =
				metadata.hookSettings as DocumentHookSettingsGitHubTracking

			return {
				text: t("notification.messages.hook-attention.missing-repository"),
				detail: settings.repository,
			}
		}
		case "missing_branch": {
			const settings =
				metadata.hookSettings as DocumentHookSettingsGitHubTracking

			return {
				text: t("notification.messages.hook-attention.missing-branch"),
				detail: settings.branch,
			}
		}
		case "tree_truncated": {
			const settings =
				metadata.hookSettings as DocumentHookSettingsGitHubTracking

			return {
				text: t("notification.messages.hook-attention.tree-truncated"),
				detail: settings.repository,
			}
		}
		case "unauthorized": {
			const settings =
				metadata.hookSettings as DocumentHookSettingsContainerImageWatcher

			return {
				text: t("notification.messages.hook-attention.unauthorized"),
				detail: settings.image,
			}
		}
		case "image_not_found": {
			const settings =
				metadata.hookSettings as DocumentHookSettingsContainerImageWatcher

			return {
				text: t("notification.messages.hook-attention.image-not-found"),
				detail: settings.image,
			}
		}
		default:
			return null
	}
}

function hookTitle(type: DocumentHookType, t: Translate): string {
	switch (type) {
		case DocumentHookType.URLWatcher:
			return t("editor.hooks.url-watcher.title")
		case DocumentHookType.GitHubTracking:
			return t("editor.hooks.github-tracking.title")
		case DocumentHookType.ScheduledReminder:
			return t("editor.hooks.scheduled-reminder.title")
		case DocumentHookType.ContainerImageWatcher:
			return t("editor.hooks.container-image-watcher.title")
		default:
			return t("notification.hook-fallback")
	}
}

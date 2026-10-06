import { describe, it } from "vitest"
import {
	hookNotificationLine,
	notificationDay,
	notificationTimeLabel,
} from "./notification"
import { DocumentHookType } from "~/utils/api/document"
import {
	NotificationCode,
	type Notification,
	type NotificationMetadataDocumentHookTriggered,
} from "~/utils/api/notification"

// noon, so a notification from this morning and one from late last night
// fall on different calendar days
const NOW = new Date(2026, 2, 14, 12, 0, 0)
const GITHUB_SETTINGS = {
	repository: "platform",
	branch: "main",
	paths: ["a.md"],
}

describe("notificationDay", () => {
	it.for([
		{
			name: "puts a notification from this morning under today",
			input: new Date(2026, 2, 14, 0, 5, 0),
			expected: "today",
		},
		{
			name: "puts a notification dated ahead of the clock under today",
			input: new Date(2026, 2, 15, 9, 0, 0),
			expected: "today",
		},
		{
			name: "puts a notification from late last night under yesterday",
			input: new Date(2026, 2, 13, 23, 55, 0),
			expected: "yesterday",
		},
		{
			name: "puts a notification from two days ago under earlier",
			input: new Date(2026, 2, 12, 23, 55, 0),
			expected: "earlier",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(notificationDay(input, NOW)).toBe(expected)
	})

	it("reads a date the api sent as a string", ({ expect }) => {
		expect(
			notificationDay(new Date(2026, 2, 13, 8, 0, 0).toISOString(), NOW),
		).toBe("yesterday")
	})
})

describe("notificationTimeLabel", () => {
	it.for([
		{
			name: "says now within the first minute",
			input: new Date(2026, 2, 14, 11, 59, 30),
			expected: "notification.now-time-label",
		},
		{
			name: "counts minutes within the first hour",
			input: new Date(2026, 2, 14, 11, 55, 0),
			expected: 'notification.time.minutes:{"count":5}',
		},
		{
			name: "counts hours within the first day",
			input: new Date(2026, 2, 14, 10, 0, 0),
			expected: 'notification.time.hours:{"count":2}',
		},
		{
			name: "counts hours for last night",
			input: new Date(2026, 2, 13, 23, 0, 0),
			expected: 'notification.time.hours:{"count":13}',
		},
		{
			name: "counts one day once yesterday is a full day back",
			input: new Date(2026, 2, 13, 9, 0, 0),
			expected: 'notification.time.days:{"count":1}',
		},
		{
			name: "shows the date before yesterday",
			input: new Date(2026, 2, 10, 9, 0, 0),
			expected: "month-day-short:2026-03-10",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(notificationTimeLabel(input, NOW, fakeT, fakeD)).toBe(expected)
	})
})

describe("hookNotificationLine", () => {
	it.for([
		{
			name: "names the page a website hook watches",
			input: {
				type: DocumentHookType.URLWatcher,
				hookSettings: { url: "https://nordpost.io/status/" },
			},
			expected: {
				text: "notification.messages.hook-triggered.url-watcher",
				detail: "nordpost.io/status",
			},
		},
		{
			name: "names the image a container hook watches",
			input: {
				type: DocumentHookType.ContainerImageWatcher,
				hookSettings: { image: "ghcr.io/lastport/relay:2.14.0" },
			},
			expected: {
				text: "notification.messages.hook-triggered.container-image-watcher",
				detail: "ghcr.io/lastport/relay:2.14.0",
			},
		},
		{
			name: "counts the changed files of a GitHub hook",
			input: {
				type: DocumentHookType.GitHubTracking,
				hookSettings: {
					repository: "platform",
					branch: "main",
					paths: ["a.md", "b.md", "c.md"],
				},
				hookSummary: { changedPaths: 2 },
			},
			expected: {
				text: 'notification.messages.hook-triggered.github-tracking:{"changed":2,"total":3}',
				detail: "main",
			},
		},
		{
			name: "leaves the count out for a GitHub hook without one",
			input: {
				type: DocumentHookType.GitHubTracking,
				hookSettings: {
					repository: "platform",
					branch: "main",
					paths: ["a.md"],
				},
				hookSummary: null,
			},
			expected: {
				text: "notification.messages.hook-triggered.github-tracking-fallback",
				detail: "main",
			},
		},
		{
			name: "says when a reminder triggered",
			input: {
				type: DocumentHookType.ScheduledReminder,
				hookSettings: {
					scale: "linear",
					duration: "168h",
					schedule: new Date(2026, 2, 14, 9, 0, 0),
				},
			},
			expected: {
				text: 'editor.hooks.scheduled-reminder.subtext-triggered:{"date":"short-with-time:2026-03-14"}',
			},
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			hookNotificationLine(
				makeNotification(NotificationCode.DocumentHookTriggered, input),
				fakeT,
				fakeD,
			),
		).toEqual(expected)
	})

	it.for([
		{
			name: "names the page a website hook cannot reach",
			input: {
				type: DocumentHookType.URLWatcher,
				status: "unreachable_url",
				hookSettings: { url: "https://status.nordpost.io" },
			},
			expected: {
				text: 'notification.messages.hook-attention.unreachable-url:{"url":"status.nordpost.io"}',
			},
		},
		{
			name: "says website change detection is not set up",
			input: {
				type: DocumentHookType.URLWatcher,
				status: "unconfigured",
				hookSettings: { url: "https://status.nordpost.io" },
			},
			expected: {
				text: "notification.messages.hook-attention.url-watcher-unconfigured",
			},
		},
		{
			name: "says GitHub is not set up",
			input: {
				type: DocumentHookType.GitHubTracking,
				status: "unconfigured",
				hookSettings: GITHUB_SETTINGS,
			},
			expected: { text: "editor.hooks.github-tracking.problems.unconfigured" },
		},
		{
			name: "says GitHub is not connected",
			input: {
				type: DocumentHookType.GitHubTracking,
				status: "missing_installation",
				hookSettings: GITHUB_SETTINGS,
			},
			expected: {
				text: "editor.hooks.github-tracking.problems.missing-installation",
			},
		},
		{
			name: "names the repository that cannot be accessed",
			input: {
				type: DocumentHookType.GitHubTracking,
				status: "missing_repository",
				hookSettings: GITHUB_SETTINGS,
			},
			expected: {
				text: "notification.messages.hook-attention.missing-repository",
				detail: "platform",
			},
		},
		{
			name: "names the branch that cannot be accessed",
			input: {
				type: DocumentHookType.GitHubTracking,
				status: "missing_branch",
				hookSettings: GITHUB_SETTINGS,
			},
			expected: {
				text: "notification.messages.hook-attention.missing-branch",
				detail: "main",
			},
		},
		{
			name: "names the repository that is too large",
			input: {
				type: DocumentHookType.GitHubTracking,
				status: "tree_truncated",
				hookSettings: GITHUB_SETTINGS,
			},
			expected: {
				text: "notification.messages.hook-attention.tree-truncated",
				detail: "platform",
			},
		},
		{
			name: "names the image a registry refused",
			input: {
				type: DocumentHookType.ContainerImageWatcher,
				status: "unauthorized",
				hookSettings: { image: "relay:2.14.0" },
			},
			expected: {
				text: "notification.messages.hook-attention.unauthorized",
				detail: "relay:2.14.0",
			},
		},
		{
			name: "names the image that no longer exists",
			input: {
				type: DocumentHookType.ContainerImageWatcher,
				status: "image_not_found",
				hookSettings: { image: "relay:2.14.0" },
			},
			expected: {
				text: "notification.messages.hook-attention.image-not-found",
				detail: "relay:2.14.0",
			},
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			hookNotificationLine(
				makeNotification(NotificationCode.DocumentHookNeedsAttention, input),
				fakeT,
				fakeD,
			),
		).toEqual(expected)
	})

	it.for([
		{
			name: "names the website hook of a trigger without settings",
			input: {
				code: NotificationCode.DocumentHookTriggered,
				type: DocumentHookType.URLWatcher,
			},
			expected:
				'notification.messages.document-hook-triggered-description:{"hook":"editor.hooks.url-watcher.title"}',
		},
		{
			name: "names the GitHub hook of a trigger without settings",
			input: {
				code: NotificationCode.DocumentHookTriggered,
				type: DocumentHookType.GitHubTracking,
			},
			expected:
				'notification.messages.document-hook-triggered-description:{"hook":"editor.hooks.github-tracking.title"}',
		},
		{
			name: "names the reminder hook of a trigger without settings",
			input: {
				code: NotificationCode.DocumentHookTriggered,
				type: DocumentHookType.ScheduledReminder,
			},
			expected:
				'notification.messages.document-hook-triggered-description:{"hook":"editor.hooks.scheduled-reminder.title"}',
		},
		{
			name: "names the container hook of a failed check without settings",
			input: {
				code: NotificationCode.DocumentHookNeedsAttention,
				type: DocumentHookType.ContainerImageWatcher,
			},
			expected:
				'notification.messages.document-hook-needs-attention-description:{"hook":"editor.hooks.container-image-watcher.title"}',
		},
		{
			name: "names a status this build does not know by its hook",
			input: {
				code: NotificationCode.DocumentHookNeedsAttention,
				type: DocumentHookType.URLWatcher,
				status: "future_status",
				hookSettings: { url: "https://status.nordpost.io" },
			},
			expected:
				'notification.messages.document-hook-needs-attention-description:{"hook":"editor.hooks.url-watcher.title"}',
		},
		{
			name: "calls a hook type this build does not know unknown",
			input: {
				code: NotificationCode.DocumentHookTriggered,
				type: "future-hook",
				hookSettings: { url: "https://status.nordpost.io" },
			},
			expected:
				'notification.messages.document-hook-triggered-description:{"hook":"notification.hook-fallback"}',
		},
	])("$name", ({ input, expected }, { expect }) => {
		const { code, ...metadata } = input

		expect(
			hookNotificationLine(makeNotification(code, metadata), fakeT, fakeD),
		).toEqual({ text: expected })
	})
})

// answers with the key and its params, so a test reads which message the
// helper picked and what it filled in
function fakeT(key: string, params?: Record<string, unknown>) {
	return params ? `${key}:${JSON.stringify(params)}` : key
}

function fakeD(value: Date, format: string) {
	const month = String(value.getMonth() + 1).padStart(2, "0")
	const day = String(value.getDate()).padStart(2, "0")

	return `${format}:${String(value.getFullYear())}-${month}-${day}`
}

function makeNotification(
	code: NotificationCode,
	metadata: Record<string, unknown>,
): Notification {
	return {
		id: "notif-1",
		userId: "user-1",
		organizationId: "org-1",
		code: code,
		metadata: {
			documentId: "doc-1",
			branchId: "branch-1",
			blockId: null,
			...metadata,
		} as NotificationMetadataDocumentHookTriggered,
		read: false,
		createdAt: NOW,
	}
}

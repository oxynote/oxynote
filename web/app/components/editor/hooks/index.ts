export function prepareHookMetadata(
	id: string,
	hookType: DocumentHookType,
	t: (key: string) => string,
): { id: string; name: string; icon: string } | null {
	switch (hookType) {
		case DocumentHookType.ScheduledReminder:
			return {
				id: id,
				name: t("editor.hooks.time-expiration.title"),
				icon: "lucide:timer",
			}
		case DocumentHookType.GitHubTracking:
			return {
				id: id,
				name: t("editor.hooks.github-tracking.title"),
				icon: "simple-icons:github",
			}
		case DocumentHookType.URLWatcher:
			return {
				id: id,
				name: t("editor.hooks.url-watcher.title"),
				icon: "mingcute:earth-2-line",
			}
		case DocumentHookType.ContainerImageWatcher:
			return {
				id: id,
				name: t("editor.hooks.container-image-watcher.title"),
				icon: "lucide:container",
			}
		default:
			return null
	}
}

import { describe, it, vi } from "vitest"
import { prepareHookMetadata } from "."

const t = (key: string) => key

describe("prepareHookMetadata", () => {
	it.for([
		{
			hookType: DocumentHookType.ScheduledReminder,
			name: "editor.hooks.time-expiration.title",
			icon: "lucide:timer",
		},
		{
			hookType: DocumentHookType.GitHubTracking,
			name: "editor.hooks.github-tracking.title",
			icon: "simple-icons:github",
		},
		{
			hookType: DocumentHookType.URLWatcher,
			name: "editor.hooks.url-watcher.title",
			icon: "mingcute:earth-2-line",
		},
		{
			hookType: DocumentHookType.ContainerImageWatcher,
			name: "editor.hooks.container-image-watcher.title",
			icon: "lucide:container",
		},
	])(
		"describes a $hookType hook with its own title and icon",
		({ hookType, name, icon }, { expect }) => {
			expect(prepareHookMetadata("hook-1", hookType, t)).toEqual({
				id: "hook-1",
				name,
				icon,
			})
		},
	)

	it("translates the title through the given translator", ({ expect }) => {
		const translate = vi.fn(() => "Scheduled reminder")

		const metadata = prepareHookMetadata(
			"hook-1",
			DocumentHookType.ScheduledReminder,
			translate,
		)

		expect(metadata?.name).toBe("Scheduled reminder")
		expect(translate).toHaveBeenCalledExactlyOnceWith(
			"editor.hooks.time-expiration.title",
		)
	})

	it("returns nothing for an unknown hook type", ({ expect }) => {
		expect(
			prepareHookMetadata("hook-1", "unsupported" as DocumentHookType, t),
		).toBeNull()
	})
})

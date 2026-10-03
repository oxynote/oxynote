import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookNotice from "./HookNotice.vue"
import type { HookStatus } from "./hook-status"
import { renderedIconNames } from "~/components/test-helpers"

const ACTION = "Dismiss"

describe("<HookNotice>", () => {
	it.for<{
		name: string
		input: HookStatus
		expected: { box: string[]; icon: string }
	}>([
		{
			name: "fresh",
			input: "fresh",
			expected: {
				box: ["bg-muted", "text-muted-foreground"],
				icon: "text-muted-foreground",
			},
		},
		{
			name: "triggered",
			input: "triggered",
			expected: {
				box: ["bg-hook-status-triggered/10", "text-status-info-foreground"],
				icon: "text-hook-status-triggered",
			},
		},
		{
			name: "needs-attention",
			input: "needs-attention",
			expected: {
				box: [
					"bg-hook-status-needs-attention/15",
					"text-status-warning-foreground",
				],
				icon: "text-hook-status-needs-attention",
			},
		},
	])(
		"colours a $name notice, its text and its icon",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountSuspended(HookNotice, {
				props: { status: input, icon: "mingcute:earth-2-line" },
				slots: { default: () => "Maintainers are notified" },
			})

			expect(wrapper.text()).toBe("Maintainers are notified")
			expect(wrapper.classes()).toEqual(expect.arrayContaining(expected.box))
			expect(renderedIconNames(wrapper)).toEqual(["mingcute:earth-2-line"])
			expect(wrapper.get(".iconify").classes()).toContain(expected.icon)
			expect(wrapper.find("button").exists()).toBe(false)
		},
	)

	it("puts its action along the bottom edge, in the notice's colour", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HookNotice, {
			props: {
				status: "triggered",
				icon: "mingcute:earth-2-line",
				actionLabel: ACTION,
			},
			slots: { default: () => "The page changed" },
		})

		const [message, action] = Array.from(
			(wrapper.element as HTMLElement).children,
		)
		expect(message?.textContent).toContain("The page changed")
		expect(message?.querySelector("button")).toBeNull()
		expect(action?.tagName).toBe("BUTTON")
		expect(action?.textContent.trim()).toBe(ACTION)
		expect(Array.from(action?.classList ?? [])).toEqual(
			expect.arrayContaining([
				"w-full",
				"border-t",
				"rounded-none",
				"text-status-info-foreground",
			]),
		)
	})

	it.for<{ name: string; input: HookStatus; expected: string[] }>([
		{
			name: "fresh",
			input: "fresh",
			expected: [
				"bg-accent/40",
				"[&:not(:disabled):hover:not(:active)]:bg-accent/70",
				"[&:not(:disabled):active]:bg-accent",
			],
		},
		{
			name: "triggered",
			input: "triggered",
			expected: [
				"bg-hook-status-triggered/5",
				"[&:not(:disabled):hover:not(:active)]:bg-hook-status-triggered/15",
				"[&:not(:disabled):active]:bg-hook-status-triggered/25",
			],
		},
		{
			name: "needs-attention",
			input: "needs-attention",
			expected: [
				"bg-hook-status-needs-attention/5",
				"[&:not(:disabled):hover:not(:active)]:bg-hook-status-needs-attention/15",
				"[&:not(:disabled):active]:bg-hook-status-needs-attention/30",
			],
		},
	])(
		"deepens the action of a $name notice on hover, and again while pressed",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountSuspended(HookNotice, {
				props: {
					status: input,
					icon: "mingcute:earth-2-line",
					actionLabel: ACTION,
				},
				slots: { default: () => "The page changed" },
			})

			expect(
				wrapper
					.get("button")
					.classes()
					.filter((c) => c.includes("bg-")),
			).toEqual(expected)
		},
	)

	it("reports a click on its action", async ({ expect }) => {
		const wrapper = await mountSuspended(HookNotice, {
			props: {
				status: "triggered",
				icon: "mingcute:earth-2-line",
				actionLabel: ACTION,
			},
			slots: { default: () => "The page changed" },
		})

		await wrapper.get("button").trigger("click")

		expect(wrapper.emitted("action")).toEqual([[]])
	})
})

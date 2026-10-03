import { beforeEach, describe, it } from "vitest"
import HookMenuHeader from "./HookMenuHeader.vue"
import type { HookStatus } from "./hook-status"
import { mountHookMenu } from "./test-helpers"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"

// the teleported menu bodies are shared, so these tests cannot interleave
describe("<HookMenuHeader>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it("shows only the title while there are no hooks", async ({ expect }) => {
		await mountHookMenu(HookMenuHeader, { statuses: [] })

		expect(header().textContent.trim()).toBe(t("editor.hooks.title"))
		expect(pills()).toHaveLength(0)
	})

	it("says all is quiet in the fresh colour when no hook needs a look", async ({
		expect,
	}) => {
		await mountHookMenu(HookMenuHeader, { statuses: ["fresh", "fresh"] })

		expect(pills().map((pill) => pill.textContent.trim())).toEqual([
			t("editor.hooks.status.fresh"),
		])
		expect(pillColours()).toEqual([
			["bg-hook-status-fresh/15", "text-status-success-foreground"],
		])
	})

	it.for<{
		name: string
		input: HookStatus[]
		expected: { label: () => string; colours: string[] }
	}>([
		{
			name: "triggered hooks",
			input: ["triggered", "fresh", "triggered"],
			expected: {
				label: () => t("editor.hooks.status.triggered", { count: 2 }),
				colours: ["bg-hook-status-triggered/15", "text-status-info-foreground"],
			},
		},
		{
			name: "one hook that needs attention",
			input: ["needs-attention", "fresh"],
			expected: {
				label: () => t("editor.hooks.status.needs-attention.one", { count: 1 }),
				colours: [
					"bg-hook-status-needs-attention/25",
					"text-status-warning-foreground",
				],
			},
		},
		{
			name: "several hooks that need attention",
			input: ["needs-attention", "needs-attention"],
			expected: {
				label: () =>
					t("editor.hooks.status.needs-attention.other", { count: 2 }),
				colours: [
					"bg-hook-status-needs-attention/25",
					"text-status-warning-foreground",
				],
			},
		},
	])(
		"counts $name in words on its colour",
		async ({ input, expected }, { expect }) => {
			await mountHookMenu(HookMenuHeader, { statuses: input })

			expect(pills().map((pill) => pill.textContent.trim())).toEqual([
				expected.label(),
			])
			expect(pillColours()).toEqual([expected.colours])
		},
	)

	it("shows both counts without words when both kinds are there", async ({
		expect,
	}) => {
		await mountHookMenu(HookMenuHeader, {
			statuses: ["triggered", "needs-attention", "triggered"],
		})

		const shown = pills()
		expect(shown).toHaveLength(2)
		expect(
			shown.map(
				(pill) =>
					pill.querySelector("[aria-hidden='true']:not(.rounded-full)")
						?.textContent,
			),
		).toEqual(["1", "2"])
		expect(
			shown.map((pill) => pill.querySelector(".sr-only")?.textContent),
		).toEqual([
			t("editor.hooks.status.needs-attention.one", { count: 1 }),
			t("editor.hooks.status.triggered", { count: 2 }),
		])
		expect(
			shown.map((pill) => pill.getAttribute("data-slot") === "tooltip-trigger"),
		).toEqual([true, true])
		expect(pillColours()).toEqual([
			["bg-hook-status-needs-attention/25", "text-status-warning-foreground"],
			["bg-hook-status-triggered/15", "text-status-info-foreground"],
		])
		expect(dotColours()).toEqual([
			["bg-hook-status-needs-attention"],
			["bg-hook-status-triggered"],
		])
	})

	it("names a count in words in its tooltip", async ({ expect }) => {
		await mountHookMenu(HookMenuHeader, {
			statuses: ["triggered", "needs-attention"],
		})
		const [pill] = pills()

		pill?.dispatchEvent(new FocusEvent("focus"))
		await nextTick()

		const id = pill?.getAttribute("aria-describedby")
		expect(id && document.getElementById(id)?.textContent.trim()).toBe(
			t("editor.hooks.status.needs-attention.one", { count: 1 }),
		)
	})
})

// the header is teleported with the menu, out of the wrapper's reach
function header(): HTMLElement {
	const label = document.body.querySelector<HTMLElement>(
		"[data-slot='dropdown-menu-label']",
	)
	if (!label) {
		throw new Error("no menu header is rendered")
	}

	return label
}

// the pills sit beside the title, which is the one child that is not
// rounded
function pills(): HTMLElement[] {
	return Array.from(header().children).filter((child): child is HTMLElement =>
		child.classList.contains("rounded-full"),
	)
}

// the ground and text colour each pill takes from its status
function pillColours(): string[][] {
	return pills().map((pill) =>
		Array.from(pill.classList).filter(
			(c) => c.startsWith("bg-") || c.startsWith("text-status"),
		),
	)
}

// the colour of the dot a pill shows in place of its words
function dotColours(): string[][] {
	return pills().map((pill) =>
		Array.from(pill.querySelector(".size-1\\.5")?.classList ?? []).filter((c) =>
			c.startsWith("bg-"),
		),
	)
}

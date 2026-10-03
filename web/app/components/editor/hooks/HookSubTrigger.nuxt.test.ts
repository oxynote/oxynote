import { beforeEach, describe, it } from "vitest"
import HookSubTrigger from "./HookSubTrigger.vue"
import { iconNames, makeHook, mountHookMenu } from "./test-helpers"
import type { HookDiffContext } from "./hook-diff"
import type { HookStatus } from "./hook-status"
import { DiffStatus } from "../diff/position-map"
import { DropdownMenuSub } from "~/components/shadcn/ui/dropdown-menu"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"

const TITLE = "Website Changes"
const SUBTITLE = "oxynote.test/docs"
const DETAIL = "unreachable"

// the teleported menu bodies are shared, so these tests cannot interleave
describe("<HookSubTrigger>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it("shows the icon, title and subtitle plainly outside the diff", async ({
		expect,
	}) => {
		await mountTrigger({})

		const row = triggerRow()
		expect(row.textContent).toContain(TITLE)
		expect(row.textContent).toContain(SUBTITLE)
		expect(iconNames(row)).toEqual([
			"mingcute:earth-2-line",
			"lucide:chevron-right",
		])
		expect(signColumn(row)).toBeNull()
		expect(statusDot(row)).toBeNull()
	})

	it("joins a subtitle's detail to it after a dot", async ({ expect }) => {
		await mountTrigger({}, { detail: DETAIL })

		expect(triggerRow().textContent).toContain(
			t("editor.hooks.subtext-detail", { subtext: SUBTITLE, detail: DETAIL }),
		)
	})

	it("adds no dot to a subtitle without a detail", async ({ expect }) => {
		await mountTrigger({})

		expect(triggerRow().textContent.trim()).toBe(`${TITLE}${SUBTITLE}`)
	})

	it("leaves out the subtitle line when there is none", async ({ expect }) => {
		await mountTrigger({}, { subtitle: false })

		expect(triggerRow().textContent.trim()).toBe(TITLE)
	})

	it.for<{
		name: string
		input: HookStatus
		expected: { dot: string; label: string | null }
	}>([
		{
			name: "fresh",
			input: "fresh",
			expected: { dot: "bg-hook-status-fresh", label: null },
		},
		{
			name: "triggered",
			input: "triggered",
			expected: {
				dot: "bg-hook-status-triggered",
				label: "editor.hooks.row-status.triggered",
			},
		},
		{
			name: "needs-attention",
			input: "needs-attention",
			expected: {
				dot: "bg-hook-status-needs-attention",
				label: "editor.hooks.row-status.needs-attention",
			},
		},
	])(
		"marks a $name hook with its dot",
		async ({ input, expected }, { expect }) => {
			await mountTrigger({ status: input })

			const row = triggerRow()
			expect(statusDot(row)?.classList).toContain(expected.dot)
			expect(row.querySelector(".sr-only")?.textContent.trim() ?? null).toBe(
				expected.label ? t(expected.label) : null,
			)
		},
	)

	it.for([
		{
			status: DiffStatus.Added,
			sign: "editor.hooks.diff.added-sign",
			label: "editor.hooks.diff.added",
			tint: "bg-diff-added/30",
		},
		{
			status: DiffStatus.Removed,
			sign: "editor.hooks.diff.removed-sign",
			label: "editor.hooks.diff.removed",
			tint: "bg-diff-removed/30",
		},
	])(
		"signs and tints a hook the diff marks $status",
		async ({ status, sign, label, tint }, { expect }) => {
			await mountTrigger({ diff: diffOf(status, true) })

			const row = triggerRow()
			expect(signColumn(row)?.textContent.trim()).toBe(t(sign))
			expect(row.querySelector(".sr-only")?.textContent.trim()).toBe(t(label))
			expect(row.classList).toContain(tint)
		},
	)

	it.for([
		{ status: DiffStatus.Added, hover: "bg-diff-added/50" },
		{ status: DiffStatus.Removed, hover: "bg-diff-removed/50" },
		{ status: DiffStatus.Unchanged, hover: "bg-accent/50" },
	])(
		"deepens the shade of a $status hook on hover",
		async ({ status, hover }, { expect }) => {
			await mountTrigger({ diff: diffOf(status) })

			const classes = Array.from(triggerRow().classList)
			expect(classes.filter((c) => c.startsWith("focus:bg-"))).toEqual([
				`focus:${hover}`,
			])
			expect(
				classes.filter((c) => c.startsWith("data-[state=open]:bg-")),
			).toEqual([`data-[state=open]:${hover}`])
			// the row keeps the menu's usual inset and rounding
			expect(classes).toContain("rounded")
			expect(classes).not.toContain("rounded-none")
		},
	)

	it("strikes through the title of a removed hook", async ({ expect }) => {
		await mountTrigger({ diff: diffOf(DiffStatus.Removed) })

		const struck = triggerRow().querySelector(".line-through")
		expect(struck?.textContent).toContain(TITLE)
		expect(struck?.textContent).not.toContain(SUBTITLE)
	})

	it("counts the changes of a modified hook", async ({ expect }) => {
		await mountTrigger({
			hook: makeHook({ settings: { url: "https://new.test" } }),
			diff: {
				status: DiffStatus.Modified,
				targetHook: makeHook({ settings: { url: "https://old.test" } }),
				signColumn: false,
			},
		})

		const row = triggerRow()
		expect(row.textContent).toContain(
			t("editor.diff-change-marker.removed", { count: 1 }),
		)
		expect(row.textContent).toContain(
			t("editor.diff-change-marker.added", { count: 1 }),
		)
		expect(row.classList).not.toContain("bg-diff-added/30")
		expect(row.classList).not.toContain("bg-diff-removed/30")
	})

	it("keeps an empty sign column for an unchanged hook", async ({ expect }) => {
		await mountTrigger({ diff: diffOf(DiffStatus.Unchanged, true) })

		const row = triggerRow()
		expect(signColumn(row)?.textContent.trim()).toBe("")
		expect(row.querySelector(".sr-only")).toBeNull()
	})

	it("leaves out the sign column when the menu has no signs", async ({
		expect,
	}) => {
		await mountTrigger({ diff: diffOf(DiffStatus.Modified) })

		expect(signColumn(triggerRow())).toBeNull()
	})
})

function mountTrigger(
	props: Record<string, unknown>,
	options: { subtitle?: boolean; detail?: string } = {},
) {
	return mountHookMenu(
		defineComponent(
			() => () =>
				h(DropdownMenuSub, null, {
					default: () =>
						h(
							HookSubTrigger,
							{ icon: "mingcute:earth-2-line", ...props },
							{
								title: () => TITLE,
								...(options.subtitle !== false && {
									subtitle: () => SUBTITLE,
								}),
								...(options.detail && {
									"subtitle-detail": () => options.detail,
								}),
							},
						),
				}),
		),
		{},
	)
}

function triggerRow(): HTMLElement {
	const row = document.body.querySelector<HTMLElement>(
		"[data-slot='dropdown-menu-sub-trigger']",
	)
	if (!row) {
		throw new Error("no hook row is rendered")
	}

	return row
}

// the sign sits in a fixed width column of its own
function signColumn(row: HTMLElement): Element | null {
	return row.querySelector(".w-2")
}

function statusDot(row: HTMLElement): Element | null {
	return row.querySelector(".size-1\\.75.rounded-full")
}

function diffOf(status: DiffStatus, signColumn = false): HookDiffContext {
	return { status: status, targetHook: null, signColumn: signColumn }
}

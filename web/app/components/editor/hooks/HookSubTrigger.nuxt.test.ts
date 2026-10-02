import { beforeEach, describe, it } from "vitest"
import HookSubTrigger from "./HookSubTrigger.vue"
import { makeHook, mountHookMenu } from "./test-helpers"
import type { HookDiffContext } from "./hook-diff"
import { DiffStatus } from "../diff/position-map"
import { DropdownMenuSub } from "~/components/shadcn/ui/dropdown-menu"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"

// the teleported menu bodies are shared, so these tests cannot interleave
describe("<HookSubTrigger>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it("shows the row plainly outside the diff", async ({ expect }) => {
		await mountTrigger({})

		const row = triggerRow()
		expect(row.textContent).toContain("Watching oxynote.test")
		expect(signColumn(row)).toBeNull()
	})

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

	it("strikes through the name of a removed hook", async ({ expect }) => {
		await mountTrigger({ diff: diffOf(DiffStatus.Removed) })

		expect(triggerRow().querySelector(".line-through")?.textContent).toContain(
			"Watching oxynote.test",
		)
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

function mountTrigger(props: Record<string, unknown>) {
	return mountHookMenu(
		defineComponent(
			() => () =>
				h(DropdownMenuSub, null, {
					default: () =>
						h(HookSubTrigger, props, {
							default: () => h("span", "Watching oxynote.test"),
						}),
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

function diffOf(status: DiffStatus, signColumn = false): HookDiffContext {
	return { status: status, targetHook: null, signColumn: signColumn }
}

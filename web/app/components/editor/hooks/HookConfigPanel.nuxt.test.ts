import { beforeEach, describe, it, vi } from "vitest"
import HookConfigPanel from "./HookConfigPanel.vue"
import {
	hookNotice,
	iconNames,
	makeHook,
	menuButton,
	menuText,
	mountHookMenu,
	openHookSubMenu,
} from "./test-helpers"
import { DiffStatus } from "../diff/position-map"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"

const TITLE = "Website Changes"
const NOTICE = "Maintainers are notified"
const FIELD = "the field"
const SUBTITLE = "oxynote.test"
const DETAIL = "unreachable"

// the editor store and the teleported menu bodies are shared, so these
// tests cannot interleave
describe("<HookConfigPanel>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().setActiveBranchProtected(false)
		useEditorMeta().setEditable(true)
	})

	it("opens its notice, fields and confirm button from its row", async ({
		expect,
	}) => {
		await mountPanel()

		await openHookSubMenu(TITLE)

		expect(hookNotice().textContent).toContain(NOTICE)
		expect(
			document.body.querySelector(`input[placeholder='${FIELD}']`),
		).not.toBeNull()
		expect(menuButton(t("editor.hooks.create")).disabled).toBe(false)
	})

	it("passes its subtitle's detail on to its row", async ({ expect }) => {
		await mountPanel()

		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", {
				subtext: SUBTITLE,
				detail: DETAIL,
			}),
		)
	})

	it("confirms a new hook", async ({ expect }) => {
		const onSubmit = vi.fn()
		await mountPanel({ onSubmit: onSubmit })
		await openHookSubMenu(TITLE)

		menuButton(t("editor.hooks.create")).click()

		expect(onSubmit).toHaveBeenCalledTimes(1)
	})

	it("updates an existing hook", async ({ expect }) => {
		const onSubmit = vi.fn()
		const onDelete = vi.fn()
		await mountPanel({
			hook: makeHook(),
			onSubmit: onSubmit,
			onDelete: onDelete,
		})
		await openHookSubMenu(TITLE)

		menuButton(t("editor.hooks.update")).click()

		expect(onSubmit).toHaveBeenCalledTimes(1)
		expect(onDelete).toHaveBeenCalledTimes(0)
	})

	it("deletes an existing hook", async ({ expect }) => {
		const onSubmit = vi.fn()
		const onDelete = vi.fn()
		await mountPanel({
			hook: makeHook(),
			onSubmit: onSubmit,
			onDelete: onDelete,
		})
		await openHookSubMenu(TITLE)

		menuButton(t("editor.hooks.delete")).click()

		expect(onDelete).toHaveBeenCalledTimes(1)
		expect(onSubmit).toHaveBeenCalledTimes(0)
	})

	it("tells a reader in reading mode how to change the hook", async ({
		expect,
	}) => {
		useEditorMeta().setEditable(false)
		await mountPanel({ hook: makeHook() })

		await openHookSubMenu(TITLE)

		expect(menuText()).toContain(t("editor.hooks.read-only-mode"))
		expect(menuText()).not.toContain(t("editor.hooks.update"))
		expect(menuText()).not.toContain(t("editor.hooks.delete"))
	})

	it("sends a reader on a protected branch to the draft branch", async ({
		expect,
	}) => {
		useEditorStore().setActiveBranchProtected(true)
		await mountPanel({ hook: makeHook() })

		await openHookSubMenu(TITLE)

		expect(menuText()).toContain(t("editor.hooks.read-only-protected"))
		expect(menuText()).not.toContain(t("editor.hooks.update"))
	})

	it("says how to edit while the diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)
		await mountPanel({ hook: makeHook() })

		await openHookSubMenu(TITLE)

		expect(collapsed(menuText())).toContain(
			t("editor.hooks.showing-changes", {
				toggle: t("editor.name-editor.review-workflow.show-diff"),
			}),
		)
		expect(menuText()).not.toContain(t("editor.hooks.update"))
	})

	it.for<{
		name: string
		input: Partial<DocumentHook>
		expected: { status: string; icon: string }
	}>([
		{
			name: "a fresh hook plainly",
			input: { score: "100", state: {}, status: "active" },
			expected: { status: "fresh", icon: "mingcute:earth-2-line" },
		},
		{
			name: "a triggered hook in its colour",
			input: { score: "0", state: {}, status: "active" },
			expected: { status: "triggered", icon: "mingcute:earth-2-line" },
		},
		{
			name: "a failed check as a warning",
			input: { score: "100", state: {}, status: "unreachable_url" },
			expected: { status: "needs-attention", icon: "mingcute:alert-fill" },
		},
	])("shows $name", async ({ input, expected }, { expect }) => {
		await mountPanel({ hook: makeHook(input) })

		await openHookSubMenu(TITLE)

		expect(hookNotice().dataset.hookStatus).toBe(expected.status)
		expect(iconNames(hookNotice())).toEqual([expected.icon])
	})

	it.for<{
		name: string
		input: Partial<DocumentHook>
		expected: boolean
	}>([
		{
			name: "says a copy is being set up",
			input: { state: null, status: "initializing" },
			expected: true,
		},
		{
			name: "says nothing of setup once the hook is set up",
			input: { state: {}, status: "active" },
			expected: false,
		},
		{
			name: "says nothing of setup for a copy that cannot check its target",
			input: { state: null, status: "unconfigured" },
			expected: false,
		},
	])("$name", async ({ input, expected }, { expect }) => {
		await mountPanel({ hook: makeHook(input) })

		await openHookSubMenu(TITLE)

		expect(menuText().includes(t("editor.hooks.initializing"))).toBe(expected)
	})

	it("shows a setup notice plainly over a triggered hook", async ({
		expect,
	}) => {
		await mountPanel({
			hook: makeHook({ score: "0" }),
			noticeStatus: "fresh",
			noticeIcon: "mingcute:information-fill",
		})

		await openHookSubMenu(TITLE)

		expect(hookNotice().dataset.hookStatus).toBe("fresh")
		expect(iconNames(hookNotice())).toEqual(["mingcute:information-fill"])
	})

	it("dismisses a triggered hook from its notice", async ({ expect }) => {
		const onAcknowledge = vi.fn()
		await mountPanel({
			hook: makeHook({ score: "0" }),
			acknowledgeLabel: t("editor.hooks.reset"),
			onAcknowledge: onAcknowledge,
		})
		await openHookSubMenu(TITLE)

		const button = menuButton(t("editor.hooks.reset"))
		button.click()

		expect(onAcknowledge).toHaveBeenCalledTimes(1)
		expect(hookNotice().contains(button)).toBe(true)
	})

	it("lets a read-only reader dismiss a triggered hook", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const onAcknowledge = vi.fn()
		await mountPanel({
			hook: makeHook({ score: "0" }),
			acknowledgeLabel: t("editor.hooks.reset"),
			onAcknowledge: onAcknowledge,
		})
		await openHookSubMenu(TITLE)

		menuButton(t("editor.hooks.reset")).click()

		expect(onAcknowledge).toHaveBeenCalledTimes(1)
	})

	it("leaves the dismissal out of the diff and says how to get it back", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		await mountPanel({
			hook: makeHook({ score: "0" }),
			diff: {
				status: DiffStatus.Unchanged,
				targetHook: null,
				signColumn: false,
			},
			acknowledgeLabel: t("editor.hooks.reset"),
		})

		await openHookSubMenu(TITLE)

		expect(menuText()).not.toContain(t("editor.hooks.reset"))
		expect(collapsed(hookNotice().textContent)).toContain(
			t("editor.hooks.dismiss-in-diff", {
				toggle: t("editor.name-editor.review-workflow.show-diff"),
			}),
		)
	})

	it("says a removed hook was removed instead of what it does", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		await mountPanel({
			hook: makeHook({ score: "0" }),
			diff: { status: DiffStatus.Removed, targetHook: null, signColumn: false },
		})

		await openHookSubMenu(TITLE)

		expect(hookNotice().textContent.trim()).toBe(t("editor.hooks.removed"))
		expect(hookNotice().dataset.hookStatus).toBe("fresh")
	})
})

function mountPanel(props: Record<string, unknown> = {}) {
	return mountHookMenu(
		defineComponent(
			() => () =>
				h(
					HookConfigPanel,
					{ icon: "mingcute:earth-2-line", ...props },
					{
						title: () => TITLE,
						subtitle: () => SUBTITLE,
						"subtitle-detail": () => DETAIL,
						notice: () => NOTICE,
						default: () => h("input", { placeholder: FIELD }),
					},
				),
		),
		{},
	)
}

// a bold part sits on lines of its own in the template, which leaves
// spaces around it in the text
function collapsed(text: string): string {
	return text.replace(/\s+/g, " ")
}

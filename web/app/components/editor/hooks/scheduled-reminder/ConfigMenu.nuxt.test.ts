import { setResponseStatus } from "h3"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ConfigMenu from "./ConfigMenu.vue"
import { DiffStatus } from "../../diff/position-map"
import {
	makeHook,
	menuButton,
	menuText,
	mountHookMenu,
	openHookSubMenu,
	readonlyFields,
} from "../test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	ANY_STRING,
	makeXid,
	mockEndpoint,
	runInApp,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	emitFrom,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import { Select } from "~/components/shadcn/ui/select"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const HOOK_ID = makeXid("hook")

const NEW_HOOK_LABEL = "editor.hooks.scheduled-reminder.title"
const SCHEDULE = new Date("2026-09-01T10:00:00Z")

function reminderHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: HOOK_ID,
		type: DocumentHookType.ScheduledReminder,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		settings: { scale: "linear", duration: "24h", schedule: SCHEDULE },
		state: { status: "active" },
		...overrides,
	})
}

function mountMenu(props: Record<string, unknown> = {}) {
	return mountHookMenu(ConfigMenu, { nodeId: "block-1", ...props })
}

async function pickDuration(wrapper: VueWrapper, duration: string) {
	emitFrom(wrapper, Select, "update:modelValue", duration)
	await nextTick()
}

// the editor store, the query cache, the mocked toast module and the
// teleported menu bodies are all shared, so these tests cannot interleave
describe("<ScheduledReminderConfigMenu>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().setReviewableDiffActive(false)
		useEditorMeta().setEditable(true)
	})

	afterEach(disposeMockEndpoints)

	it("offers to schedule a reminder", async ({ expect }) => {
		await mountMenu()

		expect(menuText()).toContain(t(NEW_HOOK_LABEL))
	})

	it("names the date and time an active reminder will fire on", async ({
		expect,
	}) => {
		await mountMenu({ hook: reminderHook() })

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.existing-item", {
				value: formatSchedule(SCHEDULE),
			}),
		)
	})

	it("reports when a triggered reminder went out", async ({ expect }) => {
		await mountMenu({ hook: reminderHook({ score: "0" }) })

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.triggered-item", {
				value: formatSchedule(SCHEDULE),
			}),
		)
	})

	it("offers the preset durations", async ({ expect }) => {
		await mountMenu()

		await openHookSubMenu(t(NEW_HOOK_LABEL))

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.duration-label"),
		)
		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.select-placeholder"),
		)
	})

	it("keeps the create button out of reach until a duration is picked", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))

		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("creates a reminder for the duration the reader picked", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await pickDuration(wrapper, "24h")

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			type: DocumentHookType.ScheduledReminder,
			branchId: BRANCH_ID,
			blockId: "block-1",
			settings: { scale: "linear", duration: "24h", schedule: ANY_STRING },
		})
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toHaveLength(1)
	})

	it("asks for a date when the reader picks a custom duration", async ({
		expect,
	}) => {
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))

		await pickDuration(wrapper, "custom")

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.calendar-placeholder"),
		)
		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("warns when the reminder cannot be created", async ({ expect }) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await pickDuration(wrapper, "24h")

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("explains what an active block reminder will do", async ({ expect }) => {
		await mountMenu({ hook: reminderHook() })

		await openHookSubMenu("Remind on")

		expect(menuText()).toContain("the block will be highlighted")
	})

	it("explains what an active document-wide reminder will do", async ({
		expect,
	}) => {
		await mountMenu({ hook: reminderHook(), nodeId: null })

		await openHookSubMenu("Remind on")

		expect(menuText()).toContain("the relevant sections will be highlighted")
	})

	it("offers no renewal for a reminder that has not fired", async ({
		expect,
	}) => {
		await mountMenu({ hook: reminderHook() })

		await openHookSubMenu("Remind on")

		expect(menuText()).not.toContain(t("editor.hooks.renew"))
	})

	it("renews a reminder that has fired", async ({ expect }) => {
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu({ hook: reminderHook({ score: "0" }) })
		await openHookSubMenu("Reminded on")
		await pickDuration(wrapper, "72h")

		menuButton(t("editor.hooks.renew")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: { scale: "linear", duration: "72h", schedule: ANY_STRING },
		})
	})

	it("warns when the reminder cannot be renewed", async ({ expect }) => {
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const wrapper = await mountMenu({ hook: reminderHook({ score: "0" }) })
		await openHookSubMenu("Reminded on")
		await pickDuration(wrapper, "72h")

		menuButton(t("editor.hooks.renew")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("deletes the reminder", async ({ expect }) => {
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => null,
		)
		await mountMenu({ hook: reminderHook() })
		await openHookSubMenu("Remind on")

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the reminder cannot be deleted", async ({ expect }) => {
		mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		await mountMenu({ hook: reminderHook() })
		await openHookSubMenu("Remind on")

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("creates nothing while no page is open", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await pickDuration(wrapper, "24h")

		menuButton(t("editor.hooks.create")).click()
		await nextTick()

		expect(calls).toHaveLength(0)
	})

	it("tells the date and time an upcoming reminder goes out", async ({
		expect,
	}) => {
		await mountMenu({ hook: reminderHook() })

		await openHookSubMenu("Remind on")

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.existing-item-block-explanation", {
				value: formatSchedule(SCHEDULE),
			}),
		)
	})

	it("tells when a triggered reminder went out while offering to renew it", async ({
		expect,
	}) => {
		await mountMenu({ hook: reminderHook({ score: "0" }) })

		await openHookSubMenu("Reminded on")

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.triggered-item-block-explanation", {
				value: formatSchedule(SCHEDULE),
			}),
		)
		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.duration-label-triggered"),
		)
	})

	describe("when the page is read only", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorMeta().setEditable(false)
		})

		it.for([
			{
				name: "an upcoming reminder on a block",
				score: "100",
				nodeId: "block-1",
				label: "Remind on",
				key: "editor.hooks.scheduled-reminder.existing-item-block-explanation",
			},
			{
				name: "an upcoming reminder on the page",
				score: "100",
				nodeId: null,
				label: "Remind on",
				key: "editor.hooks.scheduled-reminder.existing-item-full-document-explanation",
			},
			{
				name: "a triggered reminder on a block",
				score: "0",
				nodeId: "block-1",
				label: "Reminded on",
				key: "editor.hooks.scheduled-reminder.triggered-item-block-explanation",
			},
			{
				name: "a triggered reminder on the page",
				score: "0",
				nodeId: null,
				label: "Reminded on",
				key: "editor.hooks.scheduled-reminder.triggered-item-full-document-explanation",
			},
		])(
			"explains $name without a way to change it",
			async ({ score, nodeId, label, key }, { expect }) => {
				await mountMenu({
					hook: reminderHook({ score: score }),
					nodeId: nodeId,
				})

				await openHookSubMenu(label)

				expect(menuText()).toContain(
					t(key, { value: formatSchedule(SCHEDULE) }),
				)
				expect(readonlyFields()).toEqual([])
				expect(menuText()).not.toContain(
					t("editor.hooks.scheduled-reminder.duration-label-triggered"),
				)
				expect(menuText()).not.toContain(t("editor.hooks.renew"))
				expect(menuText()).not.toContain(t("editor.hooks.delete"))
			},
		)
	})

	describe("when the diff is shown", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorStore().setReviewableDiffActive(true)
		})

		it("shows the new date over the old one", async ({ expect }) => {
			const oldSchedule = new Date("2026-08-01T10:00:00Z")
			await mountMenu({
				hook: reminderHook(),
				diff: {
					status: DiffStatus.Modified,
					targetHook: reminderHook({
						settings: {
							scale: "linear",
							duration: "24h",
							schedule: oldSchedule,
						},
					}),
					signColumn: false,
				},
			})

			await openHookSubMenu("Remind on")

			expect(menuText()).toContain(
				t("editor.hooks.scheduled-reminder.existing-item-block-explanation", {
					value: formatSchedule(SCHEDULE),
				}),
			)
			expect(readonlyFields()).toEqual([
				[formatSchedule(SCHEDULE), "added"],
				[formatSchedule(oldSchedule), "removed"],
			])
		})
	})
})

function formatSchedule(date: Date): string {
	return runInApp(() => useNuxtApp().$i18n.d(date, "short-with-time"))
}

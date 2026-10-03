import { setResponseStatus } from "h3"
import { CalendarDate } from "@internationalized/date"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ConfigMenu from "./ConfigMenu.vue"
import HookConfigPanel from "../HookConfigPanel.vue"
import { DiffStatus } from "../../diff/position-map"
import {
	hookNotice,
	makeHook,
	menuButton,
	menuButtonLabels,
	menuText,
	mountHookMenu,
	openHookSubMenu,
	readonlyFields,
} from "../test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	runInApp,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	emitFrom,
	raisedToasts,
	settleMutations,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import CalendarInput from "~/components/CalendarInput.vue"
import { Select } from "~/components/shadcn/ui/select"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const HOOK_ID = makeXid("hook")
const HOOK_PATH = `/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`

const TITLE = "editor.hooks.scheduled-reminder.title"
const NOW = new Date("2026-08-29T10:00:00Z")
const SCHEDULE = new Date("2026-09-01T10:00:00Z")
const DAY_MS = 24 * 60 * 60 * 1000

function reminderHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: HOOK_ID,
		type: DocumentHookType.ScheduledReminder,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		settings: { scale: "linear", duration: "24h", schedule: SCHEDULE },
		state: { lastActiveAt: NOW },
		...overrides,
	})
}

function customReminderHook(overrides: Partial<DocumentHook> = {}) {
	return reminderHook({
		settings: { scale: "linear", duration: "custom", schedule: SCHEDULE },
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

async function pickDate(wrapper: VueWrapper, date: CalendarDate) {
	emitFrom(wrapper, CalendarInput, "update:modelValue", date)
	await nextTick()
}

// the editor store, the query cache, the mocked toast module, the faked
// clock and the teleported menu bodies are all shared, so these tests
// cannot interleave
describe("<ScheduledReminderConfigMenu>", { concurrent: false }, () => {
	beforeEach(() => {
		vi.useFakeTimers({ toFake: ["Date"] })
		vi.setSystemTime(NOW)
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().setActiveBranchProtected(false)
		useEditorMeta().setEditable(true)
	})

	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	it("offers to schedule a reminder", async ({ expect }) => {
		await mountMenu()

		expect(menuText()).toContain(t(TITLE))
		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.description"),
		)
	})

	it("says when a waiting reminder goes out", async ({ expect }) => {
		await mountMenu({ hook: reminderHook() })

		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", {
				subtext: formatSchedule(SCHEDULE),
				detail: "in 3 days",
			}),
		)
	})

	it("says how long ago a triggered reminder went out", async ({ expect }) => {
		vi.setSystemTime(new Date(SCHEDULE.getTime() + DAY_MS))

		await mountMenu({ hook: reminderHook({ score: "0" }) })

		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", {
				subtext: t("editor.hooks.scheduled-reminder.subtext-triggered", {
					date: formatSchedule(SCHEDULE),
				}),
				detail: "1 day ago",
			}),
		)
	})

	it("offers the preset durations", async ({ expect }) => {
		await mountMenu()

		await openHookSubMenu(t(TITLE))

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
		await openHookSubMenu(t(TITLE))

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
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "24h")

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			type: DocumentHookType.ScheduledReminder,
			branchId: BRANCH_ID,
			blockId: "block-1",
			settings: {
				scale: "linear",
				duration: "24h",
				schedule: new Date(NOW.getTime() + DAY_MS).toISOString(),
			},
		})
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toHaveLength(1)
	})

	it("starts empty again once a reminder is created", async ({ expect }) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, () => ({
			id: HOOK_ID,
		}))
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "24h")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		await openHookSubMenu(t(TITLE))
		expect(wrapper.findComponent(Select).props("modelValue")).toBeUndefined()
	})

	it("asks for a date when the reader picks a custom duration", async ({
		expect,
	}) => {
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))

		await pickDuration(wrapper, "custom")

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.calendar-placeholder"),
		)
		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it.for([
		{
			name: "block",
			nodeId: "block-1",
			key: "editor.hooks.scheduled-reminder.fresh-notice",
		},
		{
			name: "page",
			nodeId: null,
			key: "editor.hooks.scheduled-reminder.fresh-notice",
		},
	])(
		"says when a waiting $name reminder goes out",
		async ({ nodeId, key }, { expect }) => {
			await mountMenu({ hook: reminderHook(), nodeId: nodeId })

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().dataset.hookStatus).toBe("fresh")
			expect(hookNotice().textContent.trim()).toBe(
				t(key, {
					when: t("editor.hooks.scheduled-reminder.when-on-date", {
						date: formatSchedule(SCHEDULE),
					}),
				}),
			)
		},
	)

	it("names the picked preset in its notice", async ({ expect }) => {
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))

		await pickDuration(wrapper, "72h")

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.scheduled-reminder.new-notice", {
				when: t("editor.hooks.scheduled-reminder.when-options.72h"),
			}),
		)
	})

	it("asks for a date in its notice until one is picked", async ({
		expect,
	}) => {
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))

		await pickDuration(wrapper, "custom")

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.scheduled-reminder.new-notice", {
				when: t("editor.hooks.scheduled-reminder.when-unset"),
			}),
		)
		expect(hookNotice().querySelector(".font-semibold")).toBeNull()
	})

	it("names the picked date in its notice", async ({ expect }) => {
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "custom")

		await pickDate(wrapper, new CalendarDate(2026, 9, 10))

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.scheduled-reminder.new-notice", {
				when: t("editor.hooks.scheduled-reminder.when-on-date", {
					date: formatLong(new Date(2026, 8, 10)),
				}),
			}),
		)
	})

	it("starts a waiting reminder on its own preset", async ({ expect }) => {
		const wrapper = await mountMenu({ hook: reminderHook() })

		await openHookSubMenu(t(TITLE))

		expect(wrapper.findComponent(Select).props("modelValue")).toBe("24h")
		expect(menuButton(t("editor.hooks.update")).disabled).toBe(true)
	})

	it("starts a waiting custom reminder on its date", async ({ expect }) => {
		const wrapper = await mountMenu({ hook: customReminderHook() })

		await openHookSubMenu(t(TITLE))

		expect(wrapper.findComponent(Select).props("modelValue")).toBe("custom")
		// the input is generic, which find by component cannot type
		expect(
			String(
				wrapper.findComponent({ name: "CalendarInput" }).props("modelValue"),
			),
		).toBe(String(dateToCalendarDate(SCHEDULE)))
		expect(menuButton(t("editor.hooks.update")).disabled).toBe(true)
	})

	it("sends nothing while no duration is picked", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))

		emitFrom(wrapper, HookConfigPanel, "submit")
		await settleMutations()

		expect(calls).toHaveLength(0)
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toBeUndefined()
	})

	it("keeps the update button out of reach until a custom date is picked", async ({
		expect,
	}) => {
		const wrapper = await mountMenu({ hook: reminderHook() })
		await openHookSubMenu(t(TITLE))

		await pickDuration(wrapper, "custom")

		expect(menuButton(t("editor.hooks.update")).disabled).toBe(true)
	})

	it("updates a waiting reminder once another duration is picked", async ({
		expect,
	}) => {
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		const wrapper = await mountMenu({ hook: reminderHook() })
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "72h")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: {
				scale: "linear",
				duration: "72h",
				schedule: new Date(NOW.getTime() + 3 * DAY_MS).toISOString(),
			},
		})
	})

	it("calls a failed change to a waiting reminder an update", async ({
		expect,
	}) => {
		mockEndpoint("PUT", HOOK_PATH, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		const wrapper = await mountMenu({ hook: reminderHook() })
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "72h")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("editor.hooks.errors.update-failed") },
			])
		}, WAIT_FOR_OPTIONS)
	})

	it("asks for a new pick on a triggered reminder", async ({ expect }) => {
		const wrapper = await mountMenu({ hook: reminderHook({ score: "0" }) })

		await openHookSubMenu(t(TITLE))

		expect(menuText()).toContain(
			t("editor.hooks.scheduled-reminder.duration-label-triggered"),
		)
		expect(wrapper.findComponent(Select).props("modelValue")).toBeUndefined()
		expect(menuButton(t("editor.hooks.renew")).disabled).toBe(true)
		expect(menuButtonLabels()).not.toContain(t("editor.hooks.reset"))
	})

	it("asks for a new pick once a reminder triggers", async ({ expect }) => {
		const props = reactive<Record<string, unknown>>({
			nodeId: "block-1",
			hook: reminderHook(),
		})
		const wrapper = await mountHookMenu(ConfigMenu, props)

		props.hook = reminderHook({ score: "0" })
		await nextTick()

		expect(wrapper.findComponent(Select).exists()).toBe(false)
		await openHookSubMenu(t(TITLE))
		expect(wrapper.findComponent(Select).props("modelValue")).toBeUndefined()
	})

	it("tells when a triggered reminder went out", async ({ expect }) => {
		await mountMenu({ hook: reminderHook({ score: "0" }) })

		await openHookSubMenu(t(TITLE))

		expect(hookNotice().dataset.hookStatus).toBe("triggered")
		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.scheduled-reminder.triggered-notice", {
				date: formatSchedule(SCHEDULE),
			}),
		)
	})

	it("renews a triggered reminder", async ({ expect }) => {
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		const wrapper = await mountMenu({ hook: reminderHook({ score: "0" }) })
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "72h")

		menuButton(t("editor.hooks.renew")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: {
				scale: "linear",
				duration: "72h",
				schedule: new Date(NOW.getTime() + 3 * DAY_MS).toISOString(),
			},
		})
	})

	it("calls a failed change to a triggered reminder a renewal", async ({
		expect,
	}) => {
		mockEndpoint("PUT", HOOK_PATH, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		const wrapper = await mountMenu({ hook: reminderHook({ score: "0" }) })
		await openHookSubMenu(t(TITLE))
		await pickDuration(wrapper, "72h")

		menuButton(t("editor.hooks.renew")).click()

		await vi.waitFor(() => {
			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("editor.hooks.errors.renew-failed") },
			])
		}, WAIT_FOR_OPTIONS)
	})

	it("deletes the reminder", async ({ expect }) => {
		const calls = mockEndpoint("DELETE", HOOK_PATH, () => null)
		await mountMenu({ hook: reminderHook() })
		await openHookSubMenu(t(TITLE))

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	describe("when the page is read only", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorMeta().setEditable(false)
		})

		it("shows a waiting reminder's preset without a way to change it", async ({
			expect,
		}) => {
			const wrapper = await mountMenu({ hook: reminderHook() })

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				[presetValue("24h", SCHEDULE), "unchanged"],
			])
			expect(wrapper.findComponent(Select).exists()).toBe(false)
			expect(menuText()).toContain(t("editor.hooks.read-only-mode"))
			expect(menuButtonLabels()).toEqual([])
		})

		it("shows a custom reminder's date", async ({ expect }) => {
			await mountMenu({ hook: customReminderHook() })

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				[
					t("editor.hooks.scheduled-reminder.field-value-custom", {
						date: formatSchedule(SCHEDULE),
					}),
					"unchanged",
				],
			])
		})

		it("renews a triggered reminder by its own preset", async ({ expect }) => {
			const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
			await mountMenu({ hook: reminderHook({ score: "0" }) })
			await openHookSubMenu(t(TITLE))

			menuButton(
				t("editor.hooks.scheduled-reminder.remind-again-options.24h"),
			).click()

			await vi.waitFor(() => {
				expect(calls).toHaveLength(1)
			}, WAIT_FOR_OPTIONS)
			expect(calls[0]?.body).toEqual({
				settings: {
					scale: "linear",
					duration: "24h",
					schedule: new Date(NOW.getTime() + DAY_MS).toISOString(),
				},
			})
		})

		it("calls a failed renewal by preset a renewal", async ({ expect }) => {
			mockEndpoint("PUT", HOOK_PATH, (_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			})
			await mountMenu({ hook: reminderHook({ score: "0" }) })
			await openHookSubMenu(t(TITLE))

			menuButton(
				t("editor.hooks.scheduled-reminder.remind-again-options.24h"),
			).click()

			await vi.waitFor(() => {
				expect(raisedToasts()).toMatchObject([
					{ type: "error", title: t("editor.hooks.errors.renew-failed") },
				])
			}, WAIT_FOR_OPTIONS)
		})

		it("renews nothing for a reminder without a preset", async ({ expect }) => {
			const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
			const wrapper = await mountMenu({
				hook: customReminderHook({ score: "0" }),
			})
			await openHookSubMenu(t(TITLE))

			emitFrom(wrapper, HookConfigPanel, "acknowledge")
			await settleMutations()

			expect(calls).toHaveLength(0)
		})

		it("says a triggered custom reminder will not repeat", async ({
			expect,
		}) => {
			await mountMenu({ hook: customReminderHook({ score: "0" }) })

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().textContent).toContain(
				t("editor.hooks.scheduled-reminder.one-off-note"),
			)
			expect(menuButtonLabels()).toEqual([])
		})

		it.for([
			{
				name: "a block",
				nodeId: "block-1",
				key: "editor.hooks.scheduled-reminder.triggered-notice",
			},
			{
				name: "the page",
				nodeId: null,
				key: "editor.hooks.scheduled-reminder.triggered-notice",
			},
		])(
			"says when a triggered reminder on $name went out",
			async ({ nodeId, key }, { expect }) => {
				await mountMenu({ hook: reminderHook({ score: "0" }), nodeId: nodeId })

				await openHookSubMenu(t(TITLE))

				expect(hookNotice().textContent).toContain(
					t(key, { date: formatSchedule(SCHEDULE) }),
				)
				expect(hookNotice().textContent).not.toContain(
					t("editor.hooks.scheduled-reminder.one-off-note"),
				)
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

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				[presetValue("24h", SCHEDULE), "added"],
				[presetValue("24h", oldSchedule), "removed"],
			])
			expect(menuButtonLabels()).toEqual([])
		})

		it.for([
			{ name: "added", status: DiffStatus.Added },
			{ name: "removed", status: DiffStatus.Removed },
		])(
			"marks every field of an $name reminder",
			async ({ status, name }, { expect }) => {
				await mountMenu({
					hook: reminderHook(),
					diff: { status: status, targetHook: null, signColumn: true },
				})

				await openHookSubMenu(t(TITLE))

				expect(readonlyFields()).toEqual([[presetValue("24h", SCHEDULE), name]])
			},
		)

		it("keeps a triggered reminder from being renewed", async ({ expect }) => {
			useEditorMeta().setEditable(false)
			await mountMenu({
				hook: reminderHook({ score: "0" }),
				diff: {
					status: DiffStatus.Unchanged,
					targetHook: null,
					signColumn: false,
				},
			})

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().dataset.hookStatus).toBe("triggered")
			expect(menuButtonLabels()).toEqual([])
		})
	})
})

function formatSchedule(date: Date): string {
	return runInApp(() => useNuxtApp().$i18n.d(date, "short-with-time"))
}

function formatLong(date: Date): string {
	return runInApp(() => useNuxtApp().$i18n.d(date, "long"))
}

function presetValue(duration: string, date: Date): string {
	return t("editor.hooks.scheduled-reminder.field-value", {
		duration: t(`editor.hooks.scheduled-reminder.duration-options.${duration}`),
		date: formatSchedule(date),
	})
}

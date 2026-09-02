// shared helpers for the component test suites in this directory and its
// subdirectories. Test-only: nothing here is imported by app code, and the
// app/**/test-helpers.ts coverage exclude keeps it out of the denominator.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { vi } from "vitest"
import type { H3Event } from "h3"
import {
	mockEndpoint,
	runInApp,
	seedQueryData,
	type RecordedCall,
} from "~/composables/api/test-helpers"
import { SidebarProvider } from "./shadcn/ui/sidebar"
import { TooltipProvider } from "./shadcn/ui/tooltip"

// eslint's ts program resolves .vue imports as error typed, so a component
// handed to one of these helpers looks unsafe to it while vue-tsc types it
// fine. Taking it as `any` keeps that false positive to the single disable
// below instead of one at every call site.
type TestComponent = any

// @nuxt/icon's css mode renders every icon as <span class="iconify i-<name>">,
// so the rendered icon set is the only observable trace of which icon a
// component picked
export function renderedIconNames(wrapper: VueWrapper): string[] {
	return wrapper.findAll(".iconify").flatMap((icon) => {
		const cls = icon.classes().find((c) => c.startsWith("i-"))

		return cls ? [cls.slice("i-".length)] : []
	})
}

// finds a button by the text it renders — including its sr-only label,
// which is the only text most icon buttons have
export function findButtonByText(wrapper: VueWrapper, text: string) {
	const button = wrapper.findAll("button").find((b) => b.text().includes(text))
	if (!button) {
		throw new Error(`no button rendering "${text}"`)
	}

	return button
}

// useAuthSession reads the session, organization and account list through
// pinia-colada cache entries. Seeding them before a mount is what puts a
// component into a signed-in state without any request going out.
export function seedAuthSession(user: Record<string, unknown> | null) {
	seedQueryData(["auth", "session"], {
		data: user ? { session: { id: "session-1" }, user: user } : null,
		error: null,
	})
}

export function seedAuthOrganization(
	organization: Record<string, unknown> | null,
) {
	seedQueryData(["auth", "organization"], {
		data: organization,
		error: null,
	})
}

export function seedAuthAccounts(providerIds: string[]) {
	seedQueryData(["auth", "accounts"], {
		data: providerIds.map((providerId) => ({ providerId: providerId })),
		error: null,
	})
}

// every capability defaults to enabled here, matching what the composable
// reports before its request resolves, so each test names only the
// services it is actually gating on
export function seedCapabilities(capabilities: Partial<Capabilities> = {}) {
	seedQueryData(["capabilities"], {
		github: true,
		slack: true,
		changeDetection: true,
		search: true,
		aiAssistant: { status: AssistantStatus.Active, model: "test-model" },
		...capabilities,
	})
}

// reka-ui tooltips inject a provider context that the app installs once,
// at page level (app/pages/[[organizationSlug]]/[[documentSlug]].vue), so
// a component rendering a tooltip needs one around it to mount at all
export function mountUnderTooltipProvider(
	component: TestComponent,
	options: { props?: Record<string, unknown>; slots?: Record<string, unknown> },
) {
	return mountSuspended(TooltipProvider, {
		slots: {
			default: () => h(component, options.props, options.slots),
		},
	})
}

// useSidebar/useSidebarWidth inject a context the app installs once, at
// page level — a component reading either needs a provider around it to
// mount at all. SidebarProvider also installs the tooltip context.
export function mountUnderSidebarProvider(
	component: TestComponent,
	options: {
		props?: Record<string, unknown>
		slots?: Record<string, unknown>
		stubs?: Record<string, boolean>
	} = {},
) {
	return mountSuspended(SidebarProvider, {
		slots: {
			default: () => h(component, options.props, options.slots),
		},
		global: { stubs: options.stubs },
	})
}

// unmounting a wrapper does not always take the overlay bodies reka-ui
// teleported into <body> with it — a pending presence transition outlives
// it — and their generated ids repeat across mounts, so a stale node would
// answer the next lookup. Suites driving overlays clear them per test.
// one persistent-state ref serves a whole app, so a mount after the first
// no longer re-reads document.cookie — seed the state itself instead
export function seedPersistentState(key: string, value: unknown) {
	const state = runInApp(() =>
		usePersistentState({ key: key, defaultValue: value }),
	)

	state.value = value
}

export function clearTeleportedOverlays() {
	Array.from(document.body.children).forEach((child) => {
		if (child.id !== "__nuxt" && child.id !== "teleports") {
			child.remove()
		}
	})
}

// useMediaQuery reads matchMedia, which happy-dom does not implement — the
// composable then reports "not matching" for every query, so any layout
// that switches on viewport width needs this to pick a side deliberately
export function stubViewportMatches(matches: boolean) {
	vi.stubGlobal(
		"matchMedia",
		vi.fn((query: string) => ({
			matches: matches,
			media: query,
			onchange: null,
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			dispatchEvent: vi.fn(),
		})),
	)
}

// noUncheckedIndexedAccess types every index access as possibly undefined
// and the non-null assertion is banned, so an out-of-range index says so
// instead of failing later on an unrelated line
export function at<T>(items: T[], index: number): T {
	const item = items[index]
	if (item === undefined) {
		throw new Error(`no element at index ${index}`)
	}

	return item
}

// dropdown, context and sub menus are teleported into <body>; every
// actionable row in one carries a menuitem role
export function menuItem(text: string): HTMLElement {
	const item = Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	).find((el) => el.textContent.includes(text))
	if (!item) {
		throw new Error(`no menu item rendering "${text}"`)
	}

	return item
}

// dialog and sheet bodies are teleported into <body> too, so their buttons
// cannot be reached through the wrapper
export function teleportedButton(text: string): HTMLButtonElement {
	const button = Array.from(
		document.body.querySelectorAll<HTMLButtonElement>("button"),
	).find((el) => el.textContent.includes(text))
	if (!button) {
		throw new Error(`no teleported button rendering "${text}"`)
	}

	return button
}

// reka-ui teleports an open tooltip body out of the wrapper into <body>
// and points the trigger's aria-describedby at it. Resolving through that
// id keeps the assertion on this mount's tooltip rather than a leftover
// node from an earlier one, and yields "" while the tooltip is closed.
export function openTooltipText(wrapper: VueWrapper): string {
	const id = wrapper
		.get("[data-slot='tooltip-trigger']")
		.attributes("aria-describedby")
	if (!id) {
		return ""
	}

	return document.getElementById(id)?.textContent ?? ""
}

// the settings action modals hold their submit for delay(300) so the
// spinner is visible; driving that needs fake timers, but installing them
// before the mount leaves the nuxt app's own async setup frozen — so they
// go in once the component is up
export async function mountWithFrozenClock(
	component: TestComponent,
	options: { props?: Record<string, unknown> } = {},
) {
	const wrapper = await mountSuspended(component, { props: options.props })
	vi.useFakeTimers()

	return wrapper
}

// walks a submitted action all the way through: vee-validate's async
// validation, the delay(300) spinner hold, the request, and the state
// updates that follow — each needs its own turn of the frozen clock
export async function settleActionSubmit() {
	for (let round = 0; round < 2; round++) {
		await settleMutations()
		await vi.advanceTimersByTimeAsync(300)
	}

	await settleMutations()
}

// emits an event as if the named (or imported) child component had. eslint's
// ts program cannot type a component found inside a wrapper, so every $emit
// through one is reported unsafe; vue-tsc types them fine — funnelling them
// through here keeps that to a single disable.
export function emitFrom(
	wrapper: VueWrapper,
	target: TestComponent,
	event: string,
	...args: unknown[]
) {
	emitFromNth(wrapper, target, 0, event, ...args)
}

// same, for the nth match — a component that renders itself recursively
// has several instances of the same kind mounted at once
export function emitFromNth(
	wrapper: VueWrapper,
	target: TestComponent,
	index: number,
	event: string,
	...args: unknown[]
) {
	const found = wrapper.findAllComponents(
		typeof target === "string" ? { name: target } : target,
	)[index]
	if (!found) {
		throw new Error(`no such component at index ${index}`)
	}

	// eslint-disable-next-line @typescript-eslint/no-unsafe-call, @typescript-eslint/no-unsafe-member-access -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
	found.vm.$emit(event, ...args)
}

// vi.waitFor's one-second default is tight once the whole suite competes
// for the same cores, and a timeout there reads as a real failure
export const WAIT_FOR_OPTIONS = { timeout: 15_000 }

// a pinia-colada mutation that invalidates queries on success only settles
// after its own request, the invalidated queries' refetches, and the vue
// updates in between — several macrotasks deep, so one flush is not enough
export async function settleMutations() {
	await flushPromises()
	await flushPromises()
	await flushPromises()
}

// a better-auth call needs both spellings of its url registered: the
// absolute one is what keeps the request off the real network, and the
// bare path is what the test-time h3 app then matches a handler on (the
// origin is stripped on the way in). See vitest.nuxt-setup.ts for the
// long version. The path entry is the one that actually answers, so its
// recorded calls are what comes back.
export function mockAuthEndpoint(
	endpoint: string,
	respond: (call: RecordedCall, event: H3Event) => unknown,
	method: "GET" | "POST" = "POST",
): RecordedCall[] {
	mockEndpoint(
		method,
		`http://test.local/auth-realtime/api/auth/${endpoint}`,
		respond,
	)

	return mockEndpoint(method, `/auth-realtime/api/auth/${endpoint}`, respond)
}

// components that refetch the organization after a change bypass the
// seeded cache entry and go back to better-auth for it
export function mockAuthOrganization(organization: Record<string, unknown>) {
	seedAuthOrganization(organization)

	return mockAuthEndpoint(
		"organization/get-full-organization",
		() => organization,
		"GET",
	)
}

// resolves a message through the same i18n instance the components
// render with, so an assertion never carries a second copy of the
// english text. Named placeholders take their values here, exactly as
// the component passes them: t("sidebar.search.no-results", { query })
export function t(key: string, params: Record<string, unknown> = {}): string {
	const message = runInApp(() => useNuxtApp().$i18n.t(key, params))

	// vue-i18n answers a miss with the key itself, which would leave an
	// assertion comparing two identical key strings and passing
	if (message === key) {
		throw new Error(`no translation for "${key}"`)
	}

	return message
}

// shared helpers for the editor hook menu suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import {
	DropdownMenu,
	DropdownMenuContent,
} from "~/components/shadcn/ui/dropdown-menu"

// eslint's ts program resolves .vue imports as error typed, so a
// component handed to mountHookMenu looks unsafe to it while vue-tsc
// types it fine
type TestComponent = any

export function makeHook(overrides: Partial<DocumentHook> = {}): DocumentHook {
	return {
		id: "hook-1",
		type: DocumentHookType.URLWatcher,
		documentId: "doc-1",
		organizationId: "org-1",
		branchId: "branch-1",
		blockId: "block-1",
		settings: { url: "https://example.com" },
		state: { status: "active" },
		score: "100",
		createdAt: new Date("2026-01-01T00:00:00Z"),
		...overrides,
	}
}

// the hook menus render a dropdown sub-menu, which reka-ui only lets
// mount inside an open menu — and teleports into <body> from there
export function mountHookMenu(
	component: TestComponent,
	props: Record<string, unknown>,
) {
	return mountSuspended(DropdownMenu, {
		props: { open: true },
		slots: {
			default: () =>
				h(DropdownMenuContent, null, {
					default: () => h(component, props),
				}),
		},
	})
}

// the sub-menu body only mounts once its trigger has been opened
export async function openHookSubMenu(label: string) {
	const trigger = Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	).find((item) => item.textContent.includes(label))
	if (!trigger) {
		throw new Error(`no sub menu trigger rendering "${label}"`)
	}

	trigger.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
	trigger.click()
	await nextTick()
	await nextTick()
}

// the menu bodies live in <body>, out of the wrapper's reach
export function menuText(): string {
	return document.body.textContent
}

export function menuButton(text: string): HTMLButtonElement {
	const button = Array.from(
		document.body.querySelectorAll<HTMLButtonElement>("button"),
	).find((candidate) => candidate.textContent.includes(text))
	if (!button) {
		throw new Error(`no menu button rendering "${text}"`)
	}

	return button
}

export async function typeInMenu(value: string, index = 0) {
	const input = document.body.querySelectorAll<HTMLInputElement>("input")[index]
	if (!input) {
		throw new Error(`no menu input at index ${index}`)
	}

	input.value = value
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await nextTick()
}

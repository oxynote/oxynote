// shared helpers for the editor hook menu suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import {
	DropdownMenu,
	DropdownMenuContent,
} from "~/components/shadcn/ui/dropdown-menu"
import { TooltipProvider } from "~/components/shadcn/ui/tooltip"

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
// mount inside an open menu — and teleports into <body> from there. The
// menu header's tooltips need the provider the page installs
export function mountHookMenu(
	component: TestComponent,
	props: Record<string, unknown>,
) {
	return mountSuspended(TooltipProvider, {
		slots: {
			default: () =>
				h(
					DropdownMenu,
					{ open: true },
					{
						default: () =>
							h(DropdownMenuContent, null, {
								default: () => h(component, props),
							}),
					},
				),
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

// the read only fields in the menu bodies, each with the side of the diff
// its tint marks
export function readonlyFields(): [
	string,
	"added" | "removed" | "unchanged",
][] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>(
			"[role='textbox'][aria-readonly='true']",
		),
	).map((field) => [
		field.textContent.trim(),
		field.classList.contains("bg-diff-field-added")
			? "added"
			: field.classList.contains("bg-diff-field-removed")
				? "removed"
				: "unchanged",
	])
}

// @nuxt/icon's css mode renders an icon as <span class="iconify i-<name>">,
// so the class is the only trace of which icon a menu picked
export function iconNames(root: Element): string[] {
	return Array.from(root.querySelectorAll(".iconify")).flatMap((icon) => {
		const name = Array.from(icon.classList).find((c) => c.startsWith("i-"))

		return name ? [name.slice("i-".length)] : []
	})
}

// the notice of the open hook menu, with the status its tint shows
export function hookNotice(): HTMLElement {
	const notice = document.body.querySelector<HTMLElement>("[data-hook-status]")
	if (!notice) {
		throw new Error("no hook notice is rendered")
	}

	return notice
}

// the labels of every button in the menu bodies, for when a label also
// shows up in other text
export function menuButtonLabels(): string[] {
	return Array.from(document.body.querySelectorAll("button")).map((button) =>
		button.textContent.trim(),
	)
}

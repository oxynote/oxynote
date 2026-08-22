// shared helpers for the drag-handle menu option suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { Node as PMNode } from "@tiptap/pm/model"
import {
	DropdownMenu,
	DropdownMenuContent,
} from "~/components/shadcn/ui/dropdown-menu"

// eslint's ts program resolves .vue imports as error typed, so a
// component handed to mountMenuOptions looks unsafe to it while vue-tsc
// types it fine
type TestComponent = any

export interface HoveredBlock {
	node: PMNode
	nodePos: number
	nodeId: string
	nodeHooks: DocumentHook[] | null
	nodeHookStatus: "stale" | "fresh" | null
}

// the block the drag handle is pointing at, reduced to what the option
// components read off it
export function hoveredBlock(
	nodePos: number,
	options: {
		attrs?: Record<string, unknown>
		typeName?: string
		children?: { typeName: string; nodeSize: number }[]
	} = {},
): HoveredBlock {
	const children = options.children ?? []

	return {
		node: {
			attrs: options.attrs ?? {},
			type: { name: options.typeName ?? "testNode" },
			forEach: (
				callback: (
					child: { type: { name: string } },
					offset: number,
					index: number,
				) => void,
			) => {
				let offset = 0

				children.forEach((child, index) => {
					callback({ type: { name: child.typeName } }, offset, index)
					offset += child.nodeSize
				})
			},
		} as unknown as PMNode,
		nodePos: nodePos,
		nodeId: `block-${nodePos}`,
		nodeHooks: null,
		nodeHookStatus: null,
	}
}

// the option components render bare menu items, which reka-ui only lets
// mount inside an open menu — and teleports into <body> from there. A
// getter keeps the props reactive, so a suite can change what the handle
// is pointing at mid-test.
export function mountMenuOptions(
	component: TestComponent,
	props: Record<string, unknown> | (() => Record<string, unknown>),
) {
	return mountSuspended(DropdownMenu, {
		props: { open: true },
		slots: {
			default: () =>
				h(DropdownMenuContent, null, {
					default: () =>
						h(component, typeof props === "function" ? props() : props),
				}),
		},
	})
}

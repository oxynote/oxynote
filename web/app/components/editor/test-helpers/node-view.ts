// helpers for the component suites that mount a tiptap node view on its
// own. Test-only: the app/**/test-helpers/** coverage exclude keeps this
// out of the denominator, and nothing here is imported by app code.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { Editor } from "@tiptap/vue-3"
import type { Node as PMNode } from "@tiptap/pm/model"
import { vi } from "vitest"
import { TooltipProvider } from "~/components/shadcn/ui/tooltip"

// eslint's ts program resolves .vue imports as error typed, so a
// component handed to mountNodeView looks unsafe to it while vue-tsc
// types it fine. Taking it as `any` keeps that false positive here
// instead of at every call site.
type TestComponent = any

// NodeViewWrapper injects the drag handler and decoration classes that
// tiptap's VueNodeViewRenderer provides around a live node view — a node
// view mounted on its own has no renderer above it and would fail to
// resolve them
const nodeViewInjections = {
	onDragStart: () => undefined,
	decorationClasses: "",
}

export interface ChainedCall {
	name: string
	args: unknown[]
}

// only attrs, textContent, nodeSize and the type name are read by a node
// view rendering on its own, so a stand-in spares every suite a schema
export function makeNode(
	attrs: Record<string, unknown> = {},
	options: { typeName?: string; textContent?: string; nodeSize?: number } = {},
): PMNode {
	return {
		attrs: attrs,
		textContent: options.textContent ?? "",
		nodeSize: options.nodeSize ?? 2,
		type: { name: options.typeName ?? "testNode" },
	} as unknown as PMNode
}

export interface EditorStub {
	editor: Editor
	commands: ChainedCall[]
	on: ReturnType<typeof vi.fn>
	off: ReturnType<typeof vi.fn>
}

// stands in for the editor a node view is handed. The command chain it
// builds — editor.chain().focus().someCommand(x).run() — is recorded as
// a flat call list, so a suite asserts which commands ran, in which
// order, and with what. Anything a particular node view reads beyond
// that (state, isEditable, storage) is passed in as an override.
export function makeEditor(
	overrides: Record<string, unknown> = {},
): EditorStub {
	const commands: ChainedCall[] = []
	const chain = new Proxy(
		{},
		{
			get(_target, name: string | symbol) {
				return (...args: unknown[]) => {
					commands.push({ name: String(name), args: args })

					return String(name) === "run" ? true : chain
				}
			},
		},
	)

	const on = vi.fn()
	const off = vi.fn()

	const editor = {
		chain: () => chain,
		isEditable: true,
		on: on,
		off: off,
		state: makeEditorState(),
		...overrides,
	} as unknown as Editor

	return { editor: editor, commands: commands, on: on, off: off }
}

// the slice of EditorState a node view reads: where the selection sits
// and what the node at a position is nested in
export function makeEditorState(
	options: { from?: number; to?: number; parentTypeName?: string } = {},
): Record<string, unknown> {
	return {
		selection: { from: options.from ?? 0, to: options.to ?? 0 },
		doc: {
			resolve: () => ({
				node: () => ({ type: { name: options.parentTypeName ?? "doc" } }),
			}),
		},
	}
}

export function commandNames(commands: ChainedCall[]): string[] {
	return commands.map((command) => command.name)
}

// the arguments the named command was called with, or undefined when it
// never ran
export function commandArgs(
	commands: ChainedCall[],
	name: string,
): unknown[] | undefined {
	return commands.find((command) => command.name === name)?.args
}

// every prop tiptap passes a node view, so a suite only spells out the
// ones its assertions depend on
function nodeViewPropsFor(
	overrides: Record<string, unknown> = {},
): Record<string, unknown> {
	return {
		editor: makeEditor().editor,
		node: makeNode(),
		decorations: [],
		selected: false,
		extension: { options: {} },
		getPos: () => 0,
		updateAttributes: () => undefined,
		deleteNode: () => undefined,
		view: {},
		innerDecorations: {},
		HTMLAttributes: {},
		...overrides,
	}
}

// onError installs a vue app error handler. Without one vue re-throws
// what an async event handler rejected with, which surfaces as an
// unhandled rejection instead of something a test can assert on.
export function mountNodeView(
	component: TestComponent,
	overrides: Record<string, unknown> = {},
	options: { onError?: (error: unknown) => void } = {},
) {
	return mountSuspended(component, {
		props: nodeViewPropsFor(overrides),
		global: {
			provide: nodeViewInjections,
			...(options.onError ? { config: { errorHandler: options.onError } } : {}),
		},
	})
}

// a node view rendering a ShortcutTooltip also needs reka-ui's tooltip
// context, which the app installs once at page level
export function mountNodeViewUnderTooltipProvider(
	component: TestComponent,
	overrides: Record<string, unknown> = {},
) {
	return mountSuspended(TooltipProvider, {
		slots: {
			default: () => h(component, nodeViewPropsFor(overrides)),
		},
		global: { provide: nodeViewInjections },
	})
}

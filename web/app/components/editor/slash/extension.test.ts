import type { Editor } from "@tiptap/core"
import type {
	SuggestionKeyDownProps,
	SuggestionOptions,
	SuggestionProps,
} from "@tiptap/suggestion"
import { Schema } from "@tiptap/pm/model"
import { EditorState, TextSelection, type Transaction } from "@tiptap/pm/state"
import { beforeEach, describe, it, vi } from "vitest"
import { CODE_BLOCK_NAME } from "../blocks/node-names"
import { filterSlashItems, type CommandItem } from "./items"
import {
	processSlashCommandTransaction,
	SLASH_COMMAND_TRIGGER_CHAR,
	SlashCommands,
	type SlashCommandStorage,
} from "./extension"

type SlashSuggestionOptions = SuggestionOptions<CommandItem, CommandItem>
type SlashSuggestionRenderer = ReturnType<
	NonNullable<SlashSuggestionOptions["render"]>
>

const suggestionMock = vi.hoisted(() =>
	vi.fn((_options: unknown) => ({ suggestion: true })),
)

vi.mock("@tiptap/suggestion", () => ({ default: suggestionMock }))

// the list component is a nuxt-flavoured SFC reaching for #imports,
// which the node project cannot resolve; the extension only ever hands
// it to the renderer
vi.mock("./CommandList.vue", () => ({ default: { name: "CommandList" } }))

const { rendererControl, rendererInstances, VueRendererStub } = vi.hoisted(
	() => {
		interface Instance {
			props: Record<string, unknown>
			element: { style: Record<string, string> } | null
			ref: unknown
			updateProps: (props: Record<string, unknown>) => void
			destroy: () => void
		}

		const instances: Instance[] = []

		// vue only hands the renderer an element once the component
		// mounted, so a test can ask for one that never got there
		const control = { elementless: false }

		class VueRendererStub implements Instance {
			props: Record<string, unknown>
			element: { style: Record<string, string> } | null
			ref: unknown = null
			updateProps = vi.fn((props: Record<string, unknown>) => {
				Object.assign(this.props, props)
			})

			destroy = vi.fn()

			constructor(
				_component: unknown,
				options: { props: Record<string, unknown> },
			) {
				this.props = options.props
				this.element = control.elementless ? null : { style: {} }
				instances.push(this)
			}
		}

		return {
			rendererControl: control,
			rendererInstances: instances,
			VueRendererStub,
		}
	},
)

vi.mock("@tiptap/vue-3", async (importOriginal) => ({
	...(await importOriginal<typeof import("@tiptap/vue-3")>()),
	VueRenderer: VueRendererStub,
}))

const floatingUI = vi.hoisted(() => ({
	computePosition: vi.fn(),
	shift: vi.fn(() => "shift-middleware"),
	flip: vi.fn(() => "flip-middleware"),
	offset: vi.fn((value: number) => `offset-${value}`),
}))

vi.mock("@floating-ui/dom", () => floatingUI)

// only the ancestor node names matter to the context whitelist, so the
// content rules can stay loose as long as the names match
const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		[CODE_BLOCK_NAME]: { group: "block", content: "text*" },
		text: { group: "inline" },
	},
	marks: { bold: {} },
})

const pmDoc = schema.nodeFromJSON({
	type: "doc",
	content: [
		{ type: "paragraph", content: [{ type: "text", text: "prose" }] },
		{ type: CODE_BLOCK_NAME, content: [{ type: "text", text: "code" }] },
	],
})

// places the cursor at the start of the first block ("prose") or of the
// code block
function stateAt(inCodeBlock = false): EditorState {
	return EditorState.create({
		doc: pmDoc,
		selection: TextSelection.create(pmDoc, inCodeBlock ? 9 : 1),
	})
}

// a stand-in exposing what the extension reads: the live state, the
// view's dispatch, and the slash command storage slot
function stubEditor(storage: SlashCommandStorage) {
	const dispatch = vi.fn()

	const editor = {
		get state() {
			return stateAt()
		},
		view: { dispatch },
		storage: { slashCommands: storage },
	} as unknown as Editor

	return { editor, dispatch }
}

function suggestionOptions(storage: SlashCommandStorage, editor: Editor) {
	const addOptions = SlashCommands.config.addOptions
	const addPlugins = SlashCommands.config.addProseMirrorPlugins

	if (!addOptions || !addPlugins) {
		throw new Error("SlashCommands declares no options or plugins")
	}

	const options = addOptions.call({} as never)
	const plugins = addPlugins.call({ options, storage, editor } as never)

	const call = suggestionMock.mock.calls.at(-1)

	if (!call) {
		throw new Error("the suggestion plugin was never configured")
	}

	return { plugins, options, suggestion: call[0] as SlashSuggestionOptions }
}

function freshStorage(): SlashCommandStorage {
	const addStorage = SlashCommands.config.addStorage

	if (!addStorage) {
		throw new Error("SlashCommands declares no storage")
	}

	return addStorage.call({} as never)
}

function commandItem(title: string): CommandItem {
	return {
		title,
		description: "",
		icon: "",
		group: "text" as never,
		command: vi.fn(),
	} as unknown as CommandItem
}

// the plugin only ever inspects the state, so the rest of the allow
// props stay out of the way
function allow(
	suggestion: SlashSuggestionOptions,
	state: EditorState,
): boolean {
	return (
		suggestion.allow?.({ state } as Parameters<
			NonNullable<SlashSuggestionOptions["allow"]>
		>[0]) ?? false
	)
}

// the positioning promise is dropped inside the extension, so the test
// awaits the one the mock handed it — its own .then callback is queued
// first and therefore already ran once this resolves
async function settledPosition() {
	await floatingUI.computePosition.mock.results.at(-1)?.value
}

function renderOf(suggestion: SlashSuggestionOptions): SlashSuggestionRenderer {
	const render = suggestion.render

	if (!render) {
		throw new Error("the suggestion plugin declares no renderer")
	}

	return render()
}

// only the pressed key reaches the handler, so the view and the range
// stay out of the way
function keyDown(
	renderer: SlashSuggestionRenderer,
	event: { key: string },
): boolean {
	return (
		renderer.onKeyDown?.({ event } as unknown as SuggestionKeyDownProps) ??
		false
	)
}

describe("SLASH_COMMAND_TRIGGER_CHAR", () => {
	it("is the forward slash the suggestion plugin listens for", ({ expect }) => {
		expect(SLASH_COMMAND_TRIGGER_CHAR).toBe("/")
	})
})

// every test here reads the suggestion options off a module-level mock
// and stubs the global document, both of which are per-file singletons
describe("SlashCommands", { concurrent: false }, () => {
	beforeEach(() => {
		suggestionMock.mockClear()
		rendererInstances.length = 0
		rendererControl.elementless = false
		floatingUI.computePosition.mockReset()
		floatingUI.computePosition.mockResolvedValue({
			x: 12,
			y: 34,
			strategy: "absolute",
		})
	})

	it("names itself so the storage augmentation resolves", ({ expect }) => {
		expect(SlashCommands.name).toBe("slashCommands")
	})

	it("defaults both decoration classes", ({ expect }) => {
		expect(SlashCommands.config.addOptions?.call({} as never)).toEqual({
			decorationClass: "slash-command-filter",
			decorationEmptyClass: "slash-command-filter-empty",
		})
	})

	it("starts without an active trigger char sequence", ({ expect }) => {
		expect(freshStorage()).toEqual({ sequenceWithTriggerChar: false })
	})

	it("registers a single suggestion plugin carrying the extension options", ({
		expect,
	}) => {
		const storage = freshStorage()
		const { editor } = stubEditor(storage)
		const { plugins, options, suggestion } = suggestionOptions(storage, editor)

		expect(plugins).toEqual([{ suggestion: true }])
		expect(suggestion.char).toBe(SLASH_COMMAND_TRIGGER_CHAR)
		expect(suggestion.editor).toBe(editor)
		expect(suggestion.items).toBe(filterSlashItems)
		expect(suggestion.decorationClass).toBe(options.decorationClass)
		expect(suggestion.decorationEmptyClass).toBe(options.decorationEmptyClass)
	})

	describe("command", () => {
		it("runs the picked item's command with the editor and the range", ({
			expect,
		}) => {
			const storage = freshStorage()
			const { editor } = stubEditor(storage)
			const { suggestion } = suggestionOptions(storage, editor)
			const item = commandItem("Heading 1")
			const range = { from: 1, to: 2 }

			suggestion.command?.({ editor, range, props: item })

			expect(vi.mocked(item.command)).toHaveBeenCalledExactlyOnceWith({
				editor,
				range,
			})
		})
	})

	describe("allow", () => {
		it("blocks the list when no trigger char sequence is active", ({
			expect,
		}) => {
			const storage = freshStorage()
			const { editor } = stubEditor(storage)
			const { suggestion } = suggestionOptions(storage, editor)

			expect(allow(suggestion, stateAt())).toBe(false)
		})

		it("blocks the list inside a code block even with an active sequence", ({
			expect,
		}) => {
			const storage = freshStorage()
			storage.sequenceWithTriggerChar = true
			const { editor } = stubEditor(storage)
			const { suggestion } = suggestionOptions(storage, editor)

			expect(allow(suggestion, stateAt(true))).toBe(false)
		})

		it("allows the list in a paragraph with an active sequence", ({
			expect,
		}) => {
			const storage = freshStorage()
			storage.sequenceWithTriggerChar = true
			const { editor } = stubEditor(storage)
			const { suggestion } = suggestionOptions(storage, editor)

			expect(allow(suggestion, stateAt())).toBe(true)
		})
	})

	describe("render", () => {
		function setUp() {
			const storage = freshStorage()
			const { editor, dispatch } = stubEditor(storage)
			const { suggestion } = suggestionOptions(storage, editor)
			const appendChild = vi.fn()

			vi.stubGlobal("document", { body: { appendChild } })

			return {
				storage,
				editor,
				dispatch,
				appendChild,
				renderer: renderOf(suggestion),
			}
		}

		function startProps(
			overrides: Record<string, unknown> = {},
		): SuggestionProps<CommandItem, CommandItem> {
			return {
				editor: {},
				query: "he",
				items: [commandItem("Heading 1")],
				command: vi.fn(),
				clientRect: () => ({ top: 0, left: 0 }),
				decorationNode: { tagName: "SPAN" },
				...overrides,
			} as unknown as SuggestionProps<CommandItem, CommandItem>
		}

		it("mounts the list with the current query, items and command", ({
			expect,
		}) => {
			const { renderer } = setUp()
			const props = startProps()

			renderer.onStart?.(props)

			expect(rendererInstances).toHaveLength(1)
			expect(rendererInstances[0]?.props).toMatchObject({
				query: "he",
				items: props.items,
				command: props.command,
			})
		})

		it("appends the list to the body and positions it under the decoration", async ({
			expect,
		}) => {
			const { renderer, appendChild } = setUp()
			const props = startProps()

			renderer.onStart?.(props)
			await settledPosition()

			expect(appendChild).toHaveBeenCalledExactlyOnceWith(
				rendererInstances[0]?.element,
			)
			expect(floatingUI.computePosition).toHaveBeenCalledExactlyOnceWith(
				props.decorationNode,
				rendererInstances[0]?.element,
				{
					placement: "bottom-start",
					strategy: "absolute",
					middleware: ["shift-middleware", "flip-middleware", "offset-3"],
				},
			)
			expect(rendererInstances[0]?.element?.style).toEqual({
				position: "absolute",
				left: "12px",
				top: "34px",
			})
		})

		it.for([
			{ name: "no client rect is known", overrides: { clientRect: null } },
			{
				name: "no decoration node exists",
				overrides: { decorationNode: null },
			},
		])("skips mounting the list when $name", ({ overrides }, { expect }) => {
			const { renderer, appendChild } = setUp()

			renderer.onStart?.(startProps(overrides))

			expect(appendChild).toHaveBeenCalledTimes(0)
			expect(floatingUI.computePosition).toHaveBeenCalledTimes(0)
		})

		it("skips mounting the list when the renderer produced no element", ({
			expect,
		}) => {
			rendererControl.elementless = true
			const { renderer, appendChild } = setUp()

			renderer.onStart?.(startProps())

			expect(rendererInstances).toHaveLength(1)
			expect(appendChild).toHaveBeenCalledTimes(0)
			expect(floatingUI.computePosition).toHaveBeenCalledTimes(0)
		})

		it("closes the sequence when the list asks to close itself", ({
			expect,
		}) => {
			const { renderer, storage, dispatch } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onStart?.(startProps())
			const initClose = rendererInstances[0]?.props.initClose as () => void
			initClose()

			expect(storage.sequenceWithTriggerChar).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(1)
		})

		it("hands the new query, items and command to the mounted list", async ({
			expect,
		}) => {
			const { renderer } = setUp()
			const updated = startProps({
				query: "hea",
				items: [commandItem("Heading 2")],
			})

			renderer.onStart?.(startProps())
			renderer.onUpdate?.(updated)
			await settledPosition()

			expect(rendererInstances[0]?.updateProps).toHaveBeenCalledExactlyOnceWith(
				{
					query: "hea",
					items: updated.items,
					command: updated.command,
				},
			)
		})

		it("ignores an update that arrives before the list was mounted", ({
			expect,
		}) => {
			const { renderer } = setUp()

			renderer.onUpdate?.(startProps())

			expect(rendererInstances).toHaveLength(0)
			expect(floatingUI.computePosition).toHaveBeenCalledTimes(0)
		})

		it.for([
			{ name: "no client rect is known", overrides: { clientRect: null } },
			{
				name: "no decoration node exists",
				overrides: { decorationNode: null },
			},
		])(
			"updates the list without repositioning it when $name",
			({ overrides }, { expect }) => {
				const { renderer } = setUp()

				renderer.onStart?.(startProps())
				floatingUI.computePosition.mockClear()
				renderer.onUpdate?.(startProps(overrides))

				expect(rendererInstances[0]?.updateProps).toHaveBeenCalledTimes(1)
				expect(floatingUI.computePosition).toHaveBeenCalledTimes(0)
			},
		)

		it("closes the sequence on escape and reports the key as handled", ({
			expect,
		}) => {
			const { renderer, storage, dispatch } = setUp()
			storage.sequenceWithTriggerChar = true

			const handled = keyDown(renderer, { key: "Escape" })

			expect(handled).toBe(true)
			expect(storage.sequenceWithTriggerChar).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(1)
		})

		it("delegates the key to the mounted list", ({ expect }) => {
			const { renderer } = setUp()
			const onKeyDown = vi.fn(() => true)

			renderer.onStart?.(startProps())
			const instance = rendererInstances[0]

			if (instance) {
				instance.ref = { onKeyDown, close: vi.fn() }
			}

			const event = { key: "ArrowDown" }

			expect(keyDown(renderer, event)).toBe(true)
			expect(onKeyDown).toHaveBeenCalledExactlyOnceWith(event)
		})

		it("reports the key as unhandled without a mounted list", ({ expect }) => {
			const { renderer } = setUp()

			renderer.onStart?.(startProps())

			expect(keyDown(renderer, { key: "ArrowDown" })).toBe(false)
		})

		it("swallows typed characters while nothing matches", ({ expect }) => {
			const { renderer, storage, dispatch } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onStart?.(startProps({ items: [] }))

			expect(keyDown(renderer, { key: "z" })).toBe(false)
			expect(storage.sequenceWithTriggerChar).toBe(true)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("closes the sequence after six unmatched characters", ({ expect }) => {
			const { renderer, storage, dispatch } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onStart?.(startProps({ items: [] }))
			for (const key of ["a", "b", "c", "d", "e", "f"]) {
				keyDown(renderer, { key })
			}

			expect(storage.sequenceWithTriggerChar).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(1)
		})

		it("counts non-ascii letters as unmatched characters too", ({ expect }) => {
			const { renderer, storage } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onStart?.(startProps({ items: [] }))
			for (const key of ["ą", "ę", "ś", "ł", "ż", "ó"]) {
				keyDown(renderer, { key })
			}

			expect(storage.sequenceWithTriggerChar).toBe(false)
		})

		it("delegates named keys instead of counting them as unmatched", ({
			expect,
		}) => {
			const { renderer, storage, dispatch } = setUp()
			storage.sequenceWithTriggerChar = true
			const onKeyDown = vi.fn(() => false)

			renderer.onStart?.(startProps({ items: [] }))
			const instance = rendererInstances[0]

			if (instance) {
				instance.ref = { onKeyDown, close: vi.fn() }
			}

			for (const key of [
				"ArrowDown",
				"ArrowUp",
				"Enter",
				"Backspace",
				"Shift",
				"Tab",
			]) {
				keyDown(renderer, { key })
			}

			expect(onKeyDown).toHaveBeenCalledTimes(6)
			expect(storage.sequenceWithTriggerChar).toBe(true)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("delegates a punctuation key even when nothing matches", ({
			expect,
		}) => {
			const { renderer } = setUp()
			const onKeyDown = vi.fn(() => false)

			renderer.onStart?.(startProps({ items: [] }))
			const instance = rendererInstances[0]

			if (instance) {
				instance.ref = { onKeyDown, close: vi.fn() }
			}

			expect(keyDown(renderer, { key: "-" })).toBe(false)
			expect(onKeyDown).toHaveBeenCalledTimes(1)
		})

		it("keeps typing alive while items still match", ({ expect }) => {
			const { renderer, storage, dispatch } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onStart?.(startProps())
			for (const key of ["a", "b", "c", "d", "e", "f", "g"]) {
				keyDown(renderer, { key })
			}

			expect(storage.sequenceWithTriggerChar).toBe(true)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("stops counting unmatched characters once an update finds matches", ({
			expect,
		}) => {
			const { renderer, storage } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onStart?.(startProps({ items: [] }))
			keyDown(renderer, { key: "a" })
			renderer.onUpdate?.(startProps())
			for (const key of ["b", "c", "d", "e", "f", "g"]) {
				keyDown(renderer, { key })
			}

			expect(storage.sequenceWithTriggerChar).toBe(true)
		})

		it("closes the list and tears it down once the close animation ends", ({
			expect,
		}) => {
			const { renderer, storage } = setUp()
			storage.sequenceWithTriggerChar = true
			const close = vi.fn((afterClose: () => void) => {
				afterClose()
			})
			const remove = vi.fn()

			renderer.onStart?.(startProps())
			const instance = rendererInstances[0]

			if (instance) {
				instance.ref = { close, onKeyDown: vi.fn() }
				instance.element = {
					style: {},
					remove,
				} as unknown as { style: Record<string, string> }
			}

			renderer.onExit?.(undefined as never)

			expect(storage.sequenceWithTriggerChar).toBe(false)
			expect(close).toHaveBeenCalledTimes(1)
			expect(instance?.destroy).toHaveBeenCalledTimes(1)
			expect(remove).toHaveBeenCalledTimes(1)
		})

		it("still closes the sequence when the list was never mounted", ({
			expect,
		}) => {
			const { renderer, storage } = setUp()
			storage.sequenceWithTriggerChar = true

			renderer.onExit?.(undefined as never)

			expect(storage.sequenceWithTriggerChar).toBe(false)
		})

		it("resets the unmatched character count for the next sequence", ({
			expect,
		}) => {
			const { renderer, storage } = setUp()

			renderer.onStart?.(startProps({ items: [] }))
			for (const key of ["a", "b", "c", "d", "e", "f"]) {
				keyDown(renderer, { key })
			}

			renderer.onExit?.(undefined as never)
			storage.sequenceWithTriggerChar = true
			renderer.onStart?.(startProps({ items: [] }))
			keyDown(renderer, { key: "a" })

			expect(storage.sequenceWithTriggerChar).toBe(true)
		})
	})
})

describe("processSlashCommandTransaction", () => {
	function stubTransactionEditor() {
		const dispatch = vi.fn()
		const setMeta = vi.fn(() => "meta-tx")
		const storage: SlashCommandStorage = { sequenceWithTriggerChar: false }

		const editor = {
			get state() {
				return {
					get tr() {
						return { setMeta }
					},
				}
			},
			view: { dispatch },
			storage: { slashCommands: storage },
		} as unknown as Editor

		return { editor, dispatch, setMeta, storage }
	}

	function typed(text: string): Transaction {
		return stateAt().tr.insertText(text, 1)
	}

	it("opens the sequence and forces an empty rerender when a slash is typed", ({
		expect,
	}) => {
		const { editor, dispatch, setMeta, storage } = stubTransactionEditor()

		processSlashCommandTransaction(editor, typed("/"))

		expect(storage.sequenceWithTriggerChar).toBe(true)
		expect(setMeta).toHaveBeenCalledExactlyOnceWith("empty-rerender", true)
		expect(dispatch).toHaveBeenCalledExactlyOnceWith("meta-tx")
	})

	it("opens the sequence for a paste starting with a slash", ({ expect }) => {
		const { editor, storage } = stubTransactionEditor()

		processSlashCommandTransaction(editor, typed("/head"))

		expect(storage.sequenceWithTriggerChar).toBe(true)
	})

	it.for([
		{ name: "the document did not change", make: () => stateAt().tr },
		{
			name: "the transaction is the empty rerender itself",
			// the extension's own rerender marker, kept in sync with the
			// module-private meta key
			make: () => typed("/").setMeta("empty-rerender", true),
		},
		{
			name: "more than one step changed the document",
			make: () => stateAt().tr.insertText("/", 1).insertText("/", 2),
		},
		{
			name: "the only step is not a replacement",
			make: () => stateAt().tr.addMark(1, 4, schema.marks.bold.create()),
		},
		{ name: "an ascii word was typed", make: () => typed("head") },
		{ name: "a non-ascii word was typed", make: () => typed("wąż") },
		{ name: "punctuation was typed", make: () => typed("-") },
	])("stays idle when $name", ({ make }, { expect }) => {
		const { editor, dispatch, setMeta, storage } = stubTransactionEditor()

		processSlashCommandTransaction(editor, make())

		expect(storage.sequenceWithTriggerChar).toBe(false)
		expect(setMeta).toHaveBeenCalledTimes(0)
		expect(dispatch).toHaveBeenCalledTimes(0)
	})
})

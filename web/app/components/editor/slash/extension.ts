import { type Editor, Extension } from "@tiptap/core"
import Suggestion from "@tiptap/suggestion"
import { PluginKey, type Transaction } from "@tiptap/pm/state"
import { computePosition, flip, offset, shift } from "@floating-ui/dom"
import { VueRenderer } from "@tiptap/vue-3"
import CommandList from "./CommandList.vue"
import {
	allowSlashItemsByContext,
	filterSlashItems,
	type CommandItem,
} from "./items"
import { ReplaceStep } from "@tiptap/pm/transform"

export const SLASH_COMMAND_TRIGGER_CHAR = "/"
const ALPHANUM_REGEX = /^[a-zA-Z0-9]+$/
const ALPHANUM_EXTENDED_REGEX = /^[\p{L}\p{N}]+$/u
const EMPTY_RERENDER_KEY = "empty-rerender"
const MAX_UNKNOWN_CHARS = 5

export interface SlashCommandOptions {
	decorationClass: string
	decorationEmptyClass: string
}

export interface SlashCommandStorage {
	sequenceWithTriggerChar: boolean
}

declare module "@tiptap/core" {
	interface Storage {
		slashCommands: SlashCommandStorage
	}
}

// the VueRenderer ref is untyped, so mirror what CommandList exposes.
interface CommandListRef {
	close: (afterClose: () => void) => void
	onKeyDown: (event: KeyboardEvent) => boolean
}

// inspired by: https://github.com/ueberdosis/tiptap/tree/main/demos/src/Experiments/Commands/Vue
export const SlashCommands = Extension.create<
	SlashCommandOptions,
	SlashCommandStorage
>({
	name: "slashCommands",
	addOptions() {
		return {
			decorationClass: "slash-command-filter",
			decorationEmptyClass: "slash-command-filter-empty",
		}
	},
	addStorage() {
		return {
			sequenceWithTriggerChar: false,
		}
	},
	addProseMirrorPlugins() {
		return [
			Suggestion<CommandItem, CommandItem>({
				...this.options,
				editor: this.editor,
				char: SLASH_COMMAND_TRIGGER_CHAR,
				pluginKey: new PluginKey("slashCommands"),
				command: ({ editor, range, props }) => {
					props.command({ editor, range })
				},
				allow: ({ state }) => {
					return (
						this.storage.sequenceWithTriggerChar &&
						allowSlashItemsByContext(state)
					)
				},
				items: filterSlashItems,
				render: () => {
					const storage = this.storage
					const editor = this.editor
					let component: VueRenderer | null = null
					let unknownChars = 0
					let matchedItems = 0

					return {
						onStart: (props) => {
							// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
							component = new VueRenderer(CommandList, {
								editor: props.editor,
								props: {
									query: props.query,
									items: props.items,
									command: props.command,
									initClose: () => {
										storage.sequenceWithTriggerChar = false
										editor.view.dispatch(
											editor.state.tr.setMeta(EMPTY_RERENDER_KEY, true),
										) // trigger an empty rerender
									},
								},
							})

							matchedItems = props.items.length

							if (
								!props.clientRect ||
								!props.decorationNode ||
								!component.element
							) {
								return
							}

							document.body.appendChild(component.element)
							updateCommandListPosition(
								props.decorationNode,
								component.element as HTMLElement,
							)
						},
						onUpdate(props) {
							matchedItems = props.items.length

							if (!component) {
								return
							}

							component.updateProps({
								query: props.query,
								items: props.items,
								command: props.command,
							})

							if (!props.clientRect || !props.decorationNode) {
								return
							}

							updateCommandListPosition(
								props.decorationNode,
								component.element as HTMLElement,
							)
						},
						onKeyDown(props) {
							if (props.event.key === "Escape") {
								storage.sequenceWithTriggerChar = false
								editor.view.dispatch(
									editor.state.tr.setMeta(EMPTY_RERENDER_KEY, true),
								) // trigger an empty rerender

								return true
							}

							// key names like "ArrowDown" or "Shift" are not typed
							// characters, so only single character keys can count
							// towards the unknown character limit
							if (
								matchedItems === 0 &&
								props.event.key.length === 1 &&
								(ALPHANUM_REGEX.test(props.event.key) ||
									ALPHANUM_EXTENDED_REGEX.test(props.event.key))
							) {
								unknownChars++

								if (unknownChars > MAX_UNKNOWN_CHARS) {
									storage.sequenceWithTriggerChar = false
									editor.view.dispatch(
										editor.state.tr.setMeta(EMPTY_RERENDER_KEY, true),
									) // trigger an empty rerender
								}

								return false
							}

							const componentRef = component?.ref as
								CommandListRef | null | undefined

							// without a mounted list there is nothing to handle
							// the key, so report it as unhandled
							return componentRef?.onKeyDown(props.event) ?? false
						},
						onExit() {
							storage.sequenceWithTriggerChar = false
							unknownChars = 0
							matchedItems = 0

							// delayed removal might conflict with new
							// component creation in onStart
							const exitComp = component
							const exitRef = exitComp?.ref as CommandListRef | null | undefined

							exitRef?.close(() => {
								exitComp?.destroy()
								exitComp?.element?.remove()
							})
						},
					}
				},
			}),
		]
	},
})

function updateCommandListPosition(
	reference: Element,
	commandElem: HTMLElement,
) {
	void computePosition(reference, commandElem, {
		placement: "bottom-start",
		strategy: "absolute",
		middleware: [shift(), flip(), offset(3)],
	}).then(({ x, y, strategy }) => {
		commandElem.style.position = strategy
		commandElem.style.left = `${x}px`
		commandElem.style.top = `${y}px`
	})
}

// Must be called with every transaction.
export function processSlashCommandTransaction(
	editor: Editor,
	tx: Transaction,
) {
	if (!tx.docChanged || tx.getMeta(EMPTY_RERENDER_KEY) === true) {
		return
	}

	if (tx.steps.length === 1) {
		const step = tx.steps[0]

		if (step instanceof ReplaceStep) {
			const newText = step.slice.content.textBetween(
				0,
				step.slice.content.size,
				"",
			)
			if (newText.startsWith(SLASH_COMMAND_TRIGGER_CHAR)) {
				editor.storage.slashCommands.sequenceWithTriggerChar = true
				editor.view.dispatch(editor.state.tr.setMeta(EMPTY_RERENDER_KEY, true)) // trigger an empty rerender

				return
			}

			if (
				ALPHANUM_REGEX.test(newText) ||
				ALPHANUM_EXTENDED_REGEX.test(newText)
			) {
				return
			}
		}
	}
}

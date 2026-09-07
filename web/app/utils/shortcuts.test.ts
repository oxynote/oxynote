import type { AnyExtension, InputRule } from "@tiptap/core"
import { getExtensionField } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Collaboration from "@tiptap/extension-collaboration"
import Heading from "@tiptap/extension-heading"
import Italic from "@tiptap/extension-italic"
import {
	BulletList,
	ListItem,
	OrderedList,
	TaskList,
} from "@tiptap/extension-list"
import Strike from "@tiptap/extension-strike"
import Underline from "@tiptap/extension-underline"
import { describe, it } from "vitest"
import { CodeBlock } from "~/components/editor/blocks/code-block"
import { extensionContext } from "~/components/editor/test-helpers/editor"
import { HostOsType } from "~/composables/useDetectHost"
import {
	extractShortcutKeys,
	normalizeShortcut,
	shortcutByOS,
	SHORTCUT_ACTIONS,
	SHORTCUT_GROUPS,
} from "./shortcuts"

const shortcut = { macOS: "⌘+K", other: "Ctrl+K" }

describe("shortcutByOS", () => {
	it.for([
		{ os: HostOsType.MacOS, expected: "⌘+K" },
		{ os: HostOsType.Windows, expected: "Ctrl+K" },
		{ os: HostOsType.Linux, expected: "Ctrl+K" },
		{ os: HostOsType.Other, expected: "Ctrl+K" },
	])("returns $expected for the $os os", ({ os, expected }, { expect }) => {
		expect(shortcutByOS(shortcut, os)).toBe(expected)
	})
})

describe("normalizeShortcut", () => {
	it.for([
		{
			name: "normalizes the command symbol for the default format",
			shortcut: { macOS: "⌘+K", other: "Ctrl+K" },
			os: HostOsType.MacOS,
			format: undefined,
			expected: "meta_k",
		},
		{
			name: "normalizes control shortcuts for the default format",
			shortcut: { macOS: "⌘+K", other: "Ctrl+K" },
			os: HostOsType.Linux,
			format: undefined,
			expected: "control_k",
		},
		{
			name: "normalizes multi-key shortcuts for the default format",
			shortcut: { macOS: "⌘+⇧+P", other: "Ctrl+Shift+P" },
			os: HostOsType.MacOS,
			format: undefined,
			expected: "meta_shift_p",
		},
		{
			name: "normalizes the command symbol for the codemirror format",
			shortcut: { macOS: "⌘+K", other: "Ctrl+K" },
			os: HostOsType.MacOS,
			format: "codemirror" as const,
			expected: "Cmd-k",
		},
		{
			name: "normalizes control shortcuts for the codemirror format",
			shortcut: { macOS: "Ctrl+Space", other: "Ctrl+Space" },
			os: HostOsType.Other,
			format: "codemirror" as const,
			expected: "Ctrl-space",
		},
		{
			name: "leaves a single plain key untouched",
			shortcut: { macOS: "esc", other: "esc" },
			os: HostOsType.MacOS,
			format: undefined,
			expected: "esc",
		},
		{
			name: "binds punctuation by its key code for the default format",
			shortcut: { macOS: "⌘+⇧+\\", other: "Ctrl+Shift+\\" },
			os: HostOsType.MacOS,
			format: undefined,
			expected: "meta_shift_backslash",
		},
		{
			name: "binds the comma, period and slash by their key codes",
			shortcut: { macOS: "⌘+,", other: "Ctrl+," },
			os: HostOsType.Other,
			format: undefined,
			expected: "control_comma",
		},
	])("$name", ({ shortcut, os, format, expected }, { expect }) => {
		expect(normalizeShortcut(shortcut, os, format)).toBe(expected)
	})
})

describe("extractShortcutKeys", () => {
	it("joins keys held together with a chord connector", ({ expect }) => {
		expect(extractShortcutKeys("Ctrl+K")).toEqual([
			{ key: "Ctrl" },
			{ connector: "chord" },
			{ key: "K" },
		])
	})

	it("joins keys pressed one after another with a sequence connector", ({
		expect,
	}) => {
		expect(extractShortcutKeys("G I")).toEqual([
			{ key: "G" },
			{ connector: "sequence" },
			{ key: "I" },
		])
	})

	it("keeps a chord inside a sequence together", ({ expect }) => {
		expect(extractShortcutKeys("G Shift+I")).toEqual([
			{ key: "G" },
			{ connector: "sequence" },
			{ key: "Shift" },
			{ connector: "chord" },
			{ key: "I" },
		])
	})

	it("returns a single key without connectors", ({ expect }) => {
		expect(extractShortcutKeys("esc")).toEqual([{ key: "esc" }])
	})

	it("reads whitespace around a plus as part of the chord", ({ expect }) => {
		expect(extractShortcutKeys("Ctrl + K")).toEqual([
			{ key: "Ctrl" },
			{ connector: "chord" },
			{ key: "K" },
		])
	})
})

// the editor's own bindings, keyed by the extension name a "keys" entry
// cites. Heading is configured as the editor configures it, since its
// keymap is built from the levels it allows.
const EDITOR_EXTENSIONS: Record<string, AnyExtension> = {
	bold: Bold,
	italic: Italic,
	underline: Underline,
	strike: Strike,
	code: Code,
	heading: Heading.configure({ levels: [1, 2, 3] }),
	blockquote: Blockquote,
	bulletList: BulletList,
	orderedList: OrderedList,
	taskList: TaskList,
	listItem: ListItem,
	codeBlock: CodeBlock,
	collaboration: Collaboration,
}

function editorKeymap(name: string): Record<string, unknown> {
	const extension = EDITOR_EXTENSIONS[name]
	if (!extension) {
		throw new Error(`no editor extension registered as "${name}"`)
	}

	const build = getExtensionField<() => Record<string, unknown>>(
		extension,
		"addKeyboardShortcuts",
		extensionContext(extension, name),
	)

	return build()
}

function editorInputRules(name: string): InputRule[] {
	const extension = EDITOR_EXTENSIONS[name]
	if (!extension) {
		throw new Error(`no editor extension registered as "${name}"`)
	}

	const build = getExtensionField<(() => InputRule[]) | undefined>(
		extension,
		"addInputRules",
		extensionContext(extension, name),
	)

	return build?.() ?? []
}

function matchedBySomeRule(rules: InputRule[], text: string): boolean {
	return rules.some((rule) =>
		typeof rule.find === "function"
			? rule.find(text) !== null
			: rule.find.test(text),
	)
}

const entries = SHORTCUT_GROUPS.flatMap((group) => group.shortcuts)

describe("SHORTCUT_GROUPS", () => {
	// the modal renders nothing but these groups, so an action that never
	// lands in one is a shortcut the user can never look up
	it("files every documented action into exactly one group", ({ expect }) => {
		const grouped = entries.flatMap((entry) =>
			entry.kind === "action" ? [entry.action] : [],
		)
		const documented = Object.values(SHORTCUT_ACTIONS).filter(
			(action) => action.i18nKey !== null,
		)

		expect(grouped).toHaveLength(documented.length)
		expect(new Set(grouped).size).toBe(grouped.length)
		documented.forEach((action) => {
			expect(grouped).toContain(action)
		})
	})

	it("leaves the self-explanatory actions out", ({ expect }) => {
		const grouped = entries.flatMap((entry) =>
			entry.kind === "action" ? [entry.action] : [],
		)

		expect(grouped).not.toContain(
			SHORTCUT_ACTIONS.addSlashCommandQueryAsPlainText,
		)
		expect(grouped).not.toContain(SHORTCUT_ACTIONS.openEditorCompletionMenu)
	})

	// a "keys" entry documents a binding the editor owns; if the extension
	// drops or renames it the modal would keep advertising a dead key
	it.for(entries.flatMap((entry) => (entry.kind === "keys" ? [entry] : [])))(
		"cites a live editor binding for $i18nKey",
		({ editorKey, extension }, { expect }) => {
			expect(Object.keys(editorKeymap(extension))).toContain(editorKey)
		},
	)

	// a "typed" entry documents markdown an input rule reacts to; the
	// pattern must trip the rule the way a user would type it
	it.for([
		{ i18nKey: "shortcuts.modal.entries.md-bold.label", extension: "bold" },
		{ i18nKey: "shortcuts.modal.entries.md-italic.label", extension: "italic" },
		{
			i18nKey: "shortcuts.modal.entries.md-inline-code.label",
			extension: "code",
		},
		{
			i18nKey: "shortcuts.modal.entries.md-strikethrough.label",
			extension: "strike",
		},
		{
			i18nKey: "shortcuts.modal.entries.md-quote.label",
			extension: "blockquote",
		},
	])(
		"types a pattern the editor recognises for $i18nKey",
		({ i18nKey, extension }, { expect }) => {
			const entry = entries.find((candidate) => candidate.i18nKey === i18nKey)
			if (entry?.kind !== "typed") {
				throw new Error(`${i18nKey} is not a typed entry`)
			}

			expect(
				matchedBySomeRule(editorInputRules(extension), entry.pattern),
			).toBe(true)
		},
	)

	it("lists every typed entry in the pattern check above", ({ expect }) => {
		const typed = entries.filter((entry) => entry.kind === "typed")

		expect(typed).toHaveLength(5)
	})
})

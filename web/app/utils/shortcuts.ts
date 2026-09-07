// on macOS, partially repeatedly pressing a shortcut (cmd+K -> cmd -> cmd+K)
// key will trigger the action only once.
// https://github.com/vueuse/vueuse/pull/4691
export const SHORTCUT_ACTIONS = {
	toggleSidebar: {
		keyboardKey: {
			macOS: "⌘+\\",
			other: "Ctrl+\\",
		},
		i18nKey: "shortcuts.keys.toggle-sidebar",
	},
	addSlashCommandQueryAsPlainText: {
		keyboardKey: {
			macOS: "esc",
			other: "esc",
		},
		i18nKey: null, // explained in the slash menu already
	},
	searchForDocuments: {
		keyboardKey: {
			macOS: "⌘+K",
			other: "Ctrl+K",
		},
		i18nKey: "shortcuts.keys.search-for-documents",
	},
	toggleInbox: {
		keyboardKey: {
			macOS: "⌘+⇧+\\",
			other: "Ctrl+Shift+\\",
		},
		i18nKey: "shortcuts.keys.toggle-inbox",
	},
	createNewDocument: {
		keyboardKey: {
			macOS: "⌘+.",
			other: "Ctrl+.",
		},
		i18nKey: "shortcuts.keys.create-new-document",
	},
	toggleSettings: {
		keyboardKey: {
			macOS: "⌘+⇧+,",
			other: "Ctrl+Shift+,",
		},
		i18nKey: "shortcuts.keys.toggle-settings",
	},
	toggleShortcuts: {
		keyboardKey: {
			macOS: "⌘+/",
			other: "Ctrl+/",
		},
		i18nKey: "shortcuts.keys.toggle-shortcuts",
	},
	addParamsToSplitDocLeftSide: {
		// context-dependent (handle by the node)
		keyboardKey: {
			macOS: "⌘+⌥+P",
			other: "Ctrl+Alt+P",
		},
		i18nKey: "shortcuts.keys.add-params-to-split-doc-left-side",
	},
	addCodeBlockToSplitDocRightSide: {
		// context-dependent (handle by the node)
		keyboardKey: {
			macOS: "⌘+⌥+E",
			other: "Ctrl+Alt+E",
		},
		i18nKey: "shortcuts.keys.add-code-block-to-split-doc-right-side",
	},
	addMetricsToSplitDocRightSide: {
		// context-dependent (handle by the node)
		keyboardKey: {
			macOS: "⌘+⌥+G",
			other: "Ctrl+Alt+G",
		},
		i18nKey: "shortcuts.keys.add-metrics-to-split-doc-right-side",
	},
	openEditorCompletionMenu: {
		keyboardKey: {
			macOS: "Ctrl+Space",
			other: "Ctrl+Space",
		},
		i18nKey: null, // the shortcut is hidden/implicit
	},
}

export interface ShortcutAction {
	keyboardKey: { macOS: string; other: string }
	i18nKey: string | null
}

// one row of the shortcut modal. An "action" is bound app-wide through
// SHORTCUT_ACTIONS; "keys" is a binding the editor's tiptap extensions
// own, named here by its tiptap notation and extension so a test can check
// it still exists; "typed" is markdown the editor turns into formatting
// as it is typed, where a trailing space means the rule fires on that
// space.
type ShortcutTrigger =
	| { kind: "action"; action: ShortcutAction }
	| {
			kind: "keys"
			keyboardKey: { macOS: string; other: string }
			editorKey: string
			extension: string
	  }
	| { kind: "typed"; pattern: string }

export type ShortcutEntry = {
	i18nKey: string
	descriptionI18nKey: string
} & ShortcutTrigger

function entry(key: string, trigger: ShortcutTrigger): ShortcutEntry {
	return {
		i18nKey: `shortcuts.modal.entries.${key}.label`,
		descriptionI18nKey: `shortcuts.modal.entries.${key}.description`,
		...trigger,
	}
}

function keys(
	key: string,
	macOS: string,
	other: string,
	editorKey: string,
	extension: string,
): ShortcutEntry {
	return entry(key, {
		kind: "keys",
		keyboardKey: { macOS, other },
		editorKey,
		extension,
	})
}

// the groups the shortcut modal renders, in order. Each entry carries its
// own label and description: an action's own i18nKey is the terse text the
// tooltips show next to a button that already names the action, which reads
// as filler in a standalone list. An action whose i18nKey is null explains
// itself where it is used and stays out of the modal. Slash commands and
// the markdown that inserts blocks are not listed here: the modal reads
// them from the slash menu's own items so the two never drift.
export const SHORTCUT_GROUPS: {
	id: string
	i18nKey: string
	shortcuts: ShortcutEntry[]
}[] = [
	{
		id: "general",
		i18nKey: "shortcuts.groups.general",
		shortcuts: [
			entry("toggle-settings", {
				kind: "action",
				action: SHORTCUT_ACTIONS.toggleSettings,
			}),
			entry("toggle-shortcuts", {
				kind: "action",
				action: SHORTCUT_ACTIONS.toggleShortcuts,
			}),
			keys("undo", "⌘+Z", "Ctrl+Z", "Mod-z", "collaboration"),
			keys("redo", "⌘+⇧+Z", "Ctrl+Shift+Z", "Shift-Mod-z", "collaboration"),
		],
	},
	{
		id: "navigation",
		i18nKey: "shortcuts.groups.navigation",
		shortcuts: [
			entry("toggle-sidebar", {
				kind: "action",
				action: SHORTCUT_ACTIONS.toggleSidebar,
			}),
			entry("toggle-inbox", {
				kind: "action",
				action: SHORTCUT_ACTIONS.toggleInbox,
			}),
			entry("search-for-documents", {
				kind: "action",
				action: SHORTCUT_ACTIONS.searchForDocuments,
			}),
		],
	},
	{
		id: "pages",
		i18nKey: "shortcuts.groups.pages",
		shortcuts: [
			entry("create-new-document", {
				kind: "action",
				action: SHORTCUT_ACTIONS.createNewDocument,
			}),
		],
	},
	{
		id: "formatting",
		i18nKey: "shortcuts.groups.formatting",
		shortcuts: [
			keys("bold", "⌘+B", "Ctrl+B", "Mod-b", "bold"),
			keys("italic", "⌘+I", "Ctrl+I", "Mod-i", "italic"),
			keys("underline", "⌘+U", "Ctrl+U", "Mod-u", "underline"),
			keys("strikethrough", "⌘+⇧+S", "Ctrl+Shift+S", "Mod-Shift-s", "strike"),
			keys("inline-code", "⌘+E", "Ctrl+E", "Mod-e", "code"),
			keys("heading-1", "⌘+⌥+1", "Ctrl+Alt+1", "Mod-Alt-1", "heading"),
			keys("heading-2", "⌘+⌥+2", "Ctrl+Alt+2", "Mod-Alt-2", "heading"),
			keys("heading-3", "⌘+⌥+3", "Ctrl+Alt+3", "Mod-Alt-3", "heading"),
			keys(
				"bulleted-list",
				"⌘+⇧+8",
				"Ctrl+Shift+8",
				"Mod-Shift-8",
				"bulletList",
			),
			keys(
				"numbered-list",
				"⌘+⇧+7",
				"Ctrl+Shift+7",
				"Mod-Shift-7",
				"orderedList",
			),
			keys("checklist", "⌘+⇧+9", "Ctrl+Shift+9", "Mod-Shift-9", "taskList"),
			keys("quote", "⌘+⇧+B", "Ctrl+Shift+B", "Mod-Shift-b", "blockquote"),
			keys("code-block", "⌘+⌥+C", "Ctrl+Alt+C", "Mod-Alt-c", "codeBlock"),
			keys("indent-list-item", "Tab", "Tab", "Tab", "listItem"),
			keys("outdent-list-item", "⇧+Tab", "Shift+Tab", "Shift-Tab", "listItem"),
			keys(
				"exit-code-block",
				"⇧+Enter",
				"Shift+Enter",
				"Shift-Enter",
				"codeBlock",
			),
		],
	},
	{
		id: "split-documentation",
		i18nKey: "shortcuts.groups.split-documentation",
		shortcuts: [
			entry("add-params-to-split-doc", {
				kind: "action",
				action: SHORTCUT_ACTIONS.addParamsToSplitDocLeftSide,
			}),
			entry("add-code-block-to-split-doc", {
				kind: "action",
				action: SHORTCUT_ACTIONS.addCodeBlockToSplitDocRightSide,
			}),
			entry("add-metrics-to-split-doc", {
				kind: "action",
				action: SHORTCUT_ACTIONS.addMetricsToSplitDocRightSide,
			}),
		],
	},
	{
		id: "markdown",
		i18nKey: "shortcuts.groups.markdown",
		shortcuts: [
			entry("md-bold", { kind: "typed", pattern: "**text**" }),
			entry("md-italic", { kind: "typed", pattern: "*text*" }),
			entry("md-inline-code", { kind: "typed", pattern: "`text`" }),
			entry("md-strikethrough", { kind: "typed", pattern: "~~text~~" }),
			entry("md-quote", { kind: "typed", pattern: "> " }),
		],
	},
]

export function shortcutByOS(
	action: { macOS: string; other: string },
	osType: HostOsType,
): string {
	if (osType === HostOsType.MacOS) {
		return action.macOS
	}

	return action.other
}

const PUNCTUATION_CODE_NAMES: Record<string, string> = {
	"\\": "backslash",
	",": "comma",
	".": "period",
	"/": "slash",
}

export function normalizeShortcut(
	shortcut: { macOS: string; other: string },
	osType: HostOsType,
	format: "default" | "codemirror" = "default",
) {
	const res = shortcutByOS(shortcut, osType)

	switch (format) {
		case "default":
			return res
				.split("+")
				.map((part) => {
					const key = part.trim().toLowerCase()

					// useMagicKeys records event.key, which shift turns into "|"
					// or "<" for punctuation, and event.code, which stays
					// "backslash" or "comma" whatever else is held
					return (
						PUNCTUATION_CODE_NAMES[key] ??
						key
							.replace("⌘", "meta")
							.replace("cmd", "meta")
							.replace("command", "meta")
							.replace("ctrl", "control")
							.replace("⌥", "alt")
							.replace("option", "alt")
							.replace("⇧", "shift")
					)
				})
				.join("_")
		case "codemirror":
			return res
				.toLowerCase()
				.replace("⌘", "Cmd")
				.replace("cmd", "Cmd")
				.replace("command", "Cmd")
				.replace("ctrl", "Ctrl")
				.replace("⌥", "Alt")
				.replace("option", "Alt")
				.replace("⇧", "Shift")
				.replace(/\+/g, "-")
	}
}

// extractShortcutKeys splits a shortcut string into its keys and the
// connectors between them. "+" joins keys held together (a chord, shown
// side by side); bare whitespace joins keys pressed one after another (a
// sequence, shown with "then" between).
export function extractShortcutKeys(full: string): {
	key?: string
	connector?: "chord" | "sequence"
}[] {
	const res: { key?: string; connector?: "chord" | "sequence" }[] = []

	full
		.trim()
		.split(/(\s*\+\s*|\s+)/)
		.forEach((part) => {
			if (part === "") {
				return
			}

			if (part.includes("+")) {
				res.push({ connector: "chord" })
			} else if (/^\s+$/.test(part)) {
				res.push({ connector: "sequence" })
			} else {
				res.push({ key: part })
			}
		})

	return res
}

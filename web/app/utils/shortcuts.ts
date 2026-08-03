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
			macOS: "⌘+I",
			other: "Ctrl+I",
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
			macOS: "⌘+,",
			other: "Ctrl+,",
		},
		i18nKey: "shortcuts.keys.toggle-settings",
	},
	addParamsToSplitDocLeftSide: {
		// context-dependent (handle by the node)
		keyboardKey: {
			macOS: "⌘+E",
			other: "Ctrl+E",
		},
		i18nKey: "shortcuts.keys.add-params-to-split-doc-left-side",
	},
	addCodeBlockToSplitDocRightSide: {
		// context-dependent (handle by the node)
		keyboardKey: {
			macOS: "⌘+E",
			other: "Ctrl+E",
		},
		i18nKey: "shortcuts.keys.add-code-block-to-split-doc-right-side",
	},
	addMetricsToSplitDocRightSide: {
		// context-dependent (handle by the node)
		keyboardKey: {
			macOS: "⌘+M",
			other: "Ctrl+M",
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

export function shortcutByOS(
	action: { macOS: string; other: string },
	osType: HostOsType,
): string {
	if (osType === HostOsType.MacOS) {
		return action.macOS
	}

	return action.other
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
				.toLowerCase()
				.replace("⌘", "meta")
				.replace("cmd", "meta")
				.replace("command", "meta")
				.replace("ctrl", "control")
				.replace("⌥", "alt")
				.replace("option", "alt")
				.replace("⇧", "shift")
				.replace(/\+/g, "_")
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

// extractShortcutKeys takes a shortcut string like "Ctrl+K" and
// returns an array of objects representing each key and whether it's a
// connector (+). Only "+" is treated as a connector for now.
export function extractShortcutKeys(full: string): {
	key?: string
	connector?: boolean
}[] {
	const parts = full.split("+")
	const res: { key?: string; connector?: boolean }[] = []

	parts.forEach((part, index) => {
		res.push({ key: part.trim() })

		if (index < parts.length - 1) {
			res.push({ connector: true })
		}
	})

	return res
}

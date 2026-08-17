import { describe, it } from "vitest"
import { HostOsType } from "~/composables/useDetectHost"
import {
	extractShortcutKeys,
	normalizeShortcut,
	shortcutByOS,
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
	])("$name", ({ shortcut, os, format, expected }, { expect }) => {
		expect(normalizeShortcut(shortcut, os, format)).toBe(expected)
	})
})

describe("extractShortcutKeys", () => {
	it("splits a two-key shortcut with a connector between", ({ expect }) => {
		expect(extractShortcutKeys("Ctrl+K")).toEqual([
			{ key: "Ctrl" },
			{ connector: true },
			{ key: "K" },
		])
	})

	it("returns a single key without connectors", ({ expect }) => {
		expect(extractShortcutKeys("esc")).toEqual([{ key: "esc" }])
	})

	it("trims whitespace around keys", ({ expect }) => {
		expect(extractShortcutKeys("Ctrl + K")).toEqual([
			{ key: "Ctrl" },
			{ connector: true },
			{ key: "K" },
		])
	})
})

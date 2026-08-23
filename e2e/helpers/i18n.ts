import { readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { fileURLToPath } from "node:url"

type MessageNode = string | { [key: string]: MessageNode }

const LOCALE_DIR = fileURLToPath(
	new URL("../../web/i18n/locales/en/", import.meta.url),
)

// the locale files are combined the same way the app combines them: each
// file carries its own root namespace key, so merging them at the top
// level reproduces the object the running app resolves keys against.
const MESSAGES: Record<string, MessageNode> = {}

for (const file of readdirSync(LOCALE_DIR).filter((f) => f.endsWith(".json"))) {
	Object.assign(
		MESSAGES,
		JSON.parse(readFileSync(join(LOCALE_DIR, file), "utf8")),
	)
}

// t resolves a message the way the app does, so an assertion tracks a
// copy change instead of pinning a second, unmaintained definition of it.
export function t(key: string, values: Record<string, string> = {}): string {
	let node: MessageNode | undefined = MESSAGES

	for (const part of key.split(".")) {
		if (typeof node !== "object") {
			break
		}

		node = node[part]
	}

	if (typeof node !== "string") {
		throw new Error(`no message found for "${key}"`)
	}

	return node.replace(
		/{([\w-]+)}/g,
		(placeholder, name: string) => values[name] ?? placeholder,
	)
}

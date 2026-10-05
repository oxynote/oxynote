// clone creates a deep clone of the provided object.
export const CLONE_IGNORE_ATTR = "data-clone-ignore"

export function clone<T>(obj: T): T {
	if (obj === null || obj === undefined || typeof obj !== "object") {
		return obj
	}

	// it seems this is the only way to deep clone an object :/
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Object/assign#warning_for_deep_clone
	return JSON.parse(jsonStableStringify(obj)) as T
}

export function isValidDescendent(
	ancestor: HTMLElement | null | undefined,
	descendent: HTMLElement | null | undefined,
): boolean {
	if (!ancestor || !descendent) {
		return false
	}

	return ancestor.contains(descendent)
}

export function lastFilePathElement(filePath: string): string {
	if (!filePath) {
		return ""
	}

	const normalized = filePath.replace(/\\/g, "/")
	const parts = normalized.split("/").filter((part) => part !== "")

	return parts[parts.length - 1] ?? ""
}

export function arraysEqual<T>(
	a: T[] | null | undefined,
	b: T[] | null | undefined,
): boolean {
	if (a === b) {
		return true
	}

	if (!a || !b) {
		return false
	}

	if (a.length !== b.length) {
		return false
	}

	for (let i = 0; i < a.length; i++) {
		if (a[i] !== b[i]) {
			return false
		}
	}

	return true
}

export function extractNameFromEmail(email: string): string {
	const e = email.trim()

	const at = e.indexOf("@")
	if (at <= 0) {
		return ""
	}

	return e.slice(0, at)
}

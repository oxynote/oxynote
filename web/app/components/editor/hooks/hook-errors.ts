import { CONTAINER_IMAGE_WATCHER_ERROR_KEYS } from "./container-image-watcher/errors"
import { GITHUB_TRACKING_ERROR_KEYS } from "./github-tracking/errors"
import { URL_WATCHER_ERROR_KEYS } from "./url-watcher/errors"

// the message key for each code core refuses a hook write with. The codes
// listed here can come from a hook of any type.
const HOOK_ERROR_KEYS: Record<string, string> = {
	"document_hook.unconfigured": "editor.hooks.errors.codes.unconfigured",
	"document_hook.upstream_unavailable":
		"editor.hooks.errors.codes.upstream-unavailable",
	...GITHUB_TRACKING_ERROR_KEYS,
	...CONTAINER_IMAGE_WATCHER_ERROR_KEYS,
	...URL_WATCHER_ERROR_KEYS,
}

// hookErrorMessage returns the message for the code core refused a hook
// write with, or undefined when the code names no known reason.
export function hookErrorMessage(
	err: unknown,
	t: (key: string) => string,
): string | undefined {
	const code = (err as { data?: { code?: string } } | undefined)?.data?.code
	const key = code ? HOOK_ERROR_KEYS[code] : undefined

	return key ? t(key) : undefined
}

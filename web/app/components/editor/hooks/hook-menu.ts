import type { HookStatus } from "./hook-status"

// HookSubtitle is the second line of a hook's row. The detail follows the
// text after a dot, like a status or how far off a date is.
export interface HookSubtitle {
	text: string
	detail?: string
}

// the list of hooks keeps to this width. The floor leaves a two line row
// room to breathe, and the cap fits a phone 22.5rem wide
export const HOOK_MENU_WIDTH_CLASS =
	"min-w-[min(17rem,calc(100dvw-1.5rem))] max-w-[min(20rem,calc(100dvw-1.5rem))]"

// the type picker and the forms hold less, so their floor is lower
export const HOOK_SUBMENU_WIDTH_CLASS =
	"min-w-[min(15rem,calc(100dvw-1.5rem))] max-w-[min(20rem,calc(100dvw-1.5rem))]"

// hookNoticeKeypath picks the message of a hook's notice. A hook that is
// still being made says what it does, a triggered one what it did, and any
// other what it will do. The status is null until the hook exists.
export function hookNoticeKeypath(
	type: DocumentHookType,
	status: HookStatus | null,
): string {
	if (!status) {
		return `editor.hooks.${type}.new-notice`
	}

	return status === "triggered"
		? `editor.hooks.${type}.triggered-notice`
		: `editor.hooks.${type}.fresh-notice`
}

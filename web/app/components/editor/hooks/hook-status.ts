// HookStatus is how a hook shows in its menu: fresh while it waits,
// triggered once it went off, and needs-attention while the last check of
// what it watches failed.
export type HookStatus = "fresh" | "triggered" | "needs-attention"

export const HOOK_STATUS_DOT_CLASS: Record<HookStatus, string> = {
	fresh: "bg-hook-status-fresh",
	triggered: "bg-hook-status-triggered",
	"needs-attention": "bg-hook-status-needs-attention",
}

// a pill tints its ground with the status colour and darkens its text
export const HOOK_STATUS_PILL_CLASS: Record<HookStatus, string> = {
	fresh: "bg-hook-status-fresh/15 text-hook-status-fresh-foreground",
	triggered:
		"bg-hook-status-triggered/15 text-hook-status-triggered-foreground",
	"needs-attention":
		"bg-hook-status-needs-attention/25 text-hook-status-needs-attention-foreground",
}

// HookGroupStatus sums up the hooks of one block, or the page's own. It is
// mixed when one hook is triggered and another needs attention.
export type HookGroupStatus = HookStatus | "mixed"

// the hook icon takes the colour of the status in its data-hook-status. An
// icon is drawn through a mask, so the stripes of a mixed status show on
// it as they do on the bar.
export const HOOK_ICON_STATUS_CLASS =
	"data-[hook-status=fresh]:text-hook-status-fresh data-[hook-status=triggered]:text-hook-status-triggered data-[hook-status=needs-attention]:text-hook-status-needs-attention data-[hook-status=mixed]:bg-hook-status-mixed"

// the bar beside a block or the page title. Fresh hooks get none.
export const HOOK_BAR_STATUS_CLASS: Record<
	Exclude<HookGroupStatus, "fresh">,
	string
> = {
	triggered: "bg-hook-status-triggered",
	"needs-attention": "bg-hook-status-needs-attention",
	mixed: "bg-hook-status-mixed",
}

// hookStatus puts a failed check first. A hook that cannot check what it
// watches needs fixing before its trigger means anything. A copy still
// being set up has not failed. It shows the score its source had.
export function hookStatus(hook: DocumentHook): HookStatus {
	if (hook.status !== "active" && hook.status !== "initializing") {
		return "needs-attention"
	}

	return Number(hook.score) === 0 ? "triggered" : "fresh"
}

// hookGroupStatus is null without hooks, so that nothing is coloured as
// healthy where there is nothing to watch.
export function hookGroupStatus(hooks: DocumentHook[]): HookGroupStatus | null {
	if (!hooks.length) {
		return null
	}

	const statuses = hooks.map(hookStatus)
	const triggered = statuses.includes("triggered")
	const needsAttention = statuses.includes("needs-attention")
	if (triggered && needsAttention) {
		return "mixed"
	}

	if (triggered) {
		return "triggered"
	}

	return needsAttention ? "needs-attention" : "fresh"
}

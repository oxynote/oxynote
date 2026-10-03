<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type { HookStatus } from "./hook-status"

const props = defineProps<{
	status: HookStatus
	icon: string
	// the label of the button along the notice's bottom edge, which is left
	// out without one
	actionLabel?: string
}>()
const emit = defineEmits<{
	(e: "action"): void
}>()

const STATUS_CLASSES: Record<
	HookStatus,
	{ box: string; icon: string; action: string }
> = {
	fresh: {
		box: "bg-muted text-muted-foreground",
		icon: "text-muted-foreground",
		action:
			"border-border bg-accent/40 text-foreground [&:not(:disabled):hover:not(:active)]:bg-accent/70 [&:not(:disabled):active]:bg-accent",
	},
	triggered: {
		box: "bg-hook-status-triggered/10 text-status-info-foreground",
		icon: "text-hook-status-triggered",
		action:
			"border-hook-status-triggered/20 bg-hook-status-triggered/5 text-status-info-foreground [&:not(:disabled):hover:not(:active)]:bg-hook-status-triggered/15 [&:not(:disabled):active]:bg-hook-status-triggered/25",
	},
	"needs-attention": {
		box: "bg-hook-status-needs-attention/15 text-status-warning-foreground",
		icon: "text-hook-status-needs-attention",
		action:
			"border-hook-status-needs-attention/30 bg-hook-status-needs-attention/5 text-status-warning-foreground [&:not(:disabled):hover:not(:active)]:bg-hook-status-needs-attention/15 [&:not(:disabled):active]:bg-hook-status-needs-attention/30",
	},
}
</script>

<template>
	<div
		:data-hook-status="props.status"
		:class="
			cn(
				'flex flex-col overflow-hidden rounded-md text-xs leading-4.5',
				STATUS_CLASSES[props.status].box,
			)
		"
	>
		<div class="flex items-start gap-2 px-2 py-1.5">
			<Icon
				:name="props.icon"
				:class="
					cn('mt-0.5 size-3.5 shrink-0', STATUS_CLASSES[props.status].icon)
				"
			/>
			<!--
				the message wraps to the menu's width. Without a width of its
				own, a long sentence would stretch the menu instead
			-->
			<div class="w-0 flex-1 break-words">
				<slot />
			</div>
		</div>
		<ShadcnUiButton
			v-if="props.actionLabel"
			variant="outline-no-effect"
			size="custom"
			:class="
				cn(
					'h-7.5 w-full gap-1.5 rounded-none border-0 border-t text-xs font-bold shadow-none',
					STATUS_CLASSES[props.status].action,
				)
			"
			@click.stop="emit('action')"
		>
			<Icon name="mingcute:check-fill" class="size-3" />
			{{ props.actionLabel }}
		</ShadcnUiButton>
	</div>
</template>

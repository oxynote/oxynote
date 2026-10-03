<script lang="ts" setup>
import { cn } from "~/lib/utils"
import {
	HOOK_STATUS_DOT_CLASS,
	HOOK_STATUS_PILL_CLASS,
	type HookStatus,
} from "./hook-status"

const PILL_CLASS =
	"flex shrink-0 items-center rounded-full px-2 py-px text-xs font-semibold select-none"

const props = defineProps<{
	// one for each hook the menu lists
	statuses: HookStatus[]
}>()

const { t } = useI18n({ useScope: "global" })

// a pill for each status some hook is in, apart from fresh
const pills = computed(() => {
	const needsAttention = props.statuses.filter(
		(status) => status === "needs-attention",
	).length
	const triggered = props.statuses.filter(
		(status) => status === "triggered",
	).length

	return [
		{
			status: "needs-attention" as const,
			count: needsAttention,
			label:
				needsAttention === 1
					? t("editor.hooks.status.needs-attention.one", {
							count: needsAttention,
						})
					: t("editor.hooks.status.needs-attention.other", {
							count: needsAttention,
						}),
		},
		{
			status: "triggered" as const,
			count: triggered,
			label: t("editor.hooks.status.triggered", { count: triggered }),
		},
	].filter((pill) => pill.count > 0)
})
</script>

<template>
	<ShadcnUiDropdownMenuLabel
		class="flex items-center gap-1.5 px-2 py-1.5 text-2sm font-semibold text-muted-foreground"
	>
		<span class="min-w-0 flex-1 truncate">
			{{ $t("editor.hooks.title") }}
		</span>
		<!--
			two pills only fit beside the title without their words, so each
			shows a dot and its count, and its tooltip gives the words back
		-->
		<template v-if="pills.length > 1">
			<ShadcnUiTooltip v-for="pill in pills" :key="pill.status">
				<ShadcnUiTooltipTrigger as-child>
					<span
						:class="
							cn(
								PILL_CLASS,
								'gap-1.25 pl-1.75',
								HOOK_STATUS_PILL_CLASS[pill.status],
							)
						"
					>
						<span
							aria-hidden="true"
							:class="
								cn(
									'size-1.5 shrink-0 rounded-full',
									HOOK_STATUS_DOT_CLASS[pill.status],
								)
							"
						/>
						<span aria-hidden="true">{{ pill.count }}</span>
						<span class="sr-only">{{ pill.label }}</span>
					</span>
				</ShadcnUiTooltipTrigger>
				<ShadcnUiTooltipContent>
					{{ pill.label }}
				</ShadcnUiTooltipContent>
			</ShadcnUiTooltip>
		</template>
		<span
			v-else-if="pills[0]"
			:class="cn(PILL_CLASS, HOOK_STATUS_PILL_CLASS[pills[0].status])"
		>
			{{ pills[0].label }}
		</span>
		<span
			v-else-if="props.statuses.length"
			:class="cn(PILL_CLASS, HOOK_STATUS_PILL_CLASS.fresh)"
		>
			{{ $t("editor.hooks.status.fresh") }}
		</span>
	</ShadcnUiDropdownMenuLabel>
</template>

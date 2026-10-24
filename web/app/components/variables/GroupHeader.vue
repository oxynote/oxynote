<script lang="ts" setup>
import type { VariableGroupTreeElement } from "~/utils"

const props = defineProps<{
	group: VariableGroupTreeElement
	open: boolean
}>()
</script>
<template>
	<div class="flex h-8 items-center gap-1.5 rounded-md px-2 text-2sm">
		<slot name="start" />
		<ShadcnUiCollapsibleTrigger
			class="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 self-stretch rounded-md outline-none"
		>
			<Icon
				v-if="props.open"
				name="mingcute:down-line"
				class="size-3.5 shrink-0 text-muted-foreground"
			/>
			<Icon
				v-else
				name="mingcute:right-line"
				class="size-3.5 shrink-0 text-muted-foreground"
			/>
			<span
				class="size-2.5 shrink-0 rounded-full"
				:style="{ backgroundColor: paletteColor(props.group.color) }"
			/>
			<span class="truncate font-semibold">{{ props.group.groupName }}</span>
		</ShadcnUiCollapsibleTrigger>
		<ShadcnUiTooltip v-if="props.group.builtin" :delay-duration="600">
			<ShadcnUiTooltipTrigger as-child>
				<span
					class="inline-flex shrink-0 items-center gap-1 rounded-full bg-muted px-1.75 py-0.5 text-xs font-semibold text-muted-foreground"
				>
					<Icon name="mingcute:lock-line" class="size-3" />
					{{ $t("variable.builtin.tag") }}
				</span>
			</ShadcnUiTooltipTrigger>
			<ShadcnUiTooltipContent side="bottom" class="max-w-65 text-2sm text-wrap">
				{{ $t("variable.builtin.tag-hint") }}
			</ShadcnUiTooltipContent>
		</ShadcnUiTooltip>
		<slot name="end" />
	</div>
</template>

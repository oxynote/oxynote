<script lang="ts" setup>
import type { VariableGroupTreeElement } from "~/utils"
import { cn } from "#layers/ui/app/lib/utils"
import { useGroupOpenState } from "./group-state"
import type { VariableSection } from "./variables"

const props = defineProps<{
	group: VariableGroupTreeElement
	section: VariableSection["id"] | "edit"
}>()

const { chosen, created } = useGroupOpenState()

// a group animates only once it is toggled, so a list that mounts shows
// its groups in place instead of growing them open
const animated = ref(false)

const stateKey = computed(() => `${props.section}:${props.group.id}`)
// only the built-in group starts open, and not among the other variables.
// A group created since the page loaded starts open anywhere.
const open = computed(
	() =>
		chosen.value[stateKey.value] ??
		(created.value.includes(props.group.id) ||
			(props.group.builtin && props.section !== "other")),
)

function toggleOpen() {
	animated.value = true
	chosen.value[stateKey.value] = !open.value
}
</script>
<template>
	<ShadcnUiCollapsible :open="open" @update:open="toggleOpen">
		<slot name="header" :open="open" />
		<ShadcnUiCollapsibleContent
			:class="
				cn(
					'overflow-hidden',
					animated &&
						'data-[state=closed]:animate-collapsible-up data-[state=open]:animate-collapsible-down',
				)
			"
		>
			<!--the top padding keeps a drop marker on the first row inside the
				content, which clips while it animates-->
			<div class="relative pt-0.5 pb-1">
				<div
					class="pointer-events-none absolute top-1 bottom-2 left-3.5 z-1 w-0.5 rounded-full opacity-35"
					:style="{ backgroundColor: paletteColor(props.group.color) }"
				/>
				<slot />
			</div>
		</ShadcnUiCollapsibleContent>
	</ShadcnUiCollapsible>
</template>

<script lang="ts" setup>
import type { Node as PMNode } from "@tiptap/pm/model"
import { cn } from "#layers/ui/app/lib/utils"
import { countNodeChanges } from "./change-count"
import { DiffStatus } from "./position-map"

// the classes that place the marker belong on the pill, not on the popover
// root, which renders no element of its own. The pill sits inside the file
// card's link and the empty blocks' buttons, so a click on it stops there.
defineOptions({
	inheritAttrs: false,
})

const props = defineProps<{
	node: PMNode
}>()

// a removed grid block keeps its old places, so its marker lists where it
// was
const count = computed(() =>
	props.node.attrs.diffStatus === DiffStatus.Modified ||
	props.node.attrs.diffStatus === DiffStatus.Removed
		? countNodeChanges(props.node)
		: null,
)
</script>

<template>
	<ShadcnUiPopover v-if="count && (count.removed || count.added)">
		<ShadcnUiPopoverTrigger as-child :disabled="!$slots.default">
			<span
				v-bind="$attrs"
				contenteditable="false"
				:class="
					cn(
						'flex overflow-hidden rounded-full border border-border bg-background text-xs font-semibold transition-all select-none',
						$slots.default &&
							'cursor-pointer active:opacity-90 [&:hover:not(:active)]:opacity-70',
					)
				"
				@click.stop.prevent
			>
				<span class="sr-only">
					{{ $t("editor.diff-change-marker.label", { ...count }) }}
				</span>
				<span
					v-if="count.removed"
					aria-hidden="true"
					class="bg-diff-removed px-2 py-0.5 text-diff-removed-foreground"
				>
					{{
						$t("editor.diff-change-marker.removed", { count: count.removed })
					}}
				</span>
				<span
					v-if="count.added"
					aria-hidden="true"
					class="bg-diff-added px-2 py-0.5 text-diff-added-foreground"
				>
					{{ $t("editor.diff-change-marker.added", { count: count.added }) }}
				</span>
			</span>
		</ShadcnUiPopoverTrigger>
		<ShadcnUiPopoverContent
			v-if="$slots.default"
			align="end"
			class="w-auto max-w-[min(28rem,90dvw)] p-3"
		>
			<!-- the details of the changes, from the block around the marker -->
			<slot />
		</ShadcnUiPopoverContent>
	</ShadcnUiPopover>
</template>

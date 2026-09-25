<script lang="ts" setup>
import type { Node as PMNode } from "@tiptap/pm/model"
import { countNodeChanges } from "./change-count"
import { DiffStatus } from "./position-map"

const props = defineProps<{
	node: PMNode
}>()

const count = computed(() =>
	props.node.attrs.diffStatus === DiffStatus.Modified
		? countNodeChanges(props.node)
		: null,
)
</script>

<template>
	<span
		v-if="count && (count.removed || count.added)"
		contenteditable="false"
		class="pointer-events-none flex overflow-hidden rounded-full border border-border bg-background text-xs font-semibold select-none"
	>
		<span class="sr-only">
			{{ $t("editor.diff-change-marker.label", { ...count }) }}
		</span>
		<span
			v-if="count.removed"
			aria-hidden="true"
			class="bg-diff-removed px-2 py-0.5 text-diff-removed-foreground"
		>
			{{ $t("editor.diff-change-marker.removed", { count: count.removed }) }}
		</span>
		<span
			v-if="count.added"
			aria-hidden="true"
			class="bg-diff-added px-2 py-0.5 text-diff-added-foreground"
		>
			{{ $t("editor.diff-change-marker.added", { count: count.added }) }}
		</span>
	</span>
</template>
